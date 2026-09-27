package lnk

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestArgsAndWorkDir(t *testing.T) {
	l, err := ReadFile(filepath.Join("testdata", "args.lnk"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(filepath.Base(l.Target), "notepad.exe") {
		t.Errorf("target = %q", l.Target)
	}
	if l.Args != `-windowed --profile "My Profile"` {
		t.Errorf("args = %q", l.Args)
	}
	if !strings.EqualFold(l.WorkDir, os.Getenv("WINDIR")) {
		t.Errorf("workdir = %q", l.WorkDir)
	}
	if l.Name != "Play Notepad" {
		t.Errorf("name = %q", l.Name)
	}
}

func TestUnicodeTarget(t *testing.T) {
	l, err := ReadFile(filepath.Join("testdata", "unicode.lnk"))
	if err != nil {
		t.Fatal(err)
	}
	want := `C:\WLFixture\Café Ünïcode Game\bin\game.exe`
	if l.Target != want {
		t.Errorf("target = %q, want %q", l.Target, want)
	}
	if l.WorkDir != `C:\WLFixture\Café Ünïcode Game\bin` {
		t.Errorf("workdir = %q", l.WorkDir)
	}
}

func TestRejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("hello"), make([]byte, 200)} {
		if _, err := Parse(b); err == nil {
			t.Errorf("Parse(%d bytes) accepted garbage", len(b))
		}
	}
}

// envLink builds a link with no LinkInfo whose target is only in an
// EnvironmentVariableDataBlock of the given size.
func envLink(ansi, wide string, size int) []byte {
	b := make([]byte, 0x4C)
	binary.LittleEndian.PutUint32(b, 0x4C)
	copy(b[4:20], linkCLSID)
	binary.LittleEndian.PutUint32(b[0x14:], hasExpString|isUnicode)
	block := make([]byte, size)
	binary.LittleEndian.PutUint32(block, uint32(size))
	binary.LittleEndian.PutUint32(block[4:], 0xA0000001)
	if size >= 8+260+520 {
		copy(block[8:8+260], ansi)
		for i, r := range utf16.Encode([]rune(wide)) {
			binary.LittleEndian.PutUint16(block[8+260+2*i:], r)
		}
	}
	b = append(b, block...)
	return append(b, 0, 0, 0, 0)
}

func TestEnvironmentTarget(t *testing.T) {
	want := filepath.Join(os.Getenv("WINDIR"), "notepad.exe")
	for _, tc := range []struct {
		name       string
		ansi, wide string
		size       int
		want       string
	}{
		{"unicode", "", `%WINDIR%\notepad.exe`, 0x314, want},
		{"ansi", `%WINDIR%\notepad.exe`, "", 0x314, want},
		{"block too small", "", "", 0x100, ""},
	} {
		l, err := Parse(envLink(tc.ansi, tc.wide, tc.size))
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !strings.EqualFold(l.Target, tc.want) {
			t.Errorf("%s: target = %q, want %q", tc.name, l.Target, tc.want)
		}
	}
}

// Truncated links must fail cleanly, never panic.
func TestTruncated(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "unicode.lnk"))
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < len(b); n += 7 {
		_, _ = Parse(b[:n])
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"args.lnk", "unicode.lnk"} {
		if b, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Parse(b) })
}
