// Package profile keeps what goes with a person from PC to PC: playtime,
// achievements and settings. It lives in %APPDATA%\Seaglass\Profile, a
// folder Syncer syncs between PCs and backs up.
//
// Every PC writes only its own file per person, <owner>\<pc>.json, and
// reads the others: two PCs never write the same file, so syncing never
// makes conflict copies, and nothing is lost when PCs play offline at the
// same time. A person's playtime is the sum of their files, their last
// play and achievements the newest and the union, and their settings
// those saved last. The owner is the Syncer account playing on this PC, or
// Shared when there's none.
package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
)

// Shared owns what was played while no Syncer account was in use.
const Shared = "shared"

const (
	fileVersion = 1
	maxFile     = 16 << 20 // bytes read from another PC's file
	maxPlaytime = 100 * 365 * 24 * 3600
)

// Game is one game in one PC's file.
type Game struct {
	Title        string           `json:"title,omitempty"`      // for people reading the file
	Playtime     int64            `json:"playtime,omitempty"`   // seconds played on this PC
	LastPlayed   int64            `json:"lastPlayed,omitempty"` // unix seconds
	Achievements map[string]int64 `json:"achievements,omitempty"`
}

// File is one PC's file for one owner.
type File struct {
	Version    int              `json:"version"`
	PC         string           `json:"pc"` // the PC's name, for people reading the file
	Updated    int64            `json:"updated"`
	Games      map[string]*Game `json:"games"`
	Settings   json.RawMessage  `json:"settings,omitempty"`
	SettingsAt int64            `json:"settingsAt,omitempty"` // when those settings were saved (0: never)
}

// Total is one game summed over an owner's PCs.
type Total struct {
	Playtime     int64
	LastPlayed   int64
	Achievements map[string]int64 // id → unlock time (0 when unknown)
}

// Merged is everything an owner has, over all their PCs.
type Merged struct {
	Games      map[string]Total
	Settings   json.RawMessage // the newest settings
	SettingsAt int64
	SettingsPC string // the PC that saved them
}

var (
	reOwner = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	reID    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)
)

// ValidOwner reports whether id can name an owner's folder.
func ValidOwner(id string) bool { return reOwner.MatchString(id) }

// ValidPC reports whether id can name a PC's file.
func ValidPC(id string) bool { return reID.MatchString(id) }

// Store is this PC's side of the profile. Safe for concurrent use.
type Store struct {
	dir    string
	pc     string // file name (without .json)
	pcName string

	mu    sync.Mutex
	owner string
	mine  *File
	dirty bool
	saveT *time.Timer
	read  map[string]cached // other PCs' files, by path
	err   error             // this PC's file couldn't be read: nothing is written over it
}

type cached struct {
	mod  time.Time
	size int64
	f    *File
}

// Open opens the profile in dir for this PC, owned by Shared until
// SetOwner says otherwise.
func Open(dir, pc, pcName string) (*Store, error) {
	if !ValidPC(pc) {
		return nil, errors.New("profile: invalid PC id")
	}
	s := &Store{dir: dir, pc: pc, pcName: pcName, read: map[string]cached{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMineLocked(Shared)
	return s, s.err
}

// Dir is the profile folder.
func (s *Store) Dir() string { return s.dir }

// Owner is whose profile this PC adds to.
func (s *Store) Owner() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.owner
}

// Healthy reports whether this PC's file could be read (or didn't exist
// yet). When it couldn't, totals shouldn't replace anything.
func (s *Store) Healthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err == nil
}

// Fresh reports whether this PC has no file yet for any owner: nothing
// has been recorded on it, so what the library knows can be carried over.
func (s *Store) Fresh() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	for _, e := range entries {
		if e.IsDir() && ValidOwner(e.Name()) {
			if _, err := os.Stat(filepath.Join(s.dir, e.Name(), s.pc+".json")); err == nil {
				return false
			}
		}
	}
	return true
}

