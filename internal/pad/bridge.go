package pad

import (
	"sort"
	"time"
)

// SetBridge sets where Bridge mode hands the controller: sink gets the
// state of the controller in use whenever it changes (all zero, at rest,
// while none is connected), pads the device paths of the controllers SDL
// has open whenever they change (so they can be hidden from the game).
// Both are called on the SDL thread and must return quickly; nil stops them.
func (m *Manager) SetBridge(sink func(Raw), pads func(paths []string)) {
	m.mu.Lock()
	m.bridgeSink, m.bridgePads = sink, pads
	m.mu.Unlock()
}

func (m *Manager) bridgeCallbacks() (func(Raw), func([]string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bridgeSink, m.bridgePads
}

// BridgeRumble passes the game's rumble (0–255 for the large and the small
// motor, as an Xbox controller gets it) on to the controller in use. It
// lasts until the next call; only in Bridge mode.
func (m *Manager) BridgeRumble(large, small uint8) {
	if m.Mode() != Bridge {
		return
	}
	lo, hi := uint16(large)*257, uint16(small)*257
	select {
	case m.cmds <- func(s *sdl) {
		if !m.bridging {
			return
		}
		m.rumbleLo, m.rumbleHi = lo, hi
		m.rumbleAt = time.Time{}
		m.playBridgeRumble(s, time.Now())
	}:
	default: // the SDL thread is busy; the game sends the next change soon
	}
}

// rumbleFor is how long one SDL rumble call lasts; a game's rumble has no
// end of its own, so it is renewed before then while it's on.
const (
	rumbleFor   = 10 * time.Second
	rumbleRenew = 8 * time.Second
)

func (m *Manager) playBridgeRumble(s *sdl, now time.Time) {
	if !m.rumbleAt.IsZero() && (now.Sub(m.rumbleAt) < rumbleRenew || m.rumbleLo == 0 && m.rumbleHi == 0) {
		return
	}
	m.rumbleAt = now
	if gp := m.current(); gp != 0 {
		s.rumbleGamepad.Call(gp, uintptr(m.rumbleLo), uintptr(m.rumbleHi), uintptr(rumbleFor/time.Millisecond))
	}
}

// feedBridge reads the controller in use and hands its state to the sink
// when it changed, and the open controllers' paths when they changed.
// Runs on the SDL thread after every poll in Bridge mode.
func (m *Manager) feedBridge(s *sdl) {
	sink, padsFn := m.bridgeCallbacks()
	if m.padsDirty {
		m.padsDirty = false
		if padsFn != nil {
			padsFn(m.openPaths(s))
		}
	}
	var r Raw
	if gp := m.current(); gp != 0 {
		r = readGamepad(s, gp)
	}
	if sink != nil && (!m.bridgeSent || r != m.bridgeLast) {
		sink(r)
		m.bridgeLast, m.bridgeSent = r, true
	}
	m.playBridgeRumble(s, time.Now())
}

// readGamepad reads every button and axis of a gamepad.
func readGamepad(s *sdl, gp uintptr) Raw {
	var r Raw
	for b := range 21 { // up to the touchpad click (20)
		if v, _, _ := s.gamepadButton.Call(gp, uintptr(b)); ok(v) {
			r.Buttons |= 1 << b
		}
	}
	for a := range r.Axes {
		v, _, _ := s.gamepadAxis.Call(gp, uintptr(a))
		r.Axes[a] = int16(uint16(v))
	}
	return r
}

// openPaths are the device paths of the open controllers, sorted.
func (m *Manager) openPaths(s *sdl) []string {
	padsMu.Lock()
	defer padsMu.Unlock()
	out := []string{}
	for _, gp := range pads {
		if p, _, _ := s.gamepadPath.Call(gp.ptr); p != 0 {
			out = append(out, gostr(p))
		}
	}
	sort.Strings(out)
	return out
}

// sameHints reports whether SDL runs with the same hints in both modes,
// so switching between them needs no restart.
func sameHints(a, b Mode) bool {
	return (a == Active || a == Bridge) && (b == Active || b == Bridge)
}

// switchBridge turns Bridge mode on or off while SDL keeps running.
func (m *Manager) switchBridge(s *sdl, on bool) {
	if !on && (m.rumbleLo != 0 || m.rumbleHi != 0) {
		if gp := m.current(); gp != 0 {
			s.rumbleGamepad.Call(gp, 0, 0, 0)
		}
	}
	m.bridging, m.bridgeSent, m.padsDirty = on, false, on
	m.rumbleLo, m.rumbleHi, m.rumbleAt = 0, 0, time.Time{}
	m.pulses = nil
	if on {
		// The virtual controller may have been opened before Bridge mode.
		padsMu.Lock()
		var virtual []uint32
		for id, gp := range pads {
			if gp.virtual {
				virtual = append(virtual, id)
			}
		}
		padsMu.Unlock()
		for _, id := range virtual {
			m.close(s, id)
		}
	}
}

// isVirtualX360 reports whether a controller has the Xbox 360 wired
// controller's ids, which ViGEmBus's virtual controller reports. In Bridge
// mode those are left alone: reading the virtual controller back would
// feed it its own input. A real Xbox 360 controller is skipped too, which
// costs nothing: games read it directly anyway.
func isVirtualX360(vendor, product uint16) bool { return vendor == 0x045E && product == 0x028E }
