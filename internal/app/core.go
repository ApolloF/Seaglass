// Package app wires Seaglass together and holds the services the
// frontend calls.
package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ApolloF/Seaglass/internal/identify"
	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/fsnotify/fsnotify"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Events the frontend listens for.
const (
	EventLibraryChanged = "library:changed" // reload the whole library
	EventGamesUpdated   = "games:updated"   // these games changed; the rest didn't
	EventScanState      = "scan:state"
)

// ScanState tells the frontend what the scanner is doing.
type ScanState struct {
	Running  bool   `json:"running"`
	LastScan int64  `json:"lastScan"` // unix seconds
	TookMs   int64  `json:"tookMs"`
	Games    int    `json:"games"`
	Added    int    `json:"added"`
	Known    int    `json:"known"` // titles in the game database; 0 until it's downloaded
	Error    string `json:"error,omitempty"`
}

func init() {
	application.RegisterEvent[ScanState](EventScanState)
	application.RegisterEvent[string](EventLibraryChanged)
	application.RegisterEvent[[]library.Game](EventGamesUpdated)
}

// Core is shared by the services.
type Core struct {
	Version  string
	Lib      *library.Store
	Settings *settings.Store
	Manifest *identify.Manager
	Launch   *launch.Manager
	// Frozen keeps the library as it is: no scans, metadata, store
	// accounts or update checks (the test harness's --dev-data).
	Frozen   bool
	owned    *ownedState
	updates  *updater
	external *externalWatch
	ach      *achState
	profile  *profileState
	// setMu makes settings changes one at a time (read, change, save), so
	// settings taken from another PC don't undo one made here.
	setMu sync.Mutex

	shell       *Shell
	pad         atomic.Pointer[pad.Manager]
	lastSession int64       // the last session whose end was handled
	registered  atomic.Bool // the game list reached Syncer

	ctx    context.Context
	cancel context.CancelFunc

	scanMu  sync.Mutex // one scan at a time
	stateMu sync.Mutex
	state   ScanState
	pending chan struct{}
	watcher *fsnotify.Watcher
	// linkDirs are watched folders only shortcuts (.lnk) in matter to (the
	// desktops, when they aren't game folders too).
	linkDirs atomic.Pointer[map[string]bool]
	meta     *metaWorker
	pruned   sync.Once // unused art is cleared once per start
}

// NewCore opens the library and settings.
func NewCore(version string) (*Core, error) {
	lib, err := library.Open(filepath.Join(platform.AppDir(), "library.json"))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Core{
		Version: version, Lib: lib, Settings: settings.Open(settings.DefaultPath()),
		Manifest: identify.NewManager(platform.CacheDir("manifest")),
		ctx:      ctx, cancel: cancel, pending: make(chan struct{}, 1),
	}
	c.meta = newMetaWorker(c)
	c.Launch = launch.NewManager(c.onSession)
	c.owned = newOwnedState(c)
	c.updates = newUpdater(c)
	c.ach = newAchState(c, c.owned.client, c.meta.client)
	c.profile = newProfileState(c)
	return c, nil
}

// Start runs the first scan, keeps the game database fresh and watches
// the folders games install into.
func (c *Core) Start() {
	if c.Frozen {
		c.profile = &profileState{c: c, kick: make(chan struct{}, 1)} // a test harness's library: no profile
		c.external = newExternalWatch(c)
		c.external.set(c.Settings.Get().NoticeExternal)
		c.setState(func(s *ScanState) { s.LastScan, s.Games = time.Now().Unix(), len(c.Lib.Games()) })
		return
	}
	go c.scanLoop()
	c.profile.start()
	heapDiag("idle")
	go c.meta.run(c.ctx)
	go c.owned.loop(c.ctx)
	go c.updates.loop(c.ctx)
	c.external = newExternalWatch(c)
	c.external.set(c.Settings.Get().NoticeExternal)
	if exe, err := os.Executable(); err == nil && platform.MoveOldStartup(exe) {
		logx.Printf("start with Windows: WaterLauncher's entry is now Seaglass's")
	}
	if exe, err := os.Executable(); err == nil && platform.RepairStartup(exe) {
		logx.Printf("start with Windows: now starts %s", exe)
	}
	c.RequestScan()
	go func() {
		if !c.Manifest.Stale() {
			return
		}
		fresh, err := c.Manifest.Refresh(c.ctx)
		if err != nil {
			logx.Printf("game database refresh: %v", err)
			return
		}
		if fresh {
			logx.Printf("game database updated: %d titles", c.Manifest.Len())
			c.RequestScan() // identify again with the new data
		}
	}()
	go func() {
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-t.C:
				c.RequestScan()
			}
		}
	}()
}

