package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/syncer"
	"github.com/ApolloF/Seaglass/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// syncerProject is Syncer's home page.
const syncerProject = "https://github.com/ApolloF/syncer"

// syncerFeed is Syncer's releases, and syncerInstaller the per-user
// installer every release carries.
var syncerFeed = update.Feed{
	LatestURL:   "https://api.github.com/repos/ApolloF/syncer/releases/latest",
	AssetPrefix: syncerProject + "/releases/download/",
	Hosts:       update.GitHub.Hosts,
	Keys:        syncerReleaseKeys,
	Product:     "Syncer",
}

// syncerReleaseKeys verify Syncer's releases once they're signed the way
// Seaglass's are: SHA256SUMS and SHA256SUMS.sig, an ed25519 signature
// over "Syncer release <tag>\n" and SHA256SUMS, made with a key kept away
// from GitHub. Until then a download is only checked against the SHA-256
// GitHub serves beside it, which someone who can publish a release can
// replace too; so the person confirms each install and sees Syncer's own
// installer instead of it running silently.
var syncerReleaseKeys []ed25519.PublicKey

const syncerInstaller = "Syncer-amd64-installer.exe"

// Saves is what Syncer knows about a game's saves, for the interface.
type Saves struct {
	Installed bool            `json:"installed"` // Syncer is installed
	Outdated  bool            `json:"outdated"`  // too old for the launcher API
	Available bool            `json:"available"` // Syncer answered
	Error     string          `json:"error,omitempty"`
	Known     bool            `json:"known"` // Syncer has save folders for this game
	Folders   []syncer.Folder `json:"folders"`
}

// SyncerStatus is how Seaglass and Syncer get on, for Settings.
type SyncerStatus struct {
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	Outdated    bool   `json:"outdated"`  // too old for the launcher API
	Connected   bool   `json:"connected"` // Syncer answered
	Running     bool   `json:"running"`   // its launcher API is up (it may not have been asked to start)
	Error       string `json:"error,omitempty"`
	Syncing     bool   `json:"syncing"` // its sync engine runs
	Paused      bool   `json:"paused"`
	PausedUntil int64  `json:"pausedUntil,omitempty"`
	BackingUp   bool   `json:"backingUp"`
	LastBackup  int64  `json:"lastBackup,omitempty"`
	Games       int    `json:"games"`
	Conflicts   int    `json:"conflicts"`
	CheckedAt   int64  `json:"checkedAt"`
}

// Syncer reports Syncer's state. With start it starts Syncer (without its
// window) when it isn't running, as a launch would; otherwise it only
// looks, so opening Settings doesn't start anything.
func (s *SavesService) Syncer(start bool) SyncerStatus {
	out := SyncerStatus{CheckedAt: time.Now().Unix()}
	inst, ok := syncer.Find()
	if !ok {
		return out
	}
	out.Installed, out.Version = true, inst.Version
	if inst.Version != "" && !inst.API() {
		out.Outdated, out.Error = true, syncer.ErrOutdated.Error()
		return out
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 15*time.Second)
	defer cancel()
	cl, err := syncer.Dial(ctx, start)
	switch {
	case errors.Is(err, syncer.ErrOutdated):
		out.Outdated, out.Error = true, err.Error()
		return out
	case err != nil && !start:
		return out // not running, and not asked to start it
	case err != nil:
		out.Error = "Syncer didn't answer"
		return out
	}
	defer cl.Close()
	out.Running = true
	st, err := cl.Status(ctx)
	if err != nil {
		out.Error = err.Error()
		out.Outdated = strings.Contains(err.Error(), "API version")
		return out
	}
	out.Connected = true
	if st.Version != "" {
		out.Version = strings.TrimPrefix(st.Version, "v")
	}
	out.Syncing, out.Paused, out.BackingUp = st.Syncthing, st.Paused, st.BackingUp
	out.Games, out.Conflicts = st.Games, st.Conflicts
	if !st.PausedTill.IsZero() {
		out.PausedUntil = st.PausedTill.Unix()
	}
	if !st.LastBackup.IsZero() {
		out.LastBackup = st.LastBackup.Unix()
	}
	return out
}

