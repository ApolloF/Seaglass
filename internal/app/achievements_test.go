package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/settings"
)

// achTestCore is a Core with one Goldberg game, its unlock file under root.
func achTestCore(t *testing.T) (*Core, int64, string) {
	t.Helper()
	root := t.TempDir()
	lib, err := library.Open(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "Game")
	lib.ApplyScan([]library.Found{{Key: `c:\game`, Title: "Game", Source: "folder", Dir: game, Emulator: "Goldberg", Unofficial: true, SteamAppID: 620}}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Core{Lib: lib, Settings: settings.Open(filepath.Join(root, "settings.json")), ctx: ctx}
	a := &achState{c: c, cache: &achievements.Cache{Dir: filepath.Join(root, "cache")}, mem: map[int64]achievements.Entry{}}
	env := achievements.Env{Roaming: filepath.Join(root, "Roaming")}
	a.deps = func() achievements.Deps {
		return achievements.Deps{Env: env, Lang: "english", Cache: a.cache}
	}
	a.active = func() bool { return false }
	c.ach = a
	for p, s := range map[string]string{
		filepath.Join(game, "steam_settings", "achievements.json"):          `[{"name":"A","displayName":"Alpha"},{"name":"B","displayName":"Beta"}]`,
		filepath.Join(env.Roaming, "GSE Saves", "620", "achievements.json"): `{"A":{"earned":true,"earned_time":1700000000}}`,
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return c, lib.Games()[0].ID, filepath.Join(env.Roaming, "GSE Saves", "620", "achievements.json")
}

func TestAchievementsGetAndCache(t *testing.T) {
	c, id, unlocks := achTestCore(t)
	s := NewAchievementsService(c)
	l, err := s.Get(id, false)
	if err != nil || l.Total != 2 || l.Unlocked != 1 || l.Source != "Goldberg" {
		t.Fatalf("list %+v err %v", l, err)
	}
	first := l.UpdatedAt
	// Nothing changed: the cached list (from disk, after a restart too).
	c.ach.mem = map[int64]achievements.Entry{}
	if l, _ = s.Get(id, false); l.UpdatedAt != first || l.Unlocked != 1 {
		t.Errorf("not reused: %+v", l)
	}
	// The unlock file changed: read again.
	if err := os.WriteFile(unlocks, []byte(`{"A":{"earned":true,"earned_time":1700000000},"B":{"earned":true,"earned_time":1700000100}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(unlocks, later, later)
	if l, _ = s.Get(id, false); l.Unlocked != 2 {
		t.Errorf("changed file not read: %+v", l)
	}
	// Turned off: nothing is read.
	v := c.Settings.Get()
	v.Achievements = false
	if _, err := c.Settings.Set(v); err != nil {
		t.Fatal(err)
	}
	if l, _ = s.Get(id, true); l.Total != 0 || l.Items == nil {
		t.Errorf("off: %+v", l)
	}
	if _, err := s.Get(9999, false); err == nil {
		t.Error("unknown game: no error")
	}
}

func TestNewlyUnlocked(t *testing.T) {
	item := func(id string, on bool, at int64) achievements.Achievement {
		return achievements.Achievement{Def: achievements.Def{ID: id}, Unlocked: on, UnlockedAt: at}
	}
	before := achievements.List{Items: []achievements.Achievement{item("A", true, 1), item("B", false, 0), item("C", false, 0)}}
	after := achievements.List{Items: []achievements.Achievement{item("A", true, 1), item("B", true, 5), item("C", true, 9), item("D", false, 0)}}
	got := newlyUnlocked(before, after)
	if len(got) != 2 || got[0].ID != "C" || got[1].ID != "B" {
		t.Errorf("got %+v", got)
	}
}

// Unlocks read while the game runs (the interface asks again at each
// phase) still count as the session's: "before" is taken as it starts.
func TestSessionBaseline(t *testing.T) {
	c, id, unlocks := achTestCore(t)
	a := c.ach
	if _, err := a.get(c.ctx, id, false); err != nil {
		t.Fatal(err)
	}
	a.sessionStarted(7, id)
	a.sessionStarted(7, id) // again, later phases: kept as it was
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.mu.Lock()
		b := a.baseline[7]
		a.mu.Unlock()
		if b.GameID == id || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(unlocks, []byte(`{"A":{"earned":true,"earned_time":1700000000},"B":{"earned":true,"earned_time":1700000100}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(unlocks, later, later)
	if l, _ := a.get(c.ctx, id, false); l.Unlocked != 2 { // the interface, mid-session
		t.Fatalf("mid-session read: %+v", l)
	}
	a.mu.Lock()
	before := a.baseline[7]
	a.mu.Unlock()
	after, _ := a.get(c.ctx, id, true)
	if fresh := newlyUnlocked(before, after); len(fresh) != 1 || fresh[0].ID != "B" {
		t.Errorf("session unlocked %+v", fresh)
	}
	a.afterSession(7, id, "Game", 0)
	a.sessionStarted(8, id)
	a.forget(8)
	a.mu.Lock()
	n := len(a.baseline)
	a.mu.Unlock()
	if n != 0 {
		t.Errorf("baselines left: %d", n)
	}
}
