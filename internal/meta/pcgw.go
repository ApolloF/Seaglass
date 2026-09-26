package meta

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PCGamingWiki knows almost every PC game, whichever store sells it (or
// none): its Steam and GOG ids, where else it's sold (the Epic store's
// page, for games only Epic sells), its cover, and what controllers it
// supports, which Steam's store often doesn't say. Its Cargo API refuses
// queries now, but the page's own wikitext comes through MediaWiki's API.
const pcgwAPI = "https://www.pcgamingwiki.com/w/api.php"

// PCGW is what a PCGamingWiki page says about a game.
type PCGW struct {
	Title       string // the page's name: the game's usual title
	SteamAppID  int
	GogID       string
	EpicSlug    string // the Epic store page (store.epicgames.com/p/<slug>)
	Cover       string // file name on the wiki
	Developers  []string
	ReleaseDate string
	Genres      []string
	// Controllers: DualSense is "yes" (DualSense or DualSense Edge
	// supported), "dualshock" (DualShock 4 only), "no" (no PlayStation
	// controllers) or "" (the page doesn't say); Controller is "full",
	// "partial" or "".
	DualSense  string
	Controller string
}

var (
	reWikiField   = regexp.MustCompile(`(?m)^\|[ 	]*([a-z][a-z0-9 ]*?)[ 	]*=[ 	]*(.*)$`)
	reWikiRowArg  = regexp.MustCompile(`\{\{Infobox game/row/(developer|date|taxonomy/genres)[ 	]*\|([^}]*)\}\}`)
	reWikiEpicRow = regexp.MustCompile(`\{\{Availability/row\|\s*Epic Games Store\s*\|\s*([a-z0-9-]+)\s*\|`)
	reEpicSlug    = regexp.MustCompile(`^[a-z0-9-]{1,100}$`)
)

// waitPCGW spaces out wiki requests: it's run by volunteers.
func (c *Client) waitPCGW(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.lastPCGW.Add(time.Second))
	if wait < 0 {
		wait = 0
	}
	c.lastPCGW = time.Now().Add(wait)
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// PCGamingWiki looks a game up by title. The wiki's redirects cover most
// other spellings ("Alan Wake 2" → "Alan Wake II"); a title ending in a
// number is also tried with the other kind of numeral.
func (c *Client) PCGamingWiki(ctx context.Context, title string) (*PCGW, error) {
	title = strings.Join(strings.Fields(strings.NewReplacer("™", "", "®", "", "©", "").Replace(title)), " ")
	if title == "" {
		return nil, errNotFound
	}
	tried := map[string]bool{}
	for _, t := range []string{title, swapNumeral(title)} {
		if t == "" || tried[t] {
			continue
		}
		tried[t] = true
		p, err := c.pcgwPage(ctx, t)
		if err == nil {
			return p, nil
		}
		if err != errNotFound {
			return nil, err
		}
	}
	return nil, errNotFound
}

func (c *Client) pcgwPage(ctx context.Context, title string) (*PCGW, error) {
	if err := c.waitPCGW(ctx); err != nil {
		return nil, err
	}
	q := url.Values{"action": {"query"}, "prop": {"revisions"}, "rvprop": {"content"}, "rvslots": {"main"},
		"format": {"json"}, "redirects": {"1"}, "titles": {title}}
	var resp struct {
		Query struct {
			Pages map[string]struct {
				Title     string  `json:"title"`
				Missing   *string `json:"missing"`
				Revisions []struct {
					Slots struct {
						Main struct {
							Text string `json:"*"`
						} `json:"main"`
					} `json:"slots"`
				} `json:"revisions"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, pcgwAPI+"?"+q.Encode(), &resp, nil); err != nil {
		return nil, err
	}
	for _, p := range resp.Query.Pages {
		if p.Missing != nil || len(p.Revisions) == 0 {
			continue
		}
		text := p.Revisions[0].Slots.Main.Text
		if !strings.Contains(text, "{{Infobox game") {
			continue // a series, a company, a disambiguation page
		}
		info := parsePCGW(text)
		info.Title = p.Title
		return info, nil
	}
	return nil, errNotFound
}

// parsePCGW reads the fields WaterLauncher uses from a page's wikitext.
func parsePCGW(text string) *PCGW {
	p := &PCGW{}
	f := map[string]string{}
	for _, m := range reWikiField.FindAllStringSubmatch(text, -1) {
		if _, dup := f[m[1]]; !dup {
			f[m[1]] = strings.TrimSpace(m[2])
		}
	}
	if id, err := strconv.Atoi(firstField(f["steam appid"])); err == nil && id > 0 {
		p.SteamAppID = id
	}
	if id := firstField(f["gogcom id"]); id != "" && strings.Trim(id, "0123456789") == "" {
		p.GogID = id
	}
	p.Cover = strings.TrimSpace(f["cover"])
	for _, m := range reWikiRowArg.FindAllStringSubmatch(text, -1) {
		args := strings.Split(m[2], "|")
		switch m[1] {
		case "developer":
			if d := strings.TrimSpace(args[0]); d != "" && len(p.Developers) < 3 {
				p.Developers = append(p.Developers, d)
			}
		case "date":
			if p.ReleaseDate == "" && len(args) > 1 && strings.TrimSpace(args[0]) == "Windows" {
				p.ReleaseDate = strings.TrimSpace(args[1])
			}
		case "taxonomy/genres":
			if len(p.Genres) == 0 && len(args) > 0 {
				for _, g := range strings.Split(args[len(args)-1], ",") {
					if g = strings.TrimSpace(g); g != "" && len(p.Genres) < 3 {
						p.Genres = append(p.Genres, g)
					}
				}
			}
		}
	}
	if m := reWikiEpicRow.FindStringSubmatch(text); m != nil && reEpicSlug.MatchString(m[1]) {
		p.EpicSlug = m[1]
	}

	yes := func(k string) bool {
		v := strings.ToLower(f[k])
		return v == "true" || v == "limited" || v == "hackable"
	}
	no := func(k string) bool { return strings.ToLower(f[k]) == "false" }
	switch {
	case yes("full controller"):
		p.Controller = "full"
	case yes("controller support"):
		p.Controller = "partial"
	}
	models := strings.ToLower(f["playstation controller models"])
	switch {
	case yes("playstation controllers") && (strings.Contains(models, "dualsense") || hasModel(models, "ds") || hasModel(models, "dse")):
		p.DualSense = "yes"
	case yes("dualsense haptics support") || yes("dualsense adaptive trigger support"):
		p.DualSense = "yes"
	case yes("playstation controllers"):
		// Models not listed, or only the DualShock 4: a game that reads
		// PlayStation controllers through the usual libraries takes a
		// DualSense too, but only the DualShock 4 is known to work.
		p.DualSense = "dualshock"
	case no("playstation controllers"):
		p.DualSense = "no"
	}
	return p
}

// hasModel reports whether a comma-separated model list names this model.
func hasModel(list, model string) bool {
	for _, m := range strings.Split(list, ",") {
		if strings.TrimSpace(m) == model {
			return true
		}
	}
	return false
}

// firstField is a field's value up to any template or comment after it.
func firstField(v string) string {
	for _, cut := range []string{"{{", "<!--", "|", ",", " "} {
		if i := strings.Index(v, cut); i >= 0 {
			v = v[:i]
		}
	}
	return strings.TrimSpace(v)
}

// pcgwImage finds where the wiki serves one of its files.
func (c *Client) pcgwImage(ctx context.Context, file string) (string, error) {
	if file == "" {
		return "", errNotFound
	}
	if err := c.waitPCGW(ctx); err != nil {
		return "", err
	}
	q := url.Values{"action": {"query"}, "prop": {"imageinfo"}, "iiprop": {"url"}, "format": {"json"}, "titles": {"File:" + file}}
	var resp struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					URL string `json:"url"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, pcgwAPI+"?"+q.Encode(), &resp, nil); err != nil {
		return "", err
	}
	for _, p := range resp.Query.Pages {
		if len(p.ImageInfo) > 0 && strings.HasPrefix(p.ImageInfo[0].URL, "https://") {
			return p.ImageInfo[0].URL, nil
		}
	}
	return "", errNotFound
}

var romans = []struct {
	arabic, roman string
}{{"2", "II"}, {"3", "III"}, {"4", "IV"}, {"5", "V"}, {"6", "VI"}, {"7", "VII"}, {"8", "VIII"}, {"9", "IX"}}

// swapNumeral turns a trailing "2" into "II" and "II" into "2" ("" when
// the title doesn't end in one).
func swapNumeral(title string) string {
	i := strings.LastIndexByte(title, ' ')
	if i < 0 {
		return ""
	}
	last := title[i+1:]
	for _, r := range romans {
		switch last {
		case r.arabic:
			return title[:i+1] + r.roman
		case r.roman:
			return title[:i+1] + r.arabic
		}
	}
	return ""
}

// epicStorePage is the part of the Epic store's page content used here:
// the store's own art, description and developer. It's the public page
// data the store's website reads (no sign-in).
type epicStorePage struct {
	Pages []struct {
		Data struct {
			About struct {
				ShortDescription string `json:"shortDescription"`
				Description      string `json:"description"`
				Developer        string `json:"developerAttribution"`
				Image            struct {
					Src string `json:"src"`
				} `json:"image"`
			} `json:"about"`
			Hero struct {
				Background string `json:"backgroundImageUrl"`
				Portrait   string `json:"portraitBackgroundImageUrl"`
				Logo       struct {
					Src string `json:"src"`
				} `json:"logoImage"`
			} `json:"hero"`
			Carousel struct {
				Items []struct {
					Image struct {
						Src string `json:"src"`
					} `json:"image"`
				} `json:"items"`
			} `json:"carousel"`
		} `json:"data"`
	} `json:"pages"`
}

const epicStoreContent = "https://store-content-ipv4.ak.epicgames.com/api/en-US/content/products/"

// epicStore reads a game's page content from the Epic store.
func (c *Client) epicStore(ctx context.Context, slug string) (*epicStorePage, error) {
	if !reEpicSlug.MatchString(slug) {
		return nil, errNotFound
	}
	var p epicStorePage
	b, err := c.get(ctx, epicStoreContent+slug, 4<<20, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if len(p.Pages) == 0 {
		return nil, errNotFound
	}
	return &p, nil
}