// SavesService is the game's saves, through Syncer.
type SavesService struct {
	c *Core

	installing sync.Mutex // InstallSyncer runs
	mu         sync.Mutex
	cache      map[int64]cachedSaves
	confirm    func(title, message string) bool // asks the person yes or no
}

type cachedSaves struct {
	at time.Time
	s  Saves
}

// NewSavesService binds Syncer to core.
func NewSavesService(c *Core) *SavesService {
	return &SavesService{c: c, cache: map[int64]cachedSaves{}, confirm: askYesNo}
}

// Saves returns what Syncer knows about a game's saves. Syncer is started
// in the background (without its window) when needed. Answers are kept
// for a little while; fresh asks again right away.
func (s *SavesService) Saves(id int64, fresh bool) (Saves, error) {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return Saves{}, library.ErrNotFound
	}
	s.mu.Lock()
	if c, ok := s.cache[id]; ok && !fresh && time.Since(c.at) < 30*time.Second {
		s.mu.Unlock()
		return c.s, nil
	}
	s.mu.Unlock()

	out := Saves{Folders: []syncer.Folder{}}
	if _, ok := syncer.Installed(); !ok {
		return out, nil
	}
	out.Installed = true
	ctx, cancel := context.WithTimeout(s.c.ctx, 15*time.Second)
	defer cancel()
	start := s.c.Settings.Get().StartSyncer
	cl, err := syncer.Dial(ctx, start)
	if errors.Is(err, syncer.ErrOutdated) {
		out.Outdated, out.Error = true, err.Error()
		return out, nil
	}
	if err != nil {
		out.Error = "Syncer didn't answer"
		if !start {
			out.Error = "Syncer isn't running"
		}
		return out, nil
	}
	defer cl.Close()
	if _, err := cl.Status(ctx); err != nil {
		out.Error = err.Error()
		out.Outdated = strings.Contains(err.Error(), "API version")
		return out, nil
	}
	out.Available = true
	gs, err := cl.GameStatus(ctx, syncerGame(g))
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	out.Known = gs.Known
	if gs.Folders != nil {
		out.Folders = gs.Folders
	}
	s.mu.Lock()
	s.cache[id] = cachedSaves{time.Now(), out}
	s.mu.Unlock()
	return out, nil
}

// OpenSyncer shows Syncer's window.
func (s *SavesService) OpenSyncer() error {
	exe, ok := syncer.Installed()
	if !ok {
		return syncer.ErrNotInstalled
	}
	cmd := exec.Command(exe)
	if err := cmd.Start(); err != nil { // a second start brings the open window forward
		return err
	}
	return cmd.Process.Release()
}

// SyncerProject opens Syncer's home page.
func (s *SavesService) SyncerProject() error { return platform.OpenWebPage(syncerProject) }

