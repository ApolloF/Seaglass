package pad

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// SDL loads from the embedded, hash-checked DLL and initialises.
func TestSDLLoads(t *testing.T) {
	s, err := loadSDL()
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _ := s.init.Call(initGamepad); !ok(r) {
		t.Fatalf("SDL_Init: %s", s.errorText())
	}
	s.quit.Call()
}

func TestStartStop(t *testing.T) {
	states := make(chan State, 8)
	m := Start(func(string, bool) {}, func(st State) { states <- st })
	time.Sleep(300 * time.Millisecond)
	m.Stop()
	if st := m.State(); st.Error != "" {
		t.Errorf("state error: %s", st.Error)
	}
}

func TestSetLightValidates(t *testing.T) {
	m := &Manager{cmds: make(chan func(*sdl), 1)}
	for _, bad := range []string{"", "red", "#12345", "#gggggg", "123456#"} {
		if m.SetLight(bad) == nil {
			t.Errorf("SetLight(%q) accepted", bad)
		}
	}
	if err := m.SetLight("#3ea8eb"); err != nil {
		t.Error(err)
	}
}

// Switches from several goroutines at once (a game ending while the next
// one starts) leave SDL in the mode asked for last, never an older one.
func TestQuickModeSwitchesEndInTheLastMode(t *testing.T) {
	m := Start(func(string, bool) {}, func(State) {})
	defer m.Stop()
	for _, last := range []Mode{Passive, Active, Off, Active} {
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.SetMode(Mode(i % 3))
			}()
		}
		wg.Wait()
		m.SetMode(last)
		deadline := time.Now().Add(5 * time.Second)
		for {
			in, running := m.InMode()
			if in == last && running == (last != Off) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("asked for %s last, SDL is in %s (running %v)", last, in, running)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// A start or controller lookup counts as slow only once it takes seconds,
// so normal starts (well under a second) never warn.
func TestSlowNoteOnlyForSlowCalls(t *testing.T) {
	if n := slowNote("to start", 900*time.Millisecond); n != "" {
		t.Errorf("a normal start is noted: %q", n)
	}
	n := slowNote("to start", 25300*time.Millisecond)
	if n == "" || !strings.Contains(n, "25.3s to start") || !strings.Contains(n, "reconnecting") {
		t.Errorf("a 25 s start: %q", n)
	}
}

// Bridge mode runs SDL and hands the controller's state to the sink at
// least once (at rest when no controller is connected), so the virtual
// controller never keeps a stale press.
func TestBridgeModeFeedsTheSink(t *testing.T) {
	m := Start(func(string, bool) {}, func(State) {})
	defer m.Stop()
	got := make(chan Raw, 64)
	m.SetBridge(func(r Raw) {
		select {
		case got <- r:
		default:
		}
	}, func([]string) {})
	m.SetMode(Bridge)
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge sink never heard from the controller layer")
	}
	if in, running := m.InMode(); in != Bridge || !running {
		t.Errorf("in %s, running %v", in, running)
	}
	m.BridgeRumble(200, 100) // no controller: nothing to do, but it mustn't block
	m.SetMode(Active)
	m.BridgeRumble(200, 100) // ignored outside Bridge mode
}

func TestVirtualX360IsNotBridged(t *testing.T) {
	if !isVirtualX360(0x045E, 0x028E) {
		t.Error("the Xbox 360 ids ViGEmBus reports aren't recognised")
	}
	for _, ids := range [][2]uint16{{0x054C, 0x0CE6}, {0x045E, 0x0B13}, {0x057E, 0x2009}} {
		if isVirtualX360(ids[0], ids[1]) {
			t.Errorf("%04x:%04x counted as the virtual controller", ids[0], ids[1])
		}
	}
	if Bridge.String() != "bridge" {
		t.Errorf("Bridge.String() = %q", Bridge.String())
	}
}

// Active and Bridge read controllers alike, so switching between them
// keeps SDL running; every other switch starts it over.
func TestSameHints(t *testing.T) {
	for _, c := range []struct {
		a, b Mode
		want bool
	}{
		{Active, Bridge, true}, {Bridge, Active, true}, {Active, Passive, false},
		{Passive, Bridge, false}, {Bridge, Off, false}, {Off, Active, false},
	} {
		if got := sameHints(c.a, c.b); got != c.want {
			t.Errorf("sameHints(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
