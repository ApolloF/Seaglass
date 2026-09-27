package addons

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	idleAfter   = 5 * time.Minute
	maxRestarts = 3
	restartSpan = 10 * time.Minute
	maxLine     = 1 << 20
	stderrLines = 200
)

// Error is an error an add-on answered with; its message is meant for people.
type Error struct {
	Addon   string
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Addon + ": " + e.Message }

// Host starts add-ons when they're needed and talks to them.
type Host struct {
	version string
	logf    func(format string, args ...any)

	mu       sync.Mutex
	procs    map[string]*proc
	restarts map[string][]time.Time
}

// NewHost makes a host; version is Seaglass's, logf gets the add-ons'
// stderr and lifecycle lines.
func NewHost(version string, logf func(string, ...any)) *Host {
	return &Host{version: version, logf: logf, procs: map[string]*proc{}, restarts: map[string][]time.Time{}}
}

type proc struct {
	m     *Manifest
	cmd   *exec.Cmd
	stdin io.WriteCloser

	wmu      sync.Mutex // one write to stdin at a time; never held with mu
	mu       sync.Mutex
	next     int64
	pending  map[int64]chan reply
	progress map[int64]func(string)
	dead     chan struct{}
	idle     *time.Timer
	stopping bool // Seaglass ended it (not a crash)
}

type reply struct {
	result json.RawMessage
	err    *Error
}

// Call calls a method of an add-on whose program has the pinned hash,
// starting it first when needed. onProgress (may be nil) gets the
// add-on's progress lines while the call runs.
func (h *Host) Call(ctx context.Context, m *Manifest, pinned, method string, params, out any, onProgress func(string)) error {
	p, err := h.get(ctx, m, pinned)
	if err != nil {
		return err
	}
	return p.call(ctx, method, params, out, onProgress)
}

// Notify sends a notification to an add-on that is running (it isn't
// started for it).
func (h *Host) Notify(id, method string, params any) {
	h.mu.Lock()
	p := h.procs[id]
	h.mu.Unlock()
	if p != nil {
		_ = p.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	}
}

// Running reports whether an add-on's process runs.
func (h *Host) Running(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.procs[id] != nil
}

// Stop ends an add-on's process.
func (h *Host) Stop(id string) {
	h.mu.Lock()
	p := h.procs[id]
	delete(h.procs, id)
	h.mu.Unlock()
	if p != nil {
		p.stop()
	}
}

// StopAll ends every add-on (Seaglass is quitting).
func (h *Host) StopAll() {
	h.mu.Lock()
	ps := h.procs
	h.procs = map[string]*proc{}
	h.mu.Unlock()
	for _, p := range ps {
		p.stop()
	}
}

func (h *Host) get(ctx context.Context, m *Manifest, pinned string) (*proc, error) {
	h.mu.Lock()
	if p := h.procs[m.ID]; p != nil {
		h.mu.Unlock()
		return p, nil
	}
	// Crash isolation: an add-on that keeps dying isn't started again for a while.
	var recent []time.Time
	for _, t := range h.restarts[m.ID] {
		if time.Since(t) < restartSpan {
			recent = append(recent, t)
		}
	}
	if len(recent) >= maxRestarts {
		h.mu.Unlock()
		return nil, fmt.Errorf("%s stopped working; it'll be tried again in a few minutes", m.Name)
	}
	h.restarts[m.ID] = recent
	h.mu.Unlock()

	exe := m.ExePath()
	sum, err := HashFile(exe)
	if err != nil {
		return nil, fmt.Errorf("%s: program not found", m.Name)
	}
	if sum != pinned {
		return nil, fmt.Errorf("%s changed since you approved it; approve it again in Settings", m.Name)
	}
	p, err := h.start(m, exe)
	if err != nil {
		return nil, err
	}
	ictx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var init struct {
		Protocol int    `json:"protocol"`
		Name     string `json:"name"`
		Version  string `json:"version"`
	}
	err = p.call(ictx, "initialize", map[string]any{"protocol": Protocol, "host": map[string]string{"name": "Seaglass", "version": h.version}}, &init, nil)
	if err == nil && init.Protocol != Protocol {
		err = fmt.Errorf("%s speaks protocol %d, not %d", m.Name, init.Protocol, Protocol)
	}
	if err != nil {
		p.stop()
		return nil, err
	}
	h.logf("add-on %s %s started", m.ID, init.Version)

	h.mu.Lock()
	if other := h.procs[m.ID]; other != nil { // started twice at once: keep one
		h.mu.Unlock()
		p.stop()
		return other, nil
	}
	h.procs[m.ID] = p
	h.mu.Unlock()
	go func() {
		<-p.dead
		h.mu.Lock()
		if h.procs[m.ID] == p {
			delete(h.procs, m.ID)
		}
		p.mu.Lock()
		crashed := !p.stopping
		p.mu.Unlock()
		if crashed {
			h.restarts[m.ID] = append(h.restarts[m.ID], time.Now())
		}
		h.mu.Unlock()
	}()
	return p, nil
}

