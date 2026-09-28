package achievements

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
)

type fakeNet struct {
	gogDefs    []Def
	gogUnlocks map[string]Unlock
	gogFor     string // the game id GOG answers for
	gogErr     error  // GOG fails with this
	epicDefs   []Def
	epicRarity map[string]float64
	epicPlayer map[string]Unlock
	schema     []Def
	player     map[string]Unlock
	rarity     map[string]float64
	calls      []string
	noneFor    int
}

func (f *fakeNet) SteamAchievementSchema(_ context.Context, key string, app int, lang string) ([]Def, error) {
	f.calls = append(f.calls, "schema")
	if app == f.noneFor {
		return nil, ErrNone
	}
	return f.schema, nil
}

func (f *fakeNet) SteamPlayerAchievements(context.Context, string, string, int) (map[string]Unlock, error) {
	f.calls = append(f.calls, "player")
	return f.player, nil
}

func (f *fakeNet) SteamRarity(context.Context, int) (map[string]float64, error) {
	f.calls = append(f.calls, "rarity")
	return f.rarity, nil
}

// fakeIcons stores anything under a hash of its source.
func fakeIcons(dir string) *Icons {
	store := func(dir string, b []byte) (string, error) {
		sum := sha1.Sum(b)
		name := hex.EncodeToString(sum[:]) + ".png"
		return name, os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
	return &Icons{Dir: dir,
		Fetch: func(_ context.Context, src, dir string) (string, error) { return store(dir, []byte(src)) },
		Store: store,
	}
}

func testDeps(t *testing.T, root string, net Net) Deps {
	icons := filepath.Join(root, "icons")
	_ = os.MkdirAll(icons, 0o755)
	return Deps{Env: testEnv(root), Lang: "english", SteamRoot: filepath.Join(root, "Steam"), SteamAccounts: []string{"22"},
		Net: net, Cache: &Cache{Dir: filepath.Join(root, "cache")}, Icons: fakeIcons(icons)}
}

func TestResolveGoldberg(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"game/bin/steam_settings/achievements.json":        `[{"name":"ACH_A","displayName":"Alpha","icon":"a.jpg"},{"name":"ACH_B","displayName":"Beta","hidden":"1"}]`,
		"game/bin/steam_settings/achievement_images/a.jpg": "local icon",
		"Roaming/GSE Saves/620/achievements.json":          `{"ACH_A":{"earned":true,"earned_time":1700000000}}`,
	})
	net := &fakeNet{rarity: map[string]float64{"ACH_A": 50}}
	d := testDeps(t, root, net)
	g := library.Game{ID: 7, Source: "folder", External: true, Emulator: "Goldberg", EmuDir: "bin", Dir: filepath.Join(root, "game"), SteamAppID: 620}
	l, usedNet := Resolve(context.Background(), g, d)
	if l.Source != "Goldberg" || l.Total != 2 || l.Unlocked != 1 || l.Hint != "" {
		t.Fatalf("list %+v", l)
	}
	a := l.Items[0]
	if a.Name != "Alpha" || !a.Unlocked || !strings.HasPrefix(a.Icon, IconPrefix) || a.Percent == nil || *a.Percent != 50 {
		t.Errorf("a: %+v", a)
	}
	if !usedNet || strings.Join(net.calls, ",") != "rarity" {
		t.Errorf("calls %v", net.calls)
	}
	// Rarity is cached now.
	net.calls = nil
	Resolve(context.Background(), g, d)
	if len(net.calls) != 0 {
		t.Errorf("second resolve called %v", net.calls)
	}
}

func TestResolveNoSchemaHint(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{"Public/Documents/Steam/CODEX/620/achievements.ini": "[ACH_A]\nAchieved=1\n"})
	d := testDeps(t, root, nil)
	g := library.Game{ID: 1, Source: "installer", External: true, Emulator: "CODEX", Dir: filepath.Join(root, "game"), SteamAppID: 620}
	l, _ := Resolve(context.Background(), g, d)
	if l.Total != 1 || l.Items[0].Name != "ACH_A" || l.Hint != hintKey {
		t.Errorf("%+v", l)
	}
}

