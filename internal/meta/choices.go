package meta

import (
	"context"
	"fmt"
	"sync"

	"github.com/ApolloF/Seaglass/internal/library"
)

// Choice is one picture a game's art can be changed to: already stored
// (the interface only shows local art), with where it came from.
type Choice struct {
	Art    string `json:"art"` // /art/… URL
	Source string `json:"source"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// maxChoices caps the pictures offered per kind.
const maxChoices = 12

// ArtChoices gathers the pictures a game's cover, backdrop, hero or logo
// could be: what Steam has (its library art and every screenshot), the
// Epic store's page for a game only Epic sells, PCGamingWiki's cover and,
// with a key, SteamGridDB's community art. Each is downloaded and stored
// the way fetched art is. The current picture comes first.
func (c *Client) ArtChoices(ctx context.Context, r Request, kind Kind, current *library.Meta) ([]Choice, error) {
	type cand struct{ url, source string }
	var cands []cand
	add := func(source string, urls ...string) {
		for _, u := range urls {
			if u != "" {
				cands = append(cands, cand{u, source})
			}
		}
	}
	if r.SteamAppID > 0 {
		base := fmt.Sprintf("%ssteam/apps/%d/", steamAssetBase, r.SteamAppID)
		a, _ := c.steamArt(ctx, r.SteamAppID)
		switch kind {
		case Cover:
			if a != nil {
				add("Steam", a.Cover)
			}
			add("Steam", base+"library_600x900_2x.jpg")
		case Hero, Backdrop:
			if a != nil {
				add("Steam", a.Hero)
			}
			add("Steam", base+"library_hero_2x.jpg")
			if d, err := c.steamAppDetails(ctx, r.SteamAppID); err == nil {
				for _, s := range d.Screenshots {
					if s.Full == "" {
						continue
					}
					if orig := reShotSize.ReplaceAllString(s.Full, "$1"); orig != s.Full && kind == Backdrop {
						add("Steam screenshot", orig)
					} else {
						add("Steam screenshot", s.Full)
					}
				}
			}
		case Logo:
			if a != nil {
				add("Steam", a.LogoCandidates...)
			}
		}
	}
	wiki := r.PCGW
	if wiki == nil && r.Title != "" {
		wiki, _ = c.PCGamingWiki(ctx, r.Title)
	}
	if wiki != nil && wiki.EpicSlug != "" && r.SteamAppID == 0 {
		if p, err := c.epicStore(ctx, wiki.EpicSlug); err == nil {
			d := p.Pages[0].Data
			switch kind {
			case Cover:
				add("Epic", d.Hero.Portrait, d.About.Image.Src)
			case Hero, Backdrop:
				add("Epic", d.Hero.Background)
				for _, it := range d.Carousel.Items {
					add("Epic", it.Image.Src)
				}
			case Logo:
				add("Epic", d.Hero.Logo.Src)
			}
		}
	}
	if wiki != nil && kind == Cover {
		if u, err := c.pcgwImage(ctx, wiki.Cover); err == nil {
			add("PCGamingWiki", u)
		}
	}
	if id, err := c.sgdbGameID(ctx, r.SteamAppID, r.Title); err == nil {
		sk := map[Kind]string{Cover: "grids", Hero: "heroes", Backdrop: "heroes", Logo: "logos"}[kind]
		if ims, err := c.sgdbArt(ctx, id, sk); err == nil {
			for _, im := range ims[:min(8, len(ims))] {
				add("SteamGridDB", im.URL)
			}
		}
	}

	// Download and store them a few at a time, keeping the order.
	seen := map[string]bool{}
	var todo []cand
	for _, cd := range cands {
		if !seen[cd.url] && len(todo) < maxChoices*2 {
			seen[cd.url] = true
			todo = append(todo, cd)
		}
	}
	out := make([]*Choice, len(todo))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, cd := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			art, img, err := c.saveImage(ctx, cd.url, kind)
			if err != nil || img == nil {
				return
			}
			b := img.Bounds()
			out[i] = &Choice{Art: art, Source: cd.source, Width: b.Dx(), Height: b.Dy()}
		}()
	}
	wg.Wait()
	var list []Choice
	have := map[string]bool{}
	if cur := currentArt(current, kind); cur != "" {
		list = append(list, Choice{Art: cur, Source: "Current"})
		have[cur] = true
	}
	for _, ch := range out {
		if ch != nil && !have[ch.Art] && len(list) < maxChoices+1 {
			have[ch.Art] = true
			list = append(list, *ch)
		}
	}
	return list, nil
}

// currentArt is the game's picture of a kind now.
func currentArt(m *library.Meta, kind Kind) string {
	if m == nil {
		return ""
	}
	switch kind {
	case Cover:
		return m.Cover
	case Hero:
		return m.Hero
	case Backdrop:
		return m.Backdrop
	case Logo:
		return m.Logo
	}
	return ""
}

// SetArt makes a stored picture the game's art of a kind, and keeps it
// through later metadata refreshes. It reports false for a kind that
// can't be chosen or a name that isn't stored art.
func SetArt(m *library.Meta, kind Kind, art string, artDir string) bool {
	if m == nil || !IsStoredArt(artDir, art) {
		return false
	}
	switch kind {
	case Cover:
		m.Cover = art
	case Hero:
		m.Hero = art
		m.Tile = "" // made from the old hero; the interface puts the new one together
	case Backdrop:
		m.Backdrop = art
	case Logo:
		m.Logo = art
		m.Tile = ""
	default:
		return false
	}
	for _, o := range m.ArtOverrides {
		if o == string(kind) {
			return true
		}
	}
	m.ArtOverrides = append(m.ArtOverrides, string(kind))
	return true
}
