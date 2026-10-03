package app

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/steaminput"
	"github.com/ApolloF/Seaglass/internal/syncer"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Events about playing.
const (
	EventSession       = "launch:session"
	EventOverlayAction = "overlay:action" // controller actions while the overlay shows
	EventUIMode        = "ui:mode"        // the tray asks the interface to switch mode
)

func init() {
	application.RegisterEvent[launch.Session](EventSession)
	application.RegisterEvent[PadAction](EventOverlayAction)
	application.RegisterEvent[string](EventUIMode)
}

// Routes: how a game is started.
const (
	RouteDirect     = "direct"     // its exe, by Seaglass
	RouteStore      = "store"      // handed to its store (Steam, Epic, …)
	RouteSteamInput = "steamInput" // a non-Steam shortcut, so Steam Input handles the controller
)

// LaunchService starts games and follows them while they run.
type LaunchService struct{ c *Core }

// NewLaunchService binds launching to core.
func NewLaunchService(c *Core) *LaunchService { return &LaunchService{c} }

// Play starts a game: hooks first, then the game, then it is followed
// until it exits.
func (s *LaunchService) Play(id int64) error { return s.play(id, s.c.shell.startMode()) }

// play starts a game from where: "bigpicture", "desktop" or "" (outside the interface).
func (s *LaunchService) play(id int64, from string) error {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return library.ErrNotFound
	}
	if !g.Installed {
		return errors.New(g.DisplayTitle() + " isn't installed")
	}
	if g.LaunchURI == "" {
		if g.Exe == "" {
			return errors.New("no program found to start " + g.DisplayTitle() + ". Choose one in the game's options.")
		}
		if !platform.Within(g.Dir, g.Exe) && !g.UserExe {
			return errors.New("the game's program is outside its folder")
		}
	}
	if cur := s.c.Launch.Current(); !cur.Phase.Done() {
		if cur.GameID == id {
			return nil // already on its way
		}
		return errors.New(cur.Title + " is still running")
	}
	p := s.c.plan(g)
	p.From = from
	return s.c.Launch.Launch(s.c.ctx, p)
}

// Session returns the current (or last) game session.
func (s *LaunchService) Session() launch.Session { return s.c.Launch.Current() }

// Skip skips a hook step that is running.
func (s *LaunchService) Skip(stepID string) { s.c.Launch.Skip(stepID) }

// Answer answers a question a hook asked.
func (s *LaunchService) Answer(questionID int, option string) { s.c.Launch.Answer(questionID, option) }

// Cancel stops a launch that hasn't reached the game yet.
func (s *LaunchService) Cancel() { s.c.Launch.Cancel() }

// QuitGame ends the running game's processes right away.
func (s *LaunchService) QuitGame() error {
	err := s.c.Launch.Quit()
	if err == nil {
		logx.Printf("quit game %q", s.c.Launch.Current().Title)
	}
	return err
}

// SetUIMode tells the core which mode the interface shows.
func (s *LaunchService) SetUIMode(mode string) { s.c.shell.SetUIMode(mode) }

// CloseOverlay hides the in-game overlay.
func (s *LaunchService) CloseOverlay() { s.c.shell.CloseOverlay() }

// OpenMain opens the main window (from the overlay), leaving the game running.
func (s *LaunchService) OpenMain() {
	s.c.shell.CloseOverlay()
	s.c.shell.OpenMain()
}

// Route tells how a game would start now: direct, store or steamInput.
func (s *LaunchService) Route(id int64) string {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return RouteDirect
	}
	return route(g, s.c.padState())
}

// route picks how a game starts. Store games go through their store
// (Steam applies Steam Input to its own games). Other games go through
// Steam Input when the user chose it, or in auto mode when a PlayStation
// controller is in use and the game is known to support controllers but
// not PlayStation ones.
func route(g library.Game, ps pad.State) string {
	if g.LaunchURI != "" {
		return RouteStore
	}
	switch g.PadMode {
	case "native":
		return RouteDirect
	case "steam":
		return RouteSteamInput
	}
	if !ps.Connected || ps.Kind != pad.PlayStation || g.PadHint != "" || g.Meta == nil {
		return RouteDirect
	}
	if g.Meta.DualSense != "" && g.Meta.DualSense != "no" {
		return RouteDirect
	}
	if c := strings.ToLower(g.Meta.Controller); c == "full" || c == "partial" {
		return RouteSteamInput
	}
	return RouteDirect
}

