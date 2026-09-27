package meta

import (
	"context"
	"errors"
	"fmt"
	"image"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
)

// Version marks what Fetch gathers; metadata from an older version is
// fetched again (2: backdrops; 3: full-size backdrops, round tiles, Steam's
// own art files when its store lists none; 4: PCGamingWiki's controller
// support, the Epic store's art for games only Epic sells, backdrops
// picked from several and kept up to 4K).
const Version = 4

// reShotSize is the size Steam puts in a screenshot's file name.
var reShotSize = regexp.MustCompile(`\.\d+x\d+(\.jpg)`)

// Request says which game to fetch metadata for.
type Request struct {
	Title      string
	SteamAppID int
	GogID      string
	EpicApp    string        // namespace:item:appName, for games Steam doesn't have
	Keep       *library.Meta // previous metadata: user-chosen art is kept
	// PCGW is the game's PCGamingWiki page when the caller already looked
	// it up; otherwise Fetch looks it up by Title.
	PCGW *PCGW
}

// Fetch gathers metadata and art for one game. Missing pieces are simply
// left empty; an error means nothing at all could be fetched (and is
// ErrRateLimited when the caller should back off).
func (c *Client) Fetch(ctx context.Context, r Request) (*library.Meta, error) {
	m := &library.Meta{FetchedAt: time.Now().Unix(), Version: Version}
	var urls struct{ cover, hero, logo, icon []string }
	var backdrops []backdropCandidate
	var errs []error
	found := false
	wiki := r.PCGW
	if wiki == nil {
		if p, err := c.PCGamingWiki(ctx, r.Title); err == nil {
			wiki = p
		}
	}

	if r.SteamAppID > 0 {
		if d, err := c.steamAppDetails(ctx, r.SteamAppID); err == nil {
			found = true
			m.Source = "Steam"
			m.Description = plainText(d.Short)
			m.Developers, m.Publishers = d.Developers, d.Publishers
			for _, g := range d.Genres {
				m.Genres = append(m.Genres, g.Description)
			}
			m.ReleaseDate = d.Release.Date
			m.ReleaseYear = yearOf(d.Release.Date)
			m.Controller = d.Controller
			for _, cat := range d.Categories {
				switch cat.ID {
				case catDualSense, catDualSenseBT:
					m.DualSense = "yes"
				case catDualShock:
					if m.DualSense == "" {
						m.DualSense = "dualshock"
					}
				}
			}
			if d.HeaderImage != "" {
				urls.hero = append(urls.hero, d.HeaderImage)
			}
			// The first screenshots, picked by the developer, make a sharp
			// backdrop that isn't the same picture as the game's tile. The
			// store lists them at 1920 wide; the file as uploaded (often
			// 4K) has the same name without the size.
			for _, s := range d.Screenshots[:min(4, len(d.Screenshots))] {
				if s.Full != "" {
					alts := []string{s.Full}
					if orig := reShotSize.ReplaceAllString(s.Full, "$1"); orig != s.Full {
						alts = []string{orig, s.Full}
					}
					backdrops = append(backdrops, backdropCandidate{alts: alts})
				}
			}
		} else {
			errs = append(errs, err)
		}
		if a, err := c.steamArt(ctx, r.SteamAppID); err == nil {
			found = true
			if a.Cover != "" {
				urls.cover = append(urls.cover, a.Cover)
			}
			if a.Hero != "" {
				urls.hero = append([]string{a.Hero}, urls.hero...)
			}
			if a.Header != "" {
				urls.hero = append(urls.hero, a.Header)
			}
			urls.logo = append(urls.logo, a.LogoCandidates...)
		} else {
			errs = append(errs, err)
		}
		// Steam's CDN keeps an app's library art under fixed names, also
		// for games its store no longer lists: tried when nothing better
		// worked.
		base := fmt.Sprintf("%ssteam/apps/%d/", steamAssetBase, r.SteamAppID)
		urls.cover = append(urls.cover, base+"library_600x900_2x.jpg", base+"library_600x900.jpg")
		urls.hero = append(urls.hero, base+"library_hero.jpg", base+"header.jpg")
		// The library hero is key art without text, 3840 wide.
		backdrops = append([]backdropCandidate{{alts: []string{base + "library_hero_2x.jpg"}, bonus: keyArt}}, backdrops...)
	}
	if r.GogID != "" {
		if p, err := c.gogProduct(ctx, r.GogID); err == nil {
			found = true
			if m.Source == "" {
				m.Source = "GOG"
			}
			if m.Description == "" {
				m.Description = firstParagraph(plainText(p.Description.Lead + "\n" + p.Description.Full))
			}
			if m.ReleaseYear == 0 {
				m.ReleaseYear = yearOf(p.ReleaseDate)
			}
			if u := gogURL(p.Images.Background); u != "" {
				urls.hero = append(urls.hero, u)
			}
			if u := gogURL(p.Images.Icon); u != "" {
				urls.icon = append(urls.icon, u)
			}
		} else {
			errs = append(errs, err)
		}
	}
	// Epic's catalog, for games in an Epic library that Steam doesn't sell.
	if r.SteamAppID == 0 && r.EpicApp != "" {
		if e, err := c.epicGame(ctx, r.EpicApp); err == nil {
			found = true
			if m.Source == "" {
				m.Source = "Epic"
			}
			if m.Description == "" {
				m.Description = firstParagraph(plainText(e.Description))
			}
			if len(m.Developers) == 0 && e.Developer != "" {
				m.Developers = []string{e.Developer}
			}
			if m.ReleaseYear == 0 && len(e.ReleaseInfo) > 0 {
				m.ReleaseYear = yearOf(e.ReleaseInfo[0].DateAdded)
			}
			if u := e.image("DieselGameBoxTall", "OfferImageTall"); u != "" {
				urls.cover = append(urls.cover, u)
			}
			// The wide box art is 2560×1440: a backdrop as it is, and a hero.
			if u := e.image("DieselGameBox", "OfferImageWide", "DieselStoreFrontWide"); u != "" {
				urls.hero = append(urls.hero, u)
				backdrops = append([]backdropCandidate{{alts: []string{u}, bonus: epicArt}}, backdrops...)
			}
			if u := e.image("DieselGameBoxLogo"); u != "" {
				urls.logo = append(urls.logo, u)
			}
		} else {
			errs = append(errs, err)
		}
	}
	if wiki != nil {
		found = true
		c.addWiki(ctx, m, wiki, r.SteamAppID == 0 && r.EpicApp == "", &urls.cover, &urls.hero, &urls.logo, &urls.icon, &backdrops)
	}
	// SteamGridDB fills whatever the stores didn't have.
	if len(urls.cover) == 0 || len(urls.hero) == 0 || len(urls.logo) == 0 {
		if id, err := c.sgdbGameID(ctx, r.SteamAppID, r.Title); err == nil {
			found = true
			if m.Source == "" {
				m.Source = "SteamGridDB"
			}
			for _, k := range []struct {
				kind string
				dst  *[]string
			}{{"grids", &urls.cover}, {"heroes", &urls.hero}, {"logos", &urls.logo}} {
				if len(*k.dst) > 0 {
					continue
				}
				if ims, err := c.sgdbArt(ctx, id, k.kind); err == nil && len(ims) > 0 {
					*k.dst = append(*k.dst, ims[0].URL)
				}
			}
		}
	}
	if !found {
		// Busy: try again later rather than settle for less.
		for _, err := range errs {
			if errors.Is(err, ErrRateLimited) {
				return nil, err
			}
		}
	}

	var heroImg, coverImg, logoImg image.Image
	m.Cover, coverImg = c.firstImage(ctx, urls.cover, Cover)
	m.Hero, heroImg = c.firstImage(ctx, urls.hero, Hero)
	// Failing those, the middle of a large hero (Steam's 3840-wide one)
	// still fills the screen sharply; a small one is left to the hero.
	if len(urls.hero) > 0 {
		backdrops = append(backdrops, backdropCandidate{alts: []string{urls.hero[0]}})
	}
	m.Backdrop = c.pickBackdrop(ctx, backdrops)
	m.Logo, logoImg = c.firstImage(ctx, urls.logo, Logo)
	m.Icon, _ = c.firstImage(ctx, urls.icon, Icon)
	if !found {
		// Only Steam's art files answered (a game its store no longer
		// lists): that's still worth keeping.
		if m.Cover == "" && m.Hero == "" {
			if len(errs) > 0 {
				return nil, errors.Join(errs...)
			}
			return nil, errNotFound
		}
		m.Source = "Steam"
	}
	if m.Cover == "" && heroImg != nil {
		m.Cover, _ = c.storeImage(cropCover(heroImg), Cover)
	}
	if t := makeTile(heroImg, logoImg, coverImg); t != nil {
		m.Tile, _ = c.storeImage(t, Tile)
	}
	switch {
	case coverImg != nil:
		m.Accent = Accent(coverImg)
	case heroImg != nil:
		m.Accent = Accent(heroImg)
	}
	if m.Accent == "" && heroImg != nil {
		m.Accent = Accent(heroImg)
	}
	keepOverrides(m, r.Keep)
	return m, nil
}

