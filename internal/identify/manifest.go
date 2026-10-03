// Package identify works out which game a scanned folder is: its proper
// title and Steam or GOG id, from the ids a copy carries or, failing that,
// its name, using the community Ludusavi manifest (PCGamingWiki data on
// ~50,000 games, with their Steam and GOG ids and install folder names).
package identify

import (
	"compress/gzip"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/gamekit/ludusavi"
)

const (
	manifestURL  = ludusavi.URL
	refreshAfter = 7 * 24 * time.Hour
	indexName    = "manifest-index-v1.gob.gz"
	maxManifest  = 256 << 20
)

// Entry is one game in the manifest.
type Entry struct {
	Name        string
	SteamID     int
	GogID       string
	InstallDirs []string
	Aliases     []string // other names that point here
}

// Manager loads the manifest index from cache and refreshes it weekly.
// Only scans use the index (~15 MB in memory), so it's let go a couple of
// minutes after the last use and read again (~50 ms) by the next scan.
type Manager struct {
	dir  string
	mu   sync.Mutex
	idx  *Index
	n    int // titles in the index last loaded, kept when it's let go
	busy bool
	drop *time.Timer
	keep time.Duration
}

// NewManager uses dir for the cached index (created on demand).
func NewManager(dir string) *Manager { return &Manager{dir: dir, keep: 2 * time.Minute} }

// Len is the number of titles in the game database (0 before it exists),
// without loading it.
func (m *Manager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.n
}

// usedLocked keeps the index a while longer. m.mu is held.
func (m *Manager) usedLocked() {
	if m.idx == nil {
		return
	}
	m.n = m.idx.Len()
	if m.drop == nil {
		m.drop = time.AfterFunc(m.keep, m.release)
	} else {
		m.drop.Reset(m.keep)
	}
}

// release lets go of the index until it's needed again.
func (m *Manager) release() {
	m.mu.Lock()
	m.idx = nil
	m.mu.Unlock()
	debug.FreeOSMemory() // most of the heap was the index: give it back now
}

func (m *Manager) indexFile() string { return filepath.Join(m.dir, indexName) }
func (m *Manager) etagFile() string  { return filepath.Join(m.dir, "manifest.etag") }

// Index returns the loaded index, reading the cache on first use; nil until one exists.
func (m *Manager) Index() *Index {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idx == nil {
		es, err := readIndex(m.indexFile())
		if err == nil {
			m.idx = build(es)
		} else if pe := (*fs.PathError)(nil); !errors.As(err, &pe) {
			// A damaged cache (not a missing or locked file): drop it and
			// its etag, so the next Refresh downloads the whole manifest
			// rather than getting a 304.
			_ = os.Remove(m.indexFile())
			_ = os.Remove(m.etagFile())
		}
	}
	m.usedLocked()
	return m.idx
}

// Stale reports whether the cached index is missing or older than a week.
func (m *Manager) Stale() bool {
	fi, err := os.Stat(m.indexFile())
	return err != nil || time.Since(fi.ModTime()) > refreshAfter
}

// Refresh downloads the manifest when it changed and rebuilds the index.
// It reports whether a new index was built.
func (m *Manager) Refresh(ctx context.Context) (bool, error) {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return false, nil
	}
	m.busy = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return false, err
	}
	if tag, err := os.ReadFile(m.etagFile()); err == nil && platform.IsFile(m.indexFile()) {
		req.Header.Set("If-None-Match", strings.TrimSpace(string(tag)))
	}
	req.Header.Set("User-Agent", "Seaglass")
	c := &http.Client{Timeout: 3 * time.Minute}
	resp, err := c.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		now := time.Now()
		_ = os.Chtimes(m.indexFile(), now, now)
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, errors.New(resp.Status)
	}
	es, err := Parse(io.LimitReader(resp.Body, maxManifest))
	if err != nil {
		return false, err
	}
	if len(es) < 10000 {
		return false, fmt.Errorf("game database looks incomplete (%d entries)", len(es))
	}
	_ = os.MkdirAll(m.dir, 0o755)
	if err := writeIndex(m.indexFile(), es); err != nil {
		return false, err
	}
	if tag := resp.Header.Get("ETag"); tag != "" {
		_ = os.WriteFile(m.etagFile(), []byte(tag), 0o644)
	}
	m.mu.Lock()
	m.idx = build(es)
	m.usedLocked()
	m.mu.Unlock()
	return true, nil
}

func readIndex(p string) ([]Entry, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var es []Entry
	err = gob.NewDecoder(zr).Decode(&es)
	return es, err
}

func writeIndex(p string, es []Entry) error {
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	if err := gob.NewEncoder(zw).Encode(es); err != nil {
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Parse reads the manifest YAML (see gamekit/ludusavi).
func Parse(r io.Reader) ([]Entry, error) {
	es, err := ludusavi.Parse(r)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(es))
	for i, e := range es {
		out[i] = Entry{Name: e.Name, SteamID: e.SteamID, GogID: e.GogID, InstallDirs: e.InstallDirs, Aliases: e.Aliases}
	}
	return out, nil
}
