package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T, dir, pc string) *Store {
	t.Helper()
	s, err := Open(dir, pc, pc)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPCsAddUp(t *testing.T) {
	dir := t.TempDir()
	a, b := open(t, dir, "desk"), open(t, dir, "tv")
	for _, s := range []*Store{a, b} {
		if _, err := s.SetOwner("anna"); err != nil {
			t.Fatal(err)
		}
	}
	a.AddPlaytime("steam:1", "Game", 100)
	a.Played("steam:1", "Game", 50)
	a.AddAchievements("steam:1", "Game", map[string]int64{"WIN": 40, "EARLY": 0})
	b.AddPlaytime("steam:1", "Game", 20)
	b.Played("steam:1", "Game", 70)
	b.AddAchievements("steam:1", "Game", map[string]int64{"WIN": 30, "LATE": 60})
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Store{a, b} {
		g := s.Merged().Games["steam:1"]
		if g.Playtime != 120 || g.LastPlayed != 70 {
			t.Fatalf("totals: %+v", g)
		}
		want := map[string]int64{"WIN": 30, "EARLY": 0, "LATE": 60}
		for id, at := range want {
			if got, ok := g.Achievements[id]; !ok || got != at {
				t.Fatalf("achievement %s: %d, %v (all: %v)", id, got, ok, g.Achievements)
			}
		}
	}
}

func TestOwnersAreApart(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir, "desk")
	_, _ = s.SetOwner("anna")
	s.AddPlaytime("k", "", 10)
	_, _ = s.SetOwner("ben")
	s.AddPlaytime("k", "", 3)
	if got := s.Merged().Games["k"].Playtime; got != 3 {
		t.Fatalf("ben: %d", got)
	}
	_, _ = s.SetOwner("anna")
	if got := s.Merged().Games["k"].Playtime; got != 10 {
		t.Fatalf("anna: %d", got)
	}
}

func TestSharedGoesToFirstAccount(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir, "desk")
	s.Seed(map[string]Game{"k": {Playtime: 60, LastPlayed: 5}}, json.RawMessage(`{"theme":"dark"}`))
	if s.Fresh() {
		t.Fatal("still fresh after seeding")
	}
	s.Seed(map[string]Game{"k": {Playtime: 60}}, nil) // only once
	if _, err := s.SetOwner("anna"); err != nil {
		t.Fatal(err)
	}
	m := s.Merged()
	if m.Games["k"].Playtime != 60 || string(m.Settings) != `{"theme":"dark"}` {
		t.Fatalf("carried over: %+v %s", m.Games["k"], m.Settings)
	}
	if _, err := os.Stat(filepath.Join(dir, Shared, "desk.json")); !os.IsNotExist(err) {
		t.Fatalf("shared file left: %v", err)
	}
}

func TestNewestSettingsWin(t *testing.T) {
	dir := t.TempDir()
	a, b := open(t, dir, "desk"), open(t, dir, "tv")
	a.SetSettings(json.RawMessage(`{"n":1}`), 100)
	b.SetSettings(json.RawMessage(`{"n":2}`), 200)
	_ = a.Flush()
	_ = b.Flush()
	m := a.Merged()
	if string(m.Settings) != `{"n":2}` || m.SettingsAt != 200 || m.SettingsPC != "tv" {
		t.Fatalf("settings: %s %d %s", m.Settings, m.SettingsAt, m.SettingsPC)
	}
}

func TestBadFilesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir, "desk")
	d := filepath.Join(dir, Shared)
	_ = os.MkdirAll(d, 0o755)
	_ = os.WriteFile(filepath.Join(d, "tv.json"), []byte(`{"version":1,"games":{"k":{"playtime":-5,"lastPlayed":9}}}`), 0o644)
	_ = os.WriteFile(filepath.Join(d, "tv.sync-conflict-20260101-000000-ABC.json"), []byte(`{"version":1,"games":{"k":{"playtime":500}}}`), 0o644)
	_ = os.WriteFile(filepath.Join(d, "pc2.json"), []byte(`not json`), 0o644)
	_ = os.WriteFile(filepath.Join(d, "pc3.json"), []byte(`{"version":99,"games":{"k":{"playtime":500}}}`), 0o644)
	g := s.Merged().Games["k"]
	if g.Playtime != 0 || g.LastPlayed != 9 {
		t.Fatalf("got %+v", g)
	}
}

func TestUnreadableOwnFileIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, Shared, "desk.json")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(`{broken`), 0o644)
	s, err := Open(dir, "desk", "desk")
	if err == nil || s.Healthy() {
		t.Fatal("broken file accepted")
	}
	s.AddPlaytime("k", "", 10)
	_ = s.Flush()
	if b, _ := os.ReadFile(p); string(b) != `{broken` {
		t.Fatalf("overwritten: %s", b)
	}
}

// What was played before the first account is merged into it once, even
// when the owner is set again on the next start.
func TestSharedMergedOnce(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "pc1", "PC 1")
	if err != nil {
		t.Fatal(err)
	}
	s.AddPlaytime("steam:1", "A", 60)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetOwner("ann"); err != nil {
		t.Fatal(err)
	}
	s2, _ := Open(dir, "pc1", "PC 1") // next start: Shared, then the owner
	if _, err := s2.SetOwner("ann"); err != nil {
		t.Fatal(err)
	}
	if got := s2.Merged().Games["steam:1"].Playtime; got != 60 {
		t.Errorf("playtime %d, want 60", got)
	}
}

// Syncer brings back a launcher's data folder that was emptied: a file this
// PC wrote again since is kept, and the backup's older copy goes next to it
// as <pc>.restored-<time>.json. Both count, so nothing is lost or doubled.
func TestRestoredCopyOfThisPCsFileCounts(t *testing.T) {
	dir := t.TempDir()
	old := &File{Version: fileVersion, PC: "desk", Games: map[string]*Game{
		"steam:1": {Playtime: 7200, LastPlayed: 50, Achievements: map[string]int64{"WIN": 40}},
	}}
	b, _ := json.Marshal(old)
	if err := os.MkdirAll(filepath.Join(dir, "anna"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anna", "desk.restored-20261002-155700.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir, "desk")
	if _, err := s.SetOwner("anna"); err != nil {
		t.Fatal(err)
	}
	s.AddPlaytime("steam:1", "Game", 130)
	s.Played("steam:1", "Game", 90)
	g := s.Merged().Games["steam:1"]
	if g.Playtime != 7330 || g.LastPlayed != 90 || g.Achievements["WIN"] != 40 {
		t.Fatalf("totals: %+v", g)
	}
}
