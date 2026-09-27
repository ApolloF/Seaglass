package app

import (
	"runtime/debug"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Shell owns Seaglass's windows and tray icon. The Go core keeps
// running without any window: while a game runs the interface is closed
// to free its memory, and comes back when the game exits.
type Shell struct {
	c *Core

	mu       sync.Mutex
	main     *application.WebviewWindow
	overlay  *application.WebviewWindow
	tray     *application.SystemTray
	uiMode   string // "desktop" or "bigpicture": what the main window shows
	gameMode bool   // the main window was closed for a game
	closing  bool   // Seaglass closes a window itself (not the user)
	waitGame int    // counts closeMainWhenGameInFront calls, so only the latest acts
}

// NewShell makes the shell.
func NewShell(c *Core) *Shell {
	s := &Shell{c: c, uiMode: "desktop"}
	c.shell = s
	return s
}

// Start shows the main window and the tray icon.
func (s *Shell) Start() {
	s.OpenMain()
	s.startTray()
}

// StartHidden shows only the tray icon (a game starts from the command line).
func (s *Shell) StartHidden() { s.startTray() }

// SetUIMode records which mode the interface is in, so it comes back the
// same way after a game.
func (s *Shell) SetUIMode(mode string) {
	if mode != "desktop" && mode != "bigpicture" {
		return
	}
	s.mu.Lock()
	s.uiMode = mode
	s.mu.Unlock()
}

// OpenMain shows the main window, making it again when it was closed.
func (s *Shell) OpenMain() {
	s.mu.Lock()
	w := s.main
	if w == nil {
		w = s.newMain(s.uiMode)
		s.main = w
	}
	s.gameMode = false
	s.mu.Unlock()
	s.bringForward(w)
}

// bringForward shows w in front. Wails' Restore also takes a window out of
// full screen, so it's only used on a minimised one; big picture goes back
// to full screen if it left it.
func (s *Shell) bringForward(w *application.WebviewWindow) {
	if w.IsMinimised() {
		w.UnMinimise()
	}
	s.mu.Lock()
	bp := s.uiMode == "bigpicture"
	s.mu.Unlock()
	if bp && !w.IsFullscreen() {
		w.Fullscreen()
	}
	w.Show()
	w.Focus()
}

// newMain makes the main window; the interface starts in mode.
func (s *Shell) newMain(mode string) *application.WebviewWindow {
	url := "/"
	if mode == "bigpicture" {
		url = "/?mode=bigpicture"
	}
	start := application.WindowStateNormal
	if mode == "bigpicture" {
		// Full screen from the first frame, rather than a window that
		// grows once the interface has loaded.
		start = application.WindowStateFullscreen
	}
	w := application.Get().Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "Seaglass",
		Width:            1440,
		Height:           900,
		MinWidth:         980,
		MinHeight:        620,
		Frameless:        true,
		StartState:       start,
		BackgroundColour: application.NewRGB(10, 14, 19),
		URL:              url,
		Windows:          application.WindowsWindow{Theme: application.SystemDefault},
	})
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.mu.Lock()
		byUser := !s.closing
		if s.main == w {
			s.main = nil
		}
		s.mu.Unlock()
		// Closing the window closes Seaglass, except while a game
		// runs: then it stays in the tray, following the game.
		if byUser && !s.c.Launch.Active() {
			go application.Get().Quit()
		}
	})
	return w
}

// closeMainForGame closes the interface while a game runs.
func (s *Shell) closeMainForGame() {
	s.mu.Lock()
	w := s.main
	s.gameMode = w != nil
	s.closing = true
	s.mu.Unlock()
	if w != nil {
		w.Close()
		logx.Printf("interface closed while playing")
		// Hand the memory the interface's data used back to Windows now,
		// rather than whenever the runtime gets round to it.
		time.AfterFunc(3*time.Second, debug.FreeOSMemory)
	}
	s.mu.Lock()
	s.closing = false
	s.mu.Unlock()
}

