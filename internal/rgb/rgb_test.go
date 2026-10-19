package rgb

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// encStr is a length (counting the NUL) and the string, as OpenRGB writes it.
func encStr(s string) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(len(s)+1))
	return append(append(b, s...), 0)
}

// encMode is a protocol 6 mode description with a distinct speed, so a
// restored mode can be told apart.
func encMode(name string, speed uint32, colours ...uint32) []byte {
	b := encStr(name)
	for i := range 11 {
		v := uint32(i)
		if i == 7 {
			v = speed
		}
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(colours)))
	for _, c := range colours {
		b = binary.LittleEndian.AppendUint32(b, c)
	}
	return b
}

// encController is a protocol 6 controller data reply, cut off after the
// modes (Seaglass reads no further), with some zone bytes after them.
func encController(name string, active int32, modes ...[]byte) []byte {
	var b []byte
	b = binary.LittleEndian.AppendUint32(b, 0) // data size, filled in below
	b = binary.LittleEndian.AppendUint32(b, 5) // keyboard
	b = append(b, encStr(name)...)
	for _, s := range []string{"Akko", "desc", "1.0", "", "HID: x"} {
		b = append(b, encStr(s)...)
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(modes)))
	b = binary.LittleEndian.AppendUint32(b, uint32(active))
	for _, m := range modes {
		b = append(b, m...)
	}
	b = append(b, 1, 0, 0xde, 0xad) // zones Seaglass doesn't read
	binary.LittleEndian.PutUint32(b, uint32(len(b)))
	return b
}

func TestHeaderIsMagicThenLittleEndianFields(t *testing.T) {
	got := header(3, pktUpdateLEDs, 10)
	want := []byte{'O', 'R', 'G', 'B', 3, 0, 0, 0, 0x1a, 0x04, 0, 0, 10, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("header = % x, want % x", got, want)
	}
	dev, id, size, err := parseHeader(got)
	if err != nil || dev != 3 || id != pktUpdateLEDs || size != 10 {
		t.Fatalf("parseHeader = %d %d %d %v", dev, id, size, err)
	}
	if _, _, _, err := parseHeader([]byte("XRGB000000000000")); err == nil {
		t.Fatal("bad magic accepted")
	}
}

func TestUpdateLEDsSendsOneColourAsBGR(t *testing.T) {
	got := updateLEDs(0x11, 0x22, 0x33)
	want := []byte{10, 0, 0, 0, 1, 0, 0x11, 0x22, 0x33, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("updateLEDs = % x, want % x", got, want)
	}
}

func TestUpdateModeStartsWithItsSizeAndIndex(t *testing.T) {
	desc := encMode("Wave", 4)
	got := updateMode(2, desc)
	if n := binary.LittleEndian.Uint32(got); int(n) != len(got) {
		t.Fatalf("size field %d, packet %d", n, len(got))
	}
	if idx := binary.LittleEndian.Uint32(got[4:]); idx != 2 {
		t.Fatalf("mode index %d", idx)
	}
	if !bytes.Equal(got[8:], desc) {
		t.Fatal("mode description not sent as read")
	}
}

func TestParseControllerReadsNameActiveModeAndRawModes(t *testing.T) {
	wave, direct := encMode("Wave", 4, 0xff0000, 0x00ff00), encMode("Direct", 0)
	c, err := parseController(encController("Akko PC75B Plus", 0, wave, direct))
	if err != nil {
		t.Fatal(err)
	}
	if c.name != "Akko PC75B Plus" || c.active != 0 || len(c.modes) != 2 {
		t.Fatalf("got %+v", c)
	}
	if c.modes[0].name != "Wave" || !bytes.Equal(c.modes[0].desc, wave) || c.modes[1].name != "Direct" {
		t.Fatalf("modes %+v", c.modes)
	}
}

func TestParseControllerRejectsTruncatedOrBadData(t *testing.T) {
	full := encController("K", 0, encMode("Wave", 4, 1, 2, 3))
	if _, err := parseController(full[:len(full)-20]); err == nil {
		t.Fatal("truncated mode accepted")
	}
	if _, err := parseController(encController("K", 3, encMode("Wave", 4))); err == nil {
		t.Fatal("active mode out of range accepted")
	}
}

