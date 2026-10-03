package app

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Controller events for the interface.
const (
	EventPadAction = "pad:action"
	EventPadState  = "pad:state"
	EventPadRaw    = "pad:raw" // every button and axis, while the test screen is open
)

// PadAction is one controller action.
type PadAction struct {
	Action string `json:"action"`
	Repeat bool   `json:"repeat"`
}

func init() {
	application.RegisterEvent[PadAction](EventPadAction)
	application.RegisterEvent[pad.State](EventPadState)
	application.RegisterEvent[pad.Raw](EventPadRaw)
}

// PadService is the controller: actions, glyph family, rumble and lightbar.
type PadService struct {
	c   *Core
	mu  sync.Mutex
	mgr *pad.Manager
	// The interface asks for rumble on every move; cap it so a held
	// direction doesn't turn into one long buzz.
	lastRumble time.Time
	lastPad    string // the controller last logged, for the log
}

// NewPadService binds the controller layer to core.
func NewPadService(c *Core) *PadService { return &PadService{c: c} }

// ServiceStartup starts reading controllers.
func (s *PadService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.mgr = pad.Start(s.onAction, s.onState)
	s.c.pad.Store(s.mgr)
	return nil
}

// ServiceShutdown releases the controllers.
func (s *PadService) ServiceShutdown() error {
	if s.mgr != nil {
		s.mgr.Stop()
	}
	return nil
}

func (s *PadService) onAction(action string, repeat bool) {
	home := action == pad.Home && !repeat && s.c.Settings.Get().PSButton
	// While a game runs the controller belongs to the game: actions only
	// drive the overlay, which the PS / Xbox button opens and closes.
	if s.c.Launch.Current().Phase == launch.Running {
		if home {
			// Window calls stay off the SDL thread, which Stop waits for.
			go s.c.shell.ToggleOverlay()
		} else if s.c.shell.OverlayOpen() {
			s.c.emit(EventOverlayAction, PadAction{Action: action, Repeat: repeat})
		}
		return
	}
	// Otherwise the PS / Xbox button brings Seaglass forward, from
	// wherever the user is.
	if home {
		go s.c.shell.OpenMain()
	}
	s.c.emit(EventPadAction, PadAction{Action: action, Repeat: repeat})
}

func (s *PadService) onState(st pad.State) {
	if st.Error != "" {
		logx.Printf("controller: %s", st.Error)
	}
	// Which controller is in use goes in the log, so a report about input
	// says what it was.
	desc := ""
	if st.Connected {
		conn := "USB"
		if st.Wireless {
			conn = "Bluetooth"
		}
		desc = st.Name + " (" + string(st.Kind) + ", " + conn + ")"
	}
	s.mu.Lock()
	changed := desc != s.lastPad
	s.lastPad = desc
	s.mu.Unlock()
	if changed {
		if desc == "" {
			logx.Printf("controller: none connected")
		} else {
			logx.Printf("controller: %s", desc)
		}
	}
	s.c.emit(EventPadState, st)
}

// TestInput turns the stream of raw buttons and axes ("pad:raw") on or
// off, for the controller test screen.
func (s *PadService) TestInput(on bool) {
	if s.mgr == nil {
		return
	}
	if !on {
		s.mgr.SetRawListener(nil)
		return
	}
	s.mgr.SetRawListener(func(r pad.Raw) { s.c.emit(EventPadRaw, r) })
}

// State returns the controller in use.
func (s *PadService) State() pad.State {
	if s.mgr == nil {
		return pad.State{Battery: -1}
	}
	return s.mgr.State()
}

// Rumble plays a short effect ("tick", "bump", "confirm", "error",
// "launch") when the user has haptics on.
func (s *PadService) Rumble(effect string) {
	if s.mgr == nil || !s.c.Settings.Get().Haptics {
		return
	}
	s.mu.Lock()
	if (effect == "tick" || effect == "bump") && time.Since(s.lastRumble) < 70*time.Millisecond {
		s.mu.Unlock()
		return
	}
	s.lastRumble = time.Now()
	s.mu.Unlock()
	s.mgr.Rumble(effect)
}

var reHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// SetLight tints the DualSense lightbar (#rrggbb) when the user has that on.
func (s *PadService) SetLight(hex string) {
	if s.mgr == nil || !s.c.Settings.Get().Lightbar || !reHex.MatchString(hex) {
		return
	}
	_ = s.mgr.SetLight(hex)
}
