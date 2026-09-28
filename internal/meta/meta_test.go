package meta

import (
	"context"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/1/logo.png": true,
		"https://images-2.gog-statics.com/x.jpg":                                        true,
		"https://cdn2.steamgriddb.com/grid/x.png":                                       true,
		"http://shared.akamai.steamstatic.com/x.png":                                    false, // not https
		"https://evil.example.com/x.png":                                                false,
		"https://gog-statics.com.evil.com/x.png":                                        false,
		"https://user:pw@api.gog.com/products/1":                                        false,
		"file:///C:/Windows/win.ini":                                                    false,
	} {
		u, _ := url.Parse(raw)
		if got := allowed(u); got != want {
			t.Errorf("allowed(%s) = %v, want %v", raw, got, want)
		}
	}
}

func solid(w, h int, c color.Color) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestAccent(t *testing.T) {
	if a := Accent(solid(64, 64, color.RGBA{200, 40, 40, 255})); a == "" || !strings.HasPrefix(a, "#e") {
		t.Errorf("red image accent = %q", a)
	}
	if a := Accent(solid(64, 64, color.RGBA{128, 128, 128, 255})); a != "" {
		t.Errorf("grey image accent = %q, want none", a)
	}
}

func TestCropCoverAndFit(t *testing.T) {
	c := cropCover(solid(1920, 620, color.White))
	if b := c.Bounds(); b.Dy() != 620 || b.Dx() != 413 {
		t.Errorf("crop = %v", b)
	}
	if b := fit(solid(3000, 900, color.White), 1920).Bounds(); b.Dx() != 1920 || b.Dy() != 576 {
		t.Errorf("fit = %v", b)
	}
}

func TestCrop16x9(t *testing.T) {
	for _, tc := range []struct{ w, h, cw, ch int }{
		{3840, 1240, 2204, 1240}, // Steam's large hero: the middle
		{1024, 768, 1024, 576},   // a 4:3 screenshot: top and bottom
		{1920, 1080, 1920, 1080}, // already 16:9
	} {
		if b := crop16x9(solid(tc.w, tc.h, color.White)).Bounds(); b.Dx() != tc.cw || b.Dy() != tc.ch {
			t.Errorf("crop16x9(%dx%d) = %v, want %dx%d", tc.w, tc.h, b, tc.cw, tc.ch)
		}
	}
	// The crop shares the source's pixels: the middle of a banner that is
	// red on the sides and white in the middle is all white.
	src := solid(400, 100, color.RGBA{255, 0, 0, 255}).(*image.NRGBA)
	white := src.SubImage(image.Rect(111, 0, 289, 100)).(*image.NRGBA)
	for i := range white.Pix {
		white.Pix[i] = 255
	}
	for _, c := range []image.Image{crop16x9(src), cropCover(src)} {
		b := c.Bounds()
		for _, p := range []image.Point{b.Min, b.Max.Sub(image.Pt(1, 1))} {
			if r, g, _, _ := c.At(p.X, p.Y).RGBA(); r != 0xffff || g != 0xffff {
				t.Errorf("crop %v: pixel at %v not white", b, p)
			}
		}
	}
}

func TestFirstParagraphUTF8(t *testing.T) {
	s := strings.Repeat("é", 400) // 800 bytes, no sentence break
	got := firstParagraph(s)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") || len(got) > 600+len("…") {
		t.Errorf("firstParagraph cut %q", got[len(got)-8:])
	}
	if got := firstParagraph("x" + s); !utf8.ValidString(got) {
		t.Errorf("odd offset: invalid UTF-8 %q", got[len(got)-8:])
	}
}

func TestPlainText(t *testing.T) {
	if got := plainText(`<p>Hello &amp; <b>welcome</b></p><br>to the  game`); got != "Hello & welcome to the game" {
		t.Errorf("plainText = %q", got)
	}
	if yearOf("Aug 3, 2023") != 2023 || yearOf("Coming soon") != 0 {
		t.Error("yearOf")
	}
}

func TestArtHandler(t *testing.T) {
	dir := t.TempDir()
	name := strings.Repeat("a", 40) + ".jpg"
	if err := os.WriteFile(dir+`\`+name, []byte("jpegdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := ArtHandler(dir)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(299) }))
	for path, want := range map[string]int{
		"/art/" + name:             200,
		"/art/../secrets/sgdb.bin": 404,
		"/art/..%5Clibrary.json":   404,
		"/art/" + name + "x":       404,
		"/index.html":              299,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://wails.localhost"+path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}
}

// TestRealFetch fetches Baldur's Gate 3 and Stardew Valley from Steam:
// WL_REAL_META=1 go test -run RealFetch -v ./internal/meta
func TestRealFetch(t *testing.T) {
	if os.Getenv("WL_REAL_META") == "" {
		t.Skip("set WL_REAL_META=1 to fetch from Steam")
	}
	dir := os.Getenv("WL_REAL_META_DIR") // to look at the art afterwards
	if dir == "" {
		dir = t.TempDir()
	}
	c := NewClient(dir, nil)
	for _, id := range []int{1086940, 413150} {
		m, err := c.Fetch(context.Background(), Request{SteamAppID: id})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%d: cover=%s hero=%s backdrop=%s tile=%s logo=%s accent=%s dualsense=%q year=%d genres=%v dev=%v\n  %s", id, m.Cover, m.Hero, m.Backdrop, m.Tile, m.Logo, m.Accent, m.DualSense, m.ReleaseYear, m.Genres, m.Developers, m.Description)
		if m.Cover == "" || m.Hero == "" || m.Backdrop == "" || m.Tile == "" {
			t.Errorf("%d: missing art", id)
		}
	}
	// A game only Epic sells, from Epic's catalog.
	m, err := c.Fetch(context.Background(), Request{Title: "Fortnite", EpicApp: "fn:4fe75bbc5a674f4f9b356b5c90567da5:Fortnite"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("epic: cover=%s hero=%s backdrop=%s tile=%s logo=%s dev=%v year=%d source=%s", m.Cover, m.Hero, m.Backdrop, m.Tile, m.Logo, m.Developers, m.ReleaseYear, m.Source)
	if m.Cover == "" || m.Hero == "" || m.Backdrop == "" || m.Source != "Epic" {
		t.Error("epic: missing art")
	}
}

func TestPruneArt(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	files := map[string]bool{
		"1111111111111111111111111111111111111111.jpg":     true,  // in use
		"2222222222222222222222222222222222222222.png":     false, // unused
		"3333333333333333333333333333333333333333.jpg.tmp": false,
		"notes.txt": true, // not ours
	}
	for name := range files {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte("x"), 0o644)
		_ = os.Chtimes(p, old, old)
	}
	fresh := filepath.Join(dir, "4444444444444444444444444444444444444444.jpg")
	_ = os.WriteFile(fresh, []byte("x"), 0o644) // just written: kept
	n, _ := PruneArt(dir, map[string]bool{"/art/1111111111111111111111111111111111111111.jpg": true}, 24*time.Hour)
	if n != 2 {
		t.Errorf("removed %d, want 2", n)
	}
	for name, stays := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != stays {
			t.Errorf("%s: exists=%v, want %v", name, err == nil, stays)
		}
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a fresh file was removed")
	}
}
