package steaminput

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ApolloF/gamekit/vdf"
)

func TestID(t *testing.T) {
	// Non-Steam shortcut ids always have the top bit set.
	s := Shortcut{Name: "Test", Exe: `C:\test.exe`}
	if id := s.ID(); id&0x80000000 == 0 {
		t.Fatalf("id %x lacks the top bit", id)
	}
	if got := URI(0x80000001); got != "steam://rungameid/9223372041183297536" {
		t.Errorf("URI = %s", got)
	}
}

func TestAddAndFind(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "shortcuts.vdf")
	// An existing shortcut the user made must survive untouched.
	user := &vdf.BNode{Type: vdf.BMap, Kids: []*vdf.BNode{{Key: "shortcuts", Type: vdf.BMap, Kids: []*vdf.BNode{
		entry("0", Shortcut{Name: "Mine", Exe: `C:\mine.exe`}),
	}}}}
	b, _ := vdf.MarshalBinary(user)
	if err := os.WriteFile(f, b, 0o644); err != nil {
		t.Fatal(err)
	}

	game := Shortcut{Name: "Some Game", Exe: `D:\Games\Some Game\game.exe`, Args: "-dx11"}
	if _, ok := Find([]string{f}, game); ok {
		t.Fatal("found before adding")
	}
	if err := add([]string{f}, []Shortcut{game}); err != nil {
		t.Fatal(err)
	}
	id, ok := Find([]string{f}, game)
	if !ok || id != game.ID() {
		t.Fatalf("Find = %x, %v; want %x", id, ok, game.ID())
	}
	if !fileHas(t, f+".seaglass-backup", b) {
		t.Error("the original wasn't backed up")
	}
	// Adding again changes nothing.
	before, _ := os.ReadFile(f)
	if err := add([]string{f}, []Shortcut{game}); err != nil {
		t.Fatal(err)
	}
	if !fileHas(t, f, before) {
		t.Error("adding an existing shortcut rewrote the file")
	}
	root, _ := read(f)
	scs := root.Child("shortcuts").Kids
	if len(scs) != 2 || scs[0].String("AppName") != "Mine" || scs[1].Key != "1" || scs[1].Child("tags").String("0") != Tag {
		t.Errorf("file = %+v", scs)
	}
}

func TestAddToMissingFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config", "shortcuts.vdf")
	g := Shortcut{Name: "G", Exe: `C:\g.exe`}
	if err := add([]string{f}, []Shortcut{g}); err != nil {
		t.Fatal(err)
	}
	if _, ok := Find([]string{f}, g); !ok {
		t.Error("not found after adding to a new file")
	}
}

func fileHas(t *testing.T, p string, want []byte) bool {
	t.Helper()
	b, err := os.ReadFile(p)
	return err == nil && string(b) == string(want)
}
