// Package rgb tints an RGB keyboard to the selected game's colour through
// OpenRGB's SDK server, the way the pad package tints the DualSense
// lightbar. It is written for the Akko PC75B Plus, whose OpenRGB plugin
// shows the whole board as one LED in a "Direct" mode; any controller with
// a Direct mode and that name would do.
//
// The keyboard's firmware sets the limits: every colour change blinks the
// board, it takes at most 10 changes a second, and entering or leaving
// Direct is likely a flash write. So the mode changes once when browsing
// starts and once when it ends (a game starting, Seaglass closing), and in
// between only the colour does, at most every 100 ms.
package rgb

import (
	"bufio"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

// Where the keyboard is today: the OpenRGB GUI loads the keyboard's plugin
// and serves it on 6743, beside the OpenRGB service on 6742, which can't
// see plugin devices.
const (
	DefaultAddr = "127.0.0.1:6743"
	DefaultName = "Akko PC75B Plus"
)

const directMode = "Direct"

// Keyboard drives one OpenRGB controller. Its methods only record what is
// wanted; one goroutine owns the connection and catches up, so a slow or
// absent server never holds up the caller. A nil Keyboard does nothing.
type Keyboard struct {
	addr, name string
	logf       func(format string, args ...any)

	// Overridable in tests.
	dial    func(addr string) (net.Conn, error)
	gap     time.Duration // the least time between two colours
	retry   time.Duration // how long after a failure to try again
	timeout time.Duration // for each reply

	mu     sync.Mutex
	want   [3]byte
	wanted bool // a colour is wanted: Big Picture is showing one
	paused bool // a game is running: the keyboard is the game's
	stop   bool

	wake chan struct{}
	done chan struct{}

	// Owned by the goroutine.
	c        *conn
	id       uint32
	held     bool // the keyboard is in Direct because of Seaglass
	sent     [3]byte
	hasSent  bool
	saved    *mode // the mode to give back, as it was
	savedIdx int32
	lastSend time.Time
	failedAt time.Time
	lastErr  string
}

// New returns a keyboard that connects to addr and drives the controller
// called name. It does nothing until the first SetColour.
func New(addr, name string, logf func(format string, args ...any)) *Keyboard {
	k := &Keyboard{
		addr: addr, name: name, logf: logf,
		dial:  func(a string) (net.Conn, error) { return net.DialTimeout("tcp", a, time.Second) },
		gap:   100 * time.Millisecond,
		retry: 10 * time.Second, timeout: 2 * time.Second,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
	}
	go k.run()
	return k
}

// SetColour shows the hue of #rrggbb at full strength (see vivid). Greys
// have no hue to show, so the last colour stays instead.
func (k *Keyboard) SetColour(hex string) {
	c, ok := parseHex(hex)
	if k == nil || !ok {
		return
	}
	c, ok = vivid(c)
	if !ok {
		return
	}
	k.mu.Lock()
	k.want, k.wanted = c, true
	k.mu.Unlock()
	k.poke()
}

// Clear gives the keyboard back its own mode until the next SetColour:
// Big Picture has closed, or the user turned the feature off.
func (k *Keyboard) Clear() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.wanted = false
	k.mu.Unlock()
	k.poke()
}

// Pause gives the keyboard back while a game runs (the game, or Aurora,
// colours it then). Resume takes it again with the colour last asked for.
func (k *Keyboard) Pause()  { k.setPaused(true) }
func (k *Keyboard) Resume() { k.setPaused(false) }

func (k *Keyboard) setPaused(p bool) {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.paused = p
	k.mu.Unlock()
	k.poke()
}

// Stop gives the keyboard back and closes the connection. It waits a few
// seconds at most, so a hung server can't keep Seaglass from exiting.
func (k *Keyboard) Stop() {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.stop = true
	k.mu.Unlock()
	k.poke()
	select {
	case <-k.done:
	case <-time.After(3 * time.Second):
	}
}

func (k *Keyboard) poke() {
	select {
	case k.wake <- struct{}{}:
	default:
	}
}

func (k *Keyboard) run() {
	defer close(k.done)
	for range k.wake {
		for {
			k.mu.Lock()
			want, active, stop := k.want, k.wanted && !k.paused && !k.stop, k.stop
			k.mu.Unlock()
			if active {
				// Changes come at most every gap; the latest one wins.
				if wait := k.gap - time.Since(k.lastSend); k.hasSent && want != k.sent && wait > 0 {
					time.Sleep(wait)
					continue
				}
				k.show(want)
			} else if k.held {
				k.release()
			}
			if stop {
				k.close()
				return
			}
			break
		}
	}
}

