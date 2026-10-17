package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/vpad"
)

type fakeBridge struct {
	mu                    sync.Mutex
	plugged, closed, sent int
	last                  vpad.Report
	plugErr               error
}

func (b *fakeBridge) Plug() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.plugErr != nil {
		return b.plugErr
	}
	b.plugged++
	return nil
}

func (b *fakeBridge) Send(r vpad.Report) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent++
	b.last = r
	return nil
}

func (b *fakeBridge) Rumble(func(large, small uint8)) {}

func (b *fakeBridge) closedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func (b *fakeBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	return nil
}

type fakeHider struct {
	mu      sync.Mutex
	active  bool
	hidden  []string
	cleared int
	closed  int
}

func (h *fakeHider) Active() (bool, error) { return h.active, nil }

func (h *fakeHider) closedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *fakeHider) Hide(ids []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hidden = append([]string{}, ids...)
	return nil
}

func (h *fakeHider) Clear() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hidden = nil
	h.cleared++
	return nil
}

func (h *fakeHider) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed++
	h.hidden = nil
	return nil
}

// ctlTestCore is a core whose drivers are fakes: ViGEmBus is there when
// viGEm, and HidHide answers through h (nil: not installed).
func ctlTestCore(t *testing.T, viGEm bool, b *fakeBridge, h *fakeHider) *Core {
	t.Helper()
	root := t.TempDir()
	lib, err := library.Open(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &Core{Lib: lib, Settings: settings.Open(filepath.Join(root, "settings.json")), ctx: ctx}
	c.ctl = &controllerState{c: c, deps: bridgeDeps{
		status: func() vpad.Status { return vpad.Status{ViGEm: viGEm, HidHide: h != nil} },
		newBridge: func() (vpad.Bridge, error) {
			if !viGEm {
				return nil, vpad.ErrNoViGEm
			}
			return b, nil
		},
		newHider: func() (hider, error) {
			if h == nil {
				return nil, vpad.ErrNoHidHide
			}
			return h, nil
		},
	}}
	return c
}

func padRaw(buttons uint32, axes [6]int16) pad.Raw { return pad.Raw{Buttons: buttons, Axes: axes} }

const dsPath = `\\?\HID#VID_054C&PID_0CE6&MI_03#7&1a2b3c4d&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`

func TestBridgePlugsHidesAndUndoes(t *testing.T) {
	b, h := &fakeBridge{}, &fakeHider{active: true}
	c := ctlTestCore(t, true, b, h)
	warn, err := c.ctl.begin(true)
	if err != nil || warn != "" {
		t.Fatalf("begin: warn %q, err %v", warn, err)
	}
	if b.plugged != 1 || !c.ctl.active() {
		t.Fatalf("plugged %d, active %v", b.plugged, c.ctl.active())
	}
	// SDL opened the DualSense in Bridge mode: it's hidden by instance id.
	c.ctl.hidePads([]string{dsPath, "XInput#1"})
	if want := []string{`HID\VID_054C&PID_0CE6&MI_03\7&1A2B3C4D&0&0000`}; !reflect.DeepEqual(h.hidden, want) {
		t.Errorf("hidden %q, want %q", h.hidden, want)
	}
	// The controller's state reaches the virtual one, Y turned around.
	c.ctl.send(padRaw(1<<0, [6]int16{0, 1000, 0, 0, 0, 32767}))
	if b.sent != 1 || b.last.Buttons != vpad.A || b.last.ThumbLY != -1000 || b.last.RightTrigger != 255 {
		t.Errorf("sent %d: %+v", b.sent, b.last)
	}
	c.ctl.end()
	if b.closed != 1 || h.closed != 1 || c.ctl.active() {
		t.Errorf("after end: bridge closed %d, hider closed %d, active %v", b.closed, h.closed, c.ctl.active())
	}
	c.ctl.end() // nothing left to undo
	if b.closed != 1 || h.closed != 1 {
		t.Errorf("a second end closed again: %d %d", b.closed, h.closed)
	}
	c.ctl.send(padRaw(1, [6]int16{})) // after the game: dropped
	if b.sent != 1 {
		t.Errorf("sent after the end: %d", b.sent)
	}
}

// Without HidHide, or with its hiding off, the game still gets the virtual
// controller and the launch warns about double input.
func TestBridgeWarnsWhenItCantHide(t *testing.T) {
	for name, h := range map[string]*fakeHider{"no HidHide": nil, "hiding off": {active: false}} {
		b := &fakeBridge{}
		c := ctlTestCore(t, true, b, h)
		warn, err := c.ctl.begin(true)
		if err != nil || warn != twiceWarning {
			t.Errorf("%s: warn %q, err %v", name, warn, err)
		}
		if b.plugged != 1 {
			t.Errorf("%s: not plugged", name)
		}
		if h != nil && h.closed != 1 {
			t.Errorf("%s: HidHide left open", name)
		}
		c.ctl.end()
	}
	// With hiding switched off in Settings, HidHide isn't touched.
	b, h := &fakeBridge{}, &fakeHider{active: true}
	c := ctlTestCore(t, true, b, h)
	if warn, err := c.ctl.begin(false); err != nil || warn != "" {
		t.Errorf("hide off: warn %q, err %v", warn, err)
	}
	c.ctl.hidePads([]string{dsPath})
	if len(h.hidden) != 0 || h.cleared != 0 {
		t.Errorf("hide off: HidHide was used: %+v", h)
	}
	c.ctl.end()
}

func TestBridgeFailsWithoutViGEm(t *testing.T) {
	c := ctlTestCore(t, false, nil, nil)
	if _, err := c.ctl.begin(true); !errors.Is(err, vpad.ErrNoViGEm) || c.ctl.active() {
		t.Errorf("err %v, active %v", err, c.ctl.active())
	}
	b := &fakeBridge{plugErr: errors.New("no free slot")}
	c = ctlTestCore(t, true, b, nil)
	if _, err := c.ctl.begin(true); err == nil || b.closed != 1 || c.ctl.active() {
		t.Errorf("plug failure: err %v, closed %d, active %v", err, b.closed, c.ctl.active())
	}
}

// The plan has a controller step for the bridge route and for a game that
// would need the bridge that isn't set up; none otherwise.
func TestPlanControllerStep(t *testing.T) {
	game := func(mode string) library.Game {
		return library.Game{ID: 1, Title: "G", Installed: true, Dir: `C:\G`, Exe: `C:\G\g.exe`, PadMode: mode}
	}
	for _, tc := range []struct {
		name  string
		viGEm bool
		mode  string
		want  bool
	}{
		{"xbox mode, ViGEmBus", true, "xbox", true},
		{"xbox mode, nothing set up", false, "xbox", true},
		{"native", true, "native", false},
		{"auto without a PlayStation pad", true, "", false},
	} {
		c := ctlTestCore(t, tc.viGEm, &fakeBridge{}, nil)
		p := c.plan(game(tc.mode))
		has := false
		for _, st := range p.Before {
			has = has || st.ID == "controller"
		}
		if has != tc.want {
			t.Errorf("%s: controller step %v, want %v", tc.name, has, tc.want)
		}
	}
}

// A launch through the bridge plugs the virtual controller in before the
// game starts and unplugs it when the session ends, also when the game
// failed to start.
func TestLaunchUndoesTheBridge(t *testing.T) {
	b, h := &fakeBridge{}, &fakeHider{active: true}
	c := ctlTestCore(t, true, b, h)
	c.Launch = launch.NewManager(c.onSession)
	t.Cleanup(c.Launch.Close)
	bridged := false
	pluggedAtStart := 0
	err := c.Launch.Launch(c.ctx, launch.Plan{GameID: 1, Title: "G",
		Before: []launch.Step{c.controllerStep(&bridged)},
		Start: func() (uint32, string, error) {
			pluggedAtStart = b.plugged
			return 0, RouteBridge, errors.New("the exe is gone")
		}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !c.Launch.Current().Phase.Done() {
		if time.Now().After(deadline) {
			t.Fatalf("session still %s", c.Launch.Current().Phase)
		}
		time.Sleep(10 * time.Millisecond)
	}
	s := c.Launch.Current()
	if !bridged || pluggedAtStart != 1 {
		t.Errorf("bridged %v, plugged before the start: %d", bridged, pluggedAtStart)
	}
	if len(s.Before) != 1 || s.Before[0].Detail != "Your controller works as an Xbox controller in this game." {
		t.Errorf("step: %+v", s.Before)
	}
	// The session reads as over a moment before onSession has run.
	for b.closedCount() != 1 || h.closedCount() != 1 || c.ctl.active() {
		if time.Now().After(deadline) {
			t.Fatalf("after the session: bridge closed %d, hider closed %d, active %v", b.closedCount(), h.closedCount(), c.ctl.active())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Setting up runs the installers of the missing drivers only, stops when
// one is closed, and passes on a restart request.
func TestSetUpDrivers(t *testing.T) {
	c := ctlTestCore(t, false, nil, nil)
	var ran []string
	installed := map[string]bool{}
	c.ctl.deps.status = func() vpad.Status { return vpad.Status{ViGEm: installed["ViGEmBus"], HidHide: installed["HidHide"]} }
	c.ctl.deps.install = func(_ context.Context, in vpad.Installer) (bool, bool, error) {
		ran = append(ran, in.Name)
		installed[in.Name] = true
		return in.Name == "HidHide", false, nil
	}
	st, err := c.ctl.setUp(c.ctx)
	if err != nil || !st.ViGEm || !st.HidHide || !st.Restart {
		t.Errorf("status %+v, err %v", st, err)
	}
	if !reflect.DeepEqual(ran, []string{"ViGEmBus", "HidHide"}) {
		t.Errorf("ran %v", ran)
	}
	ran = nil
	if _, err := c.ctl.setUp(c.ctx); err != nil || len(ran) != 0 {
		t.Errorf("everything installed, ran %v (err %v)", ran, err)
	}

	installed = map[string]bool{}
	c.ctl.deps.install = func(_ context.Context, in vpad.Installer) (bool, bool, error) {
		ran = append(ran, in.Name)
		return false, true, nil // closed
	}
	if st, err := c.ctl.setUp(c.ctx); err != nil || st.ViGEm || !reflect.DeepEqual(ran, []string{"ViGEmBus"}) {
		t.Errorf("closed installer: status %+v, ran %v, err %v", st, ran, err)
	}
}

// Unhide from Settings shows the controller at once, even mid-game, and
// a later change of controllers doesn't hide it again; leftovers from a
// crash are shown too.
func TestUnhideShowsTheControllerAgain(t *testing.T) {
	b, h := &fakeBridge{}, &fakeHider{active: true}
	c := ctlTestCore(t, true, b, h)
	leftovers := 0
	c.ctl.deps.unhide = func() error { leftovers++; return nil }
	if _, err := c.ctl.begin(true); err != nil {
		t.Fatal(err)
	}
	c.ctl.hidePads([]string{dsPath})
	if err := c.ctl.unhide(); err != nil {
		t.Fatal(err)
	}
	if h.closedCount() != 1 || leftovers != 1 {
		t.Fatalf("hider closed %d, leftovers shown %d", h.closedCount(), leftovers)
	}
	c.ctl.hidePads([]string{dsPath}) // the controller reconnects mid-game
	if len(h.hidden) != 0 {
		t.Errorf("hidden again after unhiding: %q", h.hidden)
	}
	if !c.ctl.active() {
		t.Error("unhiding ended the virtual controller")
	}
	c.ctl.end()
}
