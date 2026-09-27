package achievements

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// IconPrefix is the path the interface loads stored icons from. They're
// kept apart from game art (/art/), whose pruning doesn't know them.
const IconPrefix = "/ach/"

var reIconName = regexp.MustCompile(`^[a-f0-9]{40}\.(jpg|png)$`)

// Icons downloads achievement icons (and copies local ones) into Dir,
// checked and re-encoded, content-addressed. The interface only ever sees
// /ach/ URLs: the release CSP allows images from Seaglass itself only.
type Icons struct {
	Dir   string
	Fetch func(ctx context.Context, src, dir string) (string, error) // download, check, store; returns the file name
	Store func(dir string, data []byte) (string, error)              // check and store local data

	mu    sync.Mutex
	index map[string]string // source URL or file → stored name
	fails map[string]bool   // sources that failed this run
}

func (ic *Icons) indexPath() string { return filepath.Join(ic.Dir, "index.json") }

func (ic *Icons) load() {
	if ic.index != nil {
		return
	}
	ic.index, ic.fails = map[string]string{}, map[string]bool{}
	if b, err := readSmall(ic.indexPath(), 16<<20); err == nil {
		_ = json.Unmarshal(b, &ic.index)
	}
}

func (ic *Icons) save() {
	b, err := json.Marshal(ic.index)
	if err != nil {
		return
	}
	tmp := ic.indexPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, ic.indexPath())
	}
}

// local returns the /ach/ URL of a source already stored, "" if none.
func (ic *Icons) local(src string) string {
	name := ic.index[src]
	if name == "" || !reIconName.MatchString(name) || !isFile(filepath.Join(ic.Dir, name)) {
		return ""
	}
	return IconPrefix + name
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// Localize replaces the list's icon sources with stored /ach/ URLs,
// storing what's missing. An icon that can't be had is left empty (the
// interface draws a generic one). With offline set nothing is downloaded.
func (ic *Icons) Localize(ctx context.Context, l *List, offline bool) {
	if ic == nil || l == nil {
		return
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.load()
	changed := false
	get := func(src string) string {
		if src == "" || strings.HasPrefix(src, IconPrefix) {
			return src
		}
		if u := ic.local(src); u != "" {
			return u
		}
		if ic.fails[src] || ctx.Err() != nil {
			return ""
		}
		var name string
		var err error
		switch {
		case strings.HasPrefix(src, "https://"):
			if offline || ic.Fetch == nil {
				return ""
			}
			name, err = ic.Fetch(ctx, src, ic.Dir)
		case filepath.IsAbs(src) && ic.Store != nil:
			var b []byte
			if b, err = readSmall(src, maxFile); err == nil {
				name, err = ic.Store(ic.Dir, b)
			}
		default:
			return ""
		}
		if err != nil || !reIconName.MatchString(name) {
			ic.fails[src] = true
			return ""
		}
		ic.index[src], changed = name, true
		return IconPrefix + name
	}
	for i := range l.Items {
		l.Items[i].Icon = get(l.Items[i].Icon)
		l.Items[i].IconGray = get(l.Items[i].IconGray)
	}
	if changed {
		ic.save()
	}
}