func TestResolveSSEWithWebSchema(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"Roaming/SmartSteamEmu/620/stats.bin": sseFile([3]uint32{crc32.ChecksumIEEE([]byte("ACH_B")), 1_700_000_000, 1}),
	})
	net := &fakeNet{schema: []Def{{ID: "ACH_A", Name: "A", Icon: "https://cdn.akamai.steamstatic.com/a.jpg"}, {ID: "ACH_B", Name: "B"}}}
	d := testDeps(t, root, net)
	d.SteamKey = "k"
	g := library.Game{ID: 2, Source: "folder", External: true, Emulator: "SmartSteamEmu", Dir: filepath.Join(root, "game"), SteamAppID: 620}
	l, _ := Resolve(context.Background(), g, d)
	if l.Total != 2 || l.Unlocked != 1 || !l.Items[1].Unlocked || l.Items[1].UnlockedAt != 1_700_000_000 {
		t.Errorf("%+v", l)
	}
	if !strings.HasPrefix(l.Items[0].Icon, IconPrefix) {
		t.Errorf("icon not stored: %q", l.Items[0].Icon)
	}
	// Offline: no icon downloads, nothing asked.
	d.Offline, net.calls = true, nil
	d.Icons = fakeIcons(t.TempDir())
	l, usedNet := Resolve(context.Background(), g, d)
	if usedNet || len(net.calls) != 0 || l.Items[0].Icon != "" || l.Total != 2 {
		t.Errorf("offline: net %v calls %v list %+v", usedNet, net.calls, l)
	}
}

func TestResolveUnknownEmulator(t *testing.T) {
	root := t.TempDir()
	d := testDeps(t, root, nil)
	g := library.Game{ID: 3, Source: "folder", External: true, Emulator: "PLAZA", Dir: filepath.Join(root, "game"), SteamAppID: 620}
	if l, _ := Resolve(context.Background(), g, d); !strings.Contains(l.Hint, "PLAZA") {
		t.Errorf("hint %q", l.Hint)
	}
	g.Emulator, g.SteamAppID = "Goldberg", 0
	if l, _ := Resolve(context.Background(), g, d); !strings.Contains(l.Hint, "which Steam game") {
		t.Errorf("hint %q", l.Hint)
	}
}

func TestResolveSteam(t *testing.T) {
	root := t.TempDir()
	stats := filepath.Join(root, "Steam", "appcache", "stats")
	_ = os.MkdirAll(stats, 0o755)
	_ = os.WriteFile(SteamSchemaFile(filepath.Join(root, "Steam"), 620), schemaBin(t), 0o644)
	net := &fakeNet{player: map[string]Unlock{"ACH_A": {Achieved: true, At: 5}}}
	d := testDeps(t, root, net)
	g := library.Game{ID: 4, Source: "steam", Dir: filepath.Join(root, "game"), SteamAppID: 620}

	// Schema but no stats file and no key: names shown, progress unknown.
	l, _ := Resolve(context.Background(), g, d)
	if l.Total != 2 || l.Unlocked != 0 || !strings.Contains(l.Hint, "progress") {
		t.Errorf("no stats: %+v", l)
	}
	// With a key the Web API fills in the unlocks.
	d.SteamKey, d.SteamID = "k", "7656"
	l, _ = Resolve(context.Background(), g, d)
	if l.Unlocked != 1 || l.Hint != "" {
		t.Errorf("web unlocks: %+v", l)
	}
	// A game Steam says has none: an empty list, no hint.
	g.SteamAppID, net.noneFor = 999, 999
	l, _ = Resolve(context.Background(), g, d)
	if l.Total != 0 || l.Hint != "" {
		t.Errorf("none: %+v", l)
	}
	// No schema anywhere, no key: how to get one.
	d.SteamKey, g.SteamAppID = "", 1000
	if l, _ = Resolve(context.Background(), g, d); !strings.Contains(l.Hint, "Steam Web API key") {
		t.Errorf("hint %q", l.Hint)
	}
}

func TestResolveOtherStore(t *testing.T) {
	d := testDeps(t, t.TempDir(), nil)
	l, _ := Resolve(context.Background(), library.Game{ID: 5, Source: "ea"}, d)
	if !strings.Contains(l.Hint, "EA app") || l.Items == nil {
		t.Errorf("%+v", l)
	}
}

func TestStampAndCache(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "a.json")
	s1 := Stamp([]string{f}, "english")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	s2 := Stamp([]string{f}, "english")
	if s1 == s2 || s2 != Stamp([]string{f}, "english") || s2 == Stamp([]string{f}, "german") {
		t.Error("stamp doesn't follow the files")
	}
	c := &Cache{Dir: root}
	if err := c.Save(3, Entry{Stamp: s2, List: List{GameID: 3, Total: 4}}); err != nil {
		t.Fatal(err)
	}
	if e, ok := c.Load(3); !ok || e.Stamp != s2 || e.List.Total != 4 {
		t.Errorf("load %+v %v", e, ok)
	}
	c.Clear()
	if _, ok := c.Load(3); ok {
		t.Error("cleared cache still loads")
	}
}