// Stop ends background work and saves the library.
func (c *Core) Stop() {
	c.Launch.Close()
	c.cancel()
	if c.watcher != nil {
		_ = c.watcher.Close()
	}
	if err := c.Lib.Flush(); err != nil {
		logx.Printf("saving library: %v", err)
	}
	if c.profile != nil && c.profile.store != nil {
		if err := c.profile.store.Flush(); err != nil {
			logx.Printf("saving profile: %v", err)
		}
	}
}

// quitForUpdate closes Seaglass so an update can take its place.
func (c *Core) quitForUpdate() {
	if a := application.Get(); a != nil {
		a.Quit()
	}
}

// RequestScan asks for a scan soon; requests while one runs coalesce.
func (c *Core) RequestScan() {
	select {
	case c.pending <- struct{}{}:
	default:
	}
}

func (c *Core) scanLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.pending:
			if !c.waitIdle(c.ctx) {
				return
			}
			c.scanNow()
		}
	}
}

// waitIdle holds background work (scans, metadata, store accounts) while
// a game runs, so it gets the disk, network and CPU to itself. It reports
// false when Seaglass is closing.
func (c *Core) waitIdle(ctx context.Context) bool {
	for c.Launch.Active() {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(15 * time.Second):
		}
	}
	return ctx.Err() == nil
}

func (c *Core) scanNow() {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	c.setState(func(s *ScanState) { s.Running, s.Error = true, "" })
	cfg := c.Settings.Get()
	res := scan.Run(c.ctx, scan.Options{Folders: cfg.Folders, AutoFolders: cfg.AutoFolders, DetectExternal: cfg.DetectExternal})
	if c.ctx.Err() != nil {
		return
	}
	ix := c.Manifest.Index()
	found := make([]library.Found, 0, len(res.Games))
	for _, g := range res.Games {
		found = append(found, toFound(g, ix.Identify(g), cfg))
	}
	added, removed := c.Lib.ApplyScan(found, time.Now())
	logx.Printf("scan: %d games (%d new, %d gone) in %v", len(found), added, removed, res.Took.Round(time.Millisecond))
	c.setState(func(s *ScanState) {
		s.Running, s.LastScan, s.TookMs = false, time.Now().Unix(), res.Took.Milliseconds()
		s.Games, s.Added, s.Known = len(found), added, ix.Len()
	})
	c.emit(EventLibraryChanged, "scan")
	c.rewatch(cfg)
	c.meta.queueMissing()
	c.pruned.Do(func() { go c.pruneArt() })
	if added > 0 || removed > 0 || !c.registered.Load() {
		go c.registerWithSyncer()
	}
}

// toFound turns a scanned, identified candidate into a library record.
func toFound(g scan.Candidate, m identify.Match, cfg settings.Settings) library.Found {
	f := library.Found{
		Key: platform.Key(g.Dir), Title: m.Title, SortTitle: scan.SortTitle(m.Title),
		Source: string(g.Source), Emulator: g.Emulator, EmuDir: g.EmuDir, Repacker: g.Repacker, DRMFree: g.DRMFree,
		External: g.External(), Dir: g.Dir, Exe: g.Exe, Args: g.Args, WorkDir: g.WorkDir,
		LaunchURI: g.LaunchURI, SizeBytes: g.SizeBytes, SteamAppID: m.SteamAppID, GogID: m.GogID,
		EpicApp: g.EpicApp, How: g.How, MatchHow: m.How, Confidence: m.Confidence,
		StorePlaytime: g.StorePlaytime, StoreLastPlayed: g.StoreLastPlayed, PadHint: g.PadHint,
	}
	f.SourceLabel = sourceLabel(g)
	f.NeedsReview = cfg.ReviewUncertain && m.Confidence < 70 && !g.Source.Store()
	return f
}

var storeLabels = map[scan.Source]string{
	scan.Steam: "Steam", scan.Epic: "Epic", scan.GOG: "GOG", scan.EA: "EA", scan.Ubisoft: "Ubisoft",
	scan.BattleNet: "Battle.net", scan.Xbox: "Xbox", scan.Installer: "Standalone", scan.Shortcut: "Shortcut", scan.Folder: "Folder",
}

