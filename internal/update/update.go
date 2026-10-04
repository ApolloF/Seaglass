// Package update keeps Seaglass up to date from its GitHub releases.
//
// It asks GitHub for the newest release (prereleases don't count), downloads
// the installer (or, for a copy that wasn't installed, the bare exe) together
// with its published SHA-256, and checks the hash, and the Authenticode
// publisher when the running exe is signed, before the file is ever run.
// Downloads come only from the repository's own release assets.
package update

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// Asset names every release carries. They stay the same across versions, so
// github.com/…/releases/latest/download/<name> always points at the newest.
const (
	InstallerAsset = "Seaglass-setup.exe"
	ExeAsset       = "Seaglass.exe"
)

// Feed is where releases come from.
type Feed struct {
	LatestURL   string   // GitHub's "latest release" API endpoint
	AssetPrefix string   // every download URL must start with this
	Hosts       []string // hosts a download may be redirected to
	Client      *http.Client
	// Keys are the release keys; with any, only files listed in a signed
	// SHA256SUMS are accepted.
	Keys []ed25519.PublicKey
	// Product is the name signed into each release ("Seaglass" when empty).
	Product string
}

// Signed reports whether the feed's downloads are checked against signed
// releases, not only against checksums GitHub serves beside them.
func (f Feed) Signed() bool { return len(f.Keys) > 0 }

func (f Feed) product() string {
	if f.Product == "" {
		return defaultProduct
	}
	return f.Product
}

// GitHub is Seaglass's own release feed.
var GitHub = Feed{
	LatestURL:   "https://api.github.com/repos/ApolloF/Seaglass/releases/latest",
	AssetPrefix: "https://github.com/ApolloF/Seaglass/releases/download/",
	Hosts:       []string{"github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"},
	Keys:        ReleaseKeys,
}

// ReleasesPage is where people download Seaglass by hand.
const ReleasesPage = "https://github.com/ApolloF/Seaglass/releases/latest"

// Asset is one downloadable file of a release.
type Asset struct {
	Name   string
	URL    string
	Size   int64
	Digest string // SHA-256 GitHub computed on upload ("sha256:<hex>"), when it did
}

// Release is a published Seaglass release.
type Release struct {
	Tag       string // "v1.0.0"
	Notes     string // markdown, shortened
	Page      string // release page on github.com
	Published time.Time
	assets    map[string]Asset
}

// Asset returns the release's file with this name.
func (r Release) Asset(name string) (Asset, bool) {
	a, ok := r.assets[name]
	return a, ok
}

func (f Feed) client() *http.Client {
	c := &http.Client{Timeout: 10 * time.Minute}
	if f.Client != nil {
		*c = *f.Client
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !f.hostAllowed(req.URL) {
			return fmt.Errorf("download redirected to %s, which isn't allowed", req.URL.Hostname())
		}
		return nil
	}
	return c
}

func (f Feed) hostAllowed(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, a := range f.Hosts {
		if h == a {
			return true
		}
	}
	return false
}

// inRelease reports whether raw is a plain URL below the release prefix
// (no "..", escapes or spaces that could lead it elsewhere).
func (f Feed) inRelease(raw string) bool {
	if !strings.HasPrefix(raw, f.AssetPrefix) || strings.ContainsAny(raw, "\r\n \\%") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && f.hostAllowed(u) && path.Clean(u.Path) == u.Path
}

const maxNotes = 4000

// Latest asks GitHub for the newest release.
func (f Feed) Latest(ctx context.Context) (Release, error) {
	if u, err := url.Parse(f.LatestURL); err != nil || u.Scheme != "https" || u.User != nil {
		return Release{}, errors.New("refusing to ask for releases over anything but HTTPS")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.LatestURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Seaglass (+https://github.com/ApolloF/Seaglass)")
	resp, err := f.client().Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Release{}, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, errors.New("GitHub answered " + resp.Status)
	}
	var r struct {
		Tag        string    `json:"tag_name"`
		Body       string    `json:"body"`
		HTMLURL    string    `json:"html_url"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
		Published  time.Time `json:"published_at"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&r); err != nil {
		return Release{}, err
	}
	if r.Draft || r.Prerelease {
		return Release{}, ErrNoRelease
	}
	if _, ok := parse(r.Tag); !ok {
		return Release{}, errors.New("unexpected release tag " + strconv.Quote(r.Tag))
	}
	rel := Release{Tag: r.Tag, Notes: r.Body, Page: r.HTMLURL, Published: r.Published, assets: map[string]Asset{}}
	if len(rel.Notes) > maxNotes {
		rel.Notes = strings.ToValidUTF8(rel.Notes[:maxNotes], "") + "…" // drop a rune cut in half
	}
	if !strings.HasPrefix(rel.Page, "https://github.com/") {
		rel.Page = ReleasesPage
	}
	for _, a := range r.Assets {
		if f.inRelease(a.URL) {
			rel.assets[a.Name] = Asset{Name: a.Name, URL: a.URL, Size: a.Size, Digest: a.Digest}
		}
	}
	return rel, nil
}

// ErrNoRelease means GitHub has no (non-preview) release yet.
var ErrNoRelease = errors.New("no release published yet")

// Newer reports whether version tag a is newer than b ("v1.2.3", "1.2").
// Anything that isn't a version (like "dev") is never newer nor older.
func Newer(a, b string) bool {
	va, oka := parse(a)
	vb, okb := parse(b)
	if !oka || !okb {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// Valid reports whether v is a version Seaglass can compare.
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i] // pre-release or build suffix
	}
	parts := strings.Split(v, ".")
	if v == "" || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
