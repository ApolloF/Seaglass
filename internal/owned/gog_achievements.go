package owned

import (
	"context"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/sqlite"
)

// GOG's public Galaxy client, the one open-source launchers (Heroic,
// Playnite, lgogdownloader) sign in with. The user signs in on gog.com
// themselves; Seaglass only sees the one-time code. Galaxy's own sign-in
// is never touched: using its refresh token could sign Galaxy out.
const (
	gogClientID     = "46899977096215655"
	gogClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
	gogRedirect     = "https://embed.gog.com/on_login_success?origin=client"
	gogAuthHost     = "https://auth.gog.com"
	gogGameplayHost = "https://gameplay.gog.com"
)

// GOGLoginURL is the page where the user signs in to GOG. Afterwards the
// browser lands on a page whose address holds the code.
var GOGLoginURL = gogAuthHost + "/auth?client_id=" + gogClientID +
	"&redirect_uri=" + url.QueryEscape(gogRedirect) + "&response_type=code&layout=client2"

var reGOGCode = regexp.MustCompile(`(?:^|[?&]code=)([A-Za-z0-9_\-]{20,})`)

// GOGCode finds the code in what the user pasted: the whole address the
// sign-in ended on, or the code alone.
func GOGCode(pasted string) (string, bool) {
	s := strings.TrimSpace(pasted)
	if m := reGOGCode.FindStringSubmatch(s); m != nil {
		return m[1], true
	}
	return "", false
}

// GOGToken is a signed-in GOG account. Only RefreshToken is kept (encrypted).
type GOGToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (c *Client) gogToken(ctx context.Context, q url.Values) (GOGToken, error) {
	q.Set("client_id", gogClientID)
	q.Set("client_secret", gogClientSecret)
	var t GOGToken
	err := c.do(ctx, "GET", gogAuthHost+"/token?"+q.Encode(), nil, nil, &t)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 400 || he.Status == 401) {
		return t, errors.New("GOG didn't accept the sign-in; sign in again")
	}
	if err == nil && (t.AccessToken == "" || t.RefreshToken == "" || t.UserID == "") {
		err = errors.New("GOG's sign-in answer is incomplete")
	}
	return t, err
}

// GOGSignIn trades the one-time code for tokens.
func (c *Client) GOGSignIn(ctx context.Context, code string) (GOGToken, error) {
	return c.gogToken(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {gogRedirect}})
}

// GOGRefresh gets a new access token (and refresh token).
func (c *Client) GOGRefresh(ctx context.Context, refresh string) (GOGToken, error) {
	return c.gogToken(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
}

// gogTime reads GOG's dates ("2019-06-08T12:34:56+0000").
func gogTime(s string) int64 {
	for _, layout := range []string{"2006-01-02T15:04:05-0700", time.RFC3339, time.DateTime} {
		if t, err := time.Parse(layout, s); err == nil && t.Year() > 2000 {
			return t.Unix()
		}
	}
	return 0
}

// GOGAchievements are a game's achievements with the account's progress
// and GOG's rarity, from gameplay.gog.com. game is the game's client id
// (from its goggame-<id>.info) or, failing that, its product id.
func (c *Client) GOGAchievements(ctx context.Context, access, game, userID string) ([]achievements.Def, map[string]achievements.Unlock, map[string]float64, error) {
	var resp struct {
		Items []struct {
			Key         string  `json:"achievement_key"`
			Visible     *bool   `json:"visible"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Unlocked    string  `json:"image_url_unlocked"`
			Locked      string  `json:"image_url_locked"`
			Date        *string `json:"date_unlocked"`
			Rarity      flexNum `json:"rarity"`
		} `json:"items"`
	}
	u := gogGameplayHost + "/clients/" + url.PathEscape(game) + "/users/" + url.PathEscape(userID) + "/achievements"
	err := c.do(ctx, "GET", u, map[string]string{"Authorization": "Bearer " + access}, nil, &resp)
	var he *HTTPError
	switch {
	case errors.As(err, &he) && (he.Status == 401 || he.Status == 403):
		return nil, nil, nil, errors.New("GOG didn't accept the sign-in; sign in again")
	case errors.As(err, &he) && he.Status == 404:
		return nil, nil, nil, ErrNoAchievements
	case err != nil:
		return nil, nil, nil, err
	}
	if len(resp.Items) == 0 {
		return nil, nil, nil, ErrNoAchievements
	}
	defs := make([]achievements.Def, 0, len(resp.Items))
	unlocks := map[string]achievements.Unlock{}
	rarity := map[string]float64{}
	for _, it := range resp.Items {
		if it.Key == "" {
			continue
		}
		defs = append(defs, achievements.Def{ID: it.Key, Name: it.Name, Desc: it.Description,
			Hidden: it.Visible != nil && !*it.Visible, Icon: httpsOnly(it.Unlocked), IconGray: httpsOnly(it.Locked)})
		un := achievements.Unlock{}
		if it.Date != nil && *it.Date != "" {
			un.Achieved, un.At = true, gogTime(*it.Date)
		}
		unlocks[it.Key] = un
		if p := float64(it.Rarity); p > 0 && p <= 100 {
			rarity[it.Key] = p
		}
	}
	return defs, unlocks, rarity, nil
}

// GOGUnlocks reads what GOG Galaxy's database knows the account unlocked
// in a game (by its product id): Galaxy syncs it while it runs.
func GOGUnlocks(dbPath, gogID string) (map[string]achievements.Unlock, error) {
	if dbPath == "" || gogID == "" {
		return nil, os.ErrNotExist
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if !db.HasTable("UserAchievements") {
		return nil, os.ErrNotExist
	}
	rows, err := db.Rows("UserAchievements")
	if err != nil {
		return nil, err
	}
	str := func(r map[string]any, keys ...string) string {
		for _, k := range keys {
			switch v := r[k].(type) {
			case string:
				return v
			case int64:
				return strconv.FormatInt(v, 10)
			}
		}
		return ""
	}
	key := "gog_" + gogID
	out := map[string]achievements.Unlock{}
	for _, r := range rows {
		if str(r, "gameReleaseKey", "releaseKey") != key {
			continue
		}
		id := str(r, "apikey", "apiKey", "achievementKey", "achievement_key")
		if id == "" {
			continue
		}
		t := str(r, "unlockTime", "unlockDate", "dateUnlocked")
		u := achievements.Unlock{}
		if n, err := strconv.ParseInt(t, 10, 64); err == nil && n > 0 {
			u.At = n
		} else if t != "" {
			u.At = gogTime(t)
		}
		switch v := r["isUnlocked"].(type) {
		case int64:
			u.Achieved = v != 0
		default:
			u.Achieved = u.At > 0
		}
		if !u.Achieved {
			u.At = 0
		}
		out[id] = u
	}
	return out, nil
}