func TestWashedOutSkipsNearWhiteOnly(t *testing.T) {
	for hex, want := range map[string]bool{
		"#ffffff": true, "#e8edf2": true, "#d0c8c0": true,
		"#4fd0e0": false, "#ff2020": false, "#303030": false, "#000000": false,
	} {
		c, ok := parseHex(hex)
		if !ok || washedOut(c) != want {
			t.Errorf("washedOut(%s) = %v, want %v", hex, !want, want)
		}
	}
	for _, bad := range []string{"", "ffffff", "#fff", "#gggggg", "#1234567"} {
		if _, ok := parseHex(bad); ok {
			t.Errorf("parseHex(%q) accepted", bad)
		}
	}
}

// fakeServer is an OpenRGB SDK server with a few controllers. It records
// the requests that change something and, like OpenRGB, ACKs every
// request and announces each change with a "device updated" packet.
type fakeServer struct {
	t     *testing.T
	ln    net.Listener
	ctls  map[uint32][]byte // ID → controller data
	order []uint32

	mu  sync.Mutex
	ops []string
}

func newFakeServer(t *testing.T) *fakeServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeServer{t: t, ln: ln, ctls: map[uint32][]byte{}}
	// The keyboard isn't first and its ID isn't its index, as on the PC.
	s.add(0, encController("Monitor", 0, encMode("Static", 1)))
	s.add(3, encController(DefaultName, 1, encMode("Static", 1), encMode("Wave", 4, 0x0000ff), encMode("Direct", 0)))
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeServer) add(id uint32, data []byte) {
	s.ctls[id] = data
	s.order = append(s.order, id)
}

func (s *fakeServer) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *fakeServer) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	send := func(dev, id uint32, data []byte) { c.Write(append(header(dev, id, len(data)), data...)) }
	ack := func(dev, id uint32) {
		send(dev, pktAck, binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil, id), 0))
	}
	for {
		h := make([]byte, headerSize)
		if _, err := io.ReadFull(r, h); err != nil {
			return
		}
		dev, id, size, err := parseHeader(h)
		if err != nil {
			s.t.Error(err)
			return
		}
		data := make([]byte, size)
		io.ReadFull(r, data)
		switch id {
		case pktProtocolVersion:
			send(0, id, binary.LittleEndian.AppendUint32(nil, protocol))
		case pktClientName:
			ack(0, id)
		case pktControllerCount:
			b := binary.LittleEndian.AppendUint32(nil, uint32(len(s.order)))
			for _, i := range s.order {
				b = binary.LittleEndian.AppendUint32(b, i)
			}
			send(0, id, b)
		case pktControllerData:
			send(dev, id, s.ctls[dev])
			ack(dev, id)
		case pktSetCustomMode:
			s.record(fmt.Sprintf("direct %d", dev))
			ack(dev, id)
			send(dev, 1150, []byte{1, 2, 3})
		case pktUpdateLEDs:
			s.record(fmt.Sprintf("leds %d %02x%02x%02x", dev, data[6], data[7], data[8]))
			ack(dev, id)
			send(dev, 1150, nil)
		case pktUpdateMode:
			m, err := parseController(encController("x", 0, data[8:]))
			if err != nil {
				s.t.Error(err)
			}
			s.record(fmt.Sprintf("mode %d %d %s", dev, binary.LittleEndian.Uint32(data[4:]), m.modes[0].name))
			ack(dev, id)
		default:
			s.t.Errorf("unexpected packet %d", id)
		}
	}
}

func (s *fakeServer) record(op string) {
	s.mu.Lock()
	s.ops = append(s.ops, op)
	s.mu.Unlock()
}

func (s *fakeServer) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ops...)
}

// waitOps waits until the server has seen want, in order.
func (s *fakeServer) waitOps(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Join(s.got(), "|") == strings.Join(want, "|") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("server saw %q, want %q", s.got(), want)
}

type logs struct {
	mu   sync.Mutex
	list []string
}

func (l *logs) logf(f string, a ...any) {
	l.mu.Lock()
	l.list = append(l.list, fmt.Sprintf(f, a...))
	l.mu.Unlock()
}

