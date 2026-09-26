// Package pad reads game controllers through SDL3 and turns them into the
// few actions the big picture interface understands (move, confirm, back,
// menu, …), with key repeat for held directions. It also drives rumble and
// the DualSense lightbar. SDL runs on a thread of its own and is only ever
// touched from there.
package pad

import (
	"encoding/binary"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// Actions sent to the interface.
const (
	Up      = "up"
	Down    = "down"
	Left    = "left"
	Right   = "right"
	Confirm = "confirm" // ✕ / A
	Back    = "back"    // ○ / B
	Action  = "action"  // □ / X
	Info    = "info"    // △ / Y
	Menu    = "menu"    // Options / Menu
	View    = "view"    // Create / View
	Home    = "home"    // PS / Xbox button
	LB      = "lb"
	RB      = "rb"
	LT      = "lt"
	RT      = "rt"
)

// Kind is a controller family, for the button glyphs.
type Kind string

const (
	PlayStation Kind = "playstation"
	Xbox        Kind = "xbox"
	Nintendo    Kind = "nintendo"
	Other       Kind = "other"
)

// State describes the controller in use.
type State struct {
	Connected bool   `json:"connected"`
	Name      string `json:"name"`
	Kind      Kind   `json:"kind"`
	DualSense bool   `json:"dualSense"`
	Battery   int    `json:"battery"` // percent, -1 unknown
	Wireless  bool   `json:"wireless"`
	Error     string `json:"error,omitempty"`
}

// SDL3 constants used here.
const (
	initGamepad = 0x00002000

	evQuit           = 0x100
	evJoystickHat    = 0x602
	evGamepadAxis    = 0x650
	evGamepadDown    = 0x651
	evGamepadUp      = 0x652
	evGamepadAdded   = 0x653
	evGamepadRemoved = 0x654

	typePS3 = 4
	typePS4 = 5
	typePS5 = 6
	typeSw1 = 7

	connWireless = 2
)

// SDL gamepad buttons → actions. A DualSense's touchpad click does what
// Create does: the touchpad is the big button next to it, and the one
// people press when a prompt shows a rectangle.
var buttons = map[uint8]string{
	0: Confirm, 1: Back, 2: Action, 3: Info, 4: View, 5: Home, 6: Menu,
	9: LB, 10: RB, 11: Up, 12: Down, 13: Left, 14: Right, 20: View,
}

// isDir reports whether an action is a direction (which repeats when held).
func isDir(a string) bool { return a == Up || a == Down || a == Left || a == Right }

// dirIndex numbers the directions 0–3.
func dirIndex(a string) int {
	switch a {
	case Down:
		return 1
	case Left:
		return 2
	case Right:
		return 3
	}
	return 0
}

// pulse is one step of a rumble effect: motor strengths (low is the big,
// slow motor; high the small, quick one), how long, and a pause after.
type pulse struct {
	lo, hi  uint16
	ms, gap uint32
}

// Rumble effects. They are short but firm enough to feel on a DualSense,
// whose motors are emulated by its haptic actuators and need a moment to
// spin up: very short, weak pulses get lost, so ticks came and went.
var effects = map[string][]pulse{
	"tick":    {{lo: 0x0800, hi: 0x5800, ms: 32}},                                            // moving
	"bump":    {{lo: 0x5000, hi: 0x1000, ms: 42}},                                            // can't go further
	"confirm": {{lo: 0x3800, hi: 0x7800, ms: 60}},                                            // choosing
	"error":   {{lo: 0x9000, hi: 0x3000, ms: 85, gap: 70}, {lo: 0x9000, hi: 0x3000, ms: 85}}, // not possible
	"launch":  {{lo: 0x2000, hi: 0x6000, ms: 50, gap: 90}, {lo: 0x6000, hi: 0x8000, ms: 140}},
}

const (
	repeatDelay = 380 * time.Millisecond
	repeatEvery = 115 * time.Millisecond
	stickOn     = 16000 // of 32767: a stick counts as pressed past this
	stickOff    = 11000 // and released below this
	triggerOn   = 16000

	pollActive  = 8 * time.Millisecond   // a controller is in use
	pollIdle    = 16 * time.Millisecond  // connected, untouched for a few seconds
	pollPassive = 33 * time.Millisecond  // a game runs: only the PS button matters
	pollNoPad   = 250 * time.Millisecond // nothing connected: only hotplug
)

// Manager owns the SDL thread.
type Manager struct {
	onAction func(action string, repeat bool)
	onState  func(State)

	cmds   chan func(*sdl)
	modeCh chan Mode
	quit   chan struct{}
	done   chan struct{}

	mu    sync.Mutex
	state State
	mode  Mode

	// The virtual controller (dev flag --virtual-pad), made again whenever
	// SDL starts over.
	virtual *virtualPad

	// The rumble effect playing, only touched on the SDL thread.
	pulses  []pulse
	pulseAt time.Time
}

// Start loads SDL and begins reading controllers. onAction gets every
// action (repeat=true for auto-repeat of a held direction); onState gets
// controller changes. Both are called from the SDL thread.
func Start(onAction func(string, bool), onState func(State)) *Manager {
	m := &Manager{onAction: onAction, onState: onState, cmds: make(chan func(*sdl), 16),
		modeCh: make(chan Mode, 1), quit: make(chan struct{}), done: make(chan struct{})}
	m.state.Battery = -1
	go m.loop()
	return m
}

// State returns the current controller state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Stop shuts SDL down.
func (m *Manager) Stop() {
	select {
	case <-m.quit:
	default:
		close(m.quit)
	}
	<-m.done
}

// Mode is how much of the controller WaterLauncher uses.
type Mode int

const (
	// Active is the full layer: input, rumble, lightbar.
	Active Mode = iota
	// Passive only listens, for while a game runs. Controllers are read
	// without SDL's HIDAPI drivers, so WaterLauncher never writes to one
	// or switches a DualSense or DualShock 4 into its enhanced report
	// mode; the game gets the controller exactly as it expects. Actions
	// keep coming (the PS button opens the overlay).
	Passive
	// Off releases controllers completely.
	Off
)

// SetMode switches the controller layer's mode.
func (m *Manager) SetMode(mode Mode) {
	m.mu.Lock()
	if m.mode == mode {
		m.mu.Unlock()
		return
	}
	m.mode = mode
	m.mu.Unlock()
	select {
	case <-m.modeCh: // a switch nobody has acted on yet is replaced
	default:
	}
	select {
	case m.modeCh <- mode:
	case <-m.quit:
	}
}

// Mode returns the controller layer's mode.
func (m *Manager) Mode() Mode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}

