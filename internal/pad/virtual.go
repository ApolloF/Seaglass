package pad

import (
	"errors"
	"time"
	"unsafe"
)

// A virtual controller, for testing the real app without a real one
// (WaterLauncher's --virtual-pad dev flag). SDL makes it and reads it
// like any other gamepad, so everything after SDL is the real path.

// virtualDesc mirrors SDL_VirtualJoystickDesc (SDL 3.4, x64).
type virtualDesc struct {
	Version           uint32
	Type              uint16
	_                 uint16
	Vendor, Product   uint16
	NAxes, NButtons   uint16
	NBalls, NHats     uint16
	NTouch, NSensors  uint16
	_                 [2]uint16
	ButtonMask        uint32
	AxisMask          uint32
	_                 uint32
	Name              *byte
	Touchpads         uintptr
	Sensors           uintptr
	Userdata          uintptr
	Update            uintptr
	SetPlayerIndex    uintptr
	Rumble            uintptr
	RumbleTriggers    uintptr
	SetLED            uintptr
	SendEffect        uintptr
	SetSensorsEnabled uintptr
	Cleanup           uintptr
}

// VirtualButtons names the SDL gamepad buttons a virtual controller has.
var VirtualButtons = map[string]int{
	"south": 0, "east": 1, "west": 2, "north": 3, "back": 4, "guide": 5, "start": 6,
	"leftstick": 7, "rightstick": 8, "lb": 9, "rb": 10,
	"up": 11, "down": 12, "left": 13, "right": 14, "misc": 15, "touchpad": 20,
}

// VirtualAxes names its axes (values -32768 to 32767; triggers 0 to 32767).
var VirtualAxes = map[string]int{"lx": 0, "ly": 1, "rx": 2, "ry": 3, "lt": 4, "rt": 5}

type virtualPad struct {
	kind Kind
	id   uint32
	joy  uintptr
}

// PlugVirtual plugs in a virtual controller that looks like a DualSense
// (kind PlayStation) or an Xbox controller. It stays plugged in when the
// mode changes (SDL starts over then, and it is made again).
func (m *Manager) PlugVirtual(kind Kind) error {
	m.mu.Lock()
	m.virtual = &virtualPad{kind: kind}
	m.mu.Unlock()
	return m.sdlDo(func(s *sdl) error { return m.attachVirtual(s) })
}

// UnplugVirtual removes the virtual controller.
func (m *Manager) UnplugVirtual() error {
	m.mu.Lock()
	v := m.virtual
	m.virtual = nil
	m.mu.Unlock()
	if v == nil {
		return nil
	}
	return m.sdlDo(func(s *sdl) error {
		if v.joy != 0 {
			if p, err := s.dll.FindProc("SDL_CloseJoystick"); err == nil {
				p.Call(v.joy)
			}
		}
		if v.id != 0 {
			if p, err := s.dll.FindProc("SDL_DetachVirtualJoystick"); err == nil {
				p.Call(uintptr(v.id))
			}
		}
		return nil
	})
}

// VirtualButton presses (down) or releases a virtual controller button.
func (m *Manager) VirtualButton(button int, down bool) error {
	b := uintptr(0)
	if down {
		b = 1
	}
	return m.virtualCall("SDL_SetJoystickVirtualButton", uintptr(button), b)
}

// VirtualAxis moves a virtual controller axis.
func (m *Manager) VirtualAxis(axis int, value int16) error {
	return m.virtualCall("SDL_SetJoystickVirtualAxis", uintptr(axis), uintptr(uint16(value)))
}

func (m *Manager) virtualCall(fn string, a, b uintptr) error {
	return m.sdlDo(func(s *sdl) error {
		m.mu.Lock()
		v := m.virtual
		m.mu.Unlock()
		if v == nil || v.joy == 0 {
			return errors.New("no virtual controller plugged in")
		}
		p, err := s.dll.FindProc(fn)
		if err != nil {
			return err
		}
		if r, _, _ := p.Call(v.joy, a, b); !ok(r) {
			return errors.New(s.errorText())
		}
		return nil
	})
}

// attachVirtual makes the virtual controller on the SDL thread.
func (m *Manager) attachVirtual(s *sdl) error {
	m.mu.Lock()
	v := m.virtual
	m.mu.Unlock()
	if v == nil {
		return nil
	}
	v.id, v.joy = 0, 0
	attach, err := s.dll.FindProc("SDL_AttachVirtualJoystick")
	if err != nil {
		return err
	}
	open, err := s.dll.FindProc("SDL_OpenJoystick")
	if err != nil {
		return err
	}
	d := virtualDesc{Type: 1, NAxes: 6, NButtons: 21, ButtonMask: 1<<21 - 1, AxisMask: 1<<6 - 1}
	switch v.kind {
	case Xbox:
		d.Vendor, d.Product, d.Name = 0x045e, 0x0b12, cstr("Xbox Series X Controller (virtual)")
	default:
		d.Vendor, d.Product, d.Name = 0x054c, 0x0ce6, cstr("DualSense Wireless Controller (virtual)")
	}
	d.Version = uint32(unsafe.Sizeof(d))
	id, _, _ := attach.Call(uintptr(unsafe.Pointer(&d)))
	if uint32(id) == 0 {
		return errors.New("attach virtual controller: " + s.errorText())
	}
	joy, _, _ := open.Call(id)
	if joy == 0 {
		return errors.New("open virtual controller: " + s.errorText())
	}
	v.id, v.joy = uint32(id), joy
	return nil
}

// sdlDo runs fn on the SDL thread and waits for it, in any mode.
func (m *Manager) sdlDo(fn func(*sdl) error) error {
	res := make(chan error, 1)
	select {
	case m.cmds <- func(s *sdl) { res <- fn(s) }:
	case <-m.quit:
		return errors.New("controller layer stopped")
	case <-time.After(3 * time.Second):
		return errors.New("controller layer busy")
	}
	select {
	case err := <-res:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("controller layer didn't answer")
	}
}