// firstImage stores the first candidate URL that yields a valid image.
func (c *Client) firstImage(ctx context.Context, urls []string, kind Kind) (string, image.Image) {
	for _, u := range urls {
		if art, img, err := c.saveImage(ctx, u, kind); err == nil {
			return art, img
		}
	}
	return "", nil
}

// keepOverrides carries over art the user picked themselves.
func keepOverrides(m, old *library.Meta) {
	if old == nil {
		return
	}
	for _, o := range old.ArtOverrides {
		switch o {
		case "cover":
			m.Cover = old.Cover
		case "hero":
			m.Hero = old.Hero
			if !slices.Contains(old.ArtOverrides, "backdrop") {
				m.Backdrop = "" // the user's hero, not a screenshot, behind the game
			}
			m.Tile = "" // made from the store's hero; the interface puts the user's together
		case "backdrop":
			m.Backdrop = old.Backdrop
		case "logo":
			m.Logo = old.Logo
			m.Tile = ""
		}
	}
	m.ArtOverrides = old.ArtOverrides
}

func firstParagraph(s string) string {
	if len(s) > 600 {
		if i := strings.LastIndex(s[:600], ". "); i > 200 {
			return s[:i+1]
		}
		return s[:600] + "…"
	}
	return s
}

// addWiki adds what PCGamingWiki knows: controller support (Steam's store
// often doesn't list PlayStation controllers), details Steam didn't give,
// and, for a game Steam doesn't sell (epic: no Steam app and no Epic
// library entry), the Epic store's art and the wiki's cover.
func (c *Client) addWiki(ctx context.Context, m *library.Meta, w *PCGW, epic bool, cover, hero, logo, icon *[]string, backdrops *[]backdropCandidate) {
	switch {
	case w.DualSense == "":
	case m.DualSense == "", m.DualSense == "dualshock" && w.DualSense == "yes":
		m.DualSense = w.DualSense
	}
	if m.Controller == "" {
		m.Controller = w.Controller
	}
	if len(m.Developers) == 0 {
		m.Developers = w.Developers
	}
	if len(m.Genres) == 0 {
		m.Genres = w.Genres
	}
	if m.ReleaseDate == "" && w.ReleaseDate != "" {
		m.ReleaseDate = w.ReleaseDate
		m.ReleaseYear = yearOf(w.ReleaseDate)
	}
	if epic && w.EpicSlug != "" {
		if p, err := c.epicStore(ctx, w.EpicSlug); err == nil {
			d := p.Pages[0].Data
			if m.Source == "" {
				m.Source = "Epic"
			}
			if m.Description == "" {
				m.Description = firstParagraph(plainText(firstNonEmpty(d.About.ShortDescription, d.About.Description)))
			}
			if len(m.Developers) == 0 && d.About.Developer != "" {
				m.Developers = []string{d.About.Developer}
			}
			if u := firstNonEmpty(d.Hero.Portrait, d.About.Image.Src); u != "" {
				*cover = append(*cover, u)
			}
			if d.Hero.Background != "" {
				*hero = append(*hero, d.Hero.Background)
				*backdrops = append([]backdropCandidate{{alts: []string{d.Hero.Background}, bonus: epicArt}}, *backdrops...)
			}
			for _, it := range d.Carousel.Items {
				if it.Image.Src != "" && len(*backdrops) < 6 {
					*backdrops = append(*backdrops, backdropCandidate{alts: []string{it.Image.Src}})
				}
			}
			// The store's "logo" is sometimes the square icon.
			if u := d.Hero.Logo.Src; u != "" {
				*logo = append(*logo, u)
				*icon = append(*icon, u)
			}
		}
	}
	if m.Source == "" {
		m.Source = "PCGamingWiki"
	}
	if len(*cover) == 0 {
		if u, err := c.pcgwImage(ctx, w.Cover); err == nil {
			*cover = append(*cover, u)
		}
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
