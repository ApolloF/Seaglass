package rgb

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// OpenRGB SDK packet IDs (OpenRGB's NetworkProtocol.h).
const (
	pktControllerCount = 0
	pktControllerData  = 1
	pktAck             = 10
	pktProtocolVersion = 40
	pktClientName      = 50
	pktUpdateLEDs      = 1050
	pktSetCustomMode   = 1100
	pktUpdateMode      = 1101
)

// protocol is the SDK version spoken here. From 6 on, the server answers
// every request with an ACK and addresses controllers by a stable ID
// rather than their place in the list; OpenRGB 1.1 speaks it.
const protocol = 6

const headerSize = 16

var magic = [4]byte{'O', 'R', 'G', 'B'}

var le = binary.LittleEndian

// header is "ORGB", then the controller ID, the packet ID and the size of
// the data that follows.
func header(dev, id uint32, size int) []byte {
	b := make([]byte, headerSize)
	copy(b, magic[:])
	le.PutUint32(b[4:], dev)
	le.PutUint32(b[8:], id)
	le.PutUint32(b[12:], uint32(size))
	return b
}

func parseHeader(b []byte) (dev, id, size uint32, err error) {
	if len(b) < headerSize || [4]byte(b[:4]) != magic {
		return 0, 0, 0, errors.New("not an OpenRGB packet")
	}
	return le.Uint32(b[4:]), le.Uint32(b[8:]), le.Uint32(b[12:]), nil
}

// updateLEDs sets the keyboard's one LED, which stands for the whole
// board. OpenRGB keeps colours as 0x00BBGGRR, and the data starts with its
// own size.
func updateLEDs(r, g, b byte) []byte {
	p := make([]byte, 10)
	le.PutUint32(p, uint32(len(p)))
	le.PutUint16(p[4:], 1)
	copy(p[6:], []byte{r, g, b, 0})
	return p
}

// updateMode makes mode idx active with the settings in desc, a mode as
// the controller's data describes it.
func updateMode(idx int32, desc []byte) []byte {
	p := make([]byte, 8+len(desc))
	le.PutUint32(p, uint32(len(p)))
	le.PutUint32(p[4:], uint32(idx))
	copy(p[8:], desc)
	return p
}

// controller is what Seaglass needs from a controller's data.
type controller struct {
	name   string
	active int32
	modes  []mode
}

// mode keeps its description as sent, so restoring it sends back exactly
// the speed, brightness and colours it had.
type mode struct {
	name string
	desc []byte
}

// reader walks a data block; the first read past its end sets err.
type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.off+n > len(r.b) {
		r.err = fmt.Errorf("controller data ends at %d, wanted %d more bytes", r.off, n)
		return nil
	}
	s := r.b[r.off : r.off+n]
	r.off += n
	return s
}

func (r *reader) u16() int {
	if s := r.take(2); s != nil {
		return int(le.Uint16(s))
	}
	return 0
}

func (r *reader) u32() uint32 {
	if s := r.take(4); s != nil {
		return le.Uint32(s)
	}
	return 0
}

// str reads a length (which counts the closing NUL) and the string.
func (r *reader) str() string {
	s := r.take(r.u16())
	for len(s) > 0 && s[len(s)-1] == 0 {
		s = s[:len(s)-1]
	}
	return string(s)
}

// parseController reads a protocol 6 controller data reply.
func parseController(b []byte) (controller, error) {
	r := &reader{b: b}
	var c controller
	r.u32() // data size
	r.u32() // device type
	c.name = r.str()
	for range 5 { // vendor, description, version, serial, location
		r.str()
	}
	n := r.u16()
	c.active = int32(r.u32())
	for range n {
		start := r.off
		name := r.str()
		r.take(11 * 4) // flags, speed and brightness ranges, colour counts, speed, brightness, direction, colour mode
		r.take(4 * r.u16())
		if r.err != nil {
			break
		}
		c.modes = append(c.modes, mode{name: name, desc: append([]byte(nil), b[start:r.off]...)})
	}
	if r.err != nil {
		return c, r.err
	}
	if c.active < 0 || int(c.active) >= len(c.modes) {
		return c, fmt.Errorf("active mode %d of %d", c.active, len(c.modes))
	}
	return c, nil
}

