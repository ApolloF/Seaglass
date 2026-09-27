package launch

import (
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// helpers are crash handlers and reporters games ship in their folder.
// They can outlive the game (most of all when it's ended from Task
// Manager, or crashed), and must not keep its session going.
var helpers = map[string]bool{
	"unitycrashhandler64.exe": true,
	"unitycrashhandler32.exe": true,
	"crashreportclient.exe":   true, // Unreal
	"crashpad_handler.exe":    true,
	"crashreporter.exe":       true,
	"crashsender1403.exe":     true, // CrashRpt
	"bssndrpt.exe":            true, // BugSplat
	"bssndrpt64.exe":          true,
	"werfault.exe":            true,
}

func isHelper(path string) bool { return helpers[strings.ToLower(filepath.Base(path))] }

// procInfo is what is known about one process id.
type procInfo struct {
	path    string
	started uint64
	game    bool
	helper  bool // a crash handler: never counts as the game
}

// tracker finds the processes that belong to a running game: the process
// Seaglass started, everything it starts in turn, and anything
// running from the game's folder. The folder rule covers launchers that
// hand over to the game, and games started through a store or Steam.
type tracker struct {
	dirs  []string
	root  uint32
	self  uint32
	known map[uint32]*procInfo
	image func(pid uint32) (string, uint64, error)
}

func newTracker(dirs []string, root, self uint32, image func(uint32) (string, uint64, error)) *tracker {
	return &tracker{dirs: dirs, root: root, self: self, known: map[uint32]*procInfo{}, image: image}
}

// update takes a process list and returns the game's processes.
func (t *tracker) update(ps []platform.Proc) map[uint32]uint64 {
	alive := make(map[uint32]bool, len(ps))
	parent := make(map[uint32]uint32, len(ps))
	for _, p := range ps {
		alive[p.PID] = true
		parent[p.PID] = p.PPID
	}
	for pid := range t.known {
		if !alive[pid] {
			delete(t.known, pid)
		}
	}
	// Folder rule: look up each new process's exe once.
	for _, p := range ps {
		if p.PID == 0 || p.PID == 4 || p.PID == t.self || t.known[p.PID] != nil {
			continue
		}
		info := &procInfo{}
		if path, started, err := t.image(p.PID); err == nil {
			info.path, info.started = path, started
			if isHelper(path) {
				info.helper = true
				t.known[p.PID] = info
				continue
			}
			for _, d := range t.dirs {
				if platform.Within(d, path) {
					info.game = true
					break
				}
			}
		}
		t.known[p.PID] = info
	}
	// Tree rule: the started process and its descendants, found by walking
	// each process's parents (with a bound, as parent ids can be reused).
	out := map[uint32]uint64{}
	for _, p := range ps {
		info := t.known[p.PID]
		if info == nil || info.helper {
			continue
		}
		if info.game {
			out[p.PID] = info.started
			continue
		}
		if t.root == 0 {
			continue
		}
		for pid, n := p.PID, 0; pid != 0 && n < 16; pid, n = parent[pid], n+1 {
			if pid == t.root {
				info.game = true // stays in once seen
				out[p.PID] = info.started
				break
			}
			if pid == t.self {
				break
			}
		}
	}
	return out
}
