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
	// Seaglass made a folder of its own already (its controller DLL).
	_ = os.MkdirAll(filepath.Join(Local, "Seaglass", "bin"), 0o755)
	_ = os.MkdirAll(filepath.Join(Local, OldName, "art"), 0o755)
	_ = os.MkdirAll(filepath.Join(Local, OldName, "bin"), 0o755)
	if notes := MoveOldData(false); len(notes) != 3 {
		t.Fatalf("notes = %q", notes)
	}
	if !IsDir(filepath.Join(CacheDir(), "art")) {
		t.Error("art wasn't moved into Seaglass's folder")
	}
	if !IsFile(filepath.Join(AppDir(), "library.json")) || filepath.Base(AppDir()) != "Seaglass" {
		t.Errorf("library not in %s", AppDir())
	}
	if IsDir(filepath.Join(Roaming, OldName)) {
		t.Error("old folder left behind")
	}
	if !IsDir(filepath.Join(Local, OldName, "bin")) {
		t.Error("WaterLauncher's bin, which Seaglass has too, should stay where it was")
	}
	// While WaterLauncher runs, its folders are used where they are.
	_ = os.MkdirAll(filepath.Join(Roaming, OldName), 0o755)
	_ = os.WriteFile(filepath.Join(Roaming, OldName, "library.json"), []byte("{}"), 0o644)
	MoveOldData(true)
	if filepath.Base(AppDir()) != OldName {
		t.Errorf("app dir = %s, want WaterLauncher's while it runs", AppDir())
	}
}