func (h *Host) start(m *Manifest, exe string) (*proc, error) {
	cmd := exec.Command(exe, m.Args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s couldn't start: %w", m.Name, err)
	}
	p := &proc{m: m, cmd: cmd, stdin: stdin, pending: map[int64]chan reply{}, progress: map[int64]func(string){}, dead: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(stderr)
		for n := 0; sc.Scan(); n++ {
			if n < stderrLines {
				h.logf("add-on %s: %s", m.ID, sc.Text())
			}
		}
	}()
	go p.read(stdout, h.logf)
	go func() {
		err := cmd.Wait()
		h.logf("add-on %s ended (%v)", m.ID, err)
		p.mu.Lock()
		for id, ch := range p.pending {
			close(ch)
			delete(p.pending, id)
		}
		if p.idle != nil {
			p.idle.Stop()
		}
		p.mu.Unlock()
		close(p.dead)
	}()
	return p, nil
}

func (p *proc) read(r io.Reader, logf func(string, ...any)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		var msg struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *Error          `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			logf("add-on %s wrote something that isn't JSON-RPC", p.m.ID)
			continue
		}
		if msg.ID == nil {
			var t struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(msg.Params, &t)
			if len(t.Text) > 300 {
				t.Text = t.Text[:300]
			}
			switch msg.Method {
			case "progress":
				p.mu.Lock()
				fns := make([]func(string), 0, len(p.progress))
				for _, fn := range p.progress {
					fns = append(fns, fn)
				}
				p.mu.Unlock()
				for _, fn := range fns {
					fn(t.Text)
				}
			case "log":
				logf("add-on %s: %s", p.m.ID, t.Text)
			}
			continue
		}
		p.mu.Lock()
		ch := p.pending[*msg.ID]
		delete(p.pending, *msg.ID)
		p.mu.Unlock()
		if ch != nil {
			if msg.Error != nil {
				msg.Error.Addon = p.m.Name
			}
			ch <- reply{msg.Result, msg.Error}
		}
	}
	// Stdout closed or garbled beyond repair: the add-on is done.
	p.kill()
}

func (p *proc) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// Not under mu: a write blocks while the add-on isn't reading, and the
	// reader needs mu to hand out replies, or the two would wait on each other.
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_, err = p.stdin.Write(append(b, '\n'))
	return err
}

func (p *proc) call(ctx context.Context, method string, params, out any, onProgress func(string)) error {
	p.mu.Lock()
	p.next++
	id := p.next
	ch := make(chan reply, 1)
	p.pending[id] = ch
	if onProgress != nil {
		p.progress[id] = onProgress
	}
	if p.idle != nil {
		p.idle.Stop()
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, id)
		delete(p.progress, id)
		if len(p.pending) == 0 {
			if p.idle == nil {
				p.idle = time.AfterFunc(idleAfter, p.stop)
			} else {
				p.idle.Reset(idleAfter)
			}
		}
		p.mu.Unlock()
	}()
	if err := p.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return fmt.Errorf("%s isn't running", p.m.Name)
	}
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s didn't answer in time", p.m.Name)
		}
		return ctx.Err()
	case r, ok := <-ch:
		if !ok {
			return fmt.Errorf("%s stopped", p.m.Name)
		}
		if r.err != nil {
			return r.err
		}
		if out != nil && len(r.result) > 0 {
			if err := json.Unmarshal(r.result, out); err != nil {
				return fmt.Errorf("%s answered something unexpected", p.m.Name)
			}
		}
		return nil
	}
}

// stop asks the add-on to shut down, then ends it.
func (p *proc) stop() {
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()
	// An add-on that stopped reading would block the goodbye; ending it
	// below unblocks the write.
	go func() {
		_ = p.send(map[string]any{"jsonrpc": "2.0", "method": "shutdown"})
		_ = p.stdin.Close()
	}()
	select {
	case <-p.dead:
	case <-time.After(3 * time.Second):
		p.kill()
	}
}

func (p *proc) kill() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}
