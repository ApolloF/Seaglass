package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/profile"
	"github.com/ApolloF/Seaglass/internal/settings"
)

func TestProfileKey(t *testing.T) {
	cases := []struct {
		g    library.Game
		want string
	}{
		{library.Game{SteamAppID: 10, GogID: "x"}, "steam:10"},
		{library.Game{MetaAppID: 20}, "steam:20"},
		{library.Game{GogID: "123"}, "gog:123"},
		{library.Game{EpicApp: "Fn"}, "epic:fn"},
		{library.Game{Title: "Baldur's Gate III"}, "title:baldurgate3"},
		{library.Game{Title: "™"}, ""},
	}
	for _, c := range cases {
		if got := profileKey(&c.g); got != c.want {
			t.Errorf("%+v: %q, want %q", c.g, got, c.want)
		}
	}
	// The same game gets the same key on every PC, whatever its folder.
	a := library.Game{Title: "Baldurs Gate 3", Dir: `D:\Games\BG3`}
	b := library.Game{Title: "Baldur's Gate III", CustomTitle: "", Dir: `E:\bg3`}
	if profileKey(&a) != profileKey(&b) {
		t.Errorf("%q != %q", profileKey(&a), profileKey(&b))
	}
}

// testProfileCore is a core with a library, settings and a profile in dir.
func testProfileCore(t *testing.T, dir, pc string) *Core {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	lib, err := library.Open(filepath.Join(dir, pc, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Core{Lib: lib, Settings: settings.Open(filepath.Join(dir, pc, "settings.json")), ctx: ctx}
	st, err := profile.Open(filepath.Join(dir, "Profile"), pc, pc)
	if err != nil {
		t.Fatal(err)
	}
	c.profile = &profileState{c: c, store: st, name: pc, kick: make(chan struct{}, 1), unl: map[string]map[string]int64{}}
	lib.ApplyScan([]library.Found{{Key: `c:\games\x`, Title: "X", SortTitle: "x", Source: "steam", Dir: `C:\Games\X`, SteamAppID: 7}}, time.Now())
	return c
}

func TestPlaytimeAndSettingsGoBetweenPCs(t *testing.T) {
	dir := t.TempDir()
	desk, tv := testProfileCore(t, dir, "desk"), testProfileCore(t, dir, "tv")
	for _, c := range []*Core{desk, tv} {
		c.profile.setOwner("anna")
	}
	g := desk.Lib.Games()[0]
	desk.startedPlaying(g)
	desk.addPlaytime(g, 600)
	tv.addPlaytime(tv.Lib.Games()[0], 60)
	_ = desk.profile.store.Flush()
	_ = tv.profile.store.Flush()
	desk.profile.applyAll()
	tv.profile.applyAll()
	for _, c := range []*Core{desk, tv} {
		if got := c.Lib.Games()[0].Playtime; got != 660 {
			t.Fatalf("playtime %d", got)
		}
	}
	if tv.Lib.Games()[0].LastPlayed == 0 {
		t.Fatal("last play didn't arrive")
	}

	// A setting that goes with the person arrives; one of the PC's stays.
	old := desk.Settings.Get()
	next := old
	next.Theme, next.StartInBigPicture = "light", true
	saved, err := desk.saveSettings(next)
	if err != nil {
		t.Fatal(err)
	}
	desk.profile.settingsSaved(old, saved)
	_ = desk.profile.store.Flush()
	tv.profile.applyAll()
	if s := tv.Settings.Get(); s.Theme != "light" || s.StartInBigPicture {
		t.Fatalf("tv settings: theme %q, big picture %v", s.Theme, s.StartInBigPicture)
	}
	// Taking them doesn't make them the tv's own change: nothing goes back.
	_ = tv.profile.store.Flush()
	if tv.profile.store.SettingsAt() != desk.profile.store.SettingsAt() {
		t.Fatal("taken settings got a new time")
	}

	// Ben has none of Anna's playtime.
	tv.profile.setOwner("ben")
	if got := tv.Lib.Games()[0].Playtime; got != 0 {
		t.Fatalf("ben's playtime %d", got)
	}
}

func TestUnlocksFromOtherPCs(t *testing.T) {
	dir := t.TempDir()
	desk, tv := testProfileCore(t, dir, "desk"), testProfileCore(t, dir, "tv")
	g := desk.Lib.Games()[0]
	desk.profile.addAchievements(g, []achievements.Achievement{{Def: achievements.Def{ID: "WIN"}, Unlocked: true, UnlockedAt: 99}})
	_ = desk.profile.store.Flush()
	tv.profile.applyAll()
	l := achievements.List{Items: []achievements.Achievement{
		{Def: achievements.Def{ID: "win"}},
		{Def: achievements.Def{ID: "LOSE"}, Unlocked: true},
	}, Total: 2, Unlocked: 1}
	got := tv.profile.withUnlocks(tv.Lib.Games()[0], l)
	if got.Unlocked != 2 || !got.Items[0].Unlocked || got.Items[0].UnlockedAt != 99 {
		t.Fatalf("got %+v", got)
	}
	if l.Items[0].Unlocked {
		t.Fatal("the cached list was changed")
	}
}

// Play recorded before a game was matched to a store (under its title)
// still counts once it has a store id.
func TestPlaytimeKeepsOldKeys(t *testing.T) {
	c := testProfileCore(t, t.TempDir(), "pc1")
	g := c.Lib.Games()[0]
	ks := profileKeys(&g)
	if len(ks) < 2 || ks[0] != "steam:7" {
		t.Fatalf("keys %v", ks)
	}
	c.profile.store.AddPlaytime(ks[len(ks)-1], "X", 100) // as a title
	c.profile.store.AddPlaytime("steam:7", "X", 50)
	c.profile.applyAll()
	if got, _ := c.Lib.Get(g.ID); got.Playtime != 150 {
		t.Errorf("playtime %d, want 150", got.Playtime)
	}
}
