package achievements

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ApolloF/Seaglass/internal/library"
)

type fakeNet struct {
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
	g := library.Game{ID: 7, Source: "folder", Unofficial: true, Emulator: "Goldberg", EmuDir: "bin", Dir: filepath.Join(root, "game"), SteamAppID: 620}
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
	g := library.Game{ID: 1, Source: "installer", Unofficial: true, Emulator: "CODEX", Dir: filepath.Join(root, "game"), SteamAppID: 620}
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
	g := library.Game{ID: 2, Source: "folder", Unofficial: true, Emulator: "SmartSteamEmu", Dir: filepath.Join(root, "game"), SteamAppID: 620}
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
	g := library.Game{ID: 3, Source: "folder", Unofficial: true, Emulator: "PLAZA", Dir: filepath.Join(root, "game"), SteamAppID: 620}
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
	g := library.Game{ID: 8, Source: "folder", Unofficial: true, Emulator: "Epic emulator", Dir: filepath.Join(root, "game")}
	l, _ := Resolve(context.Background(), g, d)
	if l.Source != "Epic emulator" || l.Total != 2 || l.Unlocked != 1 || l.Items[0].UnlockedAt != 0 || l.Hint != "" {
		t.Errorf("%+v", l)
	}
}