// start sets SDL's hints and initialises its gamepad layer.
func (m *Manager) start(s *sdl, passive bool) error {
	hidapi, reports := "1", "auto"
	if passive {
		hidapi, reports = "0", "0"
	}
	// WaterLauncher has no SDL window, so SDL must deliver input no matter
	// which window has focus.
	for _, h := range [][2]string{
		{"SDL_JOYSTICK_ALLOW_BACKGROUND_EVENTS", "1"},
		{"SDL_JOYSTICK_HIDAPI_PS5_PLAYER_LED", "0"},
		{"SDL_JOYSTICK_HIDAPI", hidapi},
		{"SDL_JOYSTICK_ENHANCED_REPORTS", reports},
	} {
		s.setHint.Call(uintptr(unsafe.Pointer(cstr(h[0]))), uintptr(unsafe.Pointer(cstr(h[1]))))
	}
	if r, _, _ := s.init.Call(initGamepad); !ok(r) {
		return errors.New(s.errorText())
	}
	return nil
}

// Rumble plays a short effect: "tick" (moving), "bump" (at an edge),
// "confirm", "error" or "launch". A new effect replaces one still playing.
func (m *Manager) Rumble(effect string) {
	p, ok := effects[effect]
	if !ok {
		return
	}
	m.do(func(s *sdl) {
		m.pulses, m.pulseAt = p, time.Time{}
		m.playPulses(s, time.Now())
	})
}