// show enters Direct if needed and sends c if it isn't on the board yet.
func (k *Keyboard) show(c [3]byte) {
	if !k.held {
		if !k.connect() {
			return
		}
		if err := k.enter(); err != nil {
			k.fail(err)
			return
		}
	}
	if k.hasSent && c == k.sent {
		return
	}
	if err := k.c.call(k.id, pktUpdateLEDs, updateLEDs(c[0], c[1], c[2])); err != nil {
		k.fail(err)
		return
	}
	k.sent, k.hasSent, k.lastSend = c, true, time.Now()
}

// enter remembers the keyboard's mode and switches it to Direct. The
// controller is looked up by name each time: its ID changes when OpenRGB's
// device list does.
func (k *Keyboard) enter() error {
	id, ctl, err := k.c.find(k.name)
	if err != nil {
		return err
	}
	k.id = id
	direct := false
	for _, m := range ctl.modes {
		direct = direct || m.name == directMode
	}
	if !direct {
		return errors.New(k.name + " has no Direct mode")
	}
	// Already in Direct: Seaglass left it there (it was closed before it
	// could give it back) or someone else is in Direct. Keep what was saved
	// before, if anything.
	if cur := ctl.modes[ctl.active]; cur.name != directMode {
		k.saved, k.savedIdx = &cur, ctl.active
	}
	if err := k.c.call(id, pktSetCustomMode, nil); err != nil {
		return err
	}
	k.held, k.hasSent = true, false
	k.logf("keyboard: %s follows the game", k.name)
	return nil
}

// release puts the saved mode back.
func (k *Keyboard) release() {
	k.held = false
	if k.c == nil || k.saved == nil {
		return
	}
	if err := k.c.call(k.id, pktUpdateMode, updateMode(k.savedIdx, k.saved.desc)); err != nil {
		k.fail(err)
		return
	}
	k.logf("keyboard: %s back to %s", k.name, k.saved.name)
}

// connect opens the connection if there is none. After a failure it waits
// k.retry before trying again, and tries only when something is wanted.
func (k *Keyboard) connect() bool {
	if k.c != nil {
		return true
	}
	if !k.failedAt.IsZero() && time.Since(k.failedAt) < k.retry {
		return false
	}
	nc, err := k.dial(k.addr)
	if err != nil {
		k.fail(errors.New("OpenRGB isn't serving on " + k.addr))
		return false
	}
	k.c = &conn{c: nc, r: bufio.NewReader(nc), timeout: k.timeout}
	if err := k.c.hello(); err != nil {
		k.fail(err)
		return false
	}
	return true
}

// fail drops the connection and logs the error once, until it changes.
func (k *Keyboard) fail(err error) {
	k.close()
	k.held = false
	k.failedAt = time.Now()
	msg := err.Error()
	if errors.Is(err, errMissing) {
		msg = k.name + " isn't in OpenRGB on " + k.addr
	}
	if msg != k.lastErr {
		k.lastErr = msg
		k.logf("keyboard: %s", msg)
	}
}

func (k *Keyboard) close() {
	if k.c != nil {
		_ = k.c.c.Close()
		k.c = nil
	}
}

func parseHex(s string) ([3]byte, bool) {
	if len(s) != 7 || s[0] != '#' {
		return [3]byte{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return [3]byte{}, false
	}
	return [3]byte{byte(v >> 16), byte(v >> 8), byte(v)}, true
}

// vivid keeps a colour's hue at full saturation and brightness. Seaglass's
// game accents are soft (pale oranges and sky blues), which a screen shows
// well but the keyboard's LEDs wash out into about two colours; and white
// comes out pink on the board. A colour with almost no hue (a grey) is
// refused.
func vivid(c [3]byte) ([3]byte, bool) {
	hi := max(c[0], c[1], c[2])
	lo := min(c[0], c[1], c[2])
	if int(hi)-int(lo) < 24 {
		return c, false
	}
	// Stretch each channel from [lo, hi] to [0, 255]: the hue stays, the
	// lowest channel goes to 0 and the highest to full.
	var out [3]byte
	for i, v := range c {
		out[i] = byte((int(v) - int(lo)) * 255 / (int(hi) - int(lo)))
	}
	return out, true
}
