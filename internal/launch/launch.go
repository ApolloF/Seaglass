// Package launch starts games and follows them while they run: the hooks
// before launch and after exit, the game's processes, and playtime.
//
// One game session runs at a time. It moves through phases:
//
//	preparing → starting → running → finishing → ended
//
// with failed and cancelled as other ways to end. Processes are only
// polled while a session is active, so an idle launcher costs nothing.
package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// Phase is where a session is.
type Phase string

const (
	Preparing Phase = "preparing" // hooks before launch
	Starting  Phase = "starting"  // started; waiting to see the game run
	Running   Phase = "running"
	Finishing Phase = "finishing" // hooks after exit
	Ended     Phase = "ended"
	Failed    Phase = "failed"
	Cancelled Phase = "cancelled"
)

// Done reports whether the session is over.
func (p Phase) Done() bool { return p == Ended || p == Failed || p == Cancelled || p == "" }

// Step statuses.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepSkipped = "skipped"
	StepFailed  = "failed"
)

// StepState is a hook step as the interface shows it.
type StepState struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Option is one answer to a question.
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Question is a step asking the user to decide something.
type Question struct {
	ID      int      `json:"id"`
	Text    string   `json:"text"`
	Options []Option `json:"options"`
}

// Session is the game being launched or played.
type Session struct {
	ID        int64       `json:"id"`
	GameID    int64       `json:"gameId"`
	Title     string      `json:"title"`
	Phase     Phase       `json:"phase"`
	Route     string      `json:"route"` // how it starts: "direct", "store", "steamInput"
	Before    []StepState `json:"before"`
	After     []StepState `json:"after"`
	Question  *Question   `json:"question,omitempty"`
	StartedAt int64       `json:"startedAt,omitempty"` // unix seconds; when the game was seen running
	Seconds   int64       `json:"seconds"`             // played this session
	Error     string      `json:"error,omitempty"`
	Note      string      `json:"note,omitempty"`
	// Shown: the game's own window has come to the front (until then it's
	// still loading, as far as anyone can see).
	Shown bool `json:"shown,omitempty"`
	// Crash is the exit code, as 0xC0000005, when the game crashed.
	Crash string `json:"crash,omitempty"`
}

// Step is one hook.
type Step struct {
	ID, Label string
	Timeout   time.Duration
	Run       func(ctx context.Context, s *StepContext) error
}

// ErrCancel ends the launch from a step (the user chose not to play).
var ErrCancel = errors.New("launch cancelled")

// StepContext lets a step report progress and ask the user.
type StepContext struct {
	m  *Manager
	id string
}

// Progress shows a line of detail under the step.
func (c *StepContext) Progress(detail string) {
	c.m.update(func(s *Session) { stepOf(s, c.id).Detail = detail })
}

