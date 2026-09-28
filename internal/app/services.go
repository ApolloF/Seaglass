package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// LibraryService is the game library, as the frontend sees it.
type LibraryService struct{ c *Core }

// NewLibraryService binds the library to core.
func NewLibraryService(c *Core) *LibraryService { return &LibraryService{c} }

// ServiceStartup starts scanning once the app runs.
func (s *LibraryService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.c.Start()
	return nil
}

// ServiceShutdown saves the library.
func (s *LibraryService) ServiceShutdown() error {
	s.c.Stop()
	return nil
}

// Games returns every game in the library.
func (s *LibraryService) Games() []library.Game { return s.c.Lib.Games() }

// Game returns one game.
func (s *LibraryService) Game(id int64) (library.Game, error) {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return g, library.ErrNotFound
	}
	return g, nil
}

// ScanState reports what the scanner is doing.
func (s *LibraryService) ScanState() ScanState { return s.c.State() }

// Rescan looks for games again now.
func (s *LibraryService) Rescan() { s.c.RequestScan() }

// SetFavorite marks or unmarks a favorite.
func (s *LibraryService) SetFavorite(id int64, on bool) (library.Game, error) {
	return s.update(id, func(g *library.Game) { g.Favorite = on })
}

// SetHidden hides a game from the library views (or shows it again).
func (s *LibraryService) SetHidden(id int64, on bool) (library.Game, error) {
	return s.update(id, func(g *library.Game) { g.Hidden = on })
}

// SetPadMode picks how a controller reaches the game: "" (auto), "native" or "steam".
func (s *LibraryService) SetPadMode(id int64, mode string) (library.Game, error) {
	switch mode {
	case "", "auto":
		mode = ""
	case "native", "steam":
	default:
		return library.Game{}, errors.New("unknown controller mode")
	}
	return s.update(id, func(g *library.Game) { g.PadMode = mode })
}

// Rename sets the title shown for a game ("" restores the found one).
func (s *LibraryService) Rename(id int64, title string) (library.Game, error) {
	title = strings.TrimSpace(title)
	if len(title) > 200 {
		return library.Game{}, errors.New("title is too long")
	}
	return s.update(id, func(g *library.Game) {
		g.CustomTitle = title
		if title != "" {
			g.SortTitle = scan.SortTitle(title)
		} else {
			g.SortTitle = scan.SortTitle(g.Title)
		}
	})
}

// ConfirmMatch accepts the game's identity, so it no longer waits for a check in New on this PC.
func (s *LibraryService) ConfirmMatch(id int64) (library.Game, error) {
	return s.update(id, func(g *library.Game) { g.Confirmed, g.NeedsReview = true, false })
}

// ChooseExe lets the user pick the executable that starts the game.
func (s *LibraryService) ChooseExe(id int64) (library.Game, error) {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return g, library.ErrNotFound
	}
	d := application.Get().Dialog.OpenFile().
		SetTitle("Choose the program that starts "+g.DisplayTitle()).
		CanChooseFiles(true).CanChooseDirectories(false).
		AddFilter("Programs", "*.exe")
	if g.Dir != "" {
		d.SetDirectory(g.Dir)
	}
	p, err := d.PromptForSingleSelection()
	if err != nil || p == "" {
		return g, err
	}
	p = filepath.Clean(p)
	if !strings.EqualFold(filepath.Ext(p), ".exe") || !platform.IsFile(p) {
		return g, errors.New("please choose a program (.exe)")
	}
	return s.update(id, func(g *library.Game) {
		g.Exe, g.Args, g.WorkDir, g.UserExe, g.LaunchURI = p, "", filepath.Dir(p), true, ""
	})
}

// Install asks the game's store to install it (owned, not installed).
func (s *LibraryService) Install(id int64) error {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return library.ErrNotFound
	}
	if g.InstallURI == "" {
		return errors.New(g.DisplayTitle() + " can't be installed from here")
	}
	logx.Printf("install %q through its store", g.DisplayTitle())
	return platform.OpenURI(g.InstallURI)
}

// MetaState reports metadata fetching progress.
func (s *LibraryService) MetaState() MetaState { return s.c.meta.State() }

// RefreshMetadata fetches a game's metadata and art again.
func (s *LibraryService) RefreshMetadata(id int64) error {
	if _, ok := s.c.Lib.Get(id); !ok {
		return library.ErrNotFound
	}
	s.c.meta.queueNow(id)
	return nil
}