// playPulses starts the effect's next pulse when it is due. SDL stops a
// pulse by itself when its time is up (while the loop keeps polling).
func (m *Manager) playPulses(s *sdl, now time.Time) {
	if len(m.pulses) == 0 || now.Before(m.pulseAt) {
		return
	}
	p := m.pulses[0]
	m.pulses = m.pulses[1:]
	m.pulseAt = now.Add(time.Duration(p.ms+p.gap) * time.Millisecond)
	if gp := m.current(); gp != 0 {
		s.rumbleGamepad.Call(gp, uintptr(p.lo), uintptr(p.hi), uintptr(p.ms))
	}
}

// SetLight sets the DualSense lightbar to a #rrggbb colour.
func (m *Manager) SetLight(hex string) error {
	if len(hex) != 7 || hex[0] != '#' {
		return errors.New("colour must be #rrggbb")
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return err
	}
	r, g, b := uintptr(v>>16&0xff), uintptr(v>>8&0xff), uintptr(v&0xff)
	m.do(func(s *sdl) {
		if gp := m.current(); gp != 0 {
			s.setGamepadLED.Call(gp, r, g, b)
		}
	})
	return nil
}

func (m *Manager) do(fn func(*sdl)) {
	if m.Mode() != Active {
		return // no output reports while a game has the controller
	}
	select {
	case m.cmds <- fn:
	default: // the SDL thread is busy; a dropped effect doesn't matter
	}
}

// ---- the SDL thread ----

type gamepad struct {
	ptr  uintptr
	name string
	kind Kind
	ds   bool
}

var (
	padsMu  sync.Mutex
	pads    = map[uint32]*gamepad{}
	current uint32 // the controller last used
)

func (m *Manager) current() uintptr {
	padsMu.Lock()
	defer padsMu.Unlock()
	if gp := pads[current]; gp != nil {
		return gp.ptr
	}
	return 0
}

