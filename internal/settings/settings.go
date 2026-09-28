// Package settings stores Seaglass's preferences in %APPDATA%\Seaglass\settings.json.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// Settings are the user's preferences. New fields get their default when
// an older settings file doesn't have them.
type Settings struct {
	// Library
	Folders          []string `json:"folders"`          // extra folders whose subfolders are games
	AutoFolders      bool     `json:"autoFolders"`      // also look in common game folders on every drive
	DetectUnofficial bool     `json:"detectUnofficial"` // recognise Steam emulators, cracks and repacks
	ReviewUncertain  bool     `json:"reviewUncertain"`  // keep low-confidence matches in New on this PC for a check
	ShowNotInstalled bool     `json:"showNotInstalled"` // list games that aren't installed (anymore)
	ShowOwned        bool     `json:"showOwned"`        // list games connected store accounts own but aren't installed
	OwnedGOG         bool     `json:"ownedGOG"`         // read GOG Galaxy's library for owned games
	HiddenSources    []string `json:"hiddenSources"`    // libraries whose games aren't shown (steam, epic, …, unofficial, folder)

	// Appearance
	Theme            string `json:"theme"`            // system, dark, light (desktop mode)
	BigPictureLayout string `json:"bigPictureLayout"` // deck, console, orbit

	// Big picture and controller
	OpenBigPictureOnController bool   `json:"openBigPictureOnController"`
	StartInBigPicture          bool   `json:"startInBigPicture"`
	Sounds                     bool   `json:"sounds"`
	Haptics                    bool   `json:"haptics"`
	Lightbar                   bool   `json:"lightbar"`
	PSButton                   bool   `json:"psButton"`
	Glyphs                     string `json:"glyphs"` // auto, playstation, xbox

	// While playing
	CloseWhilePlaying bool   `json:"closeWhilePlaying"` // close the interface while a game runs (frees its memory)
	PadWhilePlaying   string `json:"padWhilePlaying"`   // listen (PS button opens the overlay), off (release the controller)
	NoticeExternal    bool   `json:"noticeExternal"`    // follow games started outside Seaglass (playtime, controller)

	// Saves, through Syncer
	SyncSavesBefore  bool `json:"syncSavesBefore"`  // sync a game's saves before it starts
	BackupSavesAfter bool `json:"backupSavesAfter"` // back its saves up after it exits
	SyncWait         int  `json:"syncWait"`         // seconds to wait for a sync before playing
	StartSyncer      bool `json:"startSyncer"`      // start Syncer (without its window) when it isn't running
	SyncProfile      bool `json:"syncProfile"`      // Syncer syncs playtime, achievements and settings between PCs
	SameSettings     bool `json:"sameSettings"`     // take the settings saved last on another PC (see Portable)
	AskWhoPlays      bool `json:"askWhoPlays"`      // with several Syncer accounts, ask who's playing before a game starts

	// Achievements
	Achievements           bool `json:"achievements"`           // show achievements (read from stores and emulator files)
	ShowHiddenAchievements bool `json:"showHiddenAchievements"` // show hidden achievements before they're unlocked (spoilers)

	// Updates
	AutoUpdate bool `json:"autoUpdate"` // check GitHub for new versions and install them on the next start or while idle in the tray

	// Welcomed: the first-start welcome was seen (or skipped).
	Welcomed bool `json:"welcomed"`
}

// Sources are the libraries that can be hidden, as the interface groups
// games: by store, then unofficial copies, standalone installs and folders.
var Sources = map[string]bool{
	"steam": true, "epic": true, "gog": true, "ea": true, "ubisoft": true, "battlenet": true, "xbox": true,
	"unofficial": true, "standalone": true, "folder": true,
}

// Defaults are the settings on first start.
func Defaults() Settings {
	return Settings{
		Folders: []string{}, HiddenSources: []string{}, AutoFolders: true, DetectUnofficial: true, ReviewUncertain: true,
		Theme: "system", BigPictureLayout: "deck",
		OpenBigPictureOnController: true, Haptics: true, Lightbar: true, PSButton: true, Glyphs: "auto",
		CloseWhilePlaying: true, PadWhilePlaying: "listen", NoticeExternal: true,
		SyncSavesBefore: true, BackupSavesAfter: true, SyncWait: 60, StartSyncer: true,
		SyncProfile: true, SameSettings: true,
		AutoUpdate: true, Achievements: true,
	}
}

// Portable are the settings that go with a person to their other PCs.
// What depends on the PC stays: its game folders and stores, whether it
// starts in big picture (a TV, not a desk), whether it asks who's playing
// (a PC several people share), and the welcome.
type Portable struct {
	DetectUnofficial       bool     `json:"detectUnofficial"`
	ReviewUncertain        bool     `json:"reviewUncertain"`
	ShowNotInstalled       bool     `json:"showNotInstalled"`
	ShowOwned              bool     `json:"showOwned"`
	HiddenSources          []string `json:"hiddenSources"`
	Theme                  string   `json:"theme"`
	BigPictureLayout       string   `json:"bigPictureLayout"`
	Sounds                 bool     `json:"sounds"`
	Haptics                bool     `json:"haptics"`
	Lightbar               bool     `json:"lightbar"`
	PSButton               bool     `json:"psButton"`
	Glyphs                 string   `json:"glyphs"`
	CloseWhilePlaying      bool     `json:"closeWhilePlaying"`
	PadWhilePlaying        string   `json:"padWhilePlaying"`
	NoticeExternal         bool     `json:"noticeExternal"`
	SyncSavesBefore        bool     `json:"syncSavesBefore"`
	BackupSavesAfter       bool     `json:"backupSavesAfter"`
	SyncWait               int      `json:"syncWait"`
	Achievements           bool     `json:"achievements"`
	ShowHiddenAchievements bool     `json:"showHiddenAchievements"`
	AutoUpdate             bool     `json:"autoUpdate"`
}