// InstallSyncer installs Syncer, or updates it, from its latest release,
// for this Windows account only (no administrator). Never over the same
// or a newer version; see syncerInstallPlan for when it asks first. It
// returns once the installer has exited; the interface shows it as
// installing until then. Closing the installer isn't an error.
func (s *SavesService) InstallSyncer() error {
	if !s.installing.TryLock() {
		return errors.New("Syncer is being installed already")
	}
	defer s.installing.Unlock()
	look, cancelLook := context.WithTimeout(s.c.ctx, syncerDownloadLimit)
	rel, err := syncerFeed.Latest(look)
	cancelLook()
	if err != nil {
		return fmt.Errorf("couldn't find Syncer's latest release: %w", err)
	}
	inst, installed := syncer.Find()
	plan, err := syncerInstallPlan(rel.Tag, inst, installed, syncerFeed.Signed())
	if err != nil {
		return err
	}
	if plan.ask != "" && !s.confirm("Install Syncer", plan.ask) {
		logx.Printf("Syncer %s not installed: the person said no", rel.Tag)
		return nil
	}
	// The download's time starts after the question, which may stay open a while.
	ctx, cancel := context.WithTimeout(s.c.ctx, syncerDownloadLimit)
	defer cancel()
	dir := platform.CacheDir("syncer")
	file, _, err := syncerFeed.Download(ctx, rel, syncerInstaller, dir, nil)
	if err != nil {
		return fmt.Errorf("couldn't download Syncer: %w", err)
	}
	defer os.Remove(file)
	logx.Printf("installing Syncer %s (signed: %v)", rel.Tag, syncerFeed.Signed())
	err = runSyncerInstaller(s.c.ctx, file, plan.args, dir, plan.limit())
	if errors.Is(err, errInstallerClosed) {
		logx.Printf("Syncer %s not installed: the installer was closed", rel.Tag)
		return nil
	}
	if err != nil {
		return err
	}
	if _, ok := syncer.Find(); !ok {
		if !plan.silent() {
			logx.Printf("Syncer %s not installed: the installer finished without it", rel.Tag)
			return nil
		}
		return errors.New("Syncer's installer finished, but Syncer isn't there")
	}
	s.mu.Lock()
	clear(s.cache)
	s.mu.Unlock()
	logx.Printf("installed Syncer %s", rel.Tag)
	return nil
}

// syncerDownloadLimit bounds finding and downloading a release, and
// syncerSilentLimit a silent install, which nobody is there to finish.
// An installer with windows has no limit: it is the person's to finish or
// close, however long they take.
const (
	syncerDownloadLimit = 10 * time.Minute
	syncerSilentLimit   = 10 * time.Minute
)

// nsisCancelled is the exit code of an NSIS installer (Syncer's is one)
// that the person cancelled or closed.
const nsisCancelled = 1

var errInstallerClosed = errors.New("Syncer's installer was closed before it finished")

// runSyncerInstaller runs exe from dir and waits for it to exit. With a
// limit it runs silently: it is stopped after limit, or when ctx ends.
// Without one the person is using its windows, so nothing stops it (not
// even Seaglass quitting, which would leave a half-done install), and
// cancelling it gives errInstallerClosed.
func runSyncerInstaller(ctx context.Context, exe string, args []string, dir string, limit time.Duration) error {
	var cmd *exec.Cmd
	if limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
		cmd = exec.CommandContext(ctx, exe, args...)
	} else {
		cmd = exec.Command(exe, args...)
	}
	cmd.Dir = dir
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case limit > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("Syncer's installer didn't finish within %v", limit)
	case limit == 0 && errors.As(err, &exit) && exit.ExitCode() == nsisCancelled:
		return errInstallerClosed
	}
	return fmt.Errorf("Syncer's installer failed: %w", err)
}

// syncerPlan is how a Syncer release gets installed.
type syncerPlan struct {
	args []string // for the installer
	ask  string   // the question to confirm first ("": none)
}

func (p syncerPlan) silent() bool { return len(p.args) > 0 }

// limit is how long the installer may run: a silent one is stopped after
// syncerSilentLimit, one with windows never (0).
func (p syncerPlan) limit() time.Duration {
	if p.silent() {
		return syncerSilentLimit
	}
	return 0
}