func (f *fakeNet) EpicAchievements(_ context.Context, sandbox, locale string) ([]Def, map[string]float64, error) {
	f.calls = append(f.calls, "epic:"+sandbox+":"+locale)
	if f.epicDefs == nil {
		return nil, nil, ErrNone
	}
	return f.epicDefs, f.epicRarity, nil
}

func (f *fakeNet) EpicPlayerAchievements(_ context.Context, access, account, sandbox string) (map[string]Unlock, error) {
	f.calls = append(f.calls, "epicPlayer:"+access+":"+account)
	return f.epicPlayer, nil
}

func TestResolveEpic(t *testing.T) {
	net := &fakeNet{
		epicDefs:   []Def{{ID: "a1", Name: "First"}, {ID: "a2", Name: "Second", Hidden: true}},
		epicRarity: map[string]float64{"a1": 40},
		epicPlayer: map[string]Unlock{"a1": {Achieved: true, At: 9}},
	}
	d := testDeps(t, t.TempDir(), net)
	d.EpicLocale = "de"
	g := library.Game{ID: 6, Source: "epic", EpicApp: "ns123:item:App"}

	// Not signed in: the list, and how to see progress.
	l, _ := Resolve(context.Background(), g, d)
	if l.Total != 2 || l.Unlocked != 0 || !strings.Contains(l.Hint, "Sign in to Epic") || l.Items[0].Percent == nil {
		t.Fatalf("anonymous: %+v", l)
	}
	if net.calls[0] != "epic:ns123:de" {
		t.Errorf("calls %v", net.calls)
	}
	// Signed in: progress too; the schema comes from the cache.
	net.calls = nil
	d.Epic = func(context.Context) (string, string, error) { return "tok", "acc", nil }
	l, _ = Resolve(context.Background(), g, d)
	if l.Unlocked != 1 || l.Hint != "" || strings.Join(net.calls, ",") != "epicPlayer:tok:acc" {
		t.Errorf("signed in: %+v calls %v", l, net.calls)
	}
	// Epic says none.
	g.EpicApp = "empty:x:y"
	net.epicDefs = nil
	if l, _ = Resolve(context.Background(), g, d); l.Total != 0 || l.Hint != "" {
		t.Errorf("none: %+v", l)
	}
}

func TestResolveEpicEmulator(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"game/nemirtingasepicemu.json":                            `{"AppId":"ns9"}`,
		"Roaming/NemirtingasEpicEmu/Player/ns9/achievements.json": `[{"AchievementId":"a1","Progress":1}]`,
	})
	net := &fakeNet{epicDefs: []Def{{ID: "a1", Name: "First"}, {ID: "a2", Name: "Second"}}}
	d := testDeps(t, root, net)
	g := library.Game{ID: 8, Source: "folder", External: true, Emulator: "Epic emulator", Dir: filepath.Join(root, "game")}
	l, _ := Resolve(context.Background(), g, d)
	if l.Source != "Epic emulator" || l.Total != 2 || l.Unlocked != 1 || l.Items[0].UnlockedAt != 0 || l.Hint != "" {
		t.Errorf("%+v", l)
	}
}

func (f *fakeNet) GOGAchievements(_ context.Context, access, game, user string) ([]Def, map[string]Unlock, map[string]float64, error) {
	f.calls = append(f.calls, "gog:"+game+":"+user)
	if f.gogErr != nil {
		return nil, nil, nil, f.gogErr
	}
	if game != f.gogFor || f.gogDefs == nil {
		return nil, nil, nil, ErrNone
	}
	return f.gogDefs, f.gogUnlocks, map[string]float64{"W": 7}, nil
}

