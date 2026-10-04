// Package library keeps Seaglass's game library: what scans found,
// merged with what the user set (favorites, hidden, titles, controller
// mode, playtime). It lives in memory and is saved as one JSON file with
// atomic writes; even thousands of games stay a few megabytes.
package library

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
)

// Game is one game in the library.
type Game struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"` // identity: the install folder, lower-cased
	Title       string `json:"title"`
	CustomTitle string `json:"customTitle,omitempty"`
	SortTitle   string `json:"sortTitle"`

	Source      string `json:"source"`      // steam, epic, gog, ea, ubisoft, battlenet, xbox, installer, shortcut, folder
	SourceLabel string `json:"sourceLabel"` // "Steam", "External copy", "DRM-free · GOG", …
	External    bool   `json:"external"`
	Emulator    string `json:"emulator,omitempty"`
	EmuDir      string `json:"emuDir,omitempty"` // where the emulator sits, relative to Dir
	Repacker    string `json:"repacker,omitempty"`
	DRMFree     string `json:"drmFree,omitempty"`

	Installed  bool   `json:"installed"`
	PadHint    string `json:"padHint,omitempty"` // the game ships libScePad or SDL
	Dir        string `json:"dir"`
	Exe        string `json:"exe,omitempty"`
	Args       string `json:"args,omitempty"`
	WorkDir    string `json:"workDir,omitempty"`
	LaunchURI  string `json:"launchUri,omitempty"`
	UserExe    bool   `json:"userExe,omitempty"`    // the user picked the executable; scans leave it alone
	Owned      bool   `json:"owned,omitempty"`      // a connected store account owns it
	InstallURI string `json:"installUri,omitempty"` // asks the store to install it
	SizeBytes  int64  `json:"sizeBytes,omitempty"`

	SteamAppID int    `json:"steamAppId,omitempty"`
	MetaAppID  int    `json:"metaAppId,omitempty"` // Steam app found by a store search, used only for metadata
	GogID      string `json:"gogId,omitempty"`
	EpicApp    string `json:"epicApp,omitempty"`

	How         string `json:"how"`        // how the game was found
	MatchHow    string `json:"matchHow"`   // how its identity was established
	Confidence  int    `json:"confidence"` // 0–100
	NeedsReview bool   `json:"needsReview"`
	Confirmed   bool   `json:"confirmed,omitempty"` // the user confirmed the match

	AddedAt    int64 `json:"addedAt"`           // unix seconds
	Initial    bool  `json:"initial,omitempty"` // found by the very first scan, so not a "new" find
	SeenAt     int64 `json:"seenAt"`
	LastPlayed int64 `json:"lastPlayed,omitempty"`
	Playtime   int64 `json:"playtime,omitempty"` // seconds, tracked by Seaglass

	// What the store recorded (Steam), refreshed by every scan. The
	// interface shows whichever of the two is larger.
	StorePlaytime   int64 `json:"storePlaytime,omitempty"`
	StoreLastPlayed int64 `json:"storeLastPlayed,omitempty"`

	Favorite bool   `json:"favorite,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	PadMode  string `json:"padMode,omitempty"` // "" = auto, "native", "steam"
	// Collections are the user's own groups ("Co-op", "Finished"); a game
	// can be in several.
	Collections []string `json:"collections,omitempty"`

	Meta *Meta `json:"meta,omitempty"`
}

// Meta is store metadata and art, filled in by the metadata service.
type Meta struct {
	Description  string   `json:"description,omitempty"`
	Developers   []string `json:"developers,omitempty"`
	Publishers   []string `json:"publishers,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	ReleaseDate  string   `json:"releaseDate,omitempty"`
	ReleaseYear  int      `json:"releaseYear,omitempty"`
	DualSense    string   `json:"dualSense,omitempty"` // "yes", "no", "" (unknown)
	Controller   string   `json:"controller,omitempty"`
	Cover        string   `json:"cover,omitempty"` // local art URLs (/art/…)
	Hero         string   `json:"hero,omitempty"`
	Backdrop     string   `json:"backdrop,omitempty"` // 16:9 full-screen background
	Tile         string   `json:"tile,omitempty"`     // square: key art with the logo
	Logo         string   `json:"logo,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	Accent       string   `json:"accent,omitempty"` // CSS colour from the art
	FetchedAt    int64    `json:"fetchedAt,omitempty"`
	Source       string   `json:"source,omitempty"`  // where the metadata came from
	Version      int      `json:"version,omitempty"` // meta.Version that fetched it
	ArtOverrides []string `json:"artOverrides,omitempty"`
}

// DisplayTitle is the user's title if set, else the found one.
func (g *Game) DisplayTitle() string {
	if g.CustomTitle != "" {
		return g.CustomTitle
	}
	return g.Title
}

// Found is one game a scan found, already identified.
type Found struct {
	Key, Title, SortTitle       string
	Source, SourceLabel         string
	External                    bool
	Emulator, Repacker, DRMFree string
	EmuDir                      string
	Dir, Exe, Args, WorkDir     string
	LaunchURI                   string
	SizeBytes                   int64
	SteamAppID                  int
	GogID, EpicApp              string
	How, MatchHow               string
	Confidence                  int
	NeedsReview                 bool
	StorePlaytime               int64
	StoreLastPlayed             int64
	PadHint                     string
}

type fileData struct {
	Version int     `json:"version"`
	NextID  int64   `json:"nextId"`
	Games   []*Game `json:"games"`
}

// Store is the library. Safe for concurrent use.
type Store struct {
	path   string
	mu     sync.RWMutex
	saveMu sync.Mutex // one write of the file at a time
	next   int64
	games  map[int64]*Game
	byKey  map[string]*Game
	saveT  *time.Timer
}

// Open loads the library at path, or starts an empty one. A damaged file
// is kept aside and the copy saved at the previous start (path.bak) is
// used instead, when there is one.
func Open(path string) (*Store, error) {
	s := &Store{path: path, next: 1, games: map[int64]*Game{}, byKey: map[string]*Game{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		b, err = os.ReadFile(path + ".bak") // a save was cut short between rename steps
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
	}
	if err != nil {
		return nil, err
	}
	var d fileData
	if err := json.Unmarshal(b, &d); err != nil {
		// Keep the damaged file for inspection and fall back to the backup.
		_ = os.Rename(path, path+".broken-"+time.Now().Format("20060102-150405"))
		bak, berr := os.ReadFile(path + ".bak")
		if berr != nil || json.Unmarshal(bak, &d) != nil {
			return s, nil
		}
		b = bak
	} else {
		// This file is good: it's the one to fall back to next time.
		// Written before Open returns (a goroutine could outlive the store),
		// without the flush to disk a real save does: it's only a spare.
		if os.WriteFile(path+".bak.tmp", b, 0o644) == nil {
			_ = os.Rename(path+".bak.tmp", path+".bak")
		}
	}
	for _, g := range d.Games {
		if g == nil || g.ID <= 0 || g.Key == "" || s.games[g.ID] != nil || s.byKey[g.Key] != nil {
			continue
		}
		s.games[g.ID] = g
		s.byKey[g.Key] = g
		if g.ID >= s.next {
			s.next = g.ID + 1
		}
	}
	if d.NextID > s.next {
		s.next = d.NextID
	}
	if d.Version < 2 {
		s.migrateExternal(b)
	}
	return s, nil
}

// migrateExternal carries over what libraries saved before v1.9 called
// unofficial: the flag, and the label a scan would give now.
func (s *Store) migrateExternal(b []byte) {
	var old struct {
		Games []struct {
			ID         int64 `json:"id"`
			Unofficial bool  `json:"unofficial"`
		} `json:"games"`
	}
	if json.Unmarshal(b, &old) != nil {
		return
	}
	for _, o := range old.Games {
		if g := s.games[o.ID]; g != nil && o.Unofficial {
			g.External = true
		}
	}
	for _, g := range s.games {
		if strings.HasPrefix(g.SourceLabel, "Unofficial · ") {
			g.SourceLabel = "External copy"
		}
	}
}

// Games returns copies of all games, sorted by title.
func (s *Store) Games() []Game {
	s.mu.RLock()
	out := make([]Game, 0, len(s.games))
	for _, g := range s.games {
		out = append(out, copyGame(g))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortTitle != out[j].SortTitle {
			return out[i].SortTitle < out[j].SortTitle
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Get returns a copy of one game.
func (s *Store) Get(id int64) (Game, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	if !ok {
		return Game{}, false
	}
	return copyGame(g), true
}

// ErrNotFound means no game has that id.
var ErrNotFound = errors.New("game not found")

// Update changes one game and schedules a save.
func (s *Store) Update(id int64, fn func(g *Game)) (Game, error) {
	s.mu.Lock()
	g, ok := s.games[id]
	if !ok {
		s.mu.Unlock()
		return Game{}, ErrNotFound
	}
	fn(g)
	out := copyGame(g)
	s.scheduleSaveLocked()
	s.mu.Unlock()
	return out, nil
}

// SetPlayed sets each game's playtime and last play to what fn returns
// (ok false leaves the game alone) and returns the games that changed.
func (s *Store) SetPlayed(fn func(g *Game) (playtime, lastPlayed int64, ok bool)) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var changed []int64
	for id, g := range s.games {
		pt, lp, ok := fn(g)
		if ok && (g.Playtime != pt || g.LastPlayed != lp) {
			g.Playtime, g.LastPlayed = pt, lp
			changed = append(changed, id)
		}
	}
	if len(changed) > 0 {
		s.scheduleSaveLocked()
	}
	return changed
}

// ApplyScan merges a scan into the library. Found games are added or
// refreshed; games no longer found are kept (with their playtime) but
// marked not installed. User choices are never overwritten.
func (s *Store) ApplyScan(found []Found, now time.Time) (added, removed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := now.Unix()
	first := true // the very first scan: nothing found on this PC before
	for _, g := range s.games {
		if !g.IsOwnedOnly() {
			first = false
			break
		}
	}
	seen := map[string]bool{}
	for _, f := range found {
		seen[f.Key] = true
		g := s.byKey[f.Key]
		if g == nil {
			g = &Game{ID: s.next, Key: f.Key, AddedAt: ts, Initial: first}
			s.next++
			s.games[g.ID] = g
			s.byKey[g.Key] = g
			added++
		}
		// A game this scan can't identify but a Steam store search did (see
		// metaWorker.fetch) keeps that identity until a scan knows better.
		byStore := g.MetaAppID > 0 && f.SteamAppID == 0 && f.GogID == "" && f.Confidence < g.Confidence
		if !g.Confirmed && !byStore {
			g.Title, g.SortTitle = f.Title, f.SortTitle
		}
		if g.CustomTitle != "" {
			g.SortTitle = strings.ToLower(g.CustomTitle)
		}
		g.Source, g.SourceLabel = f.Source, f.SourceLabel
		g.External, g.Emulator, g.Repacker, g.DRMFree = f.External, f.Emulator, f.Repacker, f.DRMFree
		g.EmuDir = f.EmuDir
		g.Installed, g.Dir, g.LaunchURI, g.SizeBytes = true, f.Dir, f.LaunchURI, f.SizeBytes
		if !g.UserExe {
			g.Exe, g.Args, g.WorkDir = f.Exe, f.Args, f.WorkDir
		}
		if !g.Confirmed && !byStore {
			if g.SteamAppID != f.SteamAppID && g.Meta != nil {
				g.Meta = nil // a different game now: fetch its metadata again
			}
			g.SteamAppID, g.GogID = f.SteamAppID, f.GogID
			g.MatchHow, g.Confidence, g.NeedsReview = f.MatchHow, f.Confidence, f.NeedsReview
		}
		g.EpicApp, g.How, g.SeenAt = f.EpicApp, f.How, ts
		g.PadHint = f.PadHint
		if g.Owned {
			// The owned sync may know more (GOG Galaxy, Steam's web API);
			// store playtime only goes up.
			g.StorePlaytime, g.StoreLastPlayed = max(g.StorePlaytime, f.StorePlaytime), max(g.StoreLastPlayed, f.StoreLastPlayed)
		} else {
			g.StorePlaytime, g.StoreLastPlayed = f.StorePlaytime, f.StoreLastPlayed
		}
	}
	for k, g := range s.byKey {
		if !seen[k] && g.Installed {
			g.Installed = false
			removed++
		}
	}
	s.mergeOwnedLocked()
	s.scheduleSaveLocked()
	return added, removed
}

func (s *Store) scheduleSaveLocked() { s.scheduleSaveAfterLocked(time.Second) }

func (s *Store) scheduleSaveAfterLocked(d time.Duration) {
	if s.saveT != nil {
		s.saveT.Stop()
	}
	s.saveT = time.AfterFunc(d, func() {
		if err := s.Flush(); err != nil {
			logx.Printf("saving library: %v", err)
			// Try again later (e.g. antivirus holding library.json open).
			s.mu.Lock()
			s.scheduleSaveAfterLocked(30 * time.Second)
			s.mu.Unlock()
		}
	})
}

// Flush writes the library to disk now.
func (s *Store) Flush() error {
	// The save timer and quitting can both flush; writes mustn't interleave.
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.RLock()
	d := fileData{Version: 2, NextID: s.next}
	for _, g := range s.games {
		c := copyGame(g)
		d.Games = append(d.Games, &c)
	}
	s.mu.RUnlock()
	sort.Slice(d.Games, func(i, j int) bool { return d.Games[i].ID < d.Games[j].ID })
	b, err := json.MarshalIndent(d, "", " ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, b)
}

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

func copyGame(g *Game) Game {
	c := *g
	c.Collections = append([]string(nil), g.Collections...)
	if g.Meta != nil {
		m := *g.Meta
		m.Developers = append([]string(nil), g.Meta.Developers...)
		m.Publishers = append([]string(nil), g.Meta.Publishers...)
		m.Genres = append([]string(nil), g.Meta.Genres...)
		m.ArtOverrides = append([]string(nil), g.Meta.ArtOverrides...)
		c.Meta = &m
	}
	return c
}
