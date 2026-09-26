package pad

import (
	"testing"
	"time"
	"unsafe"
)

// sdlVirtualJoystickDesc mirrors SDL_VirtualJoystickDesc (SDL 3.4, x64).
type sdlVirtualJoystickDesc struct {
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

// A virtual gamepad made by SDL goes through the same path as a real one:
// it is detected, opened, and its buttons become actions, with repeat for
// a held direction.
func TestVirtualGamepad(t *testing.T) {
	if unsafe.Sizeof(sdlVirtualJoystickDesc{}) != 136 {
		t.Fatalf("descriptor size %d, want 136", unsafe.Sizeof(sdlVirtualJoystickDesc{}))
	}
	actions := make(chan string, 64)
	states := make(chan State, 16)
	m := Start(func(a string, repeat bool) {
		if repeat {
			a += "+"
		}
		actions <- a
	}, func(s State) { states <- s })
	defer m.Stop()

	var joy uintptr
	name := cstr("WaterLauncher Test Pad")
	run := func(fn func(s *sdl)) {
		done := make(chan struct{})
		deadline := time.After(3 * time.Second)
		for {
			select {
			case m.cmds <- func(s *sdl) { fn(s); close(done) }:
				select {
				case <-done:
					return
				case <-deadline:
					t.Fatal("SDL thread didn't run the command")
				}
			case <-deadline:
				t.Fatal("SDL thread not ready")
			}
		}
	}
	time.Sleep(300 * time.Millisecond) // SDL_Init
	run(func(s *sdl) {
		attach, err := s.dll.FindProc("SDL_AttachVirtualJoystick")
		if err != nil {
			t.Error(err)
			return
		}
		open, _ := s.dll.FindProc("SDL_OpenJoystick")
		d := sdlVirtualJoystickDesc{Type: 1, NAxes: 6, NButtons: 21, ButtonMask: 1<<21 - 1, AxisMask: 1<<6 - 1, Name: name}
		d.Version = uint32(unsafe.Sizeof(d))
		id, _, _ := attach.Call(uintptr(unsafe.Pointer(&d)))
		if uint32(id) == 0 {
			t.Errorf("attach failed: %s", s.errorText())
			return
		}
		joy, _, _ = open.Call(id)
	})
	if joy == 0 {
		t.Fatal("virtual joystick not opened")
	}
	attachPad := func() {
		joy = 0
		run(func(s *sdl) {
			attach, _ := s.dll.FindProc("SDL_AttachVirtualJoystick")
			open, _ := s.dll.FindProc("SDL_OpenJoystick")
			d := sdlVirtualJoystickDesc{Type: 1, NAxes: 6, NButtons: 21, ButtonMask: 1<<21 - 1, AxisMask: 1<<6 - 1, Name: name}
			d.Version = uint32(unsafe.Sizeof(d))
			id, _, _ := attach.Call(uintptr(unsafe.Pointer(&d)))
			joy, _, _ = open.Call(id)
		})
		if joy == 0 {
			t.Fatal("virtual joystick not opened")
		}
	}

	waitState := func(want func(State) bool) State {
		deadline := time.After(3 * time.Second)
		for {
			select {
			case st := <-states:
				if want(st) {
					return st
				}
			case <-deadline:
				t.Fatalf("state never matched; last %+v", m.State())
			}
		}
	}
	st := waitState(func(s State) bool { return s.Connected })
	if st.Name != "WaterLauncher Test Pad" {
		t.Errorf("name = %q", st.Name)
	}

	press := func(button int, down bool) {
		run(func(s *sdl) {
			set, _ := s.dll.FindProc("SDL_SetJoystickVirtualButton")
			b := uintptr(0)
			if down {
				b = 1
			}
			set.Call(joy, uintptr(button), b)
		})
	}
	expect := func(want string) {
		select {
		case a := <-actions:
			if a != want {
				t.Errorf("action %q, want %q", a, want)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("no action, want %q", want)
		}
	}

	press(0, true) // SOUTH: ✕ / A
	expect(Confirm)
	press(0, false)
	press(1, true) // EAST: ○ / B
	expect(Back)
	press(1, false)
	press(5, true) // GUIDE: PS
	expect(Home)
	press(5, false)

	// Holding D-pad down repeats after the delay.
	press(12, true)
	expect(Down)
	time.Sleep(repeatDelay + 3*repeatEvery)
	press(12, false)
	repeats := 0
	for len(actions) > 0 {
		if <-actions == Down+"+" {
			repeats++
		}
	}
	if repeats < 2 {
		t.Errorf("%d repeats, want at least 2", repeats)
	}

	// Moves the left stick; both axes change at once, as they do in a report.
	stick := func(x, y int16) {
		run(func(s *sdl) {
			set, _ := s.dll.FindProc("SDL_SetJoystickVirtualAxis")
			set.Call(joy, 0, uintptr(uint16(x)))
			set.Call(joy, 1, uintptr(uint16(y)))
		})
	}
	drain := func() []string {
		time.Sleep(60 * time.Millisecond)
		var got []string
		for len(actions) > 0 {
			got = append(got, <-actions)
		}
		return got
	}

	// A resting stick that wobbles doesn't let go of a held D-pad direction.
	press(11, true) // D-pad up
	expect(Up)
	for k := 0; k < 12; k++ {
		stick(int16(-200+k*40), int16(300+k*50))
		time.Sleep(repeatDelay / 8)
	}
	time.Sleep(3 * repeatEvery)
	press(11, false)
	repeats = 0
	for _, a := range drain() {
		if a == Up+"+" {
			repeats++
		}
	}
	if repeats < 2 {
		t.Errorf("stick wobble stopped the D-pad repeating: %d repeats", repeats)
	}

	// A stick pushed a little off straight moves one way, not two.
	stick(0, 0)
	drain()
	stick(24000, 17000)
	if got := drain(); len(got) != 1 || got[0] != Right {
		t.Errorf("diagonal stick gave %v, want [right]", got)
	}
	stick(0, 0)
	if got := drain(); len(got) != 0 {
		t.Errorf("releasing the stick gave %v", got)
	}
	// Turning it clearly the other way turns.
	stick(2000, -26000)
	if got := drain(); len(got) != 1 || got[0] != Up {
		t.Errorf("stick up gave %v, want [up]", got)
	}
	stick(0, 0)
	drain()

	// The touchpad click opens search, like Create.
	press(20, true)
	expect(View)
	press(20, false)

	// Unplugging it is noticed.
	run(func(s *sdl) {
		detach, _ := s.dll.FindProc("SDL_DetachVirtualJoystick")
		closeJ, _ := s.dll.FindProc("SDL_CloseJoystick")
		for id := range pads {
			detach.Call(uintptr(id))
		}
		closeJ.Call(joy)
	})
	waitState(func(s State) bool { return !s.Connected })

	// Passive mode (while a game runs): SDL starts over without HIDAPI,
	// input still arrives, and nothing is sent to the controller.
	m.SetMode(Passive)
	time.Sleep(300 * time.Millisecond)
	attachPad()
	waitState(func(s State) bool { return s.Connected })
	press(5, true)
	expect(Home)
	press(5, false)
	m.Rumble("confirm")
	if len(m.cmds) != 0 {
		t.Error("rumble queued in passive mode")
	}
	m.SetMode(Off)
	m.SetMode(Active)
	time.Sleep(300 * time.Millisecond)
	if m.Mode() != Active {
		t.Error("not active again")
	}
}

// A D-pad reported both as a hat and as buttons moves once, not twice.
func TestVirtualHatNotTwice(t *testing.T) {
	actions := make(chan string, 64)
	m := Start(func(a string, repeat bool) {
		if !repeat {
			actions <- a
		}
	}, func(State) {})
	defer m.Stop()
	run := func(fn func(s *sdl)) {
		done := make(chan struct{})
		select {
		case m.cmds <- func(s *sdl) { fn(s); close(done) }:
			<-done
		case <-time.After(3 * time.Second):
			t.Fatal("SDL thread not ready")
		}
	}
	time.Sleep(300 * time.Millisecond)
	var joy uintptr
	name := cstr("WaterLauncher Hat Pad")
	run(func(s *sdl) {
		attach, _ := s.dll.FindProc("SDL_AttachVirtualJoystick")
		open, _ := s.dll.FindProc("SDL_OpenJoystick")
		d := sdlVirtualJoystickDesc{Type: 1, NAxes: 6, NButtons: 11, NHats: 1, ButtonMask: 1<<11 - 1, AxisMask: 1<<6 - 1, Name: name}
		d.Version = uint32(unsafe.Sizeof(d))
		id, _, _ := attach.Call(uintptr(unsafe.Pointer(&d)))
		joy, _, _ = open.Call(id)
	})
	if joy == 0 {
		t.Fatal("virtual joystick not opened")
	}
	time.Sleep(200 * time.Millisecond)
	hat := func(v uint8) {
		run(func(s *sdl) {
			set, _ := s.dll.FindProc("SDL_SetJoystickVirtualHat")
			set.Call(joy, 0, uintptr(v))
		})
	}
	hat(1) // up
	time.Sleep(100 * time.Millisecond)
	hat(0)
	time.Sleep(100 * time.Millisecond)
	var got []string
	for len(actions) > 0 {
		got = append(got, <-actions)
	}
	if len(got) != 1 || got[0] != Up {
		t.Errorf("hat up gave %v, want [up]", got)
	}
}

// The dev flag's virtual controller: plugged in through the exported API,
// it looks like the controller asked for and survives a mode change.
func TestPlugVirtual(t *testing.T) {
	actions := make(chan string, 16)
	m := Start(func(a string, repeat bool) {
		if !repeat {
			actions <- a
		}
	}, func(State) {})
	defer m.Stop()
	time.Sleep(300 * time.Millisecond)
	if err := m.PlugVirtual(Xbox); err != nil {
		t.Fatal(err)
	}
	waitFor := func(what string, ok func(State) bool) {
		for k := 0; k < 60 && !ok(m.State()); k++ {
			time.Sleep(50 * time.Millisecond)
		}
		if !ok(m.State()) {
			t.Fatalf("%s: state %+v", what, m.State())
		}
	}
	waitFor("xbox pad", func(s State) bool { return s.Connected && s.Kind == Xbox })
	press := func(b int) {
		if err := m.VirtualButton(b, true); err != nil {
			t.Fatal(err)
		}
		time.Sleep(60 * time.Millisecond)
		_ = m.VirtualButton(b, false)
	}
	expect := func(want string) {
		select {
		case a := <-actions:
			if a != want {
				t.Errorf("action %q, want %q", a, want)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("no action, want %q", want)
		}
	}
	press(VirtualButtons["south"])
	expect(Confirm)

	// The test screen's stream: buttons as bits, axes as the gamepad
	// reports them (a virtual joystick's trigger axis runs from -32768, at
	// rest, to 32767; the gamepad's from 0).
	_ = m.VirtualAxis(VirtualAxes["rt"], -32768)
	raws := make(chan Raw, 64)
	m.SetRawListener(func(r Raw) { raws <- r })
	_ = m.VirtualButton(VirtualButtons["north"], true)
	_ = m.VirtualAxis(VirtualAxes["rt"], 20000)
	var got Raw
	for deadline := time.After(2 * time.Second); got.Buttons&(1<<3) == 0 || got.Axes[5] != 26383; {
		select {
		case got = <-raws:
		case <-deadline:
			t.Fatalf("raw state never showed north and the trigger: %+v", got)
		}
	}
	_ = m.VirtualButton(VirtualButtons["north"], false)
	_ = m.VirtualAxis(VirtualAxes["rt"], -32768)
	m.SetRawListener(nil)
	// Both also came as actions (in either order: one poll saw both).
	both := map[string]bool{}
	for k := 0; k < 2; k++ {
		select {
		case a := <-actions:
			both[a] = true
		case <-time.After(2 * time.Second):
		}
	}
	if !both[Info] || !both[RT] {
		t.Errorf("actions %v, want info and rt", both)
	}

	m.SetMode(Passive)
	time.Sleep(400 * time.Millisecond)
	waitFor("still plugged in after the mode change", func(s State) bool { return s.Connected })
	press(VirtualButtons["guide"])
	expect(Home)
	if err := m.UnplugVirtual(); err != nil {
		t.Fatal(err)
	}
	waitFor("unplugged", func(s State) bool { return !s.Connected })
}