func (l *logs) count(sub string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.list {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

func newTestKeyboard(addr string, l *logs) *Keyboard {
	k := New(addr, DefaultName, l.logf)
	k.gap = 20 * time.Millisecond
	return k
}

func TestKeyboardEntersDirectOnceAndSendsOnlyChangedColours(t *testing.T) {
	s := newFakeServer(t)
	k := newTestKeyboard(s.ln.Addr().String(), &logs{})
	defer k.Stop()
	k.SetColour("#ff0000")
	s.waitOps(t, "direct 3", "leds 3 ff0000")
	k.SetColour("#ff0000")
	k.SetColour("#ffffff") // near white: skipped
	k.SetColour("#0000ff")
	s.waitOps(t, "direct 3", "leds 3 ff0000", "leds 3 0000ff")
}

func TestKeyboardRestoresTheSavedModeForAGameAndTakesItBackAfter(t *testing.T) {
	s := newFakeServer(t)
	k := newTestKeyboard(s.ln.Addr().String(), &logs{})
	k.SetColour("#ff0000")
	s.waitOps(t, "direct 3", "leds 3 ff0000")
	k.Pause()
	s.waitOps(t, "direct 3", "leds 3 ff0000", "mode 3 1 Wave")
	k.SetColour("#00ff00") // while the game runs: remembered, not sent
	time.Sleep(50 * time.Millisecond)
	k.Resume()
	s.waitOps(t, "direct 3", "leds 3 ff0000", "mode 3 1 Wave", "direct 3", "leds 3 00ff00")
	k.Stop()
	s.waitOps(t, "direct 3", "leds 3 ff0000", "mode 3 1 Wave", "direct 3", "leds 3 00ff00", "mode 3 1 Wave")
}

func TestKeyboardClearRestoresAndStopDoesNothingMore(t *testing.T) {
	s := newFakeServer(t)
	k := newTestKeyboard(s.ln.Addr().String(), &logs{})
	k.SetColour("#ff0000")
	s.waitOps(t, "direct 3", "leds 3 ff0000")
	k.Clear()
	k.Stop()
	s.waitOps(t, "direct 3", "leds 3 ff0000", "mode 3 1 Wave")
}

func TestKeyboardSendsAtMostOneColourPerGap(t *testing.T) {
	s := newFakeServer(t)
	k := New(s.ln.Addr().String(), DefaultName, (&logs{}).logf)
	defer k.Stop()
	start := time.Now()
	for i := range 50 {
		k.SetColour(fmt.Sprintf("#%02x0000", 100+i))
		time.Sleep(20 * time.Millisecond)
	}
	elapsed := time.Since(start)
	time.Sleep(150 * time.Millisecond)
	leds := 0
	for _, op := range s.got() {
		if strings.HasPrefix(op, "leds") {
			leds++
		}
	}
	if limit := int(elapsed/(100*time.Millisecond)) + 2; leds > limit {
		t.Fatalf("%d colours in %v, limit %d", leds, elapsed, limit)
	}
	if last := s.got()[len(s.got())-1]; last != "leds 3 950000" {
		t.Fatalf("last colour %q, want the latest asked for", last)
	}
}

func TestKeyboardWithoutServerLogsOnceAndRetriesLater(t *testing.T) {
	l := &logs{}
	dials := 0
	var mu sync.Mutex
	k := New("127.0.0.1:1", DefaultName, l.logf)
	k.retry = 80 * time.Millisecond
	k.dial = func(string) (net.Conn, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return nil, errors.New("refused")
	}
	for range 5 {
		k.SetColour("#ff0000")
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	k.SetColour("#00ff00")
	time.Sleep(30 * time.Millisecond)
	k.Stop()
	mu.Lock()
	defer mu.Unlock()
	if dials != 2 {
		t.Fatalf("%d dials, want 2 (one, then one after the retry delay)", dials)
	}
	if n := l.count("isn't serving"); n != 1 {
		t.Fatalf("logged %d times, want once", n)
	}
}

func TestKeyboardMissingFromServerIsLoggedOnce(t *testing.T) {
	s := newFakeServer(t)
	delete(s.ctls, 3)
	s.order = s.order[:1]
	l := &logs{}
	k := newTestKeyboard(s.ln.Addr().String(), l)
	k.retry = 0
	for range 3 {
		k.SetColour("#ff0000")
		time.Sleep(20 * time.Millisecond)
	}
	k.Stop()
	if n := l.count("isn't in OpenRGB"); n != 1 {
		t.Fatalf("logged %d times, want once", n)
	}
	if len(s.got()) != 0 {
		t.Fatalf("server saw %q", s.got())
	}
}
