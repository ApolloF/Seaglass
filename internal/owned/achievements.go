package owned

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
)

// steamBackoff is how long Steam calls wait after Steam answered "too many requests".
const steamBackoff = 1500 * time.Millisecond

// ErrNoAchievements means the store says the game has no achievements.
var ErrNoAchievements = achievements.ErrNone

// steam makes a Steam Web API call, waiting first when Steam asked to slow down.
func (c *Client) steam(ctx context.Context, raw string, out any) error {
	c.steamMu.Lock()
	wait := time.Until(c.steamNext)
	c.steamMu.Unlock()
	if wait > 0 {
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	err := c.do(ctx, "GET", raw, nil, nil, out)
	var he *HTTPError
	if errors.As(err, &he) && he.Status == http.StatusTooManyRequests {
		c.steamMu.Lock()
		c.steamNext = time.Now().Add(steamBackoff)
		c.steamMu.Unlock()
	}
	return err
}

// flexNum reads a JSON number that Steam sometimes sends as a string.
type flexNum float64

func (f *flexNum) UnmarshalJSON(b []byte) error {
	v, err := strconv.ParseFloat(strings.Trim(string(b), `"`), 64)
	if err != nil {
		return nil // unreadable: 0
	}
	*f = flexNum(v)
	return nil
}

// SteamAchievementSchema is GetSchemaForGame: the game's achievements in
// language lang, with icon URLs. Needs the user's Web API key.
func (c *Client) SteamAchievementSchema(ctx context.Context, key string, appID int, lang string) ([]achievements.Def, error) {
	if !ValidSteamKey(key) {
		return nil, errors.New("no Steam Web API key")
	}
	q := url.Values{"key": {key}, "appid": {strconv.Itoa(appID)}, "l": {lang}}
	var resp struct {
		Game struct {
			Stats struct {
				Achievements []struct {
					Name        string  `json:"name"`
					DisplayName string  `json:"displayName"`
					Description string  `json:"description"`
					Hidden      flexNum `json:"hidden"`
					Icon        string  `json:"icon"`
					IconGray    string  `json:"icongray"`
				} `json:"achievements"`
			} `json:"availableGameStats"`
		} `json:"game"`
	}
	if err := c.steam(ctx, "https://api.steampowered.com/ISteamUserStats/GetSchemaForGame/v2/?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	list := resp.Game.Stats.Achievements
	if len(list) == 0 {
		return nil, ErrNoAchievements
	}
	out := make([]achievements.Def, 0, len(list))
	for _, a := range list {
		if a.Name == "" {
			continue
		}
		out = append(out, achievements.Def{ID: a.Name, Name: a.DisplayName, Desc: a.Description, Hidden: a.Hidden != 0, Icon: httpsOnly(a.Icon), IconGray: httpsOnly(a.IconGray)})
	}
	return out, nil
}

// SteamPlayerAchievements is GetPlayerAchievements: what the account
// unlocked, and when. Needs the key; the profile's game details must be public.
func (c *Client) SteamPlayerAchievements(ctx context.Context, key, steamID string, appID int) (map[string]achievements.Unlock, error) {
	if !ValidSteamKey(key) {
		return nil, errors.New("no Steam Web API key")
	}
	q := url.Values{"key": {key}, "steamid": {steamID}, "appid": {strconv.Itoa(appID)}}
	var resp struct {
		Stats struct {
			Success      bool   `json:"success"`
			Error        string `json:"error"`
			Achievements []struct {
				API      string  `json:"apiname"`
				Achieved flexNum `json:"achieved"`
				Time     int64   `json:"unlocktime"`
			} `json:"achievements"`
		} `json:"playerstats"`
	}
	err := c.steam(ctx, "https://api.steampowered.com/ISteamUserStats/GetPlayerAchievements/v1/?"+q.Encode(), &resp)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 400 || he.Status == 403) {
		return nil, ErrNoAchievements // no stats for this game, or a private profile
	}
	if err != nil {
		return nil, err
	}
	if !resp.Stats.Success {
		return nil, ErrNoAchievements
	}
	out := make(map[string]achievements.Unlock, len(resp.Stats.Achievements))
	for _, a := range resp.Stats.Achievements {
		u := achievements.Unlock{Achieved: a.Achieved != 0}
		if u.Achieved && a.Time > 0 {
			u.At = a.Time
		}
		out[a.API] = u
	}
	return out, nil
}

// SteamRarity is GetGlobalAchievementPercentagesForApp: the share of all
// players who have each achievement. No key needed.
func (c *Client) SteamRarity(ctx context.Context, appID int) (map[string]float64, error) {
	var resp struct {
		P struct {
			Achievements []struct {
				Name    string  `json:"name"`
				Percent flexNum `json:"percent"`
			} `json:"achievements"`
		} `json:"achievementpercentages"`
	}
	err := c.steam(ctx, "https://api.steampowered.com/ISteamUserStats/GetGlobalAchievementPercentagesForApp/v2/?gameid="+strconv.Itoa(appID), &resp)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 400 || he.Status == 403 || he.Status == 404) {
		return nil, ErrNoAchievements
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(resp.P.Achievements))
	for _, a := range resp.P.Achievements {
		if a.Name != "" && a.Percent >= 0 && a.Percent <= 100 {
			out[a.Name] = float64(a.Percent)
		}
	}
	return out, nil
}

// httpsOnly keeps https URLs only: icons are fetched later, through an allowlist.
func httpsOnly(u string) string {
	if strings.HasPrefix(u, "https://") {
		return u
	}
	return ""
}

// Client answers the achievements resolver.
var _ achievements.Net = (*Client)(nil)
