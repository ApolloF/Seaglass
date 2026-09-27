// Package owned lists the games a user's store accounts own, installed or
// not: Steam (with the user's own Web API key), GOG (from GOG Galaxy's
// local database) and Epic (after a sign-in the user completes in the
// browser). It only reads; installing is left to the store's own app.
package owned

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
)

// allowedHosts are the only hosts this package talks to.
var allowedHosts = map[string]bool{
	"api.steampowered.com":                           true,
	"account-public-service-prod03.ol.epicgames.com": true,
	"library-service.live.use1a.on.epicgames.com":    true,
	"catalog-public-service-prod06.ol.epicgames.com": true,
	"launcher.store.epicgames.com":                   true, // achievements (GraphQL)
	"auth.gog.com":                                   true,
	"gameplay.gog.com":                               true,
}

const maxResponse = 32 << 20

// Client makes the requests. The zero value is not usable; use NewClient.
type Client struct {
	http *http.Client
	base map[string]string // host → replacement base URL, for tests

	steamMu   sync.Mutex
	steamNext time.Time // after Steam said "too many requests", wait until then
}

// NewClient makes a client that only reaches the allowlisted hosts.
func NewClient() *Client {
	c := &Client{}
	c.http = &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Scheme != "https" || !allowedHosts[req.URL.Hostname()] {
				return errors.New("redirect not allowed")
			}
			return nil
		},
	}
	return c
}

func (c *Client) do(ctx context.Context, method, raw string, header map[string]string, form url.Values, out any) error {
	if form == nil {
		return c.send(ctx, method, raw, header, "", nil, out)
	}
	return c.send(ctx, method, raw, header, "application/x-www-form-urlencoded", []byte(form.Encode()), out)
}

// send makes a request with an optional body of type ctype and reads a
// JSON answer into out.
func (c *Client) send(ctx context.Context, method, raw string, header map[string]string, ctype string, payload []byte, out any) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if b := c.base[u.Host]; b != "" { // tests
		raw = b + u.Path + "?" + u.RawQuery
	} else if u.Scheme != "https" || !allowedHosts[u.Hostname()] {
		return errors.New("host not allowed")
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.Header.Set("User-Agent", "Seaglass")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the URL can carry a key; never pass it on
		}
		return fmt.Errorf("%s: %w", u.Hostname(), err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{Status: resp.StatusCode, Body: string(b[:min(len(b), 300)])}
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unexpected answer from %s", u.Hostname())
	}
	return nil
}

// HTTPError is a failed request.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d", e.Status) }

// Result is one store's list.
type Result struct {
	Store string
	Games []library.Owned
}
