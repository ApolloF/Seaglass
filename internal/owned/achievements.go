package owned

import (
	"context"
	"encoding/json"
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

// ---- Epic ----

const epicGraphQL = "https://launcher.store.epicgames.com/graphql"

const epicAchievementsQuery = `query Achievement($sandboxId: String!, $locale: String!) {
  Achievement {
    productAchievementsRecordBySandbox(sandboxId: $sandboxId, locale: $locale) {
      achievements {
        achievement {
          name hidden
          unlockedDisplayName lockedDisplayName unlockedDescription lockedDescription
          unlockedIconLink lockedIconLink
          rarity { percent }
        }
      }
    }
  }
}`

const epicPlayerQuery = `query PlayerAchievement($epicAccountId: String!, $sandboxId: String!) {
  PlayerAchievement {
    playerAchievementGameRecordsBySandbox(epicAccountId: $epicAccountId, sandboxId: $sandboxId) {
      records {
        playerAchievements {
          playerAchievement { achievementName unlocked unlockDate progress }
        }
      }
    }
  }
}`

func (c *Client) epicGraphQL(ctx context.Context, access, query string, vars map[string]string, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	h := map[string]string{}
	if access != "" {
		h["Authorization"] = "bearer " + access
	}
	return c.send(ctx, "POST", epicGraphQL, h, "application/json", body, out)
}

// EpicLocale is the Epic store locale for a Steam language name.
func EpicLocale(steamLang string) string {
	switch steamLang {
	case "german":
		return "de"
	case "french":
		return "fr"
	case "spanish":
		return "es-ES"
	case "latam":
		return "es-MX"
	case "italian":
		return "it"
	case "portuguese":
		return "pt-PT"
	case "brazilian":
		return "pt-BR"
	case "polish":
		return "pl"
	case "russian":
		return "ru"
	case "japanese":
		return "ja"
	case "koreana":
		return "ko"
	case "schinese":
		return "zh-CN"
	case "tchinese":
		return "zh-Hant"
	case "turkish":
		return "tr"
	case "dutch":
		return "nl"
	}
	return "en-US"
}

// EpicAchievements are a game's achievements on Epic (by its sandbox, the
// namespace), with Epic's own rarity. No sign-in needed.
func (c *Client) EpicAchievements(ctx context.Context, sandbox, locale string) ([]achievements.Def, map[string]float64, error) {
	var resp struct {
		Data struct {
			Achievement struct {
				Record *struct {
					Achievements []struct {
						A struct {
							Name         string `json:"name"`
							Hidden       bool   `json:"hidden"`
							UnlockedName string `json:"unlockedDisplayName"`
							LockedName   string `json:"lockedDisplayName"`
							UnlockedDesc string `json:"unlockedDescription"`
							LockedDesc   string `json:"lockedDescription"`
							UnlockedIcon string `json:"unlockedIconLink"`
							LockedIcon   string `json:"lockedIconLink"`
							Rarity       struct {
								Percent flexNum `json:"percent"`
							} `json:"rarity"`
						} `json:"achievement"`
					} `json:"achievements"`
				} `json:"productAchievementsRecordBySandbox"`
			} `json:"Achievement"`
		} `json:"data"`
	}
	if err := c.epicGraphQL(ctx, "", epicAchievementsQuery, map[string]string{"sandboxId": sandbox, "locale": locale}, &resp); err != nil {
		return nil, nil, err
	}
	rec := resp.Data.Achievement.Record
	if rec == nil || len(rec.Achievements) == 0 {
		return nil, nil, ErrNoAchievements
	}
	defs := make([]achievements.Def, 0, len(rec.Achievements))
	rarity := map[string]float64{}
	for _, x := range rec.Achievements {
		a := x.A
		if a.Name == "" {
			continue
		}
		d := achievements.Def{ID: a.Name, Name: a.UnlockedName, Desc: a.UnlockedDesc, Hidden: a.Hidden,
			Icon: httpsOnly(a.UnlockedIcon), IconGray: httpsOnly(a.LockedIcon)}
		if d.Name == "" {
			d.Name = a.LockedName
		}
		if d.Desc == "" {
			d.Desc = a.LockedDesc
		}
		defs = append(defs, d)
		if p := float64(a.Rarity.Percent); p > 0 && p <= 100 {
			rarity[a.Name] = p
		}
	}
	return defs, rarity, nil
}

// EpicPlayerAchievements is what a signed-in Epic account unlocked in a game.
func (c *Client) EpicPlayerAchievements(ctx context.Context, access, account, sandbox string) (map[string]achievements.Unlock, error) {
	var resp struct {
		Data struct {
			PA struct {
				Records *struct {
					Records []struct {
						List []struct {
							P struct {
								Name     string  `json:"achievementName"`
								Unlocked bool    `json:"unlocked"`
								Date     string  `json:"unlockDate"`
								Progress flexNum `json:"progress"`
							} `json:"playerAchievement"`
						} `json:"playerAchievements"`
					} `json:"records"`
				} `json:"playerAchievementGameRecordsBySandbox"`
			} `json:"PlayerAchievement"`
		} `json:"data"`
	}
	err := c.epicGraphQL(ctx, access, epicPlayerQuery, map[string]string{"epicAccountId": account, "sandboxId": sandbox}, &resp)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 401 || he.Status == 403) {
		return nil, errors.New("Epic didn't accept the sign-in; sign in again")
	}
	if err != nil {
		return nil, err
	}
	out := map[string]achievements.Unlock{}
	if r := resp.Data.PA.Records; r != nil {
		for _, rec := range r.Records {
			for _, x := range rec.List {
				p := x.P
				if p.Name == "" {
					continue
				}
				u := achievements.Unlock{Achieved: p.Unlocked}
				if t, err := time.Parse(time.RFC3339, p.Date); err == nil && p.Unlocked && t.Year() > 2000 {
					u.At = t.Unix()
				}
				out[p.Name] = u
			}
		}
	}
	return out, nil
}
