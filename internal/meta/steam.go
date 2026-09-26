package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Steam store categories that say something about controllers.
const (
	catFullController = 28
	catDualShock      = 55
	catDualSense      = 57
	catDualSenseBT    = 58
)

type steamDetails struct {
	Name        string   `json:"name"`
	AppID       int      `json:"steam_appid"`
	Type        string   `json:"type"`
	Short       string   `json:"short_description"`
	Developers  []string `json:"developers"`
	Publishers  []string `json:"publishers"`
	Controller  string   `json:"controller_support"`
	HeaderImage string   `json:"header_image"`
	Genres      []struct {
		Description string `json:"description"`
	} `json:"genres"`
	Categories []struct {
		ID int `json:"id"`
	} `json:"categories"`
	Release struct {
		Date string `json:"date"`
	} `json:"release_date"`
	Screenshots []struct {
		Full string `json:"path_full"`
	} `json:"screenshots"`
	// For a DLC: the game it belongs to.
	Fullgame struct {
		AppID string `json:"appid"`
		Name  string `json:"name"`
	} `json:"fullgame"`
}

// steamAppDetails reads the store page data. The response is keyed by
// the store's own id, which isn't always the requested one, so the first
// successful entry is used.
func (c *Client) steamAppDetails(ctx context.Context, appID int) (*steamDetails, error) {
	if err := c.waitSteam(ctx); err != nil {
		return nil, err
	}
	var resp map[string]struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	u := "https://store.steampowered.com/api/appdetails?l=english&cc=US&appids=" + strconv.Itoa(appID)
	if err := c.getJSON(ctx, u, &resp, nil); err != nil {
		return nil, err
	}
	for _, r := range resp {
		if !r.Success || len(r.Data) == 0 || r.Data[0] != '{' {
			continue
		}
		var d steamDetails
		if err := json.Unmarshal(r.Data, &d); err != nil {
			return nil, err
		}
		return &d, nil
	}
	return nil, errNotFound
}

// steamArt is the library art Steam lists for an app, as absolute URLs.
type steamArt struct {
	Cover, Hero, Header string
	LogoCandidates      []string
}

const steamAssetBase = "https://shared.akamai.steamstatic.com/store_item_assets/"

func (c *Client) steamArt(ctx context.Context, appID int) (*steamArt, error) {
	q, _ := json.Marshal(map[string]any{
		"ids":          []map[string]int{{"appid": appID}},
		"context":      map[string]string{"language": "english", "country_code": "US"},
		"data_request": map[string]bool{"include_assets": true},
	})
	var resp struct {
		Response struct {
			Items []struct {
				AppID  int            `json:"appid"`
				Assets map[string]any `json:"assets"`
			} `json:"store_items"`
		} `json:"response"`
	}
	u := "https://api.steampowered.com/IStoreBrowseService/GetItems/v1/?input_json=" + url.QueryEscape(string(q))
	if err := c.getJSON(ctx, u, &resp, nil); err != nil {
		return nil, err
	}
	if len(resp.Response.Items) == 0 {
		return nil, errNotFound
	}
	a := resp.Response.Items[0].Assets
	format, _ := a["asset_url_format"].(string)
	if format == "" {
		format = fmt.Sprintf("steam/apps/%d/${FILENAME}", appID)
	}
	file := func(keys ...string) string {
		for _, k := range keys {
			if name, ok := a[k].(string); ok && name != "" && !strings.ContainsAny(name, "?#\\") {
				return steamAssetBase + strings.Replace(format, "${FILENAME}", name, 1)
			}
		}
		return ""
	}
	art := &steamArt{
		Cover:  file("library_capsule_2x", "library_capsule"),
		Hero:   file("library_hero_2x", "library_hero"),
		Header: file("header_2x", "header"),
	}
	// Logos aren't listed, but live next to the app's other assets.
	base := fmt.Sprintf("%ssteam/apps/%d/", steamAssetBase, appID)
	art.LogoCandidates = []string{base + "logo_2x.png", base + "logo.png"}
	return art, nil
}

// StoreHit is one result of a Steam store search.
type StoreHit struct {
	AppID int    `json:"appId"`
	Name  string `json:"name"`
	Image string `json:"image"` // small capsule on Steam's CDN (not fetched by WaterLauncher)
}

// BaseGame is the game a DLC belongs to; false when appID is a game
// itself (or the store doesn't say). A folder named after a game's
// expansion ("Cyberpunk 2077 Phantom Liberty") holds the game.
func (c *Client) BaseGame(ctx context.Context, appID int) (StoreHit, bool) {
	d, err := c.steamAppDetails(ctx, appID)
	if err != nil || d.Type != "dlc" {
		return StoreHit{}, false
	}
	id, err := strconv.Atoi(d.Fullgame.AppID)
	if err != nil || id <= 0 || d.Fullgame.Name == "" {
		return StoreHit{}, false
	}
	return StoreHit{AppID: id, Name: d.Fullgame.Name}, true
}

// SearchSteam looks a title up on the Steam store.
func (c *Client) SearchSteam(ctx context.Context, term string) ([]StoreHit, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	if err := c.waitSteam(ctx); err != nil {
		return nil, err
	}
	var resp struct {
		Items []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Type  string `json:"type"`
			Image string `json:"tiny_image"`
		} `json:"items"`
	}
	u := "https://store.steampowered.com/api/storesearch/?l=english&cc=US&term=" + url.QueryEscape(term)
	if err := c.getJSON(ctx, u, &resp, nil); err != nil {
		return nil, err
	}
	var out []StoreHit
	for _, it := range resp.Items {
		if it.Type == "app" && it.ID > 0 {
			out = append(out, StoreHit{AppID: it.ID, Name: it.Name, Image: it.Image})
		}
	}
	return out, nil
}

var reTags = regexp.MustCompile(`<[^>]*>`)

// plainText turns store HTML into plain text.
func plainText(s string) string {
	s = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n").Replace(s)
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

var reYear = regexp.MustCompile(`\b(19[7-9]\d|20\d\d)\b`)

func yearOf(date string) int {
	if m := reYear.FindString(date); m != "" {
		y, _ := strconv.Atoi(m)
		return y
	}
	return 0
}