// syncerInstallPlan decides how release tag may be installed over the
// Syncer this PC has (inst, when installed). It refuses the same or an
// older version, so an old release with a known flaw can't be pushed back
// on. A signed release installs silently, as Seaglass's own updates do;
// anything else (or an update over a Syncer of unknown version) needs a
// yes first and runs Syncer's installer with its windows.
func syncerInstallPlan(tag string, inst syncer.Install, installed, signed bool) (syncerPlan, error) {
	if !update.Valid(tag) {
		return syncerPlan{}, fmt.Errorf("Syncer's latest release has an unexpected version %q", tag)
	}
	known := installed && update.Valid(inst.Version)
	if known && !update.Newer(tag, inst.Version) {
		return syncerPlan{}, fmt.Errorf("Syncer %s is installed, and the latest release is %s: nothing newer to install", inst.Version, tag)
	}
	if signed && (known || !installed) {
		return syncerPlan{args: []string{"/S"}}, nil
	}
	var ask string
	switch {
	case !signed:
		ask = fmt.Sprintf("Install Syncer %s from github.com/ApolloF/syncer?\n\nThis release isn't signed, so Seaglass can only check that the download arrived whole, not who made it. Syncer's installer opens next.", tag)
	default:
		ask = fmt.Sprintf("Install Syncer %s? Seaglass can't tell which version is installed now, so this could replace a newer one. Syncer's installer opens next.", tag)
	}
	return syncerPlan{ask: ask}, nil
}

// askYesNo asks the person a question in a Windows dialog over every
// window, and reports whether they said yes.
func askYesNo(title, message string) bool {
	app := application.Get()
	if app == nil {
		return false
	}
	yes := false
	d := app.Dialog.Question().SetTitle(title).SetMessage(message)
	d.AddButton("Yes").OnClick(func() { yes = true })
	d.SetDefaultButton(d.AddButton("No"))
	d.Show()
	return yes
}

// syncerGame describes a game to Syncer.
func syncerGame(g library.Game) syncer.Game {
	app := g.SteamAppID
	if app == 0 {
		app = g.MetaAppID
	}
	return syncer.Game{Title: g.DisplayTitle(), Dir: g.Dir, SteamAppID: app, GogID: g.GogID}
}

// syncerGames is every installed game, for Syncer's matching.
func (c *Core) syncerGames() []syncer.Game {
	var out []syncer.Game
	for _, g := range c.Lib.Games() {
		if g.Installed && !g.Hidden {
			out = append(out, syncerGame(g))
		}
	}
	return out
}

// registerWithSyncer tells a running Syncer which games this PC has. It
// doesn't start Syncer for it.
func (c *Core) registerWithSyncer() {
	if _, ok := syncer.Installed(); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	cl, err := syncer.Dial(ctx, false)
	if err != nil {
		return
	}
	defer cl.Close()
	if err := cl.RegisterGames(ctx, c.syncerGames()); err != nil {
		logx.Printf("syncer: registering games: %v", err)
		return
	}
	c.registered.Store(true)
}