// Portable returns the settings that go with a person.
func (v Settings) Portable() Portable {
	return Portable{
		DetectUnofficial: v.DetectUnofficial, ReviewUncertain: v.ReviewUncertain, ShowNotInstalled: v.ShowNotInstalled,
		ShowOwned: v.ShowOwned, HiddenSources: append([]string{}, v.HiddenSources...), Theme: v.Theme,
		BigPictureLayout: v.BigPictureLayout, Sounds: v.Sounds, Haptics: v.Haptics, Lightbar: v.Lightbar,
		PSButton: v.PSButton, Glyphs: v.Glyphs, CloseWhilePlaying: v.CloseWhilePlaying, PadWhilePlaying: v.PadWhilePlaying,
		NoticeExternal: v.NoticeExternal, SyncSavesBefore: v.SyncSavesBefore, BackupSavesAfter: v.BackupSavesAfter,
		SyncWait: v.SyncWait, Achievements: v.Achievements,
		ShowHiddenAchievements: v.ShowHiddenAchievements, AutoUpdate: v.AutoUpdate,
	}
}

// WithPortable returns v with another PC's portable settings, given as
// JSON. Settings the JSON doesn't have (an older Seaglass saved it) stay
// as they are.
func (v Settings) WithPortable(b []byte) (Settings, error) {
	p := v.Portable()
	if err := json.Unmarshal(b, &p); err != nil {
		return v, err
	}
	v.DetectUnofficial, v.ReviewUncertain, v.ShowNotInstalled = p.DetectUnofficial, p.ReviewUncertain, p.ShowNotInstalled
	v.ShowOwned, v.HiddenSources, v.Theme = p.ShowOwned, p.HiddenSources, p.Theme
	v.BigPictureLayout, v.Sounds, v.Haptics, v.Lightbar = p.BigPictureLayout, p.Sounds, p.Haptics, p.Lightbar
	v.PSButton, v.Glyphs, v.CloseWhilePlaying, v.PadWhilePlaying = p.PSButton, p.Glyphs, p.CloseWhilePlaying, p.PadWhilePlaying
	v.NoticeExternal, v.SyncSavesBefore, v.BackupSavesAfter = p.NoticeExternal, p.SyncSavesBefore, p.BackupSavesAfter
	v.SyncWait, v.Achievements = p.SyncWait, p.Achievements
	v.ShowHiddenAchievements, v.AutoUpdate = p.ShowHiddenAchievements, p.AutoUpdate
	return normalize(v), nil
}

// Store loads and saves settings. Safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings
}

// Open reads the settings file (defaults when missing or unreadable).
func Open(path string) *Store {
	s := &Store{path: path, cur: Defaults()}
	if b, err := os.ReadFile(path); err == nil {
		v := Defaults()
		if json.Unmarshal(b, &v) == nil {
			// Settings saved before the welcome existed belong to someone
			// who has used Seaglass already.
			var keys map[string]json.RawMessage
			if json.Unmarshal(b, &keys) == nil {
				if _, ok := keys["welcomed"]; !ok {
					v.Welcomed = true
				}
			}
			s.cur = normalize(v)
		}
	}
	return s
}

// DefaultPath is %APPDATA%\Seaglass\settings.json.
func DefaultPath() string { return filepath.Join(platform.AppDir(), "settings.json") }

// Get returns the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cur
	c.Folders = append([]string{}, s.cur.Folders...)
	c.HiddenSources = append([]string{}, s.cur.HiddenSources...)
	return c
}

// Set validates, stores and saves new settings, returning what was saved.
func (s *Store) Set(v Settings) (Settings, error) {
	v = normalize(v)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s.Get(), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return s.cur, err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return s.cur, err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return s.cur, err
	}
	s.cur = v
	return v, nil
}

func normalize(v Settings) Settings {
	var folders []string
	seen := map[string]bool{}
	for _, f := range v.Folders {
		f = filepath.Clean(strings.TrimSpace(f))
		if !filepath.IsAbs(f) || filepath.Dir(f) == f || seen[platform.Key(f)] {
			continue // drive roots would make every folder a "game"
		}
		seen[platform.Key(f)] = true
		folders = append(folders, f)
	}
	if folders == nil {
		folders = []string{}
	}
	v.Folders = folders
	hidden := []string{}
	for _, id := range v.HiddenSources {
		if Sources[id] && !slices.Contains(hidden, id) {
			hidden = append(hidden, id)
		}
	}
	v.HiddenSources = hidden
	switch v.Theme {
	case "system", "dark", "light":
	default:
		v.Theme = "system"
	}
	switch v.SyncWait {
	case 30, 60, 150, 300:
	default:
		v.SyncWait = 60
	}
	switch v.BigPictureLayout {
	case "deck", "console", "orbit":
	default:
		v.BigPictureLayout = "deck"
	}
	switch v.PadWhilePlaying {
	case "listen", "off":
	default:
		v.PadWhilePlaying = "listen"
	}
	switch v.Glyphs {
	case "auto", "playstation", "xbox":
	default:
		v.Glyphs = "auto"
	}
	return v
}
