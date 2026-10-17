package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/update"
	"github.com/ApolloF/Seaglass/internal/vpad"
)

// Texts shown in the launch panel.
const (
	setupNotice  = "Set up Seaglass's Xbox controller in Settings to play this game with your DualSense."
	twiceWarning = "Some games may see your controller twice."
)

// ControllerService is Seaglass's Xbox controller for games: whether its
// two drivers are there, and setting them up.
type ControllerService struct{ c *Core }

// NewControllerService binds the controller bridge to core.
func NewControllerService(c *Core) *ControllerService { return &ControllerService{c} }

// Status says what the drivers can do on this PC.
func (s *ControllerService) Status() vpad.Status { return s.c.ctl.deps.status() }

// SetUpDrivers downloads the official installers of the drivers that are
// missing, checks them against the hashes pinned in Seaglass, and runs
// them one after the other (each asks Windows for permission itself). It
// returns the status once they're done. Closing an installer isn't an
// error; the status then says what's still missing.
func (s *ControllerService) SetUpDrivers() (vpad.Status, error) {
	return s.c.ctl.setUp(s.c.ctx)
}

// hider hides physical controllers from games (HidHide).
type hider interface {
	Active() (bool, error)
	Hide(ids []string) error
	Clear() error
	Close() error
}

// bridgeDeps are the drivers and installers, swapped for fakes in tests.
type bridgeDeps struct {
	status    func() vpad.Status
	newBridge func() (vpad.Bridge, error)
	newHider  func() (hider, error)
	// install downloads one installer and runs it; restart reports that
	// it asked for Windows to restart, cancelled that it was closed.
	install func(ctx context.Context, in vpad.Installer) (restart, cancelled bool, err error)
}

var realBridgeDeps = bridgeDeps{
	status:    vpad.GetStatus,
	newBridge: func() (vpad.Bridge, error) { return vpad.NewX360() },
	newHider:  func() (hider, error) { return vpad.OpenHider() },
	install:   installDriver,
}

// controllerState is the controller bridge of the game being played.
type controllerState struct {
	c    *Core
	deps bridgeDeps

	setting sync.Mutex // SetUpDrivers runs

	mu     sync.Mutex
	bridge vpad.Bridge // nil when no game uses it
	hider  hider
	// hideErr is why hiding didn't work, for the launch panel; hidOnce is
	// closed once the controllers SDL opened in Bridge mode were handled.
	hideErr  error
	hidOnce  chan struct{}
	hidShut  bool
	lastSent time.Time
}

func newControllerState(c *Core) *controllerState {
	return &controllerState{c: c, deps: realBridgeDeps}
}

// active reports whether a game plays through the bridge now.
func (cs *controllerState) active() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.bridge != nil
}

// translator is what turns a PlayStation controller into an Xbox one on
// this PC.
func (c *Core) translator() translator {
	return translator{viGEm: c.ctl.deps.status().ViGEm, steam: c.Settings.Get().PadTranslator == "steam"}
}

// hideWait is how long the controller step waits for SDL to open the
// controllers in Bridge mode, so they're hidden before the game looks.
const hideWait = 3 * time.Second

// begin plugs in the virtual controller, hands the controller layer to it
// and hides the physical controllers (when hide). The game can start once
// it returns. err means there is no virtual controller; warn is set when
// it works but the real controller couldn't be hidden.
func (cs *controllerState) begin(hide bool) (warn string, err error) {
	cs.end() // a session that never ended cleanly
	b, err := cs.deps.newBridge()
	if err != nil {
		return "", err
	}
	if err := b.Plug(); err != nil {
		_ = b.Close()
		return "", err
	}
	m := cs.c.padManager()
	b.Rumble(func(large, small uint8) {
		if m != nil {
			m.BridgeRumble(large, small)
		}
	})
	cs.mu.Lock()
	cs.bridge, cs.hider, cs.hideErr = b, nil, nil
	cs.hidOnce, cs.hidShut = make(chan struct{}), false
	cs.mu.Unlock()

	if hide {
		h, herr := cs.openHider()
		cs.mu.Lock()
		cs.hider, cs.hideErr = h, herr
		cs.mu.Unlock()
	}
	if m == nil {
		cs.handled()
	} else {
		m.SetBridge(cs.send, cs.hidePads)
		m.SetMode(pad.Bridge)
	}
	cs.mu.Lock()
	once := cs.hidOnce
	cs.mu.Unlock()
	select {
	case <-once:
	case <-time.After(hideWait):
	}
	cs.mu.Lock()
	herr := cs.hideErr
	cs.mu.Unlock()
	if hide && herr != nil {
		logx.Printf("controller bridge: not hiding the controller: %v", herr)
		warn = twiceWarning
	}
	return warn, nil
}

// openHider opens HidHide when it can hide a controller for this session.
func (cs *controllerState) openHider() (hider, error) {
	h, err := cs.deps.newHider()
	if err != nil {
		return nil, err
	}
	if on, err := h.Active(); err != nil || !on {
		_ = h.Close()
		if err == nil {
			err = errors.New("HidHide's hiding is switched off")
		}
		return nil, err
	}
	return h, nil
}

// send passes the controller's state to the virtual controller. It runs on
// the SDL thread.
func (cs *controllerState) send(r pad.Raw) {
	cs.mu.Lock()
	b := cs.bridge
	cs.mu.Unlock()
	if b == nil {
		return
	}
	if err := b.Send(vpad.FromSDL(r.Buttons, r.Axes)); err != nil {
		cs.mu.Lock()
		quiet := time.Since(cs.lastSent) < time.Minute
		cs.lastSent = time.Now()
		cs.mu.Unlock()
		if !quiet {
			logx.Printf("controller bridge: %v", err)
		}
	}
}