func TestResolveGOG(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{"game/goggame-1453.info": `{"gameId":"1453","clientId":"5134"}`})
	net := &fakeNet{gogFor: "5134", gogDefs: []Def{{ID: "W", Name: "Win"}, {ID: "L", Name: "Lose"}}, gogUnlocks: map[string]Unlock{"W": {Achieved: true, At: 3}}}
	d := testDeps(t, root, net)
	d.GalaxyDB = filepath.Join(root, "galaxy.db")
	d.GOGUnlocks = func(id string) (map[string]Unlock, error) {
		if id != "1453" {
			t.Errorf("GOGUnlocks(%q)", id)
		}
		return map[string]Unlock{"W": {Achieved: true, At: 3}}, nil
	}
	g := library.Game{ID: 9, Source: "gog", GogID: "1453", Dir: filepath.Join(root, "game")}

	// Not signed in: what Galaxy's database knows, by id.
	l, _ := Resolve(context.Background(), g, d)
	if l.Total != 1 || l.Unlocked != 1 || l.Items[0].Name != "W" || !strings.Contains(l.Hint, "Sign in to GOG") || len(net.calls) != 0 {
		t.Fatalf("anonymous: %+v calls %v", l, net.calls)
	}
	// Signed in: names from GOG (by client id), progress from the database.
	d.GOG = func(context.Context) (string, string, error) { return "tok", "u1", nil }
	l, _ = Resolve(context.Background(), g, d)
	if l.Total != 2 || l.Unlocked != 1 || l.Items[0].Name != "Win" || l.Items[0].Percent == nil || l.Hint != "" || net.calls[0] != "gog:5134:u1" {
		t.Fatalf("signed in: %+v calls %v", l, net.calls)
	}
	// The schema is cached and the database has the progress: nothing asked.
	net.calls = nil
	if l, _ = Resolve(context.Background(), g, d); len(net.calls) != 0 || l.Total != 2 {
		t.Errorf("cached: %+v calls %v", l, net.calls)
	}
	// GOG fails while the database lacks the progress: the cached schema stays.
	d.GOGUnlocks = func(string) (map[string]Unlock, error) { return nil, nil }
	net.gogFor, net.gogErr, net.calls = "5134", errors.New("503"), nil
	l, _ = Resolve(context.Background(), g, d)
	if l.Total != 2 || l.Items[0].Name != "Win" || l.Hint != "GOG didn't answer; try again later." || len(net.calls) == 0 {
		t.Errorf("failed request: %+v calls %v", l, net.calls)
	}
	net.gogErr = nil
	// Only the product id works: tried after the client id.
	g2 := library.Game{ID: 10, Source: "gog", GogID: "77", Dir: filepath.Join(root, "none")}
	net.gogFor, net.calls = "77", nil
	d.GOGUnlocks = func(string) (map[string]Unlock, error) { return nil, nil }
	if l, _ = Resolve(context.Background(), g2, d); l.Unlocked != 1 || strings.Join(net.calls, ",") != "gog:77:u1" {
		t.Errorf("product id: %+v calls %v", l, net.calls)
	}
	// A DRM-free GOG copy from an installer goes the same way.
	g3 := library.Game{ID: 11, Source: "installer", DRMFree: "GOG", GogID: "1453", Dir: filepath.Join(root, "game")}
	if l, _ = Resolve(context.Background(), g3, d); l.Source != "gog" {
		t.Errorf("DRM-free copy: %+v", l)
	}
	// GOG now says 1453 has none: the schema cached before goes.
	if l.Total != 0 || l.Hint != "" {
		t.Errorf("none: %+v", l)
	}
	if GOGClientID(filepath.Join(root, "game"), "../x") != "" {
		t.Error("a path in the id must not be read")
	}
}

func TestIconsParallelAndPartial(t *testing.T) {
	dir := t.TempDir()
	ic := fakeIcons(dir)
	var inFlight, most atomic.Int32
	store := ic.Fetch
	ic.Fetch = func(ctx context.Context, src, dir string) (string, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
		return store(ctx, src, dir)
	}
	items := func(n int) *List {
		l := &List{}
		for i := 0; i < n; i++ {
			l.Items = append(l.Items, Achievement{Def: Def{ID: fmt.Sprint(i), Icon: fmt.Sprintf("https://cdn.akamai.steamstatic.com/%d.jpg", i)}})
		}
		return l
	}
	l := items(30)
	if !ic.Localize(context.Background(), l, false) || most.Load() < 2 || most.Load() > iconWorkers {
		t.Fatalf("complete run: in flight at most %d", most.Load())
	}
	for _, a := range l.Items {
		if !strings.HasPrefix(a.Icon, IconPrefix) {
			t.Fatalf("icon %q not stored", a.Icon)
		}
	}
	// Out of time: partial, and the icons it didn't get aren't marked failed.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	l2 := items(200)
	if ic.Localize(ctx, l2, false) {
		t.Fatal("a run cut short reports complete")
	}
	l3 := items(200)
	if !ic.Localize(context.Background(), l3, false) || !strings.HasPrefix(l3.Items[199].Icon, IconPrefix) {
		t.Error("icons after a cut-short run weren't fetched again")
	}
}