// Ask shows a question and waits for the answer (an option id).
func (c *StepContext) Ask(ctx context.Context, text string, opts []Option) (string, error) {
	c.m.mu.Lock()
	c.m.qseq++
	q := &Question{ID: c.m.qseq, Text: text, Options: opts}
	ch := make(chan string, 1)
	c.m.answer = ch
	c.m.mu.Unlock()
	c.m.update(func(s *Session) { s.Question = q })
	defer c.m.update(func(s *Session) { s.Question = nil })
	select {
	case a := <-ch:
		return a, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Plan describes one launch.
type Plan struct {
	GameID int64
	Title  string
	Dirs   []string // folders the game's processes run from
	Before []Step
	// Start starts the game. It runs after the Before steps, which may
	// change what it does (the route). pid is 0 when the game was handed
	// to a store or Steam.
	Start  func() (pid uint32, route string, err error)
	After  []Step
	Played func(seconds int64) // adds playtime; called every minute and at the end
	OnRun  func()              // the game was seen running
	// OnGone and OnBack follow the game's processes after that: gone is the
	// first look without any (it may have exited, or be handing over from a
	// launcher), back is when they're seen again before the session ends.
	OnGone, OnBack func()
	// How long to wait for the game to show up: stores and Steam can take
	// a while (updates, shader caches, sign-in).
	DetectTimeout time.Duration
}

// Manager runs sessions.
type Manager struct {
	onChange func(Session)
	procs    func() ([]platform.Proc, error)
	image    func(uint32) (string, uint64, error)
	end      func(pid uint32, started uint64) error
	// watch opens a process to read its exit code after it ends.
	watch func(pid uint32, started uint64) (code func() (uint32, bool), done func())
	front func() uint32 // the process whose window is in front
	now   func() time.Time
	poll  time.Duration
	quiet int // polls without a game process before it counts as closed

	mu      sync.Mutex
	cur     Session
	seq     int64
	qseq    int
	answer  chan string
	skip    map[string]context.CancelFunc
	cancel  context.CancelFunc
	tracked map[uint32]uint64
	stop    chan struct{}
	running sync.WaitGroup // the session goroutine
}

// NewManager makes a manager; onChange gets every session change.
func NewManager(onChange func(Session)) *Manager {
	return &Manager{
		onChange: onChange, procs: platform.Processes, image: platform.ProcessImage, end: platform.EndProcess,
		watch: watchExit, front: platform.ForegroundPID, now: time.Now, stop: make(chan struct{}), poll: 2 * time.Second, quiet: 3,
	}
}

// Current returns the session (the last one when none is active).
func (m *Manager) Current() Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return copySession(m.cur)
}

// IsGame reports whether a process belongs to the game being played.
func (m *Manager) IsGame(pid uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.tracked[pid]
	return ok
}

// Active reports whether a session is under way.
func (m *Manager) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.cur.Phase.Done()
}

// ErrBusy means a game is already being played.
var ErrBusy = errors.New("a game is already running")

// Launch starts a session in the background.
func (m *Manager) Launch(ctx context.Context, p Plan) error {
	m.mu.Lock()
	if !m.cur.Phase.Done() {
		m.mu.Unlock()
		return ErrBusy
	}
	m.seq++
	ctx, cancel := context.WithCancel(ctx)
	m.cancel, m.skip, m.tracked = cancel, map[string]context.CancelFunc{}, nil
	m.cur = Session{ID: m.seq, GameID: p.GameID, Title: p.Title, Phase: Preparing,
		Before: states(p.Before), After: states(p.After)}
	m.mu.Unlock()
	m.update(func(*Session) {})
	m.running.Add(1)
	go m.run(ctx, p)
	return nil
}