// plan builds the launch of one game.
func (c *Core) plan(g library.Game) launch.Plan {
	title := g.DisplayTitle()
	r := route(g, c.padState())
	var steamURI string
	var before, after []launch.Step
	cfg := c.Settings.Get()
	if _, ok := syncer.Installed(); ok {
		if cfg.AskWhoPlays {
			before = append(before, c.whoPlaysStep(g))
		} else if !cfg.SyncSavesBefore && c.profile != nil {
			c.profile.changed() // who's playing now (savesBefore looks otherwise)
		}
		known := false
		if cfg.SyncSavesBefore {
			before = append(before, c.savesBeforeStep(g, &known))
		} else {
			known = true // not checked: let the backup step ask Syncer itself
		}
		if cfg.BackupSavesAfter {
			after = append(after, c.savesAfterStep(g, &known, true))
		}
	}
	if r == RouteSteamInput {
		before = append(before, c.steamInputStep(g, &steamURI))
	}
	dirs := []string{g.Dir}
	if g.Exe != "" && !platform.Within(g.Dir, g.Exe) {
		dirs = append(dirs, filepath.Dir(g.Exe))
	}
	detect := 2 * time.Minute // a UAC prompt can wait a while
	if r != RouteDirect {
		detect = 5 * time.Minute // stores update, sign in, build shader caches
	}
	return launch.Plan{
		GameID: g.ID, Title: title, Dirs: dirs, Before: before, After: after, DetectTimeout: detect,
		Start: func() (uint32, string, error) {
			var err error
			var pid int
			used := r
			switch {
			case steamURI != "":
				err = platform.OpenURI(steamURI)
			case g.LaunchURI != "":
				used = RouteStore
				err = platform.OpenURI(g.LaunchURI)
			default:
				used = RouteDirect
				pid, err = platform.StartProcess(g.Exe, g.Args, g.WorkDir)
			}
			if err != nil {
				logx.Printf("play %q: %v", title, err)
				return 0, used, err
			}
			logx.Printf("play %q (%s)", title, used)
			// The game looks for controllers as it starts, which can be
			// long before it's seen running: hand it over now.
			c.padForGame()
			c.startedPlaying(g)
			return uint32(pid), used, nil
		},
		Played: func(secs int64) { c.addPlaytime(g, secs) },
		OnRun: func() {
			heapDiag("playing")
			c.shell.setTrayTooltip("Seaglass · playing " + title)
			c.padForGame()
			if c.Settings.Get().CloseWhilePlaying {
				c.shell.closeMainWhenGameInFront(c.Launch.IsGame)
			}
		},
		// The game's gone from the screen: the interface comes back now,
		// not only once the session has ended a few seconds later. If it
		// was a launcher handing over, it steps aside again for the game.
		OnGone: func() {
			if c.Settings.Get().CloseWhilePlaying {
				c.shell.reopenForGame(c.Launch.Current().From)
			}
		},
		OnBack: func() {
			if c.Settings.Get().CloseWhilePlaying {
				c.shell.closeMainWhenGameInFront(c.Launch.IsGame)
			}
		},
	}
}

// padForGame steps the controller layer back while a game runs.
func (c *Core) padForGame() {
	m := c.padManager()
	if m == nil {
		return
	}
	if c.Settings.Get().PadWhilePlaying == "off" {
		m.SetMode(pad.Off)
	} else {
		m.SetMode(pad.Passive)
	}
}

// onSession follows session changes: it tells the interface and, when a
// game has ended, gives the controller back and reopens the interface.
func (c *Core) onSession(s launch.Session) {
	c.emit(EventSession, s)
	// The game has exited: the controller comes back now, not after the
	// steps that follow it (a save backup can take minutes).
	if s.Phase == launch.Finishing {
		if m := c.padManager(); m != nil {
			m.SetMode(pad.Active)
		}
	}
	if !s.Phase.Done() {
		if c.ach != nil {
			c.ach.sessionStarted(s.ID, s.GameID)
		}
		return
	}
	c.stateMu.Lock()
	last := c.lastSession
	c.lastSession = s.ID
	c.stateMu.Unlock()
	if last == s.ID {
		return
	}
	if s.GameID > 0 && s.StartedAt > 0 && c.ach != nil {
		go c.ach.afterSession(s.ID, s.GameID, s.Title, 3*time.Second)
	} else if c.ach != nil {
		c.ach.forget(s.ID) // the game never ran
	}
	if s.StartedAt > 0 {
		logx.Printf("played %q for %s", s.Title, (time.Duration(s.Seconds) * time.Second).String())
	}
	for _, st := range append(append([]launch.StepState{}, s.Before...), s.After...) {
		logx.Printf("  %s: %s %s", st.Label, st.Status, st.Detail)
	}
	if m := c.padManager(); m != nil {
		m.SetMode(pad.Active)
	}
	if c.shell != nil {
		c.shell.setTrayTooltip("Seaglass")
		if s.Route == RouteExternal {
			c.shell.CloseOverlay() // the interface wasn't closed for it
		} else {
			c.shell.gameEnded(s.From)
		}
	}
}

