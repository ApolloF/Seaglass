package meta

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ApolloF/Seaglass/internal/library"
)

func writePNG(t *testing.T, p string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x += 7 {
		img.Set(x, x%h, color.RGBA{200, 40, 40, 255})
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// A console game without a SteamGridDB key still gets art: its disc's
// background becomes the backdrop and hero (and the cover, cropped), with
// nothing asked of PC stores.
func TestConsoleGameUsesItsOwnPictures(t *testing.T) {
	dir := t.TempDir()
	pic, icon := filepath.Join(dir, "PIC1.PNG"), filepath.Join(dir, "ICON0.PNG")
	writePNG(t, pic, 1920, 1080)
	writePNG(t, icon, 320, 176)
	c := NewClient(filepath.Join(dir, "art"), func() string { return "" })
	c.UseTransport(failingTransport{t})
	m, err := c.Fetch(context.Background(), Request{Title: "Skylanders Giants", Console: &library.Console{Emulator: "RPCS3", Backdrop: pic, Icon: icon, Cover: filepath.Join(dir, "missing.png")}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Backdrop == "" || m.Hero == "" || m.Cover == "" || m.Icon == "" || m.Source != "RPCS3" {
		t.Fatalf("got %+v", m)
	}
}

func TestLoadLocalRefusesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(txt, []byte("x"), 0o644)
	fake := filepath.Join(dir, "fake.png")
	_ = os.WriteFile(fake, []byte("not a picture"), 0o644)
	for _, p := range []string{"", "relative.png", txt, fake, filepath.Join(dir, "missing.png"), dir} {
		if loadLocal(p) != nil {
			t.Errorf("%q was read", p)
		}
	}
}

// failingTransport fails the test on any request: nothing may go online.
type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("request to %s", r.URL)
	return nil, errors.New("offline")
}
