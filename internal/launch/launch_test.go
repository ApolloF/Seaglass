package launch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// fakePC is a scripted process list: each poll shows the next frame, and
// the clock moves two seconds per poll.
type fakePC struct {
	mu     sync.Mutex
	frames [][]platform.Proc
	i      int
	paths  map[uint32]string
	clock  time.Time
	ended  []uint32
	fail   map[int]bool // polls whose snapshot fails
}

func (f *fakePC) procs() ([]platform.Proc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(2 * time.Second)
	fr := f.frames[min(f.i, len(f.frames)-1)]
	f.i++
	if f.fail[f.i-1] {
		return nil, errors.New("snapshot failed")
	}
	return fr, nil
}

func (f *fakePC) image(pid uint32) (string, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.paths[pid]; ok {
		return p, uint64(pid), nil
	}
	return "", 0, errors.New("access denied")
}

func (f *fakePC) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clock
}

type recorder struct {
	mu       sync.Mutex
	sessions []Session
	done     chan Session
}

func (r *recorder) on(s Session) {
	r.mu.Lock()
	r.sessions = append(r.sessions, s)
	r.mu.Unlock()
	if s.Phase.Done() && r.done != nil {
		select {
		case r.done <- s:
		default:
		}
	}
}

func newTest(f *fakePC) (*Manager, *recorder) {
	r := &recorder{done: make(chan Session, 1)}
	m := NewManager(r.on)
	m.procs, m.image, m.now, m.poll = f.procs, f.image, f.now, time.Millisecond
	m.watch, m.front = nil, nil
	m.end = func(pid uint32, _ uint64) error {
		f.mu.Lock()
		f.ended = append(f.ended, pid)
		f.mu.Unlock()
		return nil
	}
	return m, r
}

func wait(t *testing.T, r *recorder) Session {
	t.Helper()
	select {
	case s := <-r.done:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("session didn't end")
		return Session{}
	}
}

const gameDir = `D:\Games\Some Game`

func TestSessionPlaytimeAndHooks(t *testing.T) {
	sys := platform.Proc{PID: 10, PPID: 1, Name: "explorer.exe"}
	game := platform.Proc{PID: 100, PPID: 10, Name: "game.exe"}
	f := &fakePC{
		paths: map[uint32]string{10: `C:\Windows\explorer.exe`, 100: gameDir + `\game.exe`},
		frames: [][]platform.Proc{
			{sys, game}, {sys, game}, {sys, game}, {sys, game}, {sys, game}, {sys, game},
			{sys}, // closed: three quiet polls end it
		},
	}
	m, r := newTest(f)
	var played int64
	var order []string
	step := func(id string) Step {
		return Step{ID: id, Label: id, Run: func(ctx context.Context, s *StepContext) error {
			order = append(order, id)
			s.Progress("working")
			return nil
		}}
	}
	err := m.Launch(context.Background(), Plan{
		GameID: 7, Title: "Some Game", Dirs: []string{gameDir},
		Before: []Step{step("sync")}, After: []Step{step("backup")},
		Start:  func() (uint32, string, error) { order = append(order, "start"); return 100, "direct", nil },
		Played: func(s int64) { played += s },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Launch(context.Background(), Plan{Start: func() (uint32, string, error) { return 0, "", nil }}); !errors.Is(err, ErrBusy) {
		t.Errorf("second launch = %v, want ErrBusy", err)
	}
	s := wait(t, r)
	if s.Phase != Ended || s.Route != "direct" {
		t.Fatalf("end = %+v", s)
	}
	if got := len(order); got != 3 || order[0] != "sync" || order[1] != "start" || order[2] != "backup" {
		t.Errorf("order = %v", order)
	}
	// Seen on poll 1, then 5 more polls with the game: 5 × 2 s. Quiet polls don't count.
	if played != 10 || s.Seconds != 10 {
		t.Errorf("played = %d, session = %d; want 10", played, s.Seconds)
	}
	if s.Before[0].Status != StepDone || s.Before[0].Detail != "working" || s.After[0].Status != StepDone {
		t.Errorf("steps = %+v %+v", s.Before, s.After)
	}
	sawRunning := false
	for _, x := range r.sessions {
		sawRunning = sawRunning || x.Phase == Running
	}
	if !sawRunning {
		t.Error("never reported running")
	}
}

func TestLauncherHandoff(t *testing.T) {
	// The launcher (in the game folder) starts the game from elsewhere and exits.
	launcher := platform.Proc{PID: 100, PPID: 1, Name: "launcher.exe"}
	child := platform.Proc{PID: 200, PPID: 100, Name: "game.exe"}
	f := &fakePC{
		paths: map[uint32]string{100: gameDir + `\launcher.exe`, 200: `E:\Elsewhere\game.exe`},
		frames: [][]platform.Proc{
			{launcher}, {launcher, child}, {child}, {child}, {child}, {},
		},
	}
	m, r := newTest(f)
	var played int64
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir},
		Start:  func() (uint32, string, error) { return 100, "direct", nil },
		Played: func(s int64) { played += s }})
	s := wait(t, r)
	if s.Phase != Ended || played != 8 {
		t.Errorf("phase %s, played %d; want ended after 8 s", s.Phase, played)
	}
}