// SearchSteam looks up titles on the Steam store, to pick the right game.
func (s *LibraryService) SearchSteam(query string) ([]meta.StoreHit, error) {
	if len(query) > 120 {
		query = strings.ToValidUTF8(query[:120], "") // not half a character
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 20*time.Second)
	defer cancel()
	hits, err := s.c.meta.client.SearchSteam(ctx, query)
	// Nothing for it as typed: the store may know it by another name
	// ("Resident Evil 4 Remake" is "Resident Evil 4", "GTA V" is "Grand
	// Theft Auto V").
	tried := map[string]bool{scan.Normalize(query): true}
	for _, t := range append([]string{scan.StripEdition(query), scan.WithoutRemake(query), scan.ExpandAbbrev(query)}, scan.Aliases(query)...) {
		if err != nil || len(hits) > 0 {
			break
		}
		if t == "" || tried[scan.Normalize(t)] {
			continue
		}
		tried[scan.Normalize(t)] = true
		hits, err = s.c.meta.client.SearchSteam(ctx, t)
	}
	if hits == nil {
		hits = []meta.StoreHit{}
	}
	return hits, err
}

// SetMatch says which game this is: a Steam app and its name. The choice
// is kept across scans, and metadata is fetched for it.
func (s *LibraryService) SetMatch(id int64, steamAppID int, name string) (library.Game, error) {
	name = strings.TrimSpace(name)
	if steamAppID <= 0 || name == "" || len(name) > 200 {
		return library.Game{}, errors.New("choose a game from the list")
	}
	g, err := s.update(id, func(g *library.Game) {
		g.Confirmed, g.NeedsReview = true, false
		g.SteamAppID, g.MetaAppID = steamAppID, 0
		g.Title, g.SortTitle = name, scan.SortTitle(name)
		if g.CustomTitle != "" {
			g.SortTitle = scan.SortTitle(g.CustomTitle)
		}
		g.MatchHow, g.Confidence = "Chosen by you", 100
		g.Meta = nil
	})
	if err == nil {
		s.c.meta.queueNow(id)
	}
	return g, err
}

// artKinds are the pictures of a game that can be chosen.
var artKinds = map[string]meta.Kind{"cover": meta.Cover, "backdrop": meta.Backdrop, "hero": meta.Hero, "logo": meta.Logo}

// ArtChoices finds the pictures a game's cover, backdrop, hero or logo
// could be (kind), stored and ready to show; the current one comes first.
// It asks the stores, so it can take a few seconds.
func (s *LibraryService) ArtChoices(id int64, kind string) ([]meta.Choice, error) {
	k, ok := artKinds[kind]
	if !ok {
		return nil, errors.New("unknown kind of art")
	}
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return nil, library.ErrNotFound
	}
	app := g.SteamAppID
	if app == 0 {
		app = g.MetaAppID
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 45*time.Second)
	defer cancel()
	list, err := s.c.meta.client.ArtChoices(ctx, meta.Request{Title: g.DisplayTitle(), SteamAppID: app, GogID: g.GogID}, k, g.Meta)
	if list == nil {
		list = []meta.Choice{}
	}
	return list, err
}

// SetArt makes a picture from ArtChoices the game's cover, backdrop, hero
// or logo, kept through later refreshes.
func (s *LibraryService) SetArt(id int64, kind, art string) (library.Game, error) {
	k, ok := artKinds[kind]
	if !ok {
		return library.Game{}, errors.New("unknown kind of art")
	}
	var set bool
	g, err := s.update(id, func(g *library.Game) {
		if g.Meta == nil {
			g.Meta = &library.Meta{}
		}
		set = meta.SetArt(g.Meta, k, art, platform.CacheDir("art"))
	})
	if err == nil && !set {
		err = errors.New("that picture isn't available anymore")
	}
	return g, err
}

// SetCollections puts a game in these collections (and out of others).
func (s *LibraryService) SetCollections(id int64, names []string) (library.Game, error) {
	return s.update(id, func(g *library.Game) { g.Collections = library.CleanCollections(names) })
}

// RenameCollection renames a collection in every game; an empty name
// deletes it (the games stay).
func (s *LibraryService) RenameCollection(old, name string) error {
	clean := library.CleanCollections([]string{name})
	if strings.TrimSpace(name) != "" && len(clean) == 0 {
		return errors.New("that name can't be used")
	}
	next := ""
	if len(clean) > 0 {
		next = clean[0]
	}
	if ids := s.c.Lib.RenameCollection(old, next); len(ids) > 0 {
		s.c.gamesChanged(ids...)
	}
	return nil
}

// OpenFolder shows the game's folder in Explorer.
func (s *LibraryService) OpenFolder(id int64) error {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return library.ErrNotFound
	}
	return platform.ShowInExplorer(g.Dir)
}

func (s *LibraryService) update(id int64, fn func(*library.Game)) (library.Game, error) {
	g, err := s.c.Lib.Update(id, fn)
	if err == nil {
		s.c.gamesChanged(id)
	}
	return g, err
}

// SettingsService is the user's preferences.
type SettingsService struct{ c *Core }

// NewSettingsService binds settings to core.
func NewSettingsService(c *Core) *SettingsService { return &SettingsService{c} }

