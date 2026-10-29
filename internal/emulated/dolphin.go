package emulated

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// dolphin is the GameCube and Wii emulator. Its game folders are in
// Config\Dolphin.ini ([General] ISOPath0…, RecursiveISOPaths); its user
// folder is next to it when it's portable (portable.txt), else in the
// user's Documents or AppData.
var dolphin = &emulator{
	name: "Dolphin",
	exes: []string{"dolphin.exe"},
	listed: func(in *install) ([]Game, []root) {
		b, err := readSmall(filepath.Join(dolphinUserDir(in.dir), "Config", "Dolphin.ini"), 256<<10)
		if err != nil {
			return nil, nil
		}
		general := parseINI(b)["general"]
		depth := 0 // only the files in each folder
		if strings.EqualFold(general["recursiveisopaths"], "true") {
			depth = maxDepth
		}
		var roots []root
		n, _ := strconv.Atoi(general["isopaths"])
		for i := 0; i < min(n, 64); i++ {
			if p := pathFrom("", general["isopath"+strconv.Itoa(i)]); p != "" {
				roots = append(roots, root{path: p, depth: depth})
			}
		}
		return nil, roots
	},
	exts: map[string]bool{".iso": true, ".gcm": true, ".wbfs": true, ".ciso": true, ".gcz": true, ".wia": true, ".rvz": true},
	file: func(in *install, path string, size int64) (Game, bool) {
		d, ok := readDisc(path)
		if !ok {
			return Game{}, false // a PS2 or PC disc image, say
		}
		g := Game{Platform: d.platform, File: path, Serial: d.id, SizeBytes: size}
		if t := in.titleDB()[d.id]; t != "" {
			g.Title, g.TitleFrom = t, "Dolphin game database"
		} else if t := in.titleDB()[d.id[:min(4, len(d.id))]]; d.id != "" && t != "" {
			g.Title, g.TitleFrom = t, "Dolphin game database"
		} else if strings.Contains(d.title, " ") {
			g.Title, g.TitleFrom = titleCase(d.title), "disc"
		} else {
			g.Title = cleanName(filepath.Base(path))
		}
		g.BaseTitle = g.Title
		if d.id != "" {
			if p := filepath.Join(dolphinUserDir(in.dir), "Cache", "GameCovers", d.id+".png"); platform.IsFile(p) {
				g.Cover = p
			}
		}
		return g, g.Title != ""
	},
	// -b closes Dolphin when the game stops; -C applies only to this run.
	args: func(file string) string { return "-b -C Main.Display.Fullscreen=True -e " + quote(file) },
}

// dolphinUserDir is where Dolphin keeps its settings.
func dolphinUserDir(dir string) string {
	if platform.IsFile(filepath.Join(dir, "portable.txt")) {
		return filepath.Join(dir, "User")
	}
	if platform.Documents != "" {
		if d := filepath.Join(platform.Documents, "Dolphin Emulator"); platform.IsDir(d) {
			return d
		}
	}
	return filepath.Join(platform.Roaming, "Dolphin Emulator")
}

// titleDB is Dolphin's English GameTDB title list, read once.
func (in *install) titleDB() map[string]string {
	if !in.loaded {
		in.loaded = true
		if b, err := readSmall(filepath.Join(in.dir, "Sys", "wiitdb-en.txt"), 8<<20); err == nil {
			in.titles = parseTitleDB(b)
		}
	}
	return in.titles
}

// parseINI reads an ini file into sections of keys, all lower-cased.
func parseINI(b []byte) map[string]map[string]string {
	out := map[string]map[string]string{}
	sec := ""
	sc := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	sc.Buffer(make([]byte, 4096), 4096)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line[0] == ';' || line[0] == '#':
		case line[0] == '[' && strings.HasSuffix(line, "]"):
			sec = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
		default:
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if out[sec] == nil {
				out[sec] = map[string]string{}
			}
			if len(out[sec]) < 1000 {
				out[sec][strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}
	}
	return out
}
