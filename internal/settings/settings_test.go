package settings

import (
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