func (s *Store) path(owner string) string { return filepath.Join(s.dir, owner, s.pc+".json") }

// loadMineLocked reads this PC's file for owner. A damaged file (a write
// cut short by a power cut) is moved aside and the copy kept from the last
// good write (.bak) is used instead, or, without one, an empty file: one
// bad write must not stop this PC from recording for good. A file it
// can't open, or one from a newer Seaglass, is left alone and nothing is
// written over it.
func (s *Store) loadMineLocked(owner string) {
	s.owner, s.mine, s.err, s.dirty = owner, newFile(s.pcName), nil, false
	p := s.path(owner)
	b, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		s.err = err
		return
	}
	f, err := decode(b)
	if err == nil {
		s.mine = f
		return
	}
	if errors.Is(err, errNewer) {
		s.err = err
		return
	}
	if rerr := os.Rename(p, p+".corrupt-"+time.Now().Format("20060102-150405")); rerr != nil {
		s.err = err // can't keep it for inspection: don't write over it either
		return
	}
	bak, berr := os.ReadFile(p + ".bak")
	if berr != nil {
		return
	}
	f, err = decode(bak)
	switch {
	case errors.Is(err, errNewer):
		s.err = err
	case err == nil:
		s.mine, s.dirty = f, true
		_ = s.flushLocked() // put the good copy back where other PCs read it
	}
}

// errNewer means a file was written by a newer Seaglass.
var errNewer = errors.New("profile: written by a newer Seaglass")

func newFile(pcName string) *File {
	return &File{Version: fileVersion, PC: pcName, Games: map[string]*Game{}}
}

func decode(b []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Version > fileVersion {
		return nil, errNewer
	}
	if f.Games == nil {
		f.Games = map[string]*Game{}
	}
	f.Settings = compact(f.Settings)
	for k, g := range f.Games {
		if g == nil || !validKey(k) {
			delete(f.Games, k)
			continue
		}
		g.Playtime = min(max(g.Playtime, 0), maxPlaytime)
		g.LastPlayed = max(g.LastPlayed, 0)
		for id, at := range g.Achievements {
			if id == "" || len(id) > 200 {
				delete(g.Achievements, id)
			} else if at < 0 {
				g.Achievements[id] = 0
			}
		}
	}
	return &f, nil
}

// compact is v without spaces (the file is indented, so what's read back
// differs from what was written), or nil when v isn't a JSON object.
func compact(v json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	if len(v) == 0 || v[0] != '{' || json.Compact(&b, v) != nil {
		return nil
	}
	return b.Bytes()
}

func validKey(k string) bool { return k != "" && len(k) <= 300 }

