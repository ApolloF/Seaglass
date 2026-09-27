package owned

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/scan"
)

// Epic's public launcher client, the one open-source launchers (Legendary,
// Heroic, Playnite) sign in with. The user signs in on epicgames.com
// themselves; Seaglass only ever sees the one-time code.
const (
	epicClientID     = "34a02cf8f4414e29b15921876da36f9a"
	epicClientSecret = "daafbccc737745039dffe53d94fc76cf"
	epicAccountHost  = "https://account-public-service-prod03.ol.epicgames.com"
	epicLibraryHost  = "https://library-service.live.use1a.on.epicgames.com"
	epicCatalogHost  = "https://catalog-public-service-prod06.ol.epicgames.com"
)

// EpicLoginURL is the page where the user signs in to Epic. After signing
// in it shows a short JSON text with an "authorizationCode".
var EpicLoginURL = "https://www.epicgames.com/id/login?redirectUrl=" +
	url.QueryEscape("https://www.epicgames.com/id/api/redirect?clientId="+epicClientID+"&responseType=code")

var reEpicCode = regexp.MustCompile(`^[0-9a-f]{32}$`)

// EpicCode finds the authorization code in what the user pasted: the
// code itself, or the whole JSON text the sign-in page shows.
func EpicCode(pasted string) (string, bool) {
	s := strings.TrimSpace(pasted)
	if reEpicCode.MatchString(s) {
		return s, true
	}
	if m := regexp.MustCompile(`"authorizationCode"\s*:\s*"([0-9a-f]{32})"`).FindStringSubmatch(s); m != nil {
		return m[1], true
	}
	return "", false
}

// EpicToken is a signed-in Epic account. Only RefreshToken needs keeping
// (encrypted); an access token lasts a few hours.
type EpicToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	DisplayName  string    `json:"displayName"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (c *Client) epicToken(ctx context.Context, form url.Values) (EpicToken, error) {
	form.Set("token_type", "eg1")
	auth := "basic " + base64.StdEncoding.EncodeToString([]byte(epicClientID+":"+epicClientSecret))
	var t EpicToken
	err := c.do(ctx, "POST", epicAccountHost+"/account/api/oauth/token", map[string]string{"Authorization": auth}, form, &t)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 400 || he.Status == 401) {
		return t, errors.New("Epic didn't accept the sign-in; sign in again")
	}
	if err == nil && (t.AccessToken == "" || t.RefreshToken == "") {
		err = errors.New("Epic's sign-in answer is incomplete")
	}
	return t, err
}

// EpicSignIn trades the one-time code from the sign-in page for tokens.
func (c *Client) EpicSignIn(ctx context.Context, code string) (EpicToken, error) {
	return c.epicToken(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}})
}

// EpicRefresh gets a new access token (and refresh token) for a signed-in account.
func (c *Client) EpicRefresh(ctx context.Context, refresh string) (EpicToken, error) {
	return c.epicToken(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
}

type epicRecord struct {
	Namespace     string `json:"namespace"`
	CatalogItemID string `json:"catalogItemId"`
	AppName       string `json:"appName"`
	SandboxType   string `json:"sandboxType"`
}

// Epic lists the games an Epic account owns (no DLC, no Unreal Engine
// assets). Epic doesn't report playtime here.
func (c *Client) Epic(ctx context.Context, access string) ([]library.Owned, error) {
	h := map[string]string{"Authorization": "bearer " + access}
	var recs []epicRecord
	cursor := ""
	for page := 0; page < 50; page++ {
		q := url.Values{"includeMetadata": {"true"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp struct {
			Records []epicRecord `json:"records"`
			Meta    struct {
				Next string `json:"nextCursor"`
			} `json:"responseMetadata"`
		}
		if err := c.do(ctx, "GET", epicLibraryHost+"/library/api/public/items?"+q.Encode(), h, nil, &resp); err != nil {
			return nil, err
		}
		recs = append(recs, resp.Records...)
		if resp.Meta.Next == "" || resp.Meta.Next == cursor {
			break
		}
		cursor = resp.Meta.Next
	}

	// Titles and kinds come from the catalog, one namespace at a time.
	type item struct {
		Title      string `json:"title"`
		Categories []struct {
			Path string `json:"path"`
		} `json:"categories"`
		MainGame *struct {
			ID string `json:"id"`
		} `json:"mainGameItem"`
	}
	byNS := map[string][]epicRecord{}
	for _, r := range recs {
		if r.Namespace == "" || r.CatalogItemID == "" || r.AppName == "" || r.Namespace == "ue" || r.SandboxType == "PRIVATE" {
			continue
		}
		byNS[r.Namespace] = append(byNS[r.Namespace], r)
	}
	var out []library.Owned
	seen := map[string]bool{}
	for ns, rs := range byNS {
		for start := 0; start < len(rs); start += 50 {
			batch := rs[start:min(start+50, len(rs))]
			q := url.Values{"includeDLCDetails": {"true"}, "includeMainGameDetails": {"true"}, "country": {"US"}, "locale": {"en"}}
			for _, r := range batch {
				q.Add("id", r.CatalogItemID)
			}
			items := map[string]item{}
			if err := c.do(ctx, "GET", epicCatalogHost+"/catalog/api/shared/namespace/"+url.PathEscape(ns)+"/bulk/items?"+q.Encode(), h, nil, &items); err != nil {
				return nil, err
			}
			for _, r := range batch {
				it, ok := items[r.CatalogItemID]
				if !ok || strings.TrimSpace(it.Title) == "" || it.MainGame != nil {
					continue // unknown or DLC
				}
				isGame, extra := false, false
				for _, cat := range it.Categories {
					switch cat.Path {
					case "games":
						isGame = true
					case "addons", "digitalextras", "engines", "plugins", "assets":
						extra = true
					}
				}
				if !isGame || extra {
					continue
				}
				id := r.Namespace + ":" + r.CatalogItemID + ":" + r.AppName
				if seen[id] {
					continue
				}
				seen[id] = true
				out = append(out, library.Owned{
					Store: "epic", ID: id, Title: it.Title, SortTitle: scan.SortTitle(it.Title),
					InstallURI: "com.epicgames.launcher://apps/" + url.PathEscape(id) + "?action=install",
				})
			}
		}
	}
	return out, nil
}
