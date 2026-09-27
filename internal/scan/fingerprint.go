package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A game folder is read in full the first time a scan sees it; after that
// its fingerprint says whether anything that matters changed. The
// fingerprint holds the modification times of the folder itself, its
// direct subfolders and every folder that held something telling (a marker,
// a steam_api DLL, an exe), plus size and time of the telling files. Adding,
// removing or renaming a file changes its folder's time; replacing a file in
// place (a patched DLL) changes the file's own.

// maxStamps caps a fingerprint; a folder that needs more is simply read
// again every scan.
const maxStamps = 400

// folderMaxAge forces a full read now and then, for changes a fingerprint
// can't see (a new file deep in a folder that held nothing telling).
const folderMaxAge = 24 * time.Hour

type stamp struct {
	path string
	mod  int64
	size int64
}

type fingerprint struct {
	stamps []stamp
	seen   map[string]bool
	over   bool
}

func (f *fingerprint) add(p string, fi fs.FileInfo) {
	if f == nil || f.over {
		return
	}
	k := strings.ToLower(p)
	if f.seen[k] {
		return
	}
	if len(f.stamps) >= maxStamps {
		f.over = true
		return
	}
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	f.seen[k] = true
	size := int64(0)
	if !fi.IsDir() {
		size = fi.Size()
	}
	f.stamps = append(f.stamps, stamp{p, fi.ModTime().UnixNano(), size})
}

// entry records a walked entry. It stats the path rather than using the
// listing's copy: NTFS updates the times a folder's parent lists for it
// lazily, so they can differ from what a later stat sees.
func (f *fingerprint) entry(p string, _ fs.DirEntry) { f.dir(p) }

// dir records a file or folder by path.
func (f *fingerprint) dir(p string) {
	if f == nil || f.over || f.seen[strings.ToLower(p)] {
		return
	}
	if fi, err := os.Stat(p); err == nil {
		f.add(p, fi)
	}
}

// same reports whether nothing recorded has changed.
func (f *fingerprint) same() bool {
	for _, s := range f.stamps {
		fi, err := os.Stat(s.path)
		if err != nil || fi.ModTime().UnixNano() != s.mod || (!fi.IsDir() && fi.Size() != s.size) {
			return false
		}
	}
	return true
}

// folderCache keeps results of reading game folders, per folder.
type folderCache[T any] struct {
	mu sync.Mutex
	m  map[string]folderResult[T]
}

type folderResult[T any] struct {
	v  T
	fp *fingerprint
	at time.Time
}

// get returns the result for key when its folder hasn't changed.
func (c *folderCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	r, ok := c.m[key]
	c.mu.Unlock()
	if !ok || time.Since(r.at) > folderMaxAge || !r.fp.same() {
		var zero T
		return zero, false
	}
	return r.v, true
}

func (c *folderCache[T]) put(key string, v T, fp *fingerprint) {
	if fp == nil || fp.over {
		return
	}
	fp.seen = nil // not needed anymore
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]folderResult[T]{}
	}
	c.m[key] = folderResult[T]{v, fp, time.Now()}
	c.mu.Unlock()
}

func folderKey(dir string) string { return strings.ToLower(filepath.Clean(dir)) }

var (
	emulationCache folderCache[Emulation]
	exeCache       folderCache[string]
)

// detectEmulationCached is DetectEmulation, reusing the last answer for a
// folder that hasn't changed.
func detectEmulationCached(dir string, signed func(string) bool) Emulation {
	key := folderKey(dir)
	if e, ok := emulationCache.get(key); ok {
		return e
	}
	fp := &fingerprint{}
	e := detectEmulation(dir, signed, fp)
	emulationCache.put(key, e, fp)
	return e
}

// pickExeCached is PickExe, reusing the last answer for a folder that
// hasn't changed.
func pickExeCached(dir, title string) string {
	key := folderKey(dir) + "|" + title
	if exe, ok := exeCache.get(key); ok {
		return exe
	}
	fp := &fingerprint{}
	exe := pickExe(dir, title, fp)
	// An empty pick may be a folder still being extracted; look again next scan.
	if exe != "" {
		exeCache.put(key, exe, fp)
	}
	return exe
}