// SetOwner changes whose profile this PC adds to. What this PC recorded
// while no account was in use goes to the first account it gets (usually
// the person the PC belongs to). It reports whether the owner changed.
func (s *Store) SetOwner(owner string) (bool, error) {
	if !ValidOwner(owner) {
		owner = Shared
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner == s.owner && s.err == nil {
		return false, nil
	}
	if err := s.flushLocked(); err != nil {
		return false, err
	}
	s.loadMineLocked(owner)
	if s.err != nil || owner == Shared {
		return true, s.err
	}
	shared := s.path(Shared)
	b, err := os.ReadFile(shared)
	if err != nil {
		return true, nil
	}
	f, err := decode(b)
	if err != nil {
		return true, nil
	}
	// Moved aside first, so it's never merged twice (a Remove that fails
	// after the merge would bring it back on the next start).
	aside := shared + ".merged"
	if err := os.Rename(shared, aside); err != nil {
		return true, nil // merged on the next switch
	}
	mergeInto(s.mine, f)
	s.dirty = true
	if err := s.flushLocked(); err != nil {
		_ = os.Rename(aside, shared)
		s.loadMineLocked(owner) // without the merge
		return true, err
	}
	_ = os.Remove(aside)
	_ = os.Remove(filepath.Dir(shared)) // only when empty
	return true, nil
}

// mergeInto adds what src recorded to dst (both this PC's).
func mergeInto(dst, src *File) {
	for k, g := range src.Games {
		d := dst.game(k, g.Title)
		d.Playtime = min(d.Playtime+g.Playtime, maxPlaytime)
		d.LastPlayed = max(d.LastPlayed, g.LastPlayed)
		for id, at := range g.Achievements {
			d.addAchievement(id, at)
		}
	}
	if src.SettingsAt > dst.SettingsAt || dst.Settings == nil {
		dst.Settings, dst.SettingsAt = src.Settings, src.SettingsAt
	}
}

func (f *File) game(key, title string) *Game {
	g := f.Games[key]
	if g == nil {
		g = &Game{}
		f.Games[key] = g
	}
	if title != "" {
		g.Title = title
	}
	return g
}

func (g *Game) addAchievement(id string, at int64) bool {
	if id == "" || len(id) > 200 {
		return false
	}
	if g.Achievements == nil {
		g.Achievements = map[string]int64{}
	}
	old, ok := g.Achievements[id]
	switch {
	case !ok:
		g.Achievements[id] = max(at, 0)
		return true
	case at > 0 && (old == 0 || at < old):
		g.Achievements[id] = at
		return true
	}
	return false
}

// AddPlaytime records seconds played on this PC.
func (s *Store) AddPlaytime(key, title string, secs int64) {
	if !validKey(key) || secs <= 0 {
		return
	}
	s.change(func(f *File) bool {
		g := f.game(key, title)
		g.Playtime = min(g.Playtime+secs, maxPlaytime)
		return true
	})
}

// Played records that a game was started now (or at t).
func (s *Store) Played(key, title string, t int64) {
	if !validKey(key) || t <= 0 {
		return
	}
	s.change(func(f *File) bool {
		g := f.game(key, title)
		if t <= g.LastPlayed {
			return false
		}
		g.LastPlayed = t
		return true
	})
}

// AddAchievements records unlocked achievements (id → unlock time, 0 when
// unknown).
func (s *Store) AddAchievements(key, title string, unlocked map[string]int64) {
	if !validKey(key) || len(unlocked) == 0 {
		return
	}
	s.change(func(f *File) bool {
		changed := false
		g := f.game(key, title)
		for id, at := range unlocked {
			changed = g.addAchievement(id, at) || changed
		}
		return changed
	})
}

// SetSettings records this PC's settings, saved at t. The newest settings
// of all the owner's PCs are the ones that count.
func (s *Store) SetSettings(v json.RawMessage, t int64) {
	v = compact(v)
	s.change(func(f *File) bool {
		if string(f.Settings) == string(v) && f.SettingsAt == t {
			return false
		}
		f.Settings, f.SettingsAt = v, t
		return true
	})
}

// Seed carries over what this PC knew before the profile existed. It only
// does something while this PC has no file yet.
func (s *Store) Seed(games map[string]Game, settings json.RawMessage) {
	if !s.Fresh() {
		return
	}
	s.change(func(f *File) bool {
		for k, g := range games {
			if !validKey(k) {
				continue
			}
			d := f.game(k, g.Title)
			d.Playtime = min(d.Playtime+max(g.Playtime, 0), maxPlaytime)
			d.LastPlayed = max(d.LastPlayed, g.LastPlayed)
			for id, at := range g.Achievements {
				d.addAchievement(id, at)
			}
		}
		if f.Settings == nil && settings != nil {
			f.Settings = compact(settings) // SettingsAt 0: any PC's saved settings are newer
		}
		return true
	})
	_ = s.Flush()
}

func (s *Store) change(fn func(f *File) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil || !fn(s.mine) {
		return
	}
	s.dirty = true
	s.scheduleLocked(saveDelay)
}

// saveDelay gathers changes into one write; retryDelay is how long a
// failed write waits to be tried again (Syncthing or a virus scanner can
// hold the file open for a while).
var saveDelay, retryDelay = 2 * time.Second, 30 * time.Second

func (s *Store) scheduleLocked(d time.Duration) {
	if s.saveT != nil {
		s.saveT.Stop()
	}
	s.saveT = time.AfterFunc(d, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.flushLocked(); err != nil {
			logx.Printf("saving profile: %v", err)
			s.scheduleLocked(retryDelay)
		}
	})
}