func sourceLabel(g scan.Candidate) string {
	switch {
	case g.Source.Store() && g.Emulator != "":
		return storeLabels[g.Source] + " · modified"
	case g.Source.Store():
		return storeLabels[g.Source]
	case g.Emulator != "":
		return "External copy"
	case g.Repacker != "":
		return "External copy"
	case g.DRMFree != "":
		return "DRM-free · " + g.DRMFree
	}
	return storeLabels[g.Source]
}

func (c *Core) setState(fn func(*ScanState)) {
	c.stateMu.Lock()
	fn(&c.state)
	s := c.state
	c.stateMu.Unlock()
	c.emit(EventScanState, s)
}

// State returns the scanner's state.
func (c *Core) State() ScanState {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.state
}

// gamesChanged sends the interface the games that changed, so it doesn't
// reload a whole (possibly large) library for one favorite or new cover.
func (c *Core) gamesChanged(ids ...int64) {
	out := make([]library.Game, 0, len(ids))
	for _, id := range ids {
		if g, ok := c.Lib.Get(id); ok {
			out = append(out, g)
		}
	}
	if len(out) > 0 {
		c.emit(EventGamesUpdated, out)
	}
}

func (c *Core) emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}

// rewatch watches the folders whose changes mean a game was installed or
// removed; a change triggers a scan a few seconds later.
func (c *Core) rewatch(cfg settings.Settings) {
	if c.watcher == nil {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			logx.Printf("watcher: %v", err)
			return
		}
		c.watcher = w
		go c.watchLoop(w)
	}
	want := map[string]bool{}
	add := func(d string) {
		if d != "" && platform.IsDir(d) {
			want[platform.Key(d)] = true
			_ = c.watcher.Add(d)
		}
	}
	for _, d := range scan.SteamWatchDirs() {
		add(d)
	}
	add(scan.EpicManifestDir())
	for _, f := range cfg.Folders {
		add(f)
	}
	if cfg.AutoFolders {
		for _, f := range scan.AutoFolders() {
			add(f)
		}
	}
	links := map[string]bool{}
	for _, d := range []string{platform.Desktop, platform.PublicDesktop} {
		if d != "" && !want[platform.Key(d)] {
			links[platform.Key(d)] = true
		}
		add(d)
	}
	c.linkDirs.Store(&links)
	for _, d := range c.watcher.WatchList() {
		if !want[platform.Key(d)] {
			_ = c.watcher.Remove(d)
		}
	}
}

func (c *Core) watchLoop(w *fsnotify.Watcher) {
	var debounce *time.Timer
	for {
		select {
		case <-c.ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			name := strings.ToLower(ev.Name)
			// Steam rewrites manifests while downloading; only finished changes matter.
			if strings.HasSuffix(name, ".tmp") || strings.Contains(name, `\downloading`) || strings.Contains(name, `\temp`) {
				continue
			}
			// A screenshot or download saved to the desktop isn't a game.
			if l := c.linkDirs.Load(); l != nil && (*l)[platform.Key(filepath.Dir(ev.Name))] && !strings.EqualFold(filepath.Ext(ev.Name), ".lnk") {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(5*time.Second, c.RequestScan)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			logx.Printf("watcher: %v", err)
		}
	}
}

// pruneArt deletes stored art no game uses anymore.
func (c *Core) pruneArt() {
	keep := map[string]bool{}
	for _, g := range c.Lib.Games() {
		if m := g.Meta; m != nil {
			keep[m.Cover], keep[m.Hero], keep[m.Backdrop], keep[m.Tile], keep[m.Logo], keep[m.Icon] = true, true, true, true, true, true
		}
	}
	if len(keep) == 0 {
		return // an empty (or lost) library: leave the art for a restored one
	}
	if n, freed := meta.PruneArt(platform.CacheDir("art"), keep, 24*time.Hour); n > 0 {
		logx.Printf("art: removed %d unused images (%d KB)", n, freed>>10)
	}
}

func (c *Core) padManager() *pad.Manager { return c.pad.Load() }

// padState is the controller in use (none when controllers are unavailable).
func (c *Core) padState() pad.State {
	if m := c.padManager(); m != nil {
		return m.State()
	}
	return pad.State{Battery: -1}
}
