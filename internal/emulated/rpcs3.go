package emulated

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// rpcs3 is the PlayStation 3 emulator. Its game list is config\games.yml
// (serial: folder), plus the games installed to its virtual hard disk
// (dev_hdd0\game). Games are unpacked discs (PS3_GAME\USRDIR\EBOOT.BIN)
// or installed packages (USRDIR\EBOOT.BIN), booted by their EBOOT.BIN.
var rpcs3 = &emulator{
	name: "RPCS3",
	exes: []string{"rpcs3.exe"},
	listed: func(in *install) ([]Game, []root) {
		var games []Game
		var roots []root
		cfg := filepath.Join(in.dir, "config")
		if !platform.IsDir(cfg) {
			cfg = in.dir // before 0.0.30 its files were next to the program
		}
		if b, err := readSmall(filepath.Join(cfg, "games.yml"), 1<<20); err == nil {
			for _, p := range parseGamesYML(b) {
				dir := pathFrom(in.dir, p)
				if dir == "" {
					continue
				}
				if g, ok := ps3Game(dir); ok {
					g.How = "RPCS3 game list"
					games = append(games, g)
				}
				// Other games are often unpacked next to the ones RPCS3 has
				// booted (a second copy of a disc RPCS3 forgot).
				roots = append(roots, root{path: filepath.Dir(dir), depth: 1})
			}
		}
		hdd := filepath.Join(in.dir, "dev_hdd0")
		if b, err := readSmall(filepath.Join(cfg, "vfs.yml"), 64<<10); err == nil {
			if p := vfsHDD(b, in.dir); p != "" {
				hdd = p
			}
		}
		for _, sub := range []string{"game", "disc"} {
			base := filepath.Join(hdd, sub)
			ents, err := readDir(base, 2000)
			if err != nil {
				continue
			}
			for _, e := range ents {
				if !e.IsDir() {
					continue
				}
				if g, ok := ps3Game(filepath.Join(base, e.Name())); ok {
					g.How = "RPCS3 installed games"
					games = append(games, g)
				}
			}
		}
		return games, roots
	},
	folder: func(in *install, dir string, names map[string]bool) (Game, bool) {
		if !names["ps3_game"] && !(names["param.sfo"] && names["usrdir"]) {
			return Game{}, false
		}
		return ps3Game(dir)
	},
	args: func(file string) string { return "--no-gui --fullscreen " + quote(file) },
}

// ps3Game reads a PS3 game's folder: an unpacked disc, or a game
// installed from a package. Game data and updates (which can't be booted)
// aren't games.
func ps3Game(dir string) (Game, bool) {
	base := dir
	if platform.IsDir(filepath.Join(dir, "PS3_GAME")) {
		base = filepath.Join(dir, "PS3_GAME")
	}
	eboot := filepath.Join(base, "USRDIR", "EBOOT.BIN")
	if !platform.IsFile(eboot) {
		return Game{}, false
	}
	b, err := readSmall(filepath.Join(base, "PARAM.SFO"), maxSFO)
	if err != nil {
		return Game{}, false
	}
	sfo, err := parseSFO(b)
	if err != nil {
		return Game{}, false
	}
	switch sfo["CATEGORY"] {
	case "DG", "HG": // disc game, hard disk game
	default:
		return Game{}, false
	}
	serial := sfo["TITLE_ID"]
	if !isSerial(serial) {
		serial = ""
	}
	g := Game{Platform: "PS3", File: eboot, Dir: dir, Serial: serial, Title: cleanTitle(sfo["TITLE"]), TitleFrom: "PARAM.SFO"}
	if g.Title == "" {
		g.Title, g.TitleFrom = cleanName(filepath.Base(dir)), "folder name"
	}
	g.BaseTitle = g.Title
	if p := filepath.Join(base, "PIC1.PNG"); platform.IsFile(p) {
		g.Backdrop = p
	}
	if p := filepath.Join(base, "ICON0.PNG"); platform.IsFile(p) {
		g.Icon = p
	}
	return g, g.Title != ""
}

// isSerial reports whether s looks like a PS3 title id (BLES01689, NPEB00123).
func isSerial(s string) bool {
	if len(s) != 9 {
		return false
	}
	for i, r := range s {
		if i < 4 && !(r >= 'A' && r <= 'Z') || i >= 4 && !(r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// parseGamesYML reads RPCS3's games.yml, one "SERIAL: path" per line,
// and returns the paths.
func parseGamesYML(b []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 4096)
	for sc.Scan() && len(out) < 2000 {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		if v = yamlString(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// vfsHDD finds where RPCS3's virtual hard disk is in its vfs.yml
// ("/dev_hdd0/: $(EmulatorDir)dev_hdd0/").
func vfsHDD(b []byte, emuDir string) string {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 4096), 4096)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), ": ")
		if !ok || strings.Trim(k, `"'`) != "/dev_hdd0/" {
			continue
		}
		v = yamlString(v)
		v = strings.ReplaceAll(v, "$(EmulatorDir)", filepath.ToSlash(emuDir)+"/")
		return pathFrom(emuDir, v)
	}
	return ""
}

// yamlString reads a YAML scalar as RPCS3 writes them: plain, or in
// single or double quotes.
func yamlString(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		q := v[0]
		v = v[1 : len(v)-1]
		if q == '"' {
			v = strings.ReplaceAll(v, `\\`, `\`)
			v = strings.ReplaceAll(v, `\"`, `"`)
		} else {
			v = strings.ReplaceAll(v, `''`, `'`)
		}
	}
	return v
}
