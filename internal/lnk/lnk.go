// Package lnk reads Windows shell links (.lnk files): the parts of the
// MS-SHLLINK format that give the target, arguments, working folder and icon.
// Installers put a "Play …" shortcut on the desktop; for a repack that is
// the most reliable record of which executable actually starts the game.
package lnk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// Link is what a shortcut starts.
type Link struct {
	Target  string // absolute path of the target, environment variables expanded
	Args    string
	WorkDir string
	Icon    string
	Name    string // the shortcut's description
	RelPath string // target relative to the .lnk file (fallback when Target is empty)
}

const maxSize = 1 << 20

// Link flags (MS-SHLLINK 2.1.1).
const (
	hasIDList    = 1 << 0
	hasLinkInfo  = 1 << 1
	hasName      = 1 << 2
	hasRelPath   = 1 << 3
	hasWorkDir   = 1 << 4
	hasArgs      = 1 << 5
	hasIcon      = 1 << 6
	isUnicode    = 1 << 7
	hasExpString = 1 << 9
)

var linkCLSID = []byte{0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}

var errNotLink = errors.New("not a shell link")

// ReadFile parses the .lnk at path and resolves a relative target.
func ReadFile(path string) (*Link, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxSize {
		return nil, errNotLink
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	l, err := Parse(b)
	if err != nil {
		return nil, err
	}
	if l.Target == "" && l.RelPath != "" {
		l.Target = filepath.Clean(filepath.Join(filepath.Dir(path), l.RelPath))
	}
	return l, nil
}

// Parse reads a shell link from its bytes. Environment variables in the
// target, working folder and icon are expanded.
func Parse(b []byte) (*Link, error) {
	if len(b) < 0x4C || binary.LittleEndian.Uint32(b) != 0x4C || !bytes.Equal(b[4:20], linkCLSID) {
		return nil, errNotLink
	}
	c := cursor{b: b, off: 0x4C}
	flags := binary.LittleEndian.Uint32(b[0x14:])
	if flags&hasIDList != 0 {
		n, ok := c.u16()
		if !ok || !c.skip(int(n)) {
			return nil, errNotLink
		}
	}
	l := &Link{}
	if flags&hasLinkInfo != 0 {
		start := c.off
		size, ok := c.u32()
		if !ok || size < 4 || start+int(size) > len(b) {
			return nil, errNotLink
		}
		l.Target = linkInfoPath(b[start : start+int(size)])
		c.off = start + int(size)
	}
	unicode := flags&isUnicode != 0
	for _, f := range []struct {
		bit uint32
		dst *string
	}{{hasName, &l.Name}, {hasRelPath, &l.RelPath}, {hasWorkDir, &l.WorkDir}, {hasArgs, &l.Args}, {hasIcon, &l.Icon}} {
		if flags&f.bit == 0 {
			continue
		}
		s, ok := c.str(unicode)
		if !ok {
			return nil, errNotLink
		}
		*f.dst = s
	}
	// Extra data: an EnvironmentVariableDataBlock holds the target with
	// variables unexpanded (%ProgramFiles%\…) when LinkInfo has none.
	for flags&hasExpString != 0 && c.off+8 <= len(b) {
		size := int(binary.LittleEndian.Uint32(b[c.off:]))
		if size < 8 || c.off+size > len(b) {
			break
		}
		if sig := binary.LittleEndian.Uint32(b[c.off+4:]); sig == 0xA0000001 && size >= 8+260+520 && l.Target == "" {
			block := b[c.off+8:]
			if s := utf16z(block[260 : 260+520]); s != "" {
				l.Target = s
			} else {
				l.Target = ansiz(block[:260])
			}
		}
		c.off += size
	}
	l.Target = clean(expand(l.Target))
	l.WorkDir = clean(expand(l.WorkDir))
	l.Icon = expand(l.Icon)
	return l, nil
}

// linkInfoPath returns LocalBasePath + CommonPathSuffix, preferring the
// Unicode copies when the header has them.
func linkInfoPath(info []byte) string {
	if len(info) < 0x1C {
		return ""
	}
	header := binary.LittleEndian.Uint32(info[4:])
	lflags := binary.LittleEndian.Uint32(info[8:])
	if lflags&1 == 0 { // no local volume path (a network share)
		return ""
	}
	at := func(off uint32) []byte {
		if off == 0 || int(off) >= len(info) {
			return nil
		}
		return info[off:]
	}
	if header >= 0x24 && len(info) >= 0x24 {
		base := utf16z(at(binary.LittleEndian.Uint32(info[0x1C:])))
		if base != "" {
			return base + utf16z(at(binary.LittleEndian.Uint32(info[0x20:])))
		}
	}
	return ansiz(at(binary.LittleEndian.Uint32(info[0x10:]))) + ansiz(at(binary.LittleEndian.Uint32(info[0x18:])))
}

type cursor struct {
	b   []byte
	off int
}

func (c *cursor) skip(n int) bool {
	if n < 0 || c.off+n > len(c.b) {
		return false
	}
	c.off += n
	return true
}

func (c *cursor) u16() (uint16, bool) {
	if c.off+2 > len(c.b) {
		return 0, false
	}
	v := binary.LittleEndian.Uint16(c.b[c.off:])
	c.off += 2
	return v, true
}

func (c *cursor) u32() (uint32, bool) {
	if c.off+4 > len(c.b) {
		return 0, false
	}
	v := binary.LittleEndian.Uint32(c.b[c.off:])
	c.off += 4
	return v, true
}

// str reads a StringData entry: a character count, then the characters.
func (c *cursor) str(unicode bool) (string, bool) {
	n, ok := c.u16()
	if !ok {
		return "", false
	}
	size := int(n)
	if unicode {
		size *= 2
	}
	if c.off+size > len(c.b) {
		return "", false
	}
	raw := c.b[c.off : c.off+size]
	c.off += size
	if unicode {
		return decode16(raw), true
	}
	return ansi(raw), true
}

// utf16z decodes a NUL-terminated UTF-16LE string.
func utf16z(b []byte) string {
	for i := 0; i+1 < len(b); i += 2 {
		if b[i] == 0 && b[i+1] == 0 {
			return decode16(b[:i])
		}
	}
	return ""
}

func decode16(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// ansiz decodes a NUL-terminated string in the system code page.
func ansiz(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return ansi(b[:i])
	}
	return ""
}

func clean(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}
