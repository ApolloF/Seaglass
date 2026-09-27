package achievements

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
)

// ErrNone means a store says the game has no achievements.
var ErrNone = errors.New("no achievements")

// Net is what Resolve asks the stores' servers. owned.Client implements it.
type Net interface {
	SteamAchievementSchema(ctx context.Context, key string, appID int, lang string) ([]Def, error)
	SteamPlayerAchievements(ctx context.Context, key, steamID string, appID int) (map[string]Unlock, error)
	SteamRarity(ctx context.Context, appID int) (map[string]float64, error)
}

// Deps is everything Resolve works with besides the game.
type Deps struct {
	Env           Env
	Lang          string // Steam's language name: "english", "german", …
	SteamRoot     string
	SteamAccounts []string // userdata account ids, the one in use first
	SteamKey      string   // the user's Steam Web API key ("" = none)
	SteamID       string   // steamID64 of the account in use
	Net           Net      // nil: nothing is asked online
	Offline       bool     // don't go online now (a game is running)
	Cache         *Cache
	Icons         *Icons
}

func (d Deps) online() bool { return d.Net != nil && !d.Offline }

// steamApp is the Steam app a game's achievements are looked up as.
func steamApp(g library.Game) int {
	if g.SteamAppID > 0 {
		return g.SteamAppID
	}
	return g.MetaAppID
}

// emulated reports whether the game's achievements come from emulator files.
func emulated(g library.Game) bool {
	switch g.Source {
	case "steam", "epic", "gog":
		return g.Unofficial
	}
	return g.Source != "ea" && g.Source != "ubisoft" && g.Source != "battlenet" && g.Source != "xbox"
}

func emuGame(g library.Game) EmuGame {
	eg := EmuGame{Dir: g.Dir, EmuDir: g.EmuDir, Emulator: g.Emulator, AppID: steamApp(g)}
	if g.Emulator == "Epic emulator" {
		eg.EpicID = EpicEmuID(eg)
	}
	return eg
}

// Files lists the local files a game's achievements are read from, for
// Stamp: a result made from the same files is still good.
func Files(g library.Game, d Deps) []string {
	app := steamApp(g)
	var files []string
	if emulated(g) && g.Dir != "" {
		eg := emuGame(g)
		files = append(files, EmuFiles(eg, d.Env)...)
		files = append(files, filepath.Join(eg.emuPath(), "steam_settings", "achievements.json"))
	}
	if app > 0 && d.SteamRoot != "" {
		files = append(files, SteamSchemaFile(d.SteamRoot, app))
		for _, acc := range d.SteamAccounts {
			files = append(files, SteamStatsFile(d.SteamRoot, acc, app))
		}
	}
	return files
}

// Resolve reads a game's achievements from the best source it has:
//
//   - an unofficial copy (or a game from no store): its emulator's unlock
//     files, named by steam_settings, Steam's cached schema or, with a
//     key, Steam's Web API;
//   - a Steam game: Steam's own cache, else the Web API (with a key).
//
// Global rarity comes from Steam when the game's Steam app is known.
// net reports whether a store's servers were asked.
func Resolve(ctx context.Context, g library.Game, d Deps) (l *List, net bool) {
	l = &List{GameID: g.ID, Items: []Achievement{}}
	app := steamApp(g)
	steamIDs := false // the items are keyed by Steam's API names
	switch {
	case emulated(g):
		net = resolveEmu(ctx, g, d, l)
		steamIDs = l.Source != "Epic emulator"
	case g.Source == "steam":
		net = resolveSteam(ctx, app, d, l)
		steamIDs = true
	default:
		l.Hint = "Seaglass can't read achievements from " + storeName(g.Source) + " yet."
	}
	if steamIDs && app > 0 && len(l.Items) > 0 {
		if r, n := steamRarity(ctx, app, d); len(r) > 0 {
			l.SetRarity(r)
			net = net || n
		}
	}
	d.Icons.Localize(ctx, l, !d.online())
	l.Count()
	l.UpdatedAt = time.Now().Unix()
	return l, net
}

func storeName(source string) string {
	switch source {
	case "ea":
		return "the EA app"
	case "ubisoft":
		return "Ubisoft Connect"
	case "battlenet":
		return "Battle.net"
	case "xbox":
		return "the Xbox app"
	case "epic":
		return "Epic"
	case "gog":
		return "GOG"
	}
	return source
}

const hintKey = "Add a Steam Web API key in Settings → Accounts to see names and icons."

