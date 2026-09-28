package achievements

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
)

// Version is part of every cached result's key: raise it when what
// Resolve makes of the same files changes, so results are read again.
const Version = 4

// ErrNone means a store says the game has no achievements.
var ErrNone = errors.New("no achievements")

// Net is what Resolve asks the stores' servers. owned.Client implements it.
type Net interface {
	SteamAchievementSchema(ctx context.Context, key string, appID int, lang string) ([]Def, error)
	SteamPlayerAchievements(ctx context.Context, key, steamID string, appID int) (map[string]Unlock, error)
	SteamRarity(ctx context.Context, appID int) (map[string]float64, error)
	EpicAchievements(ctx context.Context, sandbox, locale string) ([]Def, map[string]float64, error)
	EpicPlayerAchievements(ctx context.Context, access, account, sandbox string) (map[string]Unlock, error)
	GOGAchievements(ctx context.Context, access, game, userID string) ([]Def, map[string]Unlock, map[string]float64, error)
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
	// EpicLocale is Epic's name for Lang ("de", "en-US", …).
	EpicLocale string
	// Epic returns a signed-in Epic account's access token; nil when
	// nobody is signed in.
	Epic func(ctx context.Context) (access, account string, err error)
	// GalaxyDB is GOG Galaxy's database ("" when there's none) and
	// GOGUnlocks reads a game's unlocks from it.
	GalaxyDB   string
	GOGUnlocks func(gogID string) (map[string]Unlock, error)
	// GOG returns a signed-in GOG account's access token and user id; nil
	// when nobody is signed in.
	GOG     func(ctx context.Context) (access, userID string, err error)
	Offline bool // don't go online now (a game is running)
	// UplayGames counts the library's games on a Uplay emulator: with only
	// one, the lone achievements folder there must be its.
	UplayGames int
	Cache      *Cache
	Icons      *Icons
}

func (d Deps) online() bool { return d.Net != nil && !d.Offline }

// steamApp is the Steam app a game's achievements are looked up as.
func steamApp(g library.Game) int {
	if g.SteamAppID > 0 {
		return g.SteamAppID
	}
	return g.MetaAppID
}

// gogGame reports whether a game's achievements are GOG's: a GOG install,
// or a DRM-free GOG copy with no emulator.
func gogGame(g library.Game) bool {
	if g.Source == "gog" {
		return !g.External
	}
	return g.GogID != "" && g.Emulator == "" && g.Source != "steam" && g.Source != "epic" && g.Source != "ea" && g.Source != "ubisoft" && g.Source != "battlenet" && g.Source != "xbox"
}

