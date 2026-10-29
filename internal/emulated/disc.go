package emulated

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"strings"
)

// disc is what a GameCube or Wii disc image says about itself.
type disc struct {
	id       string // RMCE01; "" when the image's header is compressed (GCZ)
	title    string // the disc's own name, often without spaces ("MarioKartWii")
	platform string // "GameCube" or "Wii"
}

const (
	wiiMagic = 0x5D1C9EA3
	gcMagic  = 0xC2339F3D
)

// discHeader reads the start of a disc (0x60 bytes are enough).
func discHeader(h []byte) (disc, bool) {
	if len(h) < 0x60 {
		return disc{}, false
	}
	d := disc{}
	switch {
	case binary.BigEndian.Uint32(h[0x18:]) == wiiMagic:
		d.platform = "Wii"
	case binary.BigEndian.Uint32(h[0x1C:]) == gcMagic:
		d.platform = "GameCube"
	default:
		return disc{}, false
	}
	if id := string(h[:6]); isDiscID(id) {
		d.id = id
	}
	name := h[0x20:0x60]
	if z := bytes.IndexByte(name, 0); z >= 0 {
		name = name[:z]
	}
	// Japanese discs name themselves in Shift JIS; only plain text is kept.
	ascii := true
	for _, c := range name {
		if c < 0x20 || c > 0x7E {
			ascii = false
			break
		}
	}
	if ascii {
		d.title = cleanTitle(string(name))
	}
	return d, true
}

// readDisc reads a GameCube or Wii disc image's header, in whichever of
// the formats Dolphin plays it is: a plain image (.iso, .gcm), WBFS, CISO,
// WIA or RVZ (which keep the disc's header uncompressed), or GCZ (which
// only says which console).
func readDisc(path string) (disc, bool) {
	h, err := readAt(path, 0, 0x100)
	if err != nil {
		return disc{}, false
	}
	return parseDisc(h, func(off int64) ([]byte, error) { return readAt(path, off, 0x60) })
}

// parseDisc works out the format from the first 0x100 bytes of an image;
// at reads 0x60 bytes further in, for the formats whose disc header isn't
// at the start.
func parseDisc(h []byte, at func(off int64) ([]byte, error)) (disc, bool) {
	if len(h) < 0x100 {
		return disc{}, false
	}
	switch string(h[:4]) {
	case "WBFS":
		// The disc's header follows the first hard disk sector.
		shift := h[8]
		if shift < 9 || shift > 16 {
			return disc{}, false
		}
		b, err := at(1 << shift)
		if err != nil {
			return disc{}, false
		}
		return discHeader(b)
	case "CISO":
		b, err := at(0x8000)
		if err != nil {
			return disc{}, false
		}
		return discHeader(b)
	case "WIA\x01", "RVZ\x01":
		// WIA header 1 is 0x48 bytes; header 2 starts with the disc type
		// and keeps the disc's first 0x80 bytes at 0x10.
		d, ok := discHeader(h[0x58:0xD8])
		if !ok {
			switch binary.BigEndian.Uint32(h[0x48:]) {
			case 1:
				return disc{platform: "GameCube"}, true
			case 2:
				return disc{platform: "Wii"}, true
			}
		}
		return d, ok
	}
	if binary.LittleEndian.Uint32(h) == 0xB10BC001 { // GCZ
		switch binary.LittleEndian.Uint32(h[4:]) {
		case 0:
			return disc{platform: "GameCube"}, true
		case 1:
			return disc{platform: "Wii"}, true
		}
		return disc{}, false
	}
	return discHeader(h)
}

// parseTitleDB reads one of the GameTDB title lists Dolphin ships
// (Sys\wiitdb-en.txt): "RMCE01 = Mario Kart Wii" per line.
func parseTitleDB(b []byte) map[string]string {
	m := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1024), 1024)
	for sc.Scan() && len(m) < 100000 {
		id, title, ok := strings.Cut(sc.Text(), " = ")
		id = strings.TrimSpace(id)
		if !ok || len(id) != 4 && len(id) != 6 || id == "TITLES" {
			continue
		}
		if t := cleanTitle(title); t != "" {
			m[strings.ToUpper(id)] = t
		}
	}
	return m
}
