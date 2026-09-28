package achievements

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/library"
)

// Goldberg's Uplay emulators save achievements only when their ini says
// Achievements = 1 in [Settings] (the key the Achievement Enabler scripts
// set); many releases ship without it. EnableUplay sets it.

// uplayLoaders names the ini each Uplay loader DLL reads.
var uplayLoaders = []struct{ dll, ini string }{
	{"upc_r2_loader64.dll", "upc_r2.ini"}, {"upc_r2_loader.dll", "upc_r2.ini"},
	{"uplay_r2_loader64.dll", "uplay_r2.ini"}, {"uplay_r2_loader.dll", "uplay_r2.ini"},
	{"uplay_r1_loader64.dll", "uplay_r1.ini"}, {"uplay_r1_loader.dll", "uplay_r1.ini"},
}

// uplayINIPath is the ini to turn achievements on in: the emulator's own,
// else a new one named after its loader DLL; "" when there's neither.
func uplayINIPath(g EmuGame, c uplayConfig) string {
	if c.ini != "" {
		return c.ini
	}
	emu := g.emuPath()
	for _, l := range uplayLoaders {
		if fi, err := os.Stat(filepath.Join(emu, l.dll)); err == nil && fi.Mode().IsRegular() {
			return filepath.Join(emu, l.ini)
		}
	}
	return ""
}

// EnableUplay sets Achievements = 1 in the game's Uplay emulator ini,
// keeping everything else in it as it was, and a copy of the ini from
// before next to it (<ini>.seaglass.bak, made once). It returns the ini.
func EnableUplay(g library.Game, env Env) (string, error) {
	if !UplayEmulator(g.Emulator) || g.Dir == "" {
		return "", errors.New("not a game on a Uplay emulator")
	}
	eg := emuGame(g)
	c := readUplayConfig(eg, env)
	p := uplayINIPath(eg, c)
	if p == "" {
		return "", errors.New("no Uplay emulator ini or loader found next to the game")
	}
	if c.achOn {
		return p, nil
	}
	b, err := readSmall(p, 256<<10)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if bytes.HasPrefix(b, []byte{0xff, 0xfe}) {
		return "", errors.New(filepath.Base(p) + " is saved as UTF-16; set Achievements = 1 in it by hand")
	}
	if len(b) > 0 {
		bak := p + ".seaglass.bak"
		if _, err := os.Stat(bak); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(bak, b, 0o644); err != nil {
				return "", err
			}
		}
	}
	tmp := p + ".seaglass.tmp"
	if err := os.WriteFile(tmp, setINIKey(b, "Settings", "Achievements", "1"), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return p, nil
}

// setINIKey sets key = value in [section] of an ini, replacing the key's
// line when the section has it, else adding it right after the section's
// header, else adding the section at the end. Other lines, the BOM and
// the line endings stay as they were.
func setINIKey(b []byte, section, key, value string) []byte {
	var bom []byte
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		bom, b = b[:3], b[3:]
	}
	nl := "\r\n"
	if len(b) > 0 && !bytes.Contains(b, []byte("\r\n")) {
		nl = "\n"
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // the file's last newline
	}
	set := key + " = " + value
	header := -1
	in := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "["), "]"))
			in = strings.EqualFold(name, section)
			if in && header < 0 {
				header = i
			}
			continue
		}
		if !in || t == "" || t[0] == ';' || t[0] == '#' {
			continue
		}
		if k, _, ok := strings.Cut(t, "="); ok && strings.EqualFold(strings.TrimSpace(k), key) {
			lines[i] = set
			return join(bom, lines, nl)
		}
	}
	if header >= 0 {
		lines = append(lines[:header+1], append([]string{set}, lines[header+1:]...)...)
	} else {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "["+section+"]", set)
	}
	return join(bom, lines, nl)
}

func join(bom []byte, lines []string, nl string) []byte {
	return append(append([]byte{}, bom...), strings.Join(lines, nl)+nl...)
}