func (m *Manager) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(m.done)

	s, err := loadSDL()
	if err != nil {
		m.setState(func(st *State) { st.Error = "controller support unavailable: " + err.Error() })
		<-m.quit
		return
	}
	if err := m.start(s, false); err != nil {
		m.setState(func(st *State) { st.Error = "controller support unavailable: " + err.Error() })
		<-m.quit
		return
	}
	off, passive := false, false
	defer func() {
		if !off {
			s.quit.Call()
		}
	}()

	ev := make([]byte, 128)
	// What is held down (a D-pad button, or the left stick) and when its
	// direction repeats next. They're kept apart so that the stick resting
	// in the middle doesn't let go of a D-pad direction, and the other way
	// round.
	type hold struct {
		action string
		next   time.Time
	}
	held := map[string]*hold{}
	axes := map[uint8]int16{}
	// A controller whose mapping has no D-pad buttons still reports its
	// D-pad as a hat; that's used instead, for controllers that never send
	// D-pad buttons (the others send both).
	dpadButtons := map[uint32]bool{}
	hats := map[uint32]uint8{}
	battery := time.NewTicker(30 * time.Second)
	defer battery.Stop()
	// SDL is polled; how often depends on what could happen. Without a
	// controller only hotplug matters, and an idle launcher shouldn't wake
	// the CPU 125 times a second.
	var lastInput time.Time
	every := pollIdle
	interval := func() time.Duration {
		padsMu.Lock()
		n := len(pads)
		padsMu.Unlock()
		switch {
		case off:
			return time.Hour // nothing to read
		case n == 0:
			return pollNoPad
		case passive:
			return pollPassive
		case time.Since(lastInput) < 5*time.Second || len(held) > 0 || len(m.pulses) > 0:
			return pollActive
		}
		return pollIdle
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	retune := func() {
		if next := interval(); next != every {
			every = next
			tick.Reset(every)
		}
	}

	press := func(key, a string) {
		if a == "" {
			return
		}
		m.onAction(a, false)
		if isDir(a) {
			held[key] = &hold{action: a, next: time.Now().Add(repeatDelay)}
		}
	}
	release := func(key string) { delete(held, key) }
	// The left stick points one way at a time: the axis pushed furthest, so
	// a stick pushed a little off straight doesn't move diagonally in two
	// steps. It lets go below stickOff (hysteresis, so a wobbly stick
	// doesn't stutter), and turns when the other axis clearly takes over.
	stick := func() {
		x, y := int(axes[0]), int(axes[1])
		along := func(a string) int {
			switch a {
			case Left:
				return -x
			case Right:
				return x
			case Up:
				return -y
			case Down:
				return y
			}
			return 0
		}
		cur := ""
		if h := held["stick"]; h != nil {
			cur = h.action
		}
		want := cur
		if want != "" && along(want) < stickOff {
			want = ""
		}
		ax, ay := max(x, -x), max(y, -y)
		if ax > stickOn || ay > stickOn {
			strongest := Right
			switch {
			case ax >= ay && x < 0:
				strongest = Left
			case ay > ax && y < 0:
				strongest = Up
			case ay > ax:
				strongest = Down
			}
			if want == "" || strongest != want && along(strongest) > along(want)+6000 {
				want = strongest
			}
		}
		if want != cur {
			release("stick")
			press("stick", want)
		}
	}

	for {
		select {
		case <-m.quit:
			m.closeAll(s)
			return
		case fn := <-m.cmds:
			fn(s)
			retune()
			continue
		case mode := <-m.modeCh:
			if !off {
				m.closeAll(s)
				s.quit.Call()
			}
			clear(held)
			clear(axes)
			clear(dpadButtons)
			clear(hats)
			m.pulses = nil
			off, passive = mode == Off, mode == Passive
			if !off {
				if err := m.start(s, mode == Passive); err != nil {
					m.setState(func(st *State) { st.Error = "controller support unavailable: " + err.Error() })
				} else if err := m.attachVirtual(s); err != nil {
					m.setState(func(st *State) { st.Error = err.Error() })
				}
			}
			m.refreshState(s)
			retune()
			continue
		case <-battery.C:
			m.refreshState(s)
			continue
		case <-tick.C:
		}
		if off {
			continue
		}
		stickMoved := false
		var hatNow map[uint32]uint8 // hat positions reported in this batch
		for {
			r, _, _ := s.pollEvent.Call(uintptr(unsafe.Pointer(&ev[0])))
			if !ok(r) {
				break
			}
			typ := binary.LittleEndian.Uint32(ev[0:])
			which := binary.LittleEndian.Uint32(ev[16:])
			if typ == evGamepadDown || typ == evGamepadAxis {
				lastInput = time.Now()
			}
			switch typ {
			case evGamepadAdded:
				m.open(s, which)
			case evGamepadRemoved:
				m.close(s, which)
				delete(dpadButtons, which)
				delete(hats, which)
			case evGamepadDown:
				m.use(s, which)
				if b := ev[20]; b >= 11 && b <= 14 {
					dpadButtons[which] = true
				}
				press("button"+strconv.Itoa(int(ev[20])), buttons[ev[20]])
			case evGamepadUp:
				release("button" + strconv.Itoa(int(ev[20])))
			case evJoystickHat:
				if ev[20] == 0 {
					if hatNow == nil {
						hatNow = map[uint32]uint8{}
					}
					hatNow[which] = ev[21]
				}
			case evGamepadAxis:
				m.use(s, which)
				axis := ev[20]
				v := int16(binary.LittleEndian.Uint16(ev[24:]))
				prev := axes[axis]
				axes[axis] = v
				switch axis {
				case 0, 1: // left stick: both axes are read before deciding
					stickMoved = true
				case 4, 5: // triggers
					a := LT
					if axis == 5 {
						a = RT
					}
					if v > triggerOn && prev <= triggerOn {
						m.onAction(a, false)
					}
				}
			case evQuit:
			}
		}
		if stickMoved {
			stick()
		}
		for which, v := range hatNow {
			if dpadButtons[which] {
				continue // its D-pad buttons came too
			}
			prev := hats[which]
			hats[which] = v
			for _, d := range [...]struct {
				bit uint8
				dir string
			}{{1, Up}, {2, Right}, {4, Down}, {8, Left}} {
				switch on, was := v&d.bit != 0, prev&d.bit != 0; {
				case on && !was:
					m.use(s, which)
					press("hat"+d.dir, d.dir)
				case !on && was:
					release("hat" + d.dir)
				}
			}
		}
		now := time.Now()
		var repeated [4]bool // the stick and D-pad held the same way repeat once
		for _, h := range held {
			if now.After(h.next) {
				if d := dirIndex(h.action); !repeated[d] {
					m.onAction(h.action, true)
					repeated[d] = true
				}
				h.next = now.Add(repeatEvery)
			}
		}
		m.playPulses(s, now)
		retune()
	}
}

