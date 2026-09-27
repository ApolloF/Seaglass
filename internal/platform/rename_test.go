package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// WaterLauncher's data moves to Seaglass's folders once, and only when
// Seaglass has none yet.
func TestMoveOldData(t *testing.T) {
	roaming, local := Roaming, Local
	t.Cleanup(func() { Roaming, Local, appDirOverride, localDirOverride = roaming, local, "", "" })
	Roaming, Local = t.TempDir(), t.TempDir()
	for _, base := range []string{Roaming, Local} {
		if err := os.MkdirAll(filepath.Join(base, OldName), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(Roaming, OldName, "library.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if notes := MoveOldData(); len(notes) != 2 {
		t.Fatalf("notes = %q", notes)
	}
	if !IsFile(filepath.Join(AppDir(), "library.json")) || filepath.Base(AppDir()) != "Seaglass" {
		t.Errorf("library not in %s", AppDir())
	}
	if IsDir(filepath.Join(Roaming, OldName)) || IsDir(filepath.Join(Local, OldName)) {
		t.Error("old folders left behind")
	}
	// A later WaterLauncher folder (an old copy run again) is left alone.
	_ = os.MkdirAll(filepath.Join(Roaming, OldName), 0o755)
	if notes := MoveOldData(); len(notes) != 0 {
		t.Errorf("moved again: %q", notes)
	}
}
