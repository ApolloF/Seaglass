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