// closeMainWhenGameInFront closes the interface once the game's own window
// is in front, so the launch sequence stays on screen until the game shows
// instead of the desktop in between (games and stores can take a while
// to open a window). It gives up waiting after a while: some games show
// their window from a process that isn't theirs.
func (s *Shell) closeMainWhenGameInFront(isGame func(pid uint32) bool) {
	s.mu.Lock()
	s.waitGame++
	turn := s.waitGame
	s.mu.Unlock()
	go func() {
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if pid := platform.ForegroundPID(); pid != 0 && isGame(pid) {
				time.Sleep(700 * time.Millisecond) // let it settle full screen first
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		s.mu.Lock()
		stale := turn != s.waitGame
		s.mu.Unlock()
		if !stale && s.c.Launch.Current().Phase == launch.Running {
			s.closeMainForGame()
		}
	}()
}

// reopenForGame brings the interface back as soon as the game has gone
// (its session ends a few seconds later), if it was closed for it.
func (s *Shell) reopenForGame() {
	s.mu.Lock()
	s.waitGame++ // a close still waiting for the game is off
	reopen := s.gameMode
	s.mu.Unlock()
	if reopen {
		s.OpenMain()
	}
}

// gameEnded brings the interface back when it was closed for the game.
func (s *Shell) gameEnded() {
	s.CloseOverlay()
	s.mu.Lock()
	reopen := s.gameMode || s.main == nil
	s.mu.Unlock()
	if reopen {
		s.OpenMain()
	}
}

// ToggleOverlay opens or closes the in-game overlay.
func (s *Shell) ToggleOverlay() {
	s.mu.Lock()
	open := s.overlay != nil
	s.mu.Unlock()
	if open {
		s.CloseOverlay()
	} else {
		s.OpenOverlay()
	}
}

// OpenOverlay shows the overlay over the game: a borderless window on
// top of everything. Nothing is injected into the game.
func (s *Shell) OpenOverlay() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overlay != nil {
		s.overlay.Focus()
		return
	}
	w := application.Get().Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "overlay",
		Title:            "Seaglass",
		Frameless:        true,
		AlwaysOnTop:      true,
		StartState:       application.WindowStateFullscreen,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/?view=overlay",
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:                   true,
			DisableFramelessWindowDecorations: true,
		},
	})
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.mu.Lock()
		if s.overlay == w {
			s.overlay = nil
		}
		s.mu.Unlock()
	})
	s.overlay = w
	w.Show()
	w.Focus()
}

// CloseOverlay hides the overlay again.
func (s *Shell) CloseOverlay() {
	s.mu.Lock()
	w := s.overlay
	s.overlay = nil
	s.mu.Unlock()
	if w != nil {
		w.Close()
	}
}

// OverlayOpen reports whether the overlay shows.
func (s *Shell) OverlayOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlay != nil
}

// FocusMain brings the main window forward if it is open; it reports
// whether it was.
func (s *Shell) FocusMain() bool {
	s.mu.Lock()
	w := s.main
	s.mu.Unlock()
	if w == nil {
		return false
	}
	s.bringForward(w)
	return true
}

func (s *Shell) startTray() {
	a := application.Get()
	t := a.SystemTray.New() // shows the exe's own icon
	t.SetTooltip("Seaglass")
	menu := a.NewMenu()
	menu.Add("Open Seaglass").OnClick(func(*application.Context) { s.OpenMain() })
	menu.Add("Big picture").OnClick(func(*application.Context) {
		s.SetUIMode("bigpicture")
		s.OpenMain()
		s.c.emit(EventUIMode, "bigpicture")
	})
	menu.AddSeparator()
	menu.Add("Quit Seaglass").OnClick(func(*application.Context) { a.Quit() })
	t.SetMenu(menu)
	t.OnClick(func() { s.OpenMain() })
	s.mu.Lock()
	s.tray = t
	s.mu.Unlock()
}

// setTrayTooltip shows what Seaglass is doing on the tray icon.
func (s *Shell) setTrayTooltip(text string) {
	s.mu.Lock()
	t := s.tray
	s.mu.Unlock()
	if t != nil {
		t.SetTooltip(text)
	}
}
