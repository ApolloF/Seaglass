package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A new install sees the welcome; someone updating from before it
// existed doesn't.
func TestWelcomed(t *testing.T) {
	dir := t.TempDir()
	if Open(filepath.Join(dir, "none.json")).Get().Welcomed {
		t.Error("a first start is welcomed already")
	}
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Open(old).Get().Welcomed {
		t.Error("settings from before the welcome should count as welcomed")
	}
	fresh := filepath.Join(dir, "fresh.json")
	if err := os.WriteFile(fresh, []byte(`{"theme":"dark","welcomed":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Open(fresh).Get().Welcomed {
		t.Error("an unfinished welcome should show again")
	}
}

// Only libraries the interface knows can be hidden, each once.
func TestHiddenSources(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "settings.json"))
	if got := s.Get().HiddenSources; got == nil || len(got) != 0 {
		t.Fatalf("new settings hide %v", got)
	}
	v := s.Get()
	v.HiddenSources = []string{"epic", "nope", "epic", "unofficial"}
	got, err := s.Set(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.HiddenSources) != 2 || got.HiddenSources[0] != "epic" || got.HiddenSources[1] != "unofficial" {
		t.Errorf("hidden sources = %v, want [epic unofficial]", got.HiddenSources)
	}
}

// Achievements are on for everyone, updaters included; hidden ones stay hidden.
func TestAchievementDefaults(t *testing.T) {
	old := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(old, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := Open(old).Get(); !v.Achievements || v.ShowHiddenAchievements {
		t.Errorf("achievements %v, hidden shown %v", v.Achievements, v.ShowHiddenAchievements)
	}
}

func TestAskWhoPlaysOff(t *testing.T) {
	if Defaults().AskWhoPlays {
		t.Error("asking who's playing is on by default")
	}
}

func TestPortable(t *testing.T) {
	a := Defaults()
	a.Theme, a.Glyphs, a.HiddenSources, a.Folders, a.StartInBigPicture = "light", "xbox", []string{"epic"}, []string{`D:\Games`}, true
	a.AskWhoPlays = true // a PC people take turns on
	b, err := json.Marshal(a.Portable())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Defaults().WithPortable(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "light" || got.Glyphs != "xbox" || len(got.HiddenSources) != 1 {
		t.Fatalf("not taken: %+v", got)
	}
	if len(got.Folders) != 0 || got.StartInBigPicture || got.AskWhoPlays {
		t.Fatalf("the PC's own settings changed: %+v", got)
	}
	// Bad values from another PC are normalised.
	got, _ = Defaults().WithPortable([]byte(`{"theme":"neon","syncWait":7}`))
	if got.Theme != "system" || got.SyncWait != 60 {
		t.Fatalf("not normalised: %+v", got)
	}
}