// AppInfo describes this build.
type AppInfo struct {
	Version string `json:"version"`
	DataDir string `json:"dataDir"`
	LogFile string `json:"logFile"`
	// The previous run crashed (its output is in crash-previous.log).
	CrashedLastTime bool `json:"crashedLastTime"`
}

// Info returns the version and where data is kept.
func (s *SettingsService) Info() AppInfo {
	return AppInfo{Version: s.c.Version, DataDir: platform.AppDir(), LogFile: logx.Path(), CrashedLastTime: crashedLastTime}
}

// Get returns the settings.
func (s *SettingsService) Get() settings.Settings { return s.c.Settings.Get() }

// Save stores new settings; library settings trigger a scan. Settings
// that go with the person are shared with their other PCs.
func (s *SettingsService) Save(v settings.Settings) (settings.Settings, error) {
	s.c.setMu.Lock()
	defer s.c.setMu.Unlock()
	old := s.c.Settings.Get()
	saved, err := s.c.saveSettings(v)
	if err == nil && s.c.profile != nil {
		s.c.profile.settingsSaved(old, saved)
	}
	return saved, err
}

// saveSettings stores new settings and does what changing them needs.
func (c *Core) saveSettings(v settings.Settings) (settings.Settings, error) {
	old := c.Settings.Get()
	saved, err := c.Settings.Set(v)
	if err != nil {
		return saved, err
	}
	if saved.SyncProfile && !old.SyncProfile && c.profile != nil {
		c.profile.changed()
	}
	if saved.ShowOwned && !old.ShowOwned {
		c.meta.queueMissing()
	}
	if saved.NoticeExternal != old.NoticeExternal && c.external != nil {
		c.external.set(saved.NoticeExternal)
	}
	if saved.Achievements != old.Achievements || saved.ShowHiddenAchievements != old.ShowHiddenAchievements {
		c.ach.clear()
	}
	if saved.AutoUpdate && !old.AutoUpdate {
		NewUpdateService(c).Check()
	}
	if !sameStrings(old.Folders, saved.Folders) || old.AutoFolders != saved.AutoFolders ||
		old.DetectExternal != saved.DetectExternal || old.ReviewUncertain != saved.ReviewUncertain {
		c.RequestScan()
	}
	return saved, nil
}

// AddFolder asks for a folder whose subfolders are games and adds it.
func (s *SettingsService) AddFolder() (settings.Settings, error) {
	p, err := application.Get().Dialog.OpenFile().
		SetTitle("Choose a folder that holds games").
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(false).
		PromptForSingleSelection()
	if err != nil || p == "" {
		return s.c.Settings.Get(), err
	}
	v := s.c.Settings.Get()
	v.Folders = append(v.Folders, p)
	return s.Save(v)
}

// RemoveFolder stops looking in a watched folder.
func (s *SettingsService) RemoveFolder(path string) (settings.Settings, error) {
	v := s.c.Settings.Get()
	var keep []string
	for _, f := range v.Folders {
		if !strings.EqualFold(filepath.Clean(f), filepath.Clean(path)) {
			keep = append(keep, f)
		}
	}
	v.Folders = keep
	return s.Save(v)
}

// AutoFolders lists the common game folders found on this PC.
func (s *SettingsService) AutoFolders() []string {
	f := scan.AutoFolders()
	if f == nil {
		f = []string{}
	}
	return f
}

// HasSteamGridDBKey reports whether a SteamGridDB key is stored.
func (s *SettingsService) HasSteamGridDBKey() bool { return platform.LoadSecret(sgdbSecret) != "" }

// SetSteamGridDBKey stores (or with "", removes) the SteamGridDB API key,
// encrypted for this Windows user, and fetches art the stores lacked.
func (s *SettingsService) SetSteamGridDBKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) > 128 || strings.ContainsAny(key, " \t\r\n") {
		return errors.New("that doesn't look like a SteamGridDB API key")
	}
	if err := platform.SaveSecret(sgdbSecret, key); err != nil {
		return err
	}
	if key != "" {
		s.c.meta.queueMissing()
	}
	return nil
}

// StartWithWindows reports whether Seaglass starts (in the tray) when
// you sign in to Windows.
func (s *SettingsService) StartWithWindows() platform.Startup { return platform.GetStartup() }

// SetStartWithWindows turns starting with Windows on or off.
func (s *SettingsService) SetStartWithWindows(on bool) (platform.Startup, error) {
	exe, err := os.Executable()
	if err != nil {
		return platform.GetStartup(), err
	}
	if err := platform.SetStartup(exe, on); err != nil {
		return platform.GetStartup(), err
	}
	logx.Printf("start with Windows: %v", on)
	return platform.GetStartup(), nil
}

// OpenLog shows the log file's folder.
func (s *SettingsService) OpenLog() error { return platform.ShowInExplorer(filepath.Dir(logx.Path())) }

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
