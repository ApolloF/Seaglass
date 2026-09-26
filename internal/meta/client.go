// Package meta fetches game metadata and art: descriptions, developers,
// genres, controller support, covers, hero banners, logos and icons, from
// Steam (no key needed), GOG and, with the user's key, SteamGridDB. Every
// image is downloaded from an allowlisted host, decoded, resized and
// re-encoded before it is stored, so nothing but plain pixels reaches the
// interface.
package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// allowedHosts are the only hosts WaterLauncher fetches metadata or art from.
var allowedHosts = []string{
	"store.steampowered.com",
	"api.steampowered.com",
	"shared.akamai.steamstatic.com",
	"shared.cloudflare.steamstatic.com",
	"cdn.akamai.steamstatic.com",
	"cdn.cloudflare.steamstatic.com",
	"steamcdn-a.akamaihd.net",
	"api.gog.com",
	".gog-statics.com",
	"account-public-service-prod03.ol.epicgames.com",
	"catalog-public-service-prod06.ol.epicgames.com",
	"cdn1.epicgames.com",
	"cdn2.unrealengine.com",
	"store-content-ipv4.ak.epicgames.com",
	"www.pcgamingwiki.com",
	"images.pcgamingwiki.com",
	"www.steamgriddb.com",
	"cdn2.steamgriddb.com",
	".steamgriddb.com",
}

func allowed(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, a := range allowedHosts {
		if h == a || (strings.HasPrefix(a, ".") && strings.HasSuffix(h, a)) {
			return true
		}
	}
	return false
}

// ErrNotAllowed means a URL points somewhere WaterLauncher doesn't fetch from.
var ErrNotAllowed = errors.New("host not allowed")

// Client talks to the metadata sources.
type Client struct {
	http     *http.Client
	artDir   string
	sgdbKey  func() string
	steamGap time.Duration
	pcgwGap  time.Duration

	mu        sync.Mutex
	lastSteam time.Time
	lastPCGW  time.Time
	maxTries  int    // backdrop candidates to load (0: maxBackdropTries); tests look at all
	epicTok   string // Epic catalog token, until epicExp
	epicExp   time.Time
}

// NewClient stores art in artDir; sgdbKey returns the SteamGridDB key ("" = none).
func NewClient(artDir string, sgdbKey func() string) *Client {
	c := &Client{artDir: artDir, sgdbKey: sgdbKey, steamGap: 1500 * time.Millisecond, pcgwGap: time.Second}
	c.http = &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if !allowed(req.URL) {
				return ErrNotAllowed
			}
			return nil
		},
	}
	return c
}

// UseTransport sends requests through rt and drops the waits between
// them: for tests that record and replay the sources' answers (rt then
// spaces out the requests that really go out).
func (c *Client) UseTransport(rt http.RoundTripper) {
	c.http.Transport = rt
	c.steamGap, c.pcgwGap = 0, 0
}

// waitSteam spaces out Steam store requests: the store API allows about
// 200 requests per five minutes.
func (c *Client) waitSteam(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.lastSteam.Add(c.steamGap))
	if wait < 0 {
		wait = 0
	}
	c.lastSteam = time.Now().Add(wait)
	c.mu.Unlock()
	if wait == 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// get fetches an allowlisted URL, reading at most limit bytes.
func (c *Client) get(ctx context.Context, raw string, limit int64, header map[string]string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !allowed(u) {
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, u.Hostname())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WaterLauncher (+https://github.com/ApolloF/WaterLauncher)")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w (%s)", ErrRateLimited, resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u.Hostname(), resp.Status)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%s: response too large", u.Hostname())
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: response too large", u.Hostname())
	}
	return b, nil
}

func (c *Client) getJSON(ctx context.Context, raw string, v any, header map[string]string) error {
	b, err := c.get(ctx, raw, 8<<20, header)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

var errNotFound = errors.New("not found")

// ErrRateLimited means a source asked us to slow down.
var ErrRateLimited = errors.New("rate limited")
