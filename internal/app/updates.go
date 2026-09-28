package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventUpdateState tells the interface how updating goes.
const EventUpdateState = "update:state"

func init() { application.RegisterEvent[UpdateState](EventUpdateState) }

// Update statuses.
const (
	UpdateOff         = "off"         // a development build: no updates
	UpdateIdle        = "idle"        // not checked yet
	UpdateChecking    = "checking"    // asking GitHub
	UpdateUpToDate    = "uptodate"    // this is the newest version
	UpdateAvailable   = "available"   // newer, but this copy can't update itself: download by hand
	UpdateDownloading = "downloading" // fetching and checking it
	UpdateReady       = "ready"       // downloaded and checked; installs on restart
	UpdateError       = "error"       // the last check or download failed
)

// UpdateState is what the interface shows about updates.
type UpdateState struct {
	Current   string  `json:"current"`
	Latest    string  `json:"latest,omitempty"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"` // 0 to 1 while downloading
	Notes     string  `json:"notes,omitempty"`
	Page      string  `json:"page"`
	Error     string  `json:"error,omitempty"`
	CheckedAt int64   `json:"checkedAt"` // unix seconds
	// Failed: an install of Latest was started before but this is still
	// the old version, so it isn't retried on its own.
	Failed bool `json:"failed"`
}

// Update timing: one check once start-up has settled, so a release is
// downloaded during the session it's noticed in. After that only the user
// checks (or turning automatic updates on). A game running at start puts
// the check off until it's done.
const (
	updateFirstCheck = 15 * time.Second
	updateGameWait   = 5 * time.Minute
)

type updater struct {
	c    *Core
	feed update.Feed
	dir  string // downloads and pending.json
	exe  string // the running exe
	kind string // update.KindInstaller, update.KindExe or "" (by hand)
	kick chan struct{}

	mu      sync.Mutex
	st      UpdateState
	pending *update.Pending
	busy    bool
}

func newUpdater(c *Core) *updater {
	u := &updater{c: c, feed: update.GitHub, dir: platform.CacheDir("updates"), kick: make(chan struct{}, 1)}
	u.st = UpdateState{Current: c.Version, Status: UpdateIdle, Page: update.ReleasesPage}
	if exe, err := os.Executable(); err == nil {
		u.exe = exe
	}
	if !update.Valid(c.Version) || u.exe == "" {
		u.st.Status = UpdateOff
		return u
	}
	u.kind = update.KindFor(u.exe)
	update.CleanOld(u.exe)
	p, ok := update.LoadPending(u.dir)
	if ok && !update.Newer(p.Tag, c.Version) {
		update.Clear(u.dir, "") // installed: tidy up
	} else if ok {
		u.pending = &p
		u.st.Latest, u.st.Status, u.st.Failed = p.Tag, UpdateReady, p.Attempts > 0
		if u.st.Failed {
			u.st.Error = "The update to " + p.Tag + " didn't install."
		}
	}
	return u
}

func (u *updater) state() UpdateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

func (u *updater) set(fn func(*UpdateState)) {
	u.mu.Lock()
	fn(&u.st)
	s := u.st
	u.mu.Unlock()
	u.c.emit(EventUpdateState, s)
}

// loop checks once at start while automatic updates are on, and whenever
// the user asks.
func (u *updater) loop(ctx context.Context) {
	if u.state().Status == UpdateOff {
		return
	}
	t := time.NewTimer(updateFirstCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !u.c.Settings.Get().AutoUpdate {
				continue // turning it on checks
			}
			// Not while a game runs: it may want all the bandwidth.
			if u.c.Launch.Active() {
				t.Reset(updateGameWait)
				continue
			}
			u.check(ctx)
			u.installIfIdle()
		case <-u.kick:
			u.check(ctx)
		}
	}
}

// check asks GitHub for the newest release and downloads it when it's newer.
func (u *updater) check(ctx context.Context) {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return
	}
	u.busy = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()

	u.set(func(s *UpdateState) { s.Status, s.Error, s.Progress = UpdateChecking, "", 0 })
	rel, err := u.feed.Latest(ctx)
	now := time.Now().Unix()
	switch {
	case errors.Is(err, update.ErrNoRelease):
		u.set(func(s *UpdateState) { s.Status, s.CheckedAt = UpdateUpToDate, now })
		return
	case err != nil:
		logx.Printf("update check: %v", err)
		u.set(func(s *UpdateState) { s.Status, s.Error, s.CheckedAt = UpdateError, "Couldn't reach GitHub.", now })
		return
	}
	if !update.Newer(rel.Tag, u.c.Version) {
		u.mu.Lock()
		u.pending = nil
		u.mu.Unlock()
		update.Clear(u.dir, "")
		u.set(func(s *UpdateState) {
			s.Status, s.Latest, s.CheckedAt, s.Notes, s.Page, s.Failed = UpdateUpToDate, rel.Tag, now, "", rel.Page, false
		})
		return
	}
	u.set(func(s *UpdateState) { s.Latest, s.Notes, s.Page, s.CheckedAt = rel.Tag, rel.Notes, rel.Page, now })

	u.mu.Lock()
	p := u.pending
	u.mu.Unlock()
	if p != nil && p.Tag == rel.Tag && p.Check(u.exe) == nil {
		u.set(func(s *UpdateState) { s.Status = UpdateReady })
		return
	}
	if u.kind == "" {
		u.set(func(s *UpdateState) { s.Status = UpdateAvailable })
		return
	}
	logx.Printf("update: downloading %s", rel.Tag)
	u.set(func(s *UpdateState) { s.Status, s.Failed = UpdateDownloading, false })
	name := update.InstallerAsset
	if u.kind == update.KindExe {
		name = update.ExeAsset
	}
	// An older download (and its pending.json) makes way for this one.
	u.mu.Lock()
	u.pending = nil
	u.mu.Unlock()
	update.Clear(u.dir, "")
	file, sum, err := u.feed.Download(ctx, rel, name, u.dir, func(done, total int64) {
		if total > 0 {
			u.set(func(s *UpdateState) { s.Progress = float64(done) / float64(total) })
		}
	})
	if err == nil {
		if err = update.CheckPublisher(file, u.exe); err != nil {
			_ = os.Remove(file)
		}
	}
	if err != nil {
		logx.Printf("update %s: %v", rel.Tag, err)
		u.set(func(s *UpdateState) { s.Status, s.Error = UpdateError, "The download failed: "+err.Error() })
		return
	}
	np := update.Pending{Tag: rel.Tag, File: file, SHA256: sum, Kind: u.kind}
	if err := update.SavePending(u.dir, np); err != nil {
		u.set(func(s *UpdateState) { s.Status, s.Error = UpdateError, err.Error() })
		return
	}
	u.mu.Lock()
	u.pending = &np
	u.mu.Unlock()
	logx.Printf("update: %s is ready (%s)", rel.Tag, u.kind)
	u.set(func(s *UpdateState) { s.Status, s.Progress = UpdateReady, 1 })
}