// savesBeforeStep brings the game's saves up to date before it starts:
// it stops for two versions of a save or a newer save still on its way
// from another PC, then waits for Syncer to finish syncing.
func (c *Core) savesBeforeStep(g library.Game, known *bool) launch.Step {
	title := g.DisplayTitle()
	return launch.Step{ID: "savesBefore", Label: "Sync saves", Timeout: 7 * time.Minute,
		Run: func(ctx context.Context, sc *launch.StepContext) error {
			sc.Progress("Asking Syncer…")
			cfg := c.Settings.Get()
			dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			cl, err := syncer.Dial(dctx, cfg.StartSyncer)
			cancel()
			if errors.Is(err, syncer.ErrOutdated) {
				return err
			}
			if err != nil && !cfg.StartSyncer {
				sc.Progress("Syncer isn't running")
				return nil
			}
			if err != nil {
				return errors.New("Syncer didn't answer")
			}
			defer cl.Close()
			if _, err := cl.Status(ctx); err != nil {
				return err
			}
			if !cfg.AskWhoPlays {
				c.profile.follow(ctx, cl) // the person Syncer has on this PC plays
			}
			if cl.RegisterGames(ctx, c.syncerGames()) == nil {
				c.registered.Store(true)
			}
			gs, err := cl.GameStatus(ctx, syncerGame(g))
			if err != nil {
				return err
			}
			*known = gs.Known
			if !gs.Known {
				sc.Progress("Syncer has no saves for this game")
				return nil
			}
			conflicts, newerOn := 0, ""
			var ids []string
			for _, f := range gs.Folders {
				conflicts += f.Conflicts
				if f.NewerOn != "" && newerOn == "" {
					newerOn = f.NewerOn
				}
				if f.Sync {
					ids = append(ids, f.ID)
				}
			}
			wait := time.Duration(cfg.SyncWait) * time.Second
			if conflicts > 0 {
				a, err := sc.Ask(ctx, fmt.Sprintf("%s has two versions of a save. Pick the one to keep in Syncer first, so you don't play on the wrong one.", title), []launch.Option{
					{ID: "open", Label: "Open Syncer"},
					{ID: "play", Label: "Play anyway"},
					{ID: "cancel", Label: "Cancel"},
				})
				switch {
				case err != nil:
					return err
				case a == "open":
					_ = cl.Open(ctx)
					return launch.ErrCancel
				case a == "cancel":
					return launch.ErrCancel
				}
			}
			if newerOn != "" {
				a, err := sc.Ask(ctx, fmt.Sprintf("%s has a newer save of %s that hasn't arrived on this PC yet.", newerOn, title), []launch.Option{
					{ID: "wait", Label: "Wait for it"},
					{ID: "play", Label: "Play anyway"},
					{ID: "cancel", Label: "Cancel"},
				})
				switch {
				case err != nil:
					return err
				case a == "cancel":
					return launch.ErrCancel
				case a == "wait":
					wait = max(wait, 2*time.Minute+30*time.Second)
				}
			}
			if len(ids) == 0 {
				sc.Progress("Backed up, not synced")
				return nil
			}
			sc.Progress("Syncing…")
			res, err := cl.SyncNow(ctx, ids, wait)
			if err != nil {
				return err
			}
			for _, r := range res {
				if !r.Done {
					if r.State == "paused" {
						return errors.New("Syncing is paused in Syncer")
					}
					return errors.New("Still syncing; the other PC may be off")
				}
			}
			sc.Progress("Up to date")
			return nil
		}}
}

// savesAfterStep backs the game's saves up once it has exited. With wait
// false it only starts the backup (Syncer finishes it on its own), for a
// game started elsewhere, where nobody is waiting on a launch sequence.
func (c *Core) savesAfterStep(g library.Game, known *bool, wait bool) launch.Step {
	return launch.Step{ID: "savesAfter", Label: "Back up saves", Timeout: 3 * time.Minute,
		Run: func(ctx context.Context, sc *launch.StepContext) error {
			start := c.Settings.Get().StartSyncer
			dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			cl, err := syncer.Dial(dctx, start)
			cancel()
			if errors.Is(err, syncer.ErrOutdated) {
				return err
			}
			if err != nil && !start {
				sc.Progress("Syncer isn't running")
				return nil
			}
			if err != nil {
				return errors.New("Syncer didn't answer")
			}
			defer cl.Close()
			if !*known {
				// A first play may have made the save folder; Syncer finds
				// new games on its own, so only back up what it knows.
				gs, err := cl.GameStatus(ctx, syncerGame(g))
				if err != nil {
					return err
				}
				if !gs.Known {
					sc.Progress("Syncer has no saves for this game")
					return nil
				}
			}
			sc.Progress("Backing up…")
			if !wait {
				_, err := cl.BackupNow(ctx, false, 0)
				if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
					return err
				}
				sc.Progress("Backing up in the background")
				return nil
			}
			r, err := cl.BackupNow(ctx, true, 2*time.Minute+30*time.Second)
			switch {
			case err != nil && strings.Contains(strings.ToLower(err.Error()), "already"):
				sc.Progress("Syncer is already backing up")
			case err != nil:
				return err
			case !r.Finished:
				sc.Progress("Backing up in the background")
			case !r.OK && len(r.Errors) > 0:
				return errors.New(strings.TrimSpace(r.Errors[0]))
			default:
				sc.Progress("Backed up")
			}
			return nil
		}}
}
