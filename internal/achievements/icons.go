package achievements

import (
	"context"
	"encoding/json"
	"errors"
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

// iconWorkers is how many icons are fetched at once.
const iconWorkers = 6

// Localize replaces the list's icon sources with stored /ach/ URLs,
// storing what's missing, a few at a time. An icon that can't be had is
// left empty (the interface draws a generic one). With offline set nothing
// is downloaded. It reports false when ctx ended before every icon was
// tried: the list is then worth reading again later.
func (ic *Icons) Localize(ctx context.Context, l *List, offline bool) (complete bool) {
	if ic == nil || l == nil {
		return true
	}
	ic.mu.Lock()
	ic.load()
	var todo []string
	queued := map[string]bool{}
	for _, it := range l.Items {
		for _, src := range []string{it.Icon, it.IconGray} {
			if src == "" || strings.HasPrefix(src, IconPrefix) || queued[src] || ic.fails[src] || ic.local(src) != "" {
				continue
			}
			if strings.HasPrefix(src, "https://") && (offline || ic.Fetch == nil) {
				continue
			}
			queued[src] = true
			todo = append(todo, src)
		}
	}
	ic.mu.Unlock()

	got := make(map[string]string, len(todo))
	var gotMu sync.Mutex
	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < min(iconWorkers, len(todo)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for src := range jobs {
				name, err := ic.store(ctx, src)
				if ctx.Err() != nil {
					continue // cut short: not a failure of this icon
				}
				gotMu.Lock()
				if err != nil || !reIconName.MatchString(name) {
					got[src] = ""
				} else {
					got[src] = name
				}
				gotMu.Unlock()
			}
		}()
	}
	for _, src := range todo {
		if ctx.Err() != nil {
			break
		}
		jobs <- src
	}
	close(jobs)
	wg.Wait()

	ic.mu.Lock()
	defer ic.mu.Unlock()
	changed := false
	for src, name := range got {
		if name == "" {
			ic.fails[src] = true
		} else {
			ic.index[src], changed = name, true
		}
	}
	if changed {
		ic.save()
	}
	get := func(src string) string {
		if src == "" || strings.HasPrefix(src, IconPrefix) {
			return src
		}
		return ic.local(src)
	}
	for i := range l.Items {
		l.Items[i].Icon = get(l.Items[i].Icon)
		l.Items[i].IconGray = get(l.Items[i].IconGray)
	}
	return len(got) == len(todo)
}

// store fetches (https) or reads (a local file) one icon and stores it.
func (ic *Icons) store(ctx context.Context, src string) (string, error) {
	if strings.HasPrefix(src, "https://") {
		return ic.Fetch(ctx, src, ic.Dir)
	}
	if !filepath.IsAbs(src) || ic.Store == nil {
		return "", errors.New("not an icon source")
	}
	b, err := readSmall(src, maxFile)
	if err != nil {
		return "", err
	}
	return ic.Store(ic.Dir, b)
}