// conn is one connection to an OpenRGB SDK server.
type conn struct {
	c       net.Conn
	r       *bufio.Reader
	timeout time.Duration
}

func (c *conn) send(dev, id uint32, data []byte) error {
	_ = c.c.SetWriteDeadline(time.Now().Add(c.timeout))
	_, err := c.c.Write(append(header(dev, id, len(data)), data...))
	return err
}

func (c *conn) recv() (dev, id uint32, data []byte, err error) {
	_ = c.c.SetReadDeadline(time.Now().Add(c.timeout))
	h := make([]byte, headerSize)
	if _, err = io.ReadFull(c.r, h); err != nil {
		return
	}
	var size uint32
	if dev, id, size, err = parseHeader(h); err != nil {
		return
	}
	if size > 1<<20 {
		return 0, 0, nil, fmt.Errorf("packet of %d bytes", size)
	}
	data = make([]byte, size)
	_, err = io.ReadFull(c.r, data)
	return
}

// reply waits for packet id from controller dev. Whatever else the server
// sends in between (ACKs, "device updated" after every change) is skipped.
func (c *conn) reply(dev, id uint32) ([]byte, error) {
	for {
		d, i, data, err := c.recv()
		if err != nil {
			return nil, err
		}
		if d == dev && i == id {
			return data, nil
		}
	}
}

// ackStatus names OpenRGB's NetPacketStatus values.
var ackStatus = map[uint32]string{1: "error", 2: "unsupported", 3: "not allowed", 4: "invalid ID", 5: "invalid data"}

// call sends a request and waits for the server's ACK of it.
func (c *conn) call(dev, id uint32, data []byte) error {
	if err := c.send(dev, id, data); err != nil {
		return err
	}
	for {
		d, i, ack, err := c.recv()
		if err != nil {
			return err
		}
		if i != pktAck || d != dev || len(ack) < 8 || le.Uint32(ack) != id {
			continue
		}
		if st := le.Uint32(ack[4:]); st != 0 {
			s := ackStatus[st]
			if s == "" {
				s = fmt.Sprint(st)
			}
			return fmt.Errorf("packet %d: %s", id, s)
		}
		return nil
	}
}

// errOldServer is a server older than protocol 6 (OpenRGB before 1.0).
var errOldServer = errors.New("OpenRGB is too old (needs 1.1 or later)")

// hello agrees on the protocol and names the client.
func (c *conn) hello() error {
	v := make([]byte, 4)
	le.PutUint32(v, protocol)
	if err := c.send(0, pktProtocolVersion, v); err != nil {
		return err
	}
	b, err := c.reply(0, pktProtocolVersion)
	if err != nil {
		return err
	}
	if len(b) < 4 || le.Uint32(b) < protocol {
		return errOldServer
	}
	return c.send(0, pktClientName, []byte("Seaglass\x00"))
}

// find returns the ID and data of the controller with the given name.
func (c *conn) find(name string) (uint32, controller, error) {
	if err := c.send(0, pktControllerCount, nil); err != nil {
		return 0, controller{}, err
	}
	b, err := c.reply(0, pktControllerCount)
	if err != nil {
		return 0, controller{}, err
	}
	if len(b) < 4 {
		return 0, controller{}, errors.New("short controller count")
	}
	n := int(le.Uint32(b))
	if len(b) < 4+4*n {
		return 0, controller{}, errors.New("short controller ID list")
	}
	for i := range n {
		id := le.Uint32(b[4+4*i:])
		if err := c.send(id, pktControllerData, nil); err != nil {
			return 0, controller{}, err
		}
		data, err := c.reply(id, pktControllerData)
		if err != nil {
			return 0, controller{}, err
		}
		ctl, err := parseController(data)
		if ctl.name != name {
			continue
		}
		if err != nil {
			return 0, controller{}, fmt.Errorf("%s: %w", name, err)
		}
		return id, ctl, nil
	}
	return 0, controller{}, errMissing
}

var errMissing = errors.New("keyboard not found")