// A gap between a launcher and its game: gone, then back, then gone for good.
func TestGoneAndBack(t *testing.T) {
	game := platform.Proc{PID: 100, PPID: 1, Name: "game.exe"}
	f := &fakePC{
		paths:  map[uint32]string{100: gameDir + `\game.exe`},
		frames: [][]platform.Proc{{game}, {}, {game}, {game}, {}},
	}
	m, r := newTest(f)
	var mu sync.Mutex
	var events []string
	note := func(e string) func() {
		return func() {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}
	}
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir},
		Start: func() (uint32, string, error) { return 100, "direct", nil },
		OnRun: note("run"), OnGone: note("gone"), OnBack: note("back")})
	wait(t, r)
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(events, " "); got != "run gone back gone" {
		t.Errorf("events %q, want %q", got, "run gone back gone")
	}
}

func TestNeverSeen(t *testing.T) {
	f := &fakePC{frames: [][]platform.Proc{{{PID: 5, Name: "other.exe"}}}, paths: map[uint32]string{5: `C:\x\other.exe`}}
	m, r := newTest(f)
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir}, DetectTimeout: 10 * time.Second,
		Start: func() (uint32, string, error) { return 0, "store", nil }})
	s := wait(t, r)
	if s.Phase != Ended || s.Note == "" || s.StartedAt != 0 {
		t.Errorf("session = %+v", s)
	}
}

func TestSkipAskAndCancel(t *testing.T) {
	f := &fakePC{frames: [][]platform.Proc{{}}, paths: map[uint32]string{}}
	m, r := newTest(f)
	started := false
	answered := make(chan string, 1)
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir},
		Before: []Step{
			{ID: "slow", Label: "Slow", Timeout: time.Minute, Run: func(ctx context.Context, _ *StepContext) error {
				<-ctx.Done()
				return ctx.Err()
			}},
			{ID: "ask", Label: "Ask", Timeout: time.Minute, Run: func(ctx context.Context, s *StepContext) error {
				a, err := s.Ask(ctx, "Restart Steam?", []Option{{"yes", "Yes"}, {"no", "No"}})
				answered <- a
				if a == "no" {
					return ErrCancel
				}
				return err
			}},
		},
		Start: func() (uint32, string, error) { started = true; return 0, "", nil }})

	waitFor(t, m, func(s Session) bool { return len(s.Before) > 0 && s.Before[0].Status == StepRunning })
	m.Skip("slow")
	s := waitFor(t, m, func(s Session) bool { return s.Question != nil })
	m.Answer(s.Question.ID+1, "yes") // a stale question id is ignored
	m.Answer(s.Question.ID, "no")
	if a := <-answered; a != "no" {
		t.Errorf("answer = %q", a)
	}
	end := wait(t, r)
	if end.Phase != Cancelled || started || end.Before[0].Status != StepSkipped || end.Question != nil {
		t.Errorf("end = %+v (started %v)", end, started)
	}
}

func waitFor(t *testing.T, m *Manager, ok func(Session) bool) Session {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.Current(); ok(s) {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
	return Session{}
}

func TestQuit(t *testing.T) {
	game := platform.Proc{PID: 100, Name: "game.exe"}
	f := &fakePC{frames: [][]platform.Proc{{game}}, paths: map[uint32]string{100: gameDir + `\game.exe`}}
	m, _ := newTest(f)
	if err := m.Quit(); err == nil {
		t.Error("quit with nothing running must fail")
	}
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir}, Start: func() (uint32, string, error) { return 100, "direct", nil }})
	waitFor(t, m, func(s Session) bool { return s.Phase == Running })
	if err := m.Quit(); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ended) != 1 || f.ended[0] != 100 {
		t.Errorf("ended %v", f.ended)
	}
}