// Flush writes this PC's file now.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

func (s *Store) flushLocked() error {
	if !s.dirty || s.err != nil {
		return nil
	}
	s.mine.Version, s.mine.PC, s.mine.Updated = fileVersion, s.pcName, time.Now().Unix()
	b, err := json.MarshalIndent(s.mine, "", " ")
	if err != nil {
		return err
	}
	p := s.path(s.owner)
	if err := writeAtomic(p, b); err != nil {
		return err
	}
	s.dirty = false
	// The spare loadMineLocked falls back to, written after the file itself
	// so one of the two is always whole. Not a ".json": other PCs skip it.
	_ = writeAtomic(p+".bak", b)
	return nil
}

// writeAtomic replaces path with b: written to a temporary file and
// flushed to disk before the rename, so a power cut leaves the old file or
// the new one, never a torn one.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Merged sums up the owner's files from every PC. Files other PCs wrote
// are only read again when they changed.
func (s *Store) Merged() Merged {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := Merged{Games: map[string]Total{}}
	files := []*File{s.mine}
	pcs := []string{s.pcName}
	dir := filepath.Join(s.dir, s.owner)
	entries, _ := os.ReadDir(dir)
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.EqualFold(filepath.Ext(name), ".json") || strings.EqualFold(name, s.pc+".json") {
			continue
		}
		if !ValidPC(strings.TrimSuffix(name, filepath.Ext(name))) || isConflictCopy(name) {
			continue
		}
		p := filepath.Join(dir, name)
		seen[p] = true
		fi, err := e.Info()
		if err != nil || fi.Size() > maxFile {
			continue
		}
		c, ok := s.read[p]
		if !ok || !c.mod.Equal(fi.ModTime()) || c.size != fi.Size() {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			f, err := decode(b)
			if err != nil {
				continue
			}
			c = cached{fi.ModTime(), fi.Size(), f}
			s.read[p] = c
		}
		files = append(files, c.f)
		pcs = append(pcs, cmpOr(c.f.PC, strings.TrimSuffix(name, filepath.Ext(name))))
	}
	for p := range s.read {
		if !seen[p] {
			delete(s.read, p)
		}
	}
	for i, f := range files {
		for k, g := range f.Games {
			t := m.Games[k]
			t.Playtime = min(t.Playtime+g.Playtime, maxPlaytime)
			t.LastPlayed = max(t.LastPlayed, g.LastPlayed)
			for id, at := range g.Achievements {
				if t.Achievements == nil {
					t.Achievements = map[string]int64{}
				}
				if old, ok := t.Achievements[id]; !ok || (at > 0 && (old == 0 || at < old)) {
					t.Achievements[id] = at
				}
			}
			m.Games[k] = t
		}
		if f.Settings != nil && (m.Settings == nil || f.SettingsAt > m.SettingsAt) {
			m.Settings, m.SettingsAt, m.SettingsPC = f.Settings, f.SettingsAt, pcs[i]
		}
	}
	return m
}

// SettingsAt is when this PC's settings were saved, on this PC or on the
// PC they were taken from.
func (s *Store) SettingsAt() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mine.SettingsAt
}

// isConflictCopy spots Syncthing's conflict copies
// ("pc.sync-conflict-20260101-120000-ABCDEFG.json"). No PC writes another's
// file, so there shouldn't be any; if there are, they'd count twice.
func isConflictCopy(name string) bool {
	return strings.Contains(strings.ToLower(name), ".sync-conflict-")
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