func resolveEmu(ctx context.Context, g library.Game, d Deps, l *List) (net bool) {
	eg := emuGame(g)
	res, _, found := ReadEmu(eg, d.Env)
	l.Source = g.Emulator
	if found {
		l.Source = res.Source
	}
	if l.Source == "" {
		l.Source = "emulator"
	}
	var defs []Def
	if g.Dir != "" {
		defs, _, _ = GoldbergSchema(eg, d.Lang)
	}
	if len(defs) == 0 && eg.AppID > 0 {
		defs, _ = SteamSchema(d.SteamRoot, eg.AppID, d.Lang)
	}
	none := false
	if len(defs) == 0 && eg.AppID > 0 && l.Source != "Epic emulator" {
		defs, none, net = steamWebSchema(ctx, eg.AppID, d)
	}
	unlocks := res.Unlocks
	if res.Hashed {
		unlocks = unhash(defs, res.Unlocks)
	}
	l.Items = Merge(defs, unlocks)
	switch {
	case !found && Unknown(g.Emulator):
		l.Hint = "Seaglass can't read " + g.Emulator + "'s achievement files yet."
	case eg.AppID == 0 && l.Source != "Epic emulator":
		l.Hint = "Seaglass doesn't know which Steam game this is, so it can't find its achievements."
	case len(defs) == 0 && none:
		// the game has none
	case len(defs) == 0 && (found || res.Hashed) && d.SteamKey == "":
		l.Hint = hintKey
	case len(defs) == 0 && !found:
		l.Hint = "No achievements found for this copy."
	}
	return net
}

// unhash matches SmartSteamEmu's hashed keys to the schema's IDs.
func unhash(defs []Def, hashed map[string]Unlock) map[string]Unlock {
	out := map[string]Unlock{}
	for _, d := range defs {
		if u, ok := hashed[SSEHash(d.ID)]; ok {
			out[d.ID] = u
		}
	}
	return out
}

func resolveSteam(ctx context.Context, app int, d Deps, l *List) (net bool) {
	l.Source = "steam"
	if app <= 0 {
		l.Hint = "Seaglass doesn't know this game's Steam app."
		return false
	}
	defs, unlocks, _, _ := SteamLocal(d.SteamRoot, d.SteamAccounts, app, d.Lang)
	none := false
	if len(defs) == 0 {
		var n bool
		defs, none, n = steamWebSchema(ctx, app, d)
		net = net || n
	}
	if unlocks == nil && len(defs) > 0 && d.SteamKey != "" && d.SteamID != "" && d.online() {
		u, err := d.Net.SteamPlayerAchievements(ctx, d.SteamKey, d.SteamID, app)
		if err == nil {
			unlocks = u
		}
		net = true
	}
	l.Items = Merge(defs, unlocks)
	switch {
	case len(defs) == 0 && none:
	case len(defs) == 0 && d.SteamKey == "":
		l.Hint = "Play it once through Steam, or " + lowerFirst(hintKey)
	case len(defs) == 0:
		l.Hint = "Steam didn't answer; try again later."
	case unlocks == nil:
		l.Hint = "Play it once through Steam to see your progress."
	}
	return net
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]+'a'-'A') + s[1:]
}

// steamWebSchema asks Steam's Web API for a schema (with the user's key),
// through the schema cache. none: Steam says the game has no achievements.
func steamWebSchema(ctx context.Context, app int, d Deps) (defs []Def, none, net bool) {
	id := strconv.Itoa(app)
	if s, ok := d.Cache.schema("steam", id, d.Lang, SchemaTTL); ok {
		return s.Defs, s.None, false
	}
	if d.SteamKey == "" || !d.online() {
		return nil, false, false
	}
	defs, err := d.Net.SteamAchievementSchema(ctx, d.SteamKey, app, d.Lang)
	switch {
	case errors.Is(err, ErrNone):
		d.Cache.putSchema("steam", id, d.Lang, schemaFile{None: true})
		return nil, true, true
	case err != nil:
		return nil, false, true
	}
	d.Cache.putSchema("steam", id, d.Lang, schemaFile{Defs: defs})
	return defs, false, true
}

// steamRarity is Steam's global unlock percentages, through the cache.
func steamRarity(ctx context.Context, app int, d Deps) (map[string]float64, bool) {
	id := strconv.Itoa(app)
	if s, ok := d.Cache.schema("rarity-steam", id, "", RarityTTL); ok {
		return s.Rarity, false
	}
	if !d.online() {
		return nil, false
	}
	r, err := d.Net.SteamRarity(ctx, app)
	if err != nil && !errors.Is(err, ErrNone) {
		return nil, true
	}
	d.Cache.putSchema("rarity-steam", id, "", schemaFile{Rarity: r, None: len(r) == 0})
	return r, true
}