func TestUsableDirs(t *testing.T) {
	got := UsableDirs([]string{`C:\`, platform.ProgramFiles, `C:\Users`, "", "relative", gameDir})
	if len(got) != 1 || got[0] != gameDir {
		t.Errorf("UsableDirs = %v", got)
	}
}

// Quitting Seaglass mid-game hands over the playtime counted so far
// before Close returns, so it's saved with the library.
func TestCloseSavesPlaytime(t *testing.T) {
	sys := platform.Proc{PID: 10, PPID: 1, Name: "explorer.exe"}
	game := platform.Proc{PID: 100, PPID: 10, Name: "game.exe"}
	f := &fakePC{
		paths:  map[uint32]string{10: `C:\Windows\explorer.exe`, 100: gameDir + `\game.exe`},
		frames: [][]platform.Proc{{sys, game}}, // runs until closed
	}
	m, _ := newTest(f)
	var mu sync.Mutex
	var played int64
	running := make(chan struct{}, 1)
	err := m.Launch(context.Background(), Plan{
		GameID: 7, Title: "Some Game", Dirs: []string{gameDir},
		Start:  func() (uint32, string, error) { return 100, "direct", nil },
		Played: func(s int64) { mu.Lock(); played += s; mu.Unlock() },
		OnRun:  func() { running <- struct{}{} },
	})
	if err != nil {
		t.Fatal(err)
	}
	<-running
	time.Sleep(20 * time.Millisecond) // a few polls: some seconds counted
	m.Close()
	mu.Lock()
	defer mu.Unlock()
	if played == 0 {
		t.Error("Close returned before the playtime was handed over")
	}
}

// A game whose last process ends with a crash code is reported as
// crashed; its window coming to the front marks it shown.
func TestCrashAndShown(t *testing.T) {
	sys := platform.Proc{PID: 10, PPID: 1, Name: "explorer.exe"}
	launcher := platform.Proc{PID: 100, PPID: 10, Name: "launcher.exe"}
	game := platform.Proc{PID: 200, PPID: 100, Name: "game.exe"}
	for _, tc := range []struct {
		code uint32
		want string
	}{{0xC0000005, "0xC0000005"}, {0, ""}, {1, ""}, {0xFFFFFFFF, ""}, {0xC000013A, ""}} {
		f := &fakePC{
			paths: map[uint32]string{10: `C:\Windows\explorer.exe`, 100: gameDir + `\launcher.exe`, 200: gameDir + `\game.exe`},
			frames: [][]platform.Proc{
				{sys, launcher}, {sys, launcher, game}, {sys, game}, {sys, game}, {sys},
			},
		}
		m, r := newTest(f)
		m.front = func() uint32 { return 200 }
		m.watch = func(pid uint32, _ uint64) (func() (uint32, bool), func()) {
			code := uint32(0) // the launcher hands over cleanly
			if pid == 200 {
				code = tc.code
			}
			return func() (uint32, bool) { return code, true }, func() {}
		}
		if err := m.Launch(context.Background(), Plan{Title: "Some Game", Dirs: []string{gameDir}, Start: func() (uint32, string, error) { return 100, "direct", nil }}); err != nil {
			t.Fatal(err)
		}
		s := wait(t, r)
		if s.Crash != tc.want || !s.Shown {
			t.Errorf("exit %#x: crash %q shown %v, want %q and shown", tc.code, s.Crash, s.Shown, tc.want)
		}
	}
}

// A crash handler from the game's folder, left running after the game was
// ended from Task Manager, doesn't keep the session going.
func TestCrashHandlerLeftBehind(t *testing.T) {
	game := platform.Proc{PID: 100, PPID: 1, Name: "game.exe"}
	handler := platform.Proc{PID: 101, PPID: 100, Name: "UnityCrashHandler64.exe"}
	f := &fakePC{
		paths: map[uint32]string{100: gameDir + `\game.exe`, 101: gameDir + `\UnityCrashHandler64.exe`},
		frames: [][]platform.Proc{
			{game, handler}, {game, handler}, {handler},
		},
	}
	m, r := newTest(f)
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir},
		Start: func() (uint32, string, error) { return 100, "direct", nil }})
	if s := wait(t, r); s.Phase != Ended || s.StartedAt == 0 {
		t.Errorf("phase %s, started %d; want ended after running", s.Phase, s.StartedAt)
	}
}

// A failed process snapshot mid-game isn't the game closing: no gone or
// back, and the time across it still counts.
func TestSnapshotFails(t *testing.T) {
	game := platform.Proc{PID: 100, PPID: 1, Name: "game.exe"}
	f := &fakePC{
		paths:  map[uint32]string{100: gameDir + `\game.exe`},
		frames: [][]platform.Proc{{game}, {game}, {game}, {game}, {game}, {game}, {}},
		fail:   map[int]bool{2: true, 3: true, 4: true},
	}
	m, r := newTest(f)
	var mu sync.Mutex
	var events []string
	note := func(e string) func() {
		return func() {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}
	}
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir},
		Start: func() (uint32, string, error) { return 100, "direct", nil },
		OnRun: note("run"), OnGone: note("gone"), OnBack: note("back")})
	s := wait(t, r)
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(events, " "); got != "run gone" {
		t.Errorf("events %q, want %q", got, "run gone")
	}
	// Seen on poll 1, then 5 more polls (three of them failed) until it closes.
	if s.Phase != Ended || s.Seconds != 10 {
		t.Errorf("phase %s, seconds %d; want ended after 10", s.Phase, s.Seconds)
	}
}

// Quit succeeds when some of the game's processes had already ended.
func TestQuitSomeGone(t *testing.T) {
	launcher := platform.Proc{PID: 100, Name: "launcher.exe"}
	game := platform.Proc{PID: 200, PPID: 100, Name: "game.exe"}
	f := &fakePC{frames: [][]platform.Proc{{launcher, game}},
		paths: map[uint32]string{100: gameDir + `\launcher.exe`, 200: gameDir + `\game.exe`}}
	m, _ := newTest(f)
	_ = m.Launch(context.Background(), Plan{Dirs: []string{gameDir}, Start: func() (uint32, string, error) { return 100, "direct", nil }})
	waitFor(t, m, func(s Session) bool { return s.Phase == Running && m.IsGame(200) })
	m.end = func(pid uint32, _ uint64) error {
		if pid == 100 {
			return errors.New("process ended already")
		}
		return nil
	}
	if err := m.Quit(); err != nil {
		t.Errorf("quit = %v, want nil", err)
	}
	m.end = func(uint32, uint64) error { return errors.New("access denied") }
	if err := m.Quit(); err == nil {
		t.Error("quit that ended nothing must fail")
	}
	m.Close()
}

// Nothing starts once Seaglass is closing.
func TestLaunchAfterClose(t *testing.T) {
	f := &fakePC{frames: [][]platform.Proc{{}}, paths: map[uint32]string{}}
	m, _ := newTest(f)
	m.Close()
	started := false
	err := m.Launch(context.Background(), Plan{Start: func() (uint32, string, error) { started = true; return 1, "direct", nil }})
	if !errors.Is(err, ErrClosed) || started {
		t.Errorf("launch after close = %v, started %v", err, started)
	}
}

// The started process's id, handed out again to another program after it
// exited, doesn't make that program (or its children) the game.
func TestRootPIDReused(t *testing.T) {
	type img struct {
		path    string
		started uint64
	}
	images := map[uint32]img{100: {gameDir + `\launcher.exe`, 1}, 200: {`E:\Elsewhere\game.exe`, 2}}
	tr := newTracker([]string{gameDir}, 100, 1, func(pid uint32) (string, uint64, error) {
		if i, ok := images[pid]; ok {
			return i.path, i.started, nil
		}
		return "", 0, errors.New("access denied")
	})
	child := platform.Proc{PID: 200, PPID: 100, Name: "game.exe"}
	if got := tr.update([]platform.Proc{{PID: 100, Name: "launcher.exe"}, child}); len(got) != 2 {
		t.Fatalf("with launcher: %v", got)
	}
	tr.update([]platform.Proc{child}) // the launcher exits
	images[100] = img{`C:\Apps\other.exe`, 9}
	images[300] = img{`C:\Apps\helper.exe`, 10}
	got := tr.update([]platform.Proc{{PID: 100, Name: "other.exe"}, {PID: 300, PPID: 100, Name: "helper.exe"}, child})
	if _, ok := got[200]; !ok || len(got) != 1 {
		t.Errorf("after reuse: %v, want only 200", got)
	}
	// Reused within one poll: the cached entry is looked up again.
	images[200] = img{`C:\Apps\tab.exe`, 11}
	got = tr.update([]platform.Proc{{PID: 100, Name: "other.exe"}, {PID: 200, PPID: 1, Name: "tab.exe"}})
	if len(got) != 0 {
		t.Errorf("after 200 reused: %v, want none", got)
	}
}

// A game process that can't be read the moment it appears (it has only
// just started) is looked up again on the next polls, so the folder rule
// still finds it.
func TestLookupRetriedAfterAFailure(t *testing.T) {
	ready := false
	tr := newTracker([]string{gameDir}, 0, 1, func(pid uint32) (string, uint64, error) {
		if !ready {
			return "", 0, errors.New("access denied")
		}
		return gameDir + `\game.exe`, 5, nil
	})
	ps := []platform.Proc{{PID: 300, PPID: 4000, Name: "game.exe"}}
	if got := tr.update(ps); len(got) != 0 {
		t.Fatalf("unreadable: %v", got)
	}
	ready = true
	if got := tr.update(ps); got[300] != 5 {
		t.Errorf("readable on the next poll: %v, want 300", got)
	}
}