// hidePads hides the controllers SDL has open (by device path) from games.
// It runs on the SDL thread whenever they change: a controller plugged in
// mid-game is hidden too.
func (cs *controllerState) hidePads(paths []string) {
	defer cs.handled()
	cs.mu.Lock()
	h := cs.hider
	cs.mu.Unlock()
	if h == nil {
		return
	}
	var ids []string
	for _, p := range paths {
		if id, ok := vpad.InstanceID(p); ok {
			ids = append(ids, id)
		}
	}
	err := h.Clear()
	if err == nil {
		err = h.Hide(ids)
	}
	if err == nil && len(paths) > 0 && len(ids) == 0 {
		err = errors.New("the controller isn't a HID device")
	}
	cs.mu.Lock()
	cs.hideErr = err
	cs.mu.Unlock()
	if err == nil && len(ids) > 0 {
		logx.Printf("controller bridge: hiding %d controller(s) from games", len(ids))
	}
}

// handled says the controllers were looked at once.
func (cs *controllerState) handled() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.hidOnce != nil && !cs.hidShut {
		close(cs.hidOnce)
		cs.hidShut = true
	}
}

// end unplugs the virtual controller and shows the real one again. Safe to
// call when nothing runs. The controller layer's mode is the caller's.
func (cs *controllerState) end() {
	cs.mu.Lock()
	b, h := cs.bridge, cs.hider
	cs.bridge, cs.hider = nil, nil
	cs.mu.Unlock()
	if b == nil && h == nil {
		return
	}
	if m := cs.c.padManager(); m != nil {
		m.SetBridge(nil, nil)
	}
	if b != nil {
		if err := b.Close(); err != nil {
			logx.Printf("controller bridge: unplugging: %v", err)
		}
	}
	if h != nil {
		if err := h.Close(); err != nil {
			logx.Printf("controller bridge: showing the controller again: %v", err)
		}
	}
	cs.handled()
}

// controllerStep starts the virtual controller before the game, since some
// games only look for controllers as they start. Without one the game
// still starts, the way it would without Seaglass.
func (c *Core) controllerStep(bridged *bool) launch.Step {
	return launch.Step{ID: "controller", Label: "Xbox controller", Timeout: 30 * time.Second,
		Run: func(ctx context.Context, sc *launch.StepContext) error {
			warn, err := c.ctl.begin(c.Settings.Get().HidePadWhilePlaying)
			if err != nil {
				logx.Printf("controller bridge: %v", err)
				if errors.Is(err, vpad.ErrNoViGEm) {
					sc.Progress(setupNotice)
				} else {
					sc.Progress("Couldn't start the Xbox controller. Starting without it.")
				}
				return nil
			}
			*bridged = true
			if warn != "" {
				sc.Progress(warn)
			} else {
				sc.Progress("Your controller works as an Xbox controller in this game.")
			}
			return nil
		}}
}

// setupStep only tells the person the game would need Seaglass's Xbox
// controller, which isn't set up.
func setupStep() launch.Step {
	return launch.Step{ID: "controller", Label: "Xbox controller", Timeout: time.Second,
		Run: func(ctx context.Context, sc *launch.StepContext) error {
			sc.Progress(setupNotice)
			return nil
		}}
}

// setUp installs the missing drivers.
func (cs *controllerState) setUp(ctx context.Context) (vpad.Status, error) {
	if !cs.setting.TryLock() {
		return cs.deps.status(), errors.New("the drivers are being set up already")
	}
	defer cs.setting.Unlock()
	restart := false
	for _, in := range vpad.Needed(cs.deps.status()) {
		logx.Printf("controller: installing %s", in.Name)
		r, cancelled, err := cs.deps.install(ctx, in)
		if err != nil {
			return cs.deps.status(), err
		}
		if cancelled {
			logx.Printf("controller: %s's installer was closed", in.Name)
			break
		}
		restart = restart || r
	}
	st := cs.deps.status()
	st.Restart = st.Restart || restart
	return st, nil
}

// Windows Installer exit codes the driver installers (WiX bundles) use.
const (
	exitRebootRequired  = 3010 // ERROR_SUCCESS_REBOOT_REQUIRED
	exitRebootInitiated = 1641 // ERROR_SUCCESS_REBOOT_INITIATED
	exitUserCancelled   = 1602 // ERROR_INSTALL_USEREXIT
)

// installDriver downloads a driver's installer, checks it and runs it.
func installDriver(ctx context.Context, in vpad.Installer) (restart, cancelled bool, err error) {
	feed := update.Feed{AssetPrefix: in.Repo + "/releases/download/", Hosts: update.GitHub.Hosts}
	dctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	dir := platform.CacheDir("drivers")
	file, err := feed.DownloadPinned(dctx, in.URL, in.SHA256, dir)
	if err != nil {
		return false, false, fmt.Errorf("couldn't download %s: %w", in.Name, err)
	}
	defer os.Remove(file)
	code, err := platform.RunInstaller(file, dir)
	if err != nil {
		return false, false, fmt.Errorf("couldn't start %s's installer: %w", in.Name, err)
	}
	switch code {
	case 0:
		return false, false, nil
	case exitRebootRequired, exitRebootInitiated:
		return true, false, nil
	case exitUserCancelled:
		return false, true, nil
	}
	return false, false, fmt.Errorf("%s's installer failed (exit code %d)", in.Name, code)
}