// Games with nothing to look up get no card: an empty source, no items, no hint.
func TestResolveNothingToShow(t *testing.T) {
	root := t.TempDir()
	d := testDeps(t, root, &fakeNet{schema: []Def{{ID: "A"}}})
	d.SteamKey = "k"
	for name, g := range map[string]library.Game{
		"plain folder":               {ID: 1, Source: "folder", Dir: filepath.Join(root, "a")},
		"folder matched to Steam":    {ID: 2, Source: "folder", Dir: filepath.Join(root, "b"), SteamAppID: 620},
		"repack without an emulator": {ID: 3, Source: "installer", External: true, Repacker: "DODI", Dir: filepath.Join(root, "c"), MetaAppID: 620},
		"Steam without an app":       {ID: 4, Source: "steam", Dir: filepath.Join(root, "d")},
		"Epic without an id":         {ID: 5, Source: "epic", Dir: filepath.Join(root, "e")},
		"GOG without an id":          {ID: 6, Source: "gog", Dir: filepath.Join(root, "f")},
	} {
		l, _ := Resolve(context.Background(), g, d)
		if l.Source != "" || l.Total != 0 || l.Hint != "" {
			t.Errorf("%s: %+v", name, l)
		}
	}
	// An unlock file found without a detected emulator still shows.
	mk(t, root, map[string]string{"Roaming/GSE Saves/620/achievements.json": `{"A":{"earned":true}}`})
	if l, _ := Resolve(context.Background(), library.Game{ID: 7, Source: "folder", Dir: filepath.Join(root, "g"), SteamAppID: 620}, d); l.Source != "Goldberg" || l.Unlocked != 1 {
		t.Errorf("unlock file without a marker: %+v", l)
	}
}

// Files lists what each kind of game is read from, and nothing else.
func TestFiles(t *testing.T) {
	root := t.TempDir()
	d := testDeps(t, root, &fakeNet{})
	d.GalaxyDB = filepath.Join(root, "galaxy.db")
	dir := filepath.Join(root, "game")
	goldberg := filepath.Join(dir, "bin", "steam_settings", "achievements.json")
	schema := SteamSchemaFile(d.SteamRoot, 620)
	for name, c := range map[string]struct {
		g        library.Game
		has, not []string
	}{
		"goldberg": {library.Game{Source: "folder", External: true, Emulator: "Goldberg", EmuDir: "bin", Dir: dir, SteamAppID: 620, GogID: "1453"},
			[]string{goldberg, schema, SteamStatsFile(d.SteamRoot, "22", 620)}, []string{d.GalaxyDB}},
		"uplay": {library.Game{Source: "folder", External: true, Emulator: "Uplay emulator", EmuDir: "bin", Dir: dir},
			[]string{filepath.Join(dir, "bin", "achievements_schema.json")}, []string{goldberg, d.GalaxyDB}},
		"gog": {library.Game{Source: "gog", GogID: "1453", Dir: dir},
			[]string{d.GalaxyDB, d.GalaxyDB + "-wal"}, nil},
		"steam": {library.Game{Source: "steam", SteamAppID: 620, GogID: "1453", Dir: dir},
			[]string{schema}, []string{d.GalaxyDB, d.GalaxyDB + "-wal", goldberg}},
	} {
		files := Files(c.g, d)
		for _, f := range c.has {
			if !slices.Contains(files, f) {
				t.Errorf("%s: %s missing from %v", name, f, files)
			}
		}
		for _, f := range c.not {
			if slices.Contains(files, f) {
				t.Errorf("%s: %s in %v", name, f, files)
			}
		}
	}
}

// An icon that fails for want of a connection is tried again; one that's
// bad is given up on, and the list counts as complete.
func TestIconsRetryAfterNetworkError(t *testing.T) {
	dir := t.TempDir()
	ic := fakeIcons(dir)
	store := ic.Fetch
	down := true
	ic.Fetch = func(ctx context.Context, src, dir string) (string, error) {
		if strings.HasSuffix(src, "bad.jpg") {
			return "", &url.Error{Op: "Get", URL: src, Err: errors.New("redirect refused")}
		}
		if down {
			return "", &url.Error{Op: "Get", URL: src, Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("no route to host")}}
		}
		return store(ctx, src, dir)
	}
	items := func() *List {
		return &List{Items: []Achievement{
			{Def: Def{ID: "a", Icon: "https://cdn.akamai.steamstatic.com/a.jpg"}},
			{Def: Def{ID: "b", Icon: "https://cdn.akamai.steamstatic.com/bad.jpg"}},
		}}
	}
	l := items()
	if ic.Localize(context.Background(), l, false) || l.Items[0].Icon != "" {
		t.Fatalf("offline run: %+v", l.Items)
	}
	down = false
	l = items()
	if !ic.Localize(context.Background(), l, false) || !strings.HasPrefix(l.Items[0].Icon, IconPrefix) || l.Items[1].Icon != "" {
		t.Errorf("online again: %+v", l.Items)
	}
}
