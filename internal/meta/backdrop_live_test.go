package meta

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// TestRealBackdrops writes every backdrop candidate of some well-known
// games with its score, to judge the picker by eye:
//
//	WL_REAL_META=1 WL_REAL_META_DIR=… go test -run RealBackdrops -v ./internal/meta
func TestRealBackdrops(t *testing.T) {
	dir := os.Getenv("WL_REAL_META_DIR")
	if os.Getenv("WL_REAL_META") == "" || dir == "" {
		t.Skip("set WL_REAL_META=1 and WL_REAL_META_DIR to fetch from Steam")
	}
	c := NewClient(filepath.Join(dir, "art"), nil)
	c.maxTries = 99 // see every candidate
	games := []struct {
		title string
		app   int
	}{{"Hades", 1145360}, {"Stardew Valley", 413150}, {"Baldur's Gate 3", 1086940}, {"Elden Ring", 1245620}, {"Cyberpunk 2077", 1091500}, {"Cult of the Lamb", 1313140}, {"Alan Wake II", 0}}
	for _, g := range games {
		n := 0
		traceBackdrop = func(src string, img image.Image, score float64) {
			n++
			f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("%s %d %.2f.jpg", g.title, n, score)))
			_ = jpeg.Encode(f, fit(img, 640), nil)
			f.Close()
		}
		m, err := c.Fetch(context.Background(), Request{Title: g.title, SteamAppID: g.app})
		if err != nil {
			t.Errorf("%s: %v", g.title, err)
			continue
		}
		t.Logf("%s: backdrop %s", g.title, m.Backdrop)
	}
	traceBackdrop = nil
}
