package meta

import (
	"context"
	"image"
	"image/color"
	"math/rand"
	"os"
	"testing"
)

// A page shaped like PCGamingWiki's (fields and templates as the wiki
// writes them), for a game only Epic sells.
const wikiEpicOnly = `{{Infobox game
|cover        = Some Game 2 cover.png
|developers   = 
{{Infobox game/row/developer|Studio One}}
{{Infobox game/row/developer|Studio Two}}
|release dates= 
{{Infobox game/row/date|Windows|October 27, 2023}}
|taxonomy     =
{{Infobox game/row/taxonomy/genres        | Action, Horror, Adventure, Puzzle }}
|steam appid  = 
|gogcom id    = 
}}
==Availability==
{{Availability|
{{Availability/row| Epic Games Store | some-game-2 | DRM-free* | Deluxe Edition also available. |  | Windows }}
}}
===Input===
{{Input
|controller support        = true
|full controller           = true
|playstation controllers   = true
|playstation controller models = DS4, DS, DSE
|dualsense haptics support  = limited 
}}`

func TestParsePCGW(t *testing.T) {
	p := parsePCGW(wikiEpicOnly)
	if p.SteamAppID != 0 || p.GogID != "" || p.EpicSlug != "some-game-2" || p.Cover != "Some Game 2 cover.png" {
		t.Errorf("ids: %+v", p)
	}
	if len(p.Developers) != 2 || p.Developers[0] != "Studio One" || p.ReleaseDate != "October 27, 2023" || len(p.Genres) != 3 || p.Genres[0] != "Action" {
		t.Errorf("details: %+v", p)
	}
	if p.DualSense != "yes" || p.Controller != "full" {
		t.Errorf("controllers: %q %q", p.DualSense, p.Controller)
	}

	for _, tc := range []struct{ text, ds, ctl string }{
		{"|steam appid = 413150\n|playstation controllers = true\n|playstation controller models = DualShock 4, DualSense\n|full controller = true", "yes", "full"},
		{"|playstation controllers = true\n|playstation controller models = DualShock 4\n|controller support = true", "dualshock", "partial"},
		{"|playstation controllers = false\n|controller support = true\n|full controller = false", "no", "partial"},
		{"|controller support = false", "", ""},
		{"|playstation controllers = unknown", "", ""},
	} {
		p := parsePCGW(tc.text)
		if p.DualSense != tc.ds || p.Controller != tc.ctl {
			t.Errorf("%q: got %q %q, want %q %q", tc.text, p.DualSense, p.Controller, tc.ds, tc.ctl)
		}
	}
	if p := parsePCGW("|steam appid  = 1086940, 1419430\n|gogcom id = 1456460669 <!-- x -->"); p.SteamAppID != 1086940 || p.GogID != "1456460669" {
		t.Errorf("first id: %+v", p)
	}
	if p := parsePCGW("{{Availability/row| Epic Games Store | ../../evil | DRM }}"); p.EpicSlug != "" {
		t.Errorf("bad slug accepted: %q", p.EpicSlug)
	}
}

func TestSwapNumeral(t *testing.T) {
	for in, want := range map[string]string{"Alan Wake 2": "Alan Wake II", "Alan Wake II": "Alan Wake 2", "Hades": "", "Diablo IV": "Diablo 4", "Portal 10": ""} {
		if got := swapNumeral(in); got != want {
			t.Errorf("swapNumeral(%q) = %q, want %q", in, got, want)
		}
	}
}

// A calm, dark left side and a sharp picture beat busy, bright or small ones.
func TestBackdropScore(t *testing.T) {
	fill := func(w, h int, px func(x, y int) color.Color) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, px(x, y))
			}
		}
		return img
	}
	r := rand.New(rand.NewSource(1))
	// Dark on the left, a lit subject on the right.
	calm := func(w, h int) image.Image {
		return fill(w, h, func(x, y int) color.Color {
			if x > w*6/10 {
				v := uint8(120 + r.Intn(120))
				return color.RGBA{v, v / 2, 40, 255}
			}
			return color.RGBA{20, 24, 30, 255}
		})
	}
	noisy := fill(1920, 1080, func(x, y int) color.Color { v := uint8(r.Intn(256)); return color.RGBA{v, v, v, 255} })
	bright := fill(1920, 1080, func(x, y int) color.Color {
		if x > 1500 {
			return color.RGBA{30, 30, 30, 255}
		}
		return color.RGBA{235, 235, 225, 255}
	})
	flat := fill(1920, 1080, func(x, y int) color.Color { return color.RGBA{5, 5, 5, 255} })
	s4k, s1080 := backdropScore(calm(3840, 2160)), backdropScore(calm(1920, 1080))
	sn, sb, sf := backdropScore(noisy), backdropScore(bright), backdropScore(flat)
	t.Logf("calm 4K %.2f, calm 1080p %.2f, noisy %.2f, bright %.2f, flat %.2f", s4k, s1080, sn, sb, sf)
	if !(s4k > s1080 && s1080 > sn && s1080 > sb && sf == 0) {
		t.Error("backdrop scores in the wrong order")
	}
	if s4k < goodBackdrop {
		t.Errorf("a calm 4K picture should be good enough to stop at: %.2f", s4k)
	}
}

// TestRealWiki reads PCGamingWiki and the Epic store for a game only Epic
// sells: WL_REAL_META=1 go test -run RealWiki -v ./internal/meta
func TestRealWiki(t *testing.T) {
	if os.Getenv("WL_REAL_META") == "" {
		t.Skip("set WL_REAL_META=1 to fetch from PCGamingWiki")
	}
	dir := os.Getenv("WL_REAL_META_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	c := NewClient(dir, nil)
	ctx := context.Background()
	p, err := c.PCGamingWiki(ctx, "Alan Wake 2")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wiki: %+v", p)
	if p.Title != "Alan Wake II" || p.EpicSlug == "" || p.DualSense != "yes" {
		t.Errorf("wiki: %+v", p)
	}
	m, err := c.Fetch(ctx, Request{Title: "Alan Wake II", PCGW: p})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("meta: source=%s cover=%s hero=%s backdrop=%s logo=%s icon=%s dualsense=%q controller=%q dev=%v year=%d\n  %s", m.Source, m.Cover, m.Hero, m.Backdrop, m.Logo, m.Icon, m.DualSense, m.Controller, m.Developers, m.ReleaseYear, m.Description)
	if m.Cover == "" || m.Hero == "" || m.Backdrop == "" || m.DualSense != "yes" || m.Description == "" {
		t.Error("missing metadata")
	}
	// A Steam game gets its PlayStation support from the wiki too.
	m, err = c.Fetch(ctx, Request{Title: "Hades", SteamAppID: 1145360})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("hades: dualsense=%q controller=%q backdrop=%s", m.DualSense, m.Controller, m.Backdrop)
	if m.DualSense != "yes" {
		t.Error("Hades supports the DualSense")
	}
}
