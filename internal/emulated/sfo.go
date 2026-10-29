package emulated

import (
	"encoding/binary"
	"errors"
	"strings"
)

// maxSFO bounds a PARAM.SFO file (real ones are about 1 KB).
const maxSFO = 64 << 10

var errSFO = errors.New("not a PARAM.SFO file")

// parseSFO reads a PS3 game's PARAM.SFO: a table of keys (TITLE,
// TITLE_ID, CATEGORY, …) with text or number values. Only text values are
// returned.
func parseSFO(b []byte) (map[string]string, error) {
	if len(b) < 20 || string(b[:4]) != "\x00PSF" {
		return nil, errSFO
	}
	le := binary.LittleEndian
	keys, data, n := le.Uint32(b[8:]), le.Uint32(b[12:]), le.Uint32(b[16:])
	if n > 256 || uint64(20)+uint64(n)*16 > uint64(len(b)) || uint64(keys) > uint64(len(b)) || uint64(data) > uint64(len(b)) {
		return nil, errSFO
	}
	out := map[string]string{}
	for i := uint32(0); i < n; i++ {
		e := b[20+i*16:]
		keyOff, format := uint64(le.Uint16(e[0:])), le.Uint16(e[2:])
		size, off := uint64(le.Uint32(e[4:])), uint64(le.Uint32(e[12:]))
		k := uint64(keys) + keyOff
		if k >= uint64(len(b)) {
			return nil, errSFO
		}
		end := strings.IndexByte(string(b[k:]), 0)
		if end <= 0 || end > 64 {
			return nil, errSFO
		}
		key := string(b[k : k+uint64(end)])
		// 0x0204 is UTF-8 text ending in a zero, 0x0004 the same without
		// one; 0x0404 is a number.
		if format != 0x0204 && format != 0x0004 {
			continue
		}
		v := uint64(data) + off
		if size > 4096 || v+size > uint64(len(b)) {
			return nil, errSFO
		}
		val := string(b[v : v+size])
		if z := strings.IndexByte(val, 0); z >= 0 {
			val = val[:z]
		}
		out[key] = val
	}
	return out, nil
}
