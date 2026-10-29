package emulated

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// Bounds of the walk through the emulators' game folders. Dolphin looks
// through its folders without a depth limit; games sit a few folders down
// at most, and a folder of PC games next to them mustn't make a scan slow.
const (
	walkBudget  = 4 * time.Second
	maxEntries  = 60000 // folder entries looked at, over all folders
	maxDepth    = 5     // below a folder searched with its subfolders
	maxDirLimit = 5000  // entries read from one folder
)

// walker goes through the emulators' game folders once, offering each
// file and folder to every installed emulator.
type walker struct {
	ctx      context.Context
	ins      []*install
	deadline time.Time
	entries  int
	stopped  string
	seen     map[string]bool // files already found (listed by an emulator)
	found    []Game
}

func (w *walker) walk(r root) {
	if w.stopped != "" || !platform.IsDir(r.path) {
		return
	}
	// A network folder could take minutes to walk, or hang while its
	// server is away; the games an emulator lists there are still found.
	if isRemote(r.path) {
		return
	}
	w.dir(r.path, min(r.depth, maxDepth), true)
}

func (w *walker) dir(dir string, depth int, top bool) {
	switch {
	case w.stopped != "":
		return
	case w.ctx.Err() != nil:
		w.stopped = "cancelled"
		return
	case time.Now().After(w.deadline):
		w.stopped = "took too long"
		return
	case w.entries >= maxEntries:
		w.stopped = "too many files"
		return
	}
	ents, err := readDir(dir, maxDirLimit)
	if err != nil {
		return
	}
	w.entries += len(ents)
	names := make(map[string]bool, len(ents))
	for _, e := range ents {
		names[strings.ToLower(e.Name())] = true
	}
	// The folder may be a game in itself (an unpacked disc).
	for _, in := range w.ins {
		if in.emu.folder == nil {
			continue
		}
		if g, ok := in.emu.folder(in, dir, names); ok {
			w.add(in, g)
			return
		}
	}
	// A folder with programs in it is a PC game or an app: its files and
	// subfolders aren't console games.
	if !top && hasPrograms(names) {
		return
	}
	var files []Game
	var owners []*install
	for _, e := range ents {
		if e.IsDir() || !e.Type().IsRegular() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, in := range w.ins {
			if in.emu.file == nil || !in.emu.exts[ext] {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if w.seen[platform.Key(p)] {
				break
			}
			fi, err := e.Info()
			if err != nil {
				break
			}
			if g, ok := in.emu.file(in, p, fi.Size()); ok {
				files = append(files, g)
				owners = append(owners, in)
				break
			}
		}
	}
	// A game alone in its own folder is best named by the folder: files
	// are often named by a code ("fk-mk8.wud", "RMCE01.wbfs").
	for i, g := range files {
		if g.TitleFrom == "" && len(files) == 1 && !top {
			if t := cleanName(filepath.Base(dir)); t != "" && !isPlatformName(t) {
				g.Title, g.BaseTitle, g.TitleFrom = t, t, "folder name"
			}
		}
		if g.TitleFrom == "" {
			g.TitleFrom = "file name"
		}
		w.add(owners[i], g)
	}
	if depth <= 0 {
		return
	}
	for _, e := range ents {
		if e.IsDir() && !skipDir(e.Name()) {
			w.dir(filepath.Join(dir, e.Name()), depth-1, false)
		}
	}
}

func (w *walker) add(in *install, g Game) {
	k := platform.Key(g.File)
	if w.seen[k] {
		return
	}
	w.seen[k] = true
	g.How = in.emu.name + " game folder"
	w.found = append(w.found, in.fill(g))
}

func hasPrograms(names map[string]bool) bool {
	for n := range names {
		if strings.HasSuffix(n, ".exe") || strings.HasSuffix(n, ".dll") {
			return true
		}
	}
	return false
}

// skipDir leaves out folders no game is in.
func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
		return true
	}
	switch strings.ToLower(name) {
	case "system volume information", "windows", "node_modules", "__pycache__", "shadercache", "shaders", "cache", "logs":
		return true
	}
	return false
}

// isPlatformName reports whether a folder is named after a console
// ("Wii", "PS3"): a folder of games, not one game.
func isPlatformName(s string) bool {
	switch strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(s)) {
	case "wii", "wiiu", "gamecube", "gc", "ngc", "ps3", "playstation3", "games", "roms", "isos", "wbfs":
		return true
	}
	return false
}

// fileSize is a file's size, 0 when it can't be read.
func fileSize(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return fi.Size()
}
