package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/identify"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
)

// TestMatchAudit runs real games, named the ways stores, installers,
// repacks and folders name them (testdata/match_audit.tsv, made by
// gen_match_audit.py), through what Seaglass does to find out which
// game a folder is: the game database, the Steam store search, then
// PCGamingWiki. Each case says which game it is (or that it isn't one),
// so a wrong match counts as much as a miss:
//
//	WL_MATCH_AUDIT=1 go test -run MatchAudit -v -timeout 2h ./internal/app
//
// Answers are kept in WL_MATCH_CACHE (default: a folder in %TEMP%), so a
// second run takes seconds; delete it to ask the sources again.
func TestMatchAudit(t *testing.T) {
	if os.Getenv("WL_MATCH_AUDIT") == "" {
		t.Skip("set WL_MATCH_AUDIT=1 to query the Steam store and PCGamingWiki")
	}
	ix := identify.NewManager(platform.CacheDir("manifest")).Index()
	if ix == nil {
		t.Skip("no game database in the cache yet (run Seaglass once)")
	}
	cache := os.Getenv("WL_MATCH_CACHE")
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "wl-match-cache")
	}
	client := meta.NewClient(t.TempDir(), nil)
	client.UseTransport(&recorder{dir: cache, next: map[string]time.Time{}})
	w := &metaWorker{client: client}

	f, err := os.Open(filepath.Join("testdata", "match_audit.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type result struct {
		kind, input, want, got, how string
		ok, wrong                   bool
	}
	var results []result
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 3 {
			t.Fatalf("bad line %q", line)
		}
		kind, input, want := cols[0], cols[1], cols[2]
		c := auditCandidate(kind, input)
		got, how := auditIdentify(ctx, t, ix, w, c)
		r := result{kind: kind, input: input, want: want, got: got, how: how}
		switch {
		case want == "-":
			r.ok = got == ""
			r.wrong = got != ""
		case got == "":
		default:
			r.ok = sameGame(got, want)
			r.wrong = !r.ok
		}
		results = append(results, r)
	}
	var games, right, wrong, missed, falsePos int
	for _, r := range results {
		if r.want != "-" {
			games++
			if r.ok {
				right++
			}
		}
		switch {
		case r.wrong && r.want == "-":
			falsePos++
			t.Logf("NOT A GAME, matched  %-12s %-58q → %q (%s)", r.kind, r.input, r.got, r.how)
		case r.wrong:
			wrong++
			t.Logf("WRONG                %-12s %-58q → %q, want %q (%s)", r.kind, r.input, r.got, r.want, r.how)
		case !r.ok:
			missed++
			t.Logf("MISS                 %-12s %-58q want %q (%s)", r.kind, r.input, r.want, r.how)
		}
	}
	pct := 100 * float64(right) / float64(max(1, games))
	t.Logf("%d cases: %d games found right of %d (%.1f %%), %d wrong, %d missed; %d of %d non-games matched",
		len(results), right, games, pct, wrong, missed, falsePos, len(results)-games)
	if pct < 97 || wrong > 0 || falsePos > 0 {
		t.Errorf("want at least 97 %% found right and no wrong matches")
	}
}

// auditCandidate is what a scan would have found.
func auditCandidate(kind, input string) scan.Candidate {
	switch {
	case strings.HasPrefix(kind, "store:"):
		return scan.Candidate{Title: input, Source: scan.Source(strings.TrimPrefix(kind, "store:")), How: "store"}
	case kind == "installer":
		name, dir, _ := strings.Cut(input, "|")
		return scan.Candidate{Title: scan.CleanTitle(name), Dir: dir, Source: scan.Installer, TitleTrusted: true}
	}
	return scan.Candidate{Title: scan.CleanTitle(filepath.Base(input)), Dir: input, Source: scan.Folder}
}

// auditIdentify does what a scan and the metadata worker do to name a
// game, and returns the name it ends up with ("" when nothing knew it).
func auditIdentify(ctx context.Context, t *testing.T, ix *identify.Index, w *metaWorker, c scan.Candidate) (string, string) {
	m := ix.Identify(c)
	if m.SteamAppID > 0 {
		if n := ix.SteamName(m.SteamAppID); n != "" {
			return n, m.How
		}
		return m.Title, m.How
	}
	if m.GogID != "" {
		return m.Title, m.How
	}
	if !looksLikeGameName(m.Title) {
		return "", "not a game name"
	}
	h, ok, err := w.searchStore(ctx, m.Title)
	if err != nil {
		t.Logf("search %q: %v", m.Title, err)
	}
	if ok {
		return h.Name, storeMatchHow
	}
	if p, err := w.client.PCGamingWiki(ctx, m.Title); err == nil {
		return p.Title, wikiMatchHow
	}
	return "", m.How
}

// sameGame compares a found name with the accepted ones ("|" between
// them), ignoring editions and how they're written.
func sameGame(got, want string) bool {
	// A year in brackets tells games of one name apart ("Tomb Raider
	// (2013)", not "(1996)"), so it counts: the data lists it where
	// needed.
	key := func(s string) string { return scan.LooseKey(scan.StripEdition(s)) }
	g := key(got)
	for _, w := range strings.Split(want, "|") {
		if key(w) == g {
			return true
		}
	}
	return false
}

// recorder answers GET requests from a folder of earlier answers, and
// fetches (spaced out per host, as the real client does) what it hasn't
// seen yet.
type recorder struct {
	dir  string
	mu   sync.Mutex
	next map[string]time.Time
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	sum := sha1.Sum([]byte(req.Method + " " + req.URL.String()))
	p := filepath.Join(r.dir, hex.EncodeToString(sum[:]))
	if b, err := os.ReadFile(p); err == nil {
		status, body, _ := bytes.Cut(b, []byte("\n"))
		code := 0
		fmt.Sscan(string(status), &code)
		return &http.Response{StatusCode: code, Status: http.StatusText(code), Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}, Request: req, ContentLength: int64(len(body))}, nil
	}
	gap := map[string]time.Duration{"store.steampowered.com": 1500 * time.Millisecond, "www.pcgamingwiki.com": time.Second}[req.URL.Hostname()]
	r.mu.Lock()
	at := r.next[req.URL.Hostname()]
	if now := time.Now(); at.Before(now) {
		at = now
	}
	r.next[req.URL.Hostname()] = at.Add(gap)
	r.mu.Unlock()
	time.Sleep(time.Until(at))
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		_ = os.MkdirAll(r.dir, 0o755)
		_ = os.WriteFile(p, append([]byte(fmt.Sprintf("%d\n", resp.StatusCode)), body...), 0o644)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}