func (m *Manager) open(s *sdl, id uint32) {
	r, _, _ := s.openGamepad.Call(uintptr(id))
	if r == 0 {
		return
	}
	name, _, _ := s.gamepadName.Call(r)
	t, _, _ := s.gamepadType.Call(r)
	gp := &gamepad{ptr: r, name: gostr(name)}
	switch int32(t) {
	case typePS3, typePS4, typePS5:
		gp.kind = PlayStation
	case typeSw1, 8, 9, 10:
		gp.kind = Nintendo
	case 2, 3:
		gp.kind = Xbox
	default:
		if strings.Contains(strings.ToLower(gp.name), "xbox") {
			gp.kind = Xbox
		} else {
			gp.kind = Other
		}
	}
	gp.ds = int32(t) == typePS5
	padsMu.Lock()
	pads[id] = gp
	current = id
	padsMu.Unlock()
	m.refreshState(s)
}

func (m *Manager) close(s *sdl, id uint32) {
	padsMu.Lock()
	gp := pads[id]
	delete(pads, id)
	if current == id {
		current = 0
		for other := range pads {
			current = other
			break
		}
	}
	padsMu.Unlock()
	if gp != nil {
		s.closeGamepad.Call(gp.ptr)
	}
	m.refreshState(s)
}

func (m *Manager) closeAll(s *sdl) {
	padsMu.Lock()
	defer padsMu.Unlock()
	for id, gp := range pads {
		s.closeGamepad.Call(gp.ptr)
		delete(pads, id)
	}
	current = 0
}

// use makes the controller that was just touched the current one.
func (m *Manager) use(s *sdl, id uint32) {
	padsMu.Lock()
	changed := current != id && pads[id] != nil
	if changed {
		current = id
	}
	padsMu.Unlock()
	if changed {
		m.refreshState(s)
	}
}

func (m *Manager) refreshState(s *sdl) {
	padsMu.Lock()
	gp := pads[current]
	padsMu.Unlock()
	m.setState(func(st *State) {
		*st = State{Battery: -1}
		if gp == nil {
			return
		}
		st.Connected, st.Name, st.Kind, st.DualSense = true, gp.name, gp.kind, gp.ds
		var pct int32 = -1
		s.gamepadPowerInfo.Call(gp.ptr, uintptr(unsafe.Pointer(&pct)))
		st.Battery = int(pct)
		c, _, _ := s.gamepadConnectionState.Call(gp.ptr)
		st.Wireless = int32(c) == connWireless
	})
}

func (m *Manager) setState(fn func(*State)) {
	m.mu.Lock()
	fn(&m.state)
	st := m.state
	m.mu.Unlock()
	if m.onState != nil {
		m.onState(st)
	}
}
