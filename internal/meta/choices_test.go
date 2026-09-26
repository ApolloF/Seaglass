package meta

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ApolloF/WaterLauncher/internal/library"
)

// A chosen picture must be one WaterLauncher stored, and is kept as the
// user's choice.
func TestSetArt(t *testing.T) {
	dir := t.TempDir()
	name := "0123456789abcdef0123456789abcdef01234567.jpg"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &library.Meta{Hero: "/art/old.jpg", Tile: "/art/tile.jpg"}
	for _, bad := range []string{"/art/../secret.jpg", "/art/ffffffffffffffffffffffffffffffffffffffff.jpg", `C:\x.jpg`, "https://evil.example/a.jpg"} {
		if SetArt(m, Backdrop, bad, dir) {
			t.Errorf("accepted %q", bad)
		}
	}
	if !SetArt(m, Hero, "/art/"+name, dir) || m.Hero != "/art/"+name || m.Tile != "" {
		t.Errorf("hero not set: %+v", m)
	}
	SetArt(m, Hero, "/art/"+name, dir)
	if !slices.Equal(m.ArtOverrides, []string{"hero"}) {
		t.Errorf("overrides %q", m.ArtOverrides)
	}
	if SetArt(m, Icon, "/art/"+name, dir) {
		t.Error("icons aren't chosen")
	}
}