// install puts the waiting update in place and restarts Seaglass.
func (u *updater) install() error {
	if u.c.Launch.Active() {
		return errors.New("finish playing first: installing the update restarts Seaglass")
	}
	u.mu.Lock()
	p := u.pending
	u.mu.Unlock()
	if p == nil {
		return errors.New("no update is ready")
	}
	if err := applyPending(*p, u.dir, u.exe, false); err != nil {
		logx.Printf("update %s: %v", p.Tag, err)
		return err
	}
	u.c.quitForUpdate()
	return nil
}

// installIfIdle installs a ready update straight away when nobody would
// notice: Seaglass shows only its tray icon (started with Windows, or
// for a shortcut's game) and no game runs. It comes back in the tray.
func (u *updater) installIfIdle() {
	u.mu.Lock()
	p := u.pending
	u.mu.Unlock()
	sh := u.c.shell
	if p == nil || p.Attempts > 0 || sh == nil || !sh.Hidden() || u.c.Launch.Active() || !u.c.Settings.Get().AutoUpdate {
		return
	}
	if err := applyPending(*p, u.dir, u.exe, true); err != nil {
		logx.Printf("update %s in the tray: %v", p.Tag, err)
		q := *p
		q.Attempts++
		u.mu.Lock()
		u.pending = &q
		u.mu.Unlock()
		u.set(func(s *UpdateState) { s.Failed, s.Error = true, "The update to "+p.Tag+" didn't install." })
		return
	}
	u.c.quitForUpdate()
}

// applyPending starts installing p: the installer runs (and restarts
// Seaglass), or the exe is swapped and started. The caller exits.
func applyPending(p update.Pending, dir, exe string, tray bool) error {
	if err := p.Check(exe); err != nil {
		update.Clear(dir, "")
		return err
	}
	p.Attempts++
	if err := update.SavePending(dir, p); err != nil {
		return err
	}
	logx.Printf("update: installing %s (%s)", p.Tag, p.Kind)
	if p.Kind == update.KindInstaller {
		return update.RunInstaller(p.File, filepath.Dir(exe), true, tray)
	}
	if err := update.SwapExe(p.File, exe); err != nil {
		return err
	}
	args := "--updated"
	if tray {
		args += " --tray"
	}
	_, err := platform.StartProcess(exe, args, filepath.Dir(exe))
	return err
}

// ApplyPendingUpdate installs a downloaded update as Seaglass starts,
// before anything else runs, when automatic updates are on. It reports
// whether it did; Seaglass then exits and the new version starts. An
// update whose install was already tried once waits for the user.
func ApplyPendingUpdate(version string, tray bool) bool {
	exe, err := os.Executable()
	if err != nil || !update.Valid(version) {
		return false
	}
	update.CleanOld(exe)
	dir := platform.CacheDir("updates")
	p, ok := update.LoadPending(dir)
	if !ok {
		return false
	}
	if !update.Newer(p.Tag, version) {
		update.Clear(dir, "") // installed: tidy up
		return false
	}
	if p.Attempts > 0 || !settings.Open(settings.DefaultPath()).Get().AutoUpdate {
		return false
	}
	if err := applyPending(p, dir, exe, tray); err != nil {
		logx.Printf("update %s at start: %v", p.Tag, err)
		return false
	}
	return true
}

// UpdateService lets the interface see and drive updates.
type UpdateService struct{ c *Core }

// NewUpdateService binds updates to core.
func NewUpdateService(c *Core) *UpdateService { return &UpdateService{c} }

// State reports what the updater knows.
func (s *UpdateService) State() UpdateState { return s.c.updates.state() }

// Check looks for a new version now (and downloads it).
func (s *UpdateService) Check() {
	if s.c.updates.state().Status == UpdateOff {
		return
	}
	select {
	case s.c.updates.kick <- struct{}{}:
	default:
	}
}

// Install installs the downloaded update and restarts Seaglass.
func (s *UpdateService) Install() error { return s.c.updates.install() }

// OpenReleasePage opens the newest release on GitHub.
func (s *UpdateService) OpenReleasePage() error {
	return platform.OpenWebPage(s.c.updates.state().Page)
}