// emulated reports whether the game's achievements come from emulator files.
func emulated(g library.Game) bool {
	switch g.Source {
	case "steam", "epic", "gog":
		return g.External
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
	if emulated(g) && g.Dir != "" && UplayEmulator(g.Emulator) {
		files = append(files, UplayFiles(emuGame(g), d.Env)...)
	} else if emulated(g) && g.Dir != "" {
		eg := emuGame(g)
		files = append(files, EmuFiles(eg, d.Env)...)
		files = append(files, filepath.Join(eg.emuPath(), "steam_settings", "achievements.json"))
	}
	if gogGame(g) && g.GogID != "" && d.GalaxyDB != "" {
		files = append(files, d.GalaxyDB, d.GalaxyDB+"-wal")
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
//   - an external copy (or a game from no store): its emulator's unlock
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
	case gogGame(g):
		net = resolveGOG(ctx, g, d, l)
	case emulated(g):
		net = resolveEmu(ctx, g, d, l)
		steamIDs = l.Source != "Epic emulator"
	case g.Source == "steam":
		net = resolveSteam(ctx, app, d, l)
		steamIDs = true
	case g.Source == "epic":
		net = resolveEpic(ctx, g, d, l)
	default:
		l.Hint = "Seaglass can't read achievements from " + storeName(g.Source) + " yet."
	}
	if steamIDs && app > 0 && len(l.Items) > 0 {
		if r, n := steamRarity(ctx, app, d); len(r) > 0 {
			l.SetRarity(r)
			net = net || n
		}
	}
	l.Partial = !d.Icons.Localize(ctx, l, !d.online())
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
	if UplayEmulator(g.Emulator) && g.Dir != "" {
		return resolveUplay(ctx, g, d, l)
	}
	eg := emuGame(g)
	res, _, found := ReadEmu(eg, d.Env)
	if g.Emulator == "" && !found {
		// No emulator and no unlock file: nothing tracks achievements for
		// this copy (a plain folder, a DRM-free game), so there's nothing
		// to show, not even every achievement as locked.
		return false
	}
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
	if len(defs) == 0 && l.Source == "Epic emulator" && eg.EpicID != "" {
		var rarity map[string]float64
		defs, rarity, none, net = epicSchema(ctx, eg.EpicID, d)
		defer l.SetRarity(rarity)
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

// resolveUplay reads a Ubisoft game on a Uplay emulator. Names come from
// the emulator's own schema, else the Steam version's (Ubisoft's
// achievement ids are the numbers Steam's API names end in).
func resolveUplay(ctx context.Context, g library.Game, d Deps, l *List) (net bool) {
	eg := emuGame(g)
	l.Source = g.Emulator
	defs, _ := UplaySchema(eg)
	none := false
	if len(defs) == 0 && eg.AppID > 0 {
		defs, _ = SteamSchema(d.SteamRoot, eg.AppID, d.Lang)
		if len(defs) == 0 {
			defs, none, net = steamWebSchema(ctx, eg.AppID, d)
		}
	}
	known := make([]string, 0, len(defs))
	for _, def := range defs {
		known = append(known, def.ID)
	}
	res, _, seen, found := ReadUplay(eg, d.Env, known, d.UplayGames == 1)
	unlocks := res.Unlocks
	if len(defs) > 0 {
		unlocks = matchByNumber(defs, unlocks)
	}
	l.Items = Merge(defs, unlocks)
	c := readUplayConfig(eg, d.Env)
	switch {
	case !found && !c.achOn && uplayINIPath(eg, c) != "":
		// Off in the ini, so whatever else is saved isn't this game's.
		l.Hint = "No achievement file found. Uplay emulators save achievements only when their ini has Achievements = 1."
		if g.Emulator == "VOICES38" {
			l.Hint = "No achievement file found. Uplay emulators save achievements only when their ini has Achievements = 1, and VOICES38's own loader may not save them at all."
		}
		l.Fix = FixUplayINI
	case !found && seen > 0:
		l.Hint = "Seaglass found several Ubisoft games' achievements in Goldberg UplayEmu Saves and can't tell which are this game's."
	case !found && c.achOn && g.LastPlayed > c.iniAt:
		// Turned on, played since, and still nothing saved: this loader
		// doesn't save them (VOICES38's own seems not to). Nothing to show.
		l.Source, l.Items = "", []Achievement{}
	case !found && c.achOn:
		l.Hint = "Achievements are turned on in " + filepath.Base(c.ini) + ". Play the game and they'll show up here; if it still saves none, this card goes away."
	case !found:
		l.Hint = "No achievement file found. Uplay emulators save achievements only when their ini has Achievements = 1."
	case len(defs) == 0 && none:
	case len(defs) == 0 && eg.AppID > 0 && d.SteamKey == "":
		l.Hint = hintKey
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
	if app <= 0 {
		return false // nothing to look up: no card
	}
	l.Source = "steam"
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

// EpicSandbox is an Epic game's sandbox: the namespace of
// "namespace:catalogItem:appName".
func EpicSandbox(epicApp string) string {
	ns, _, _ := strings.Cut(epicApp, ":")
	return ns
}

func resolveEpic(ctx context.Context, g library.Game, d Deps, l *List) (net bool) {
	sandbox := EpicSandbox(g.EpicApp)
	if sandbox == "" {
		return false // nothing to look up: no card
	}
	l.Source = "epic"
	defs, rarity, none, net := epicSchema(ctx, sandbox, d)
	var unlocks map[string]Unlock
	signedIn := d.Epic != nil
	if signedIn && len(defs) > 0 && d.online() {
		access, account, err := d.Epic(ctx)
		if err == nil {
			unlocks, err = d.Net.EpicPlayerAchievements(ctx, access, account, sandbox)
			net = true
		}
		if err != nil {
			l.Hint = "Epic didn't answer about your progress; try again later."
		}
	}
	l.Items = Merge(defs, unlocks)
	l.SetRarity(rarity)
	switch {
	case len(defs) == 0 && none:
	case len(defs) == 0 && !d.online():
		l.Hint = "Seaglass reads Epic achievements once the game is closed."
	case len(defs) == 0:
		l.Hint = "Epic didn't answer; try again later."
	case !signedIn:
		l.Hint = "Sign in to Epic in Settings → Accounts to see your progress."
	}
	return net
}

// epicSchema is an Epic game's achievements and rarity, through the cache.
func epicSchema(ctx context.Context, sandbox string, d Deps) (defs []Def, rarity map[string]float64, none, net bool) {
	locale := d.EpicLocale
	if locale == "" {
		locale = "en-US"
	}
	if s, ok := d.Cache.schema("epic", sandbox, locale, RarityTTL); ok {
		return s.Defs, s.Rarity, s.None, false
	}
	if !d.online() {
		return nil, nil, false, false
	}
	defs, rarity, err := d.Net.EpicAchievements(ctx, sandbox, locale)
	switch {
	case errors.Is(err, ErrNone):
		d.Cache.putSchema("epic", sandbox, locale, schemaFile{None: true})
		return nil, nil, true, true
	case err != nil:
		return nil, nil, false, true
	}
	d.Cache.putSchema("epic", sandbox, locale, schemaFile{Defs: defs, Rarity: rarity})
	return defs, rarity, false, true
}

// GOGClientID reads a GOG game's client id from its goggame-<id>.info.
func GOGClientID(dir, gogID string) string {
	if dir == "" || gogID == "" || strings.ContainsAny(gogID, `\/.:`) {
		return ""
	}
	b, err := readSmall(filepath.Join(dir, "goggame-"+gogID+".info"), 1<<20)
	if err != nil {
		return ""
	}
	var info struct {
		ClientID string `json:"clientId"`
	}
	if json.Unmarshal([]byte(text(b)), &info) != nil {
		return ""
	}
	return info.ClientID
}

func resolveGOG(ctx context.Context, g library.Game, d Deps, l *List) (net bool) {
	if g.GogID == "" {
		return false // nothing to look up: no card
	}
	l.Source = "gog"
	var local map[string]Unlock
	if d.GOGUnlocks != nil {
		local, _ = d.GOGUnlocks(g.GogID)
	}
	cached, haveSchema := d.Cache.schema("gog", g.GogID, "", RarityTTL)
	defs, rarity, unlocks := cached.Defs, cached.Rarity, local
	// Signed in: GOG's servers have names, icons and progress. Asked when
	// there's no schema yet, or Galaxy's database doesn't know the progress.
	if d.GOG != nil && d.online() && (!haveSchema || local == nil) {
		net = true
		access, user, err := d.GOG(ctx)
		if err == nil {
			// Fetched apart: a failed request keeps the cached schema.
			var nd []Def
			var remote map[string]Unlock
			var nr map[string]float64
			for _, game := range []string{GOGClientID(g.Dir, g.GogID), g.GogID} {
				if game == "" {
					continue
				}
				nd, remote, nr, err = d.Net.GOGAchievements(ctx, access, game, user)
				if err == nil || !errors.Is(err, ErrNone) {
					break
				}
			}
			switch {
			case errors.Is(err, ErrNone):
				d.Cache.putSchema("gog", g.GogID, "", schemaFile{None: true})
				cached.None = true
				defs, rarity = nil, nil
			case err == nil:
				defs, rarity = nd, nr
				d.Cache.putSchema("gog", g.GogID, "", schemaFile{Defs: defs, Rarity: rarity})
				if unlocks == nil {
					unlocks = remote
				}
			}
		}
		if err != nil && !errors.Is(err, ErrNone) {
			l.Hint = "GOG didn't answer; try again later."
		}
	}
	l.Items = Merge(defs, unlocks)
	l.SetRarity(rarity)
	switch {
	case l.Hint != "" || cached.None:
	case d.GOG == nil && len(l.Items) > 0:
		l.Hint = "Sign in to GOG in Settings → Accounts to see names and icons."
	case d.GOG == nil:
		l.Hint = "Sign in to GOG in Settings → Accounts to see this game's achievements."
	case len(defs) == 0 && !d.online():
		l.Hint = "Seaglass reads GOG achievements once the game is closed."
	}
	return net
}