// steamInputStep makes sure the game has a Steam shortcut, then sets
// uri so it starts through Steam. Steam must be closed to add one; the
// user decides whether to restart it.
func (c *Core) steamInputStep(g library.Game, uri *string) launch.Step {
	return launch.Step{ID: "steamInput", Label: "Steam Input", Timeout: 3 * time.Minute,
		Run: func(ctx context.Context, sc *launch.StepContext) error {
			files, err := steaminput.Files()
			if err != nil {
				return err
			}
			sh := shortcutFor(g)
			if id, ok := steaminput.Find(files, sh); ok {
				*uri = steaminput.URI(id)
				sc.Progress("Starts through Steam")
				return nil
			}
			if steaminput.Running() {
				a, err := sc.Ask(ctx, "Steam needs to restart once to add "+g.DisplayTitle()+" for Steam Input.", []launch.Option{
					{ID: "restart", Label: "Restart Steam"},
					{ID: "direct", Label: "Start without Steam Input"},
					{ID: "cancel", Label: "Cancel"},
				})
				switch {
				case err != nil:
					return err
				case a == "cancel":
					return launch.ErrCancel
				case a == "direct":
					sc.Progress("Starting without Steam Input")
					return nil
				}
				sc.Progress("Closing Steam…")
				sctx, cancel := context.WithTimeout(ctx, time.Minute)
				err = steaminput.Shutdown(sctx)
				cancel()
				if err != nil {
					return err
				}
			}
			// Add every game set to Steam Input at once, so Steam restarts only once.
			list := []steaminput.Shortcut{sh}
			for _, o := range c.Lib.Games() {
				if o.ID != g.ID && o.Installed && o.PadMode == "steam" && o.LaunchURI == "" && o.Exe != "" {
					list = append(list, shortcutFor(o))
				}
			}
			sc.Progress("Adding to Steam…")
			if err := steaminput.Add(files, list); err != nil {
				return err
			}
			logx.Printf("steam input: added %d shortcut(s)", len(list))
			*uri = steaminput.URI(sh.ID())
			sc.Progress("Added to Steam")
			return nil
		}}
}

func shortcutFor(g library.Game) steaminput.Shortcut {
	return steaminput.Shortcut{Name: g.DisplayTitle(), Exe: g.Exe, WorkDir: g.WorkDir, Args: g.Args}
}

// Args are Seaglass's command-line options.
type Args struct {
	Play    int64 // --play <id>: start this game without the interface
	Tray    bool  // --tray: start in the tray (at sign-in)
	Quit    bool  // --quit: close the running Seaglass (installer)
	Updated bool  // --updated: started by an update
	// --diagnostics: write a diagnostics report to the desktop and exit,
	// for when the interface won't open.
	Diagnostics bool
	// Dev flags for the test harness (dev builds only; see dev.go).
	Dev DevArgs
}

// ParseArgs reads the command line; unknown arguments are ignored.
func ParseArgs(args []string) Args {
	var a Args
	a.Play, _ = PlayArg(args)
	for _, s := range args {
		switch s {
		case "--tray":
			a.Tray = true
		case "--quit":
			a.Quit = true
		case "--updated":
			a.Updated = true
		case "--diagnostics":
			a.Diagnostics = true
		default:
			parseDevArg(&a.Dev, s)
		}
	}
	return a
}

// PlayArg finds "--play <id>" in command-line arguments, so a desktop
// shortcut can start a game through Seaglass.
func PlayArg(args []string) (int64, bool) {
	for i, a := range args {
		if a == "--play" && i+1 < len(args) {
			id, err := strconv.ParseInt(args[i+1], 10, 64)
			return id, err == nil && id > 0
		}
	}
	return 0, false
}

// PlayFromArgs starts the game named by "--play <id>", if any. The
// interface isn't opened for it; the game comes first.
func (s *LaunchService) PlayFromArgs(args []string) bool {
	id, ok := PlayArg(args)
	if !ok {
		return false
	}
	if err := s.play(id, ""); err != nil {
		logx.Printf("--play %d: %v", id, err)
		s.c.shell.OpenMain()
	}
	return true
}