// Close stops following the game (Seaglass is quitting). It waits a
// moment for the playtime counted so far to be handed to Played, so it's
// in the library before that is saved.
func (m *Manager) Close() {
	m.mu.Lock()
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	if m.cancel != nil {
		m.cancel() // hooks before the game started stop too
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() {
		m.running.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// Skip stops a running step and moves on.
func (m *Manager) Skip(stepID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.skip[stepID]; c != nil {
		c()
	}
}

// Answer answers the open question.
func (m *Manager) Answer(questionID int, option string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur.Question == nil || m.cur.Question.ID != questionID || m.answer == nil {
		return
	}
	select {
	case m.answer <- option:
	default:
	}
}

// Cancel stops a launch that hasn't reached the game yet.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if (m.cur.Phase == Preparing || m.cur.Phase == Starting) && m.cancel != nil {
		m.cancel()
	}
}

// Quit ends the game's processes right away.
func (m *Manager) Quit() error {
	m.mu.Lock()
	procs := m.tracked
	running := m.cur.Phase == Running
	m.mu.Unlock()
	if !running || len(procs) == 0 {
		return errors.New("no game is running")
	}
	var first error
	for pid, started := range procs {
		if err := m.end(pid, started); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Manager) run(ctx context.Context, p Plan) {
	defer m.running.Done()
	defer func() {
		m.mu.Lock()
		m.cancel()
		m.mu.Unlock()
	}()
	for _, st := range p.Before {
		if err := m.step(ctx, st, &m.cur.Before); errors.Is(err, ErrCancel) || ctx.Err() != nil {
			m.update(func(s *Session) { s.Phase = Cancelled })
			return
		}
	}
	pid, route, err := p.Start()
	if err != nil {
		m.update(func(s *Session) { s.Phase, s.Error = Failed, err.Error() })
		return
	}
	m.update(func(s *Session) { s.Phase, s.Route = Starting, route })

	played := m.follow(ctx, p, pid)
	if played < 0 {
		return // cancelled or never seen
	}
	m.update(func(s *Session) { s.Phase = Finishing })
	for _, st := range p.After {
		_ = m.step(context.Background(), st, &m.cur.After)
	}
	m.update(func(s *Session) { s.Phase = Ended })
}

// follow waits for the game to run and until it closes, counting
// playtime. It returns the seconds played, or -1 when the session ended
// without the game being seen.
func (m *Manager) follow(ctx context.Context, p Plan, pid uint32) int64 {
	t := newTracker(UsableDirs(p.Dirs), pid, uint32(os.Getpid()), m.image)
	timeout := p.DetectTimeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	began := m.now()
	var seenAt, last time.Time
	var total, unsaved, gap float64
	quiet := 0
	// Each game process is held open, so how it ended can be read: the
	// last one to go decides whether the game crashed.
	type watched struct {
		code func() (uint32, bool)
		done func()
	}
	watches := map[uint32]watched{}
	var lastExit uint32
	defer func() {
		for _, w := range watches {
			w.done()
		}
	}()
	reap := func(game map[uint32]uint64) {
		for pid, w := range watches {
			if _, alive := game[pid]; alive {
				continue
			}
			if c, ok := w.code(); ok {
				lastExit = c
			}
			w.done()
			delete(watches, pid)
		}
		for pid, started := range game {
			if _, ok := watches[pid]; !ok && m.watch != nil {
				if c, done := m.watch(pid, started); c != nil {
					watches[pid] = watched{c, done}
				}
			}
		}
	}
	shown := false
	flush := func() {
		if s := int64(unsaved); s > 0 && p.Played != nil {
			p.Played(s)
			unsaved -= float64(s)
		}
	}
	tick := time.NewTicker(m.poll)
	defer tick.Stop()
	for {
		ps, err := m.procs()
		now := m.now()
		var game map[uint32]uint64
		if err == nil {
			game = t.update(ps)
		}
		m.mu.Lock()
		m.tracked = game
		m.mu.Unlock()
		if err == nil {
			reap(game)
		}
		if !shown && len(game) > 0 && m.front != nil {
			if _, ok := game[m.front()]; ok {
				shown = true
				m.update(func(s *Session) { s.Shown = true })
			}
		}
		switch {
		case len(game) > 0 && seenAt.IsZero():
			seenAt, last = now, now
			m.update(func(s *Session) { s.Phase, s.StartedAt = Running, now.Unix() })
			if p.OnRun != nil {
				p.OnRun()
			}
		case !seenAt.IsZero():
			// Count wall time between polls, but not a gap from sleep or
			// hibernation. Time without a game process only counts when
			// the game turns out to still be there (a launcher handoff).
			if d := now.Sub(last); d > 0 && d < 30*time.Second {
				gap += d.Seconds()
			}
			last = now
			if len(game) == 0 {
				quiet++
				if quiet == 1 && p.OnGone != nil {
					p.OnGone()
				}
				if quiet >= m.quiet {
					flush()
					secs := int64(total)
					crash := ""
					if platform.Crashed(lastExit) {
						crash = fmt.Sprintf("0x%08X", lastExit)
					}
					m.update(func(s *Session) { s.Seconds, s.Crash = secs, crash })
					return secs
				}
				break
			}
			if quiet > 0 && p.OnBack != nil {
				p.OnBack()
			}
			quiet = 0
			total += gap
			unsaved += gap
			gap = 0
			if unsaved >= 60 {
				flush()
			}
			secs := int64(total)
			m.update(func(s *Session) { s.Seconds = secs })
		case now.Sub(began) > timeout:
			m.update(func(s *Session) {
				s.Phase = Ended
				s.Note = "Seaglass couldn't see the game running, so no playtime was counted."
			})
			return -1
		}
		select {
		case <-m.stop:
			total += gap
			unsaved += gap
			flush()
			return -1
		case <-ctx.Done():
			if seenAt.IsZero() {
				m.update(func(s *Session) { s.Phase = Cancelled })
				return -1
			}
			// Cancel only applies before the game runs; keep following it.
			ctx = context.Background()
		case <-tick.C:
		}
	}
}

func (m *Manager) step(ctx context.Context, st Step, list *[]StepState) error {
	timeout := st.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	sctx, cancel := context.WithTimeout(ctx, timeout)
	skipCtx, skip := context.WithCancel(sctx)
	defer cancel()
	defer skip()
	m.mu.Lock()
	m.skip[st.ID] = skip
	m.mu.Unlock()
	m.setStep(list, st.ID, StepRunning, text(""))

	err := st.Run(skipCtx, &StepContext{m: m, id: st.ID})
	m.mu.Lock()
	delete(m.skip, st.ID)
	m.mu.Unlock()
	switch {
	case errors.Is(err, ErrCancel):
		m.setStep(list, st.ID, StepSkipped, text(""))
		return err
	case ctx.Err() != nil:
		m.setStep(list, st.ID, StepSkipped, text(""))
		return ctx.Err()
	case sctx.Err() == context.DeadlineExceeded:
		m.setStep(list, st.ID, StepFailed, text("Took too long"))
	case skipCtx.Err() != nil:
		m.setStep(list, st.ID, StepSkipped, text("Skipped"))
	case err != nil:
		m.setStep(list, st.ID, StepFailed, text(err.Error()))
	default:
		m.setStep(list, st.ID, StepDone, nil) // keeps the step's last progress line
	}
	return nil
}

// setStep changes a step's status, and its detail unless detail is nil.
func (m *Manager) setStep(list *[]StepState, id, status string, detail *string) {
	m.update(func(s *Session) {
		for i := range *list {
			if (*list)[i].ID == id {
				(*list)[i].Status = status
				if detail != nil {
					(*list)[i].Detail = *detail
				}
			}
		}
	})
}

func text(s string) *string { return &s }

func (m *Manager) update(fn func(*Session)) {
	m.mu.Lock()
	fn(&m.cur)
	s := copySession(m.cur)
	m.mu.Unlock()
	if m.onChange != nil {
		m.onChange(s)
	}
}

func stepOf(s *Session, id string) *StepState {
	for i := range s.Before {
		if s.Before[i].ID == id {
			return &s.Before[i]
		}
	}
	for i := range s.After {
		if s.After[i].ID == id {
			return &s.After[i]
		}
	}
	return &StepState{}
}

func states(steps []Step) []StepState {
	out := make([]StepState, 0, len(steps))
	for _, s := range steps {
		out = append(out, StepState{ID: s.ID, Label: s.Label, Status: StepPending})
	}
	return out
}

func copySession(s Session) Session {
	s.Before = append([]StepState(nil), s.Before...)
	s.After = append([]StepState(nil), s.After...)
	if s.Before == nil {
		s.Before = []StepState{}
	}
	if s.After == nil {
		s.After = []StepState{}
	}
	if s.Question != nil {
		q := *s.Question
		q.Options = append([]Option(nil), q.Options...)
		s.Question = &q
	}
	return s
}

// UsableDirs drops folders too broad to say a process belongs to the game
// (a drive root, Program Files, the user's profile, …).
func UsableDirs(dirs []string) []string {
	broad := map[string]bool{}
	for _, d := range []string{platform.ProgramFiles, platform.ProgramFilesX86, platform.Profile, platform.Desktop,
		platform.PublicDesktop, platform.Local, platform.Roaming, platform.ProgramData, platform.Public, platform.WindowsDir} {
		if d != "" {
			broad[platform.Key(d)] = true
		}
	}
	var out []string
	for _, d := range dirs {
		if d == "" || !filepath.IsAbs(d) {
			continue
		}
		c := filepath.Clean(d)
		if filepath.Dir(c) == c || broad[platform.Key(c)] || strings.EqualFold(filepath.Base(c), "Users") {
			continue
		}
		out = append(out, c)
	}
	return out
}

func watchExit(pid uint32, started uint64) (func() (uint32, bool), func()) {
	w, err := platform.WatchExit(pid, started)
	if err != nil {
		return nil, nil
	}
	return w.Code, w.Close
}
