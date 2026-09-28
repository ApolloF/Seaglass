package app

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/syncer"
)

// RouteExternal is a game started outside Seaglass (from Steam, a
// desktop shortcut, …) that Seaglass noticed and follows.
const RouteExternal = "external"

// externalWatch notices games started outside Seaglass: Windows says
// which process's window came to the front, and when that process runs
// from an installed game's folder, a session follows it like one started
// here (playtime, the controller let go, the PS button for the overlay),
// without hooks and without closing the interface.
type externalWatch struct {
	c      *Core
	events chan uint32

	mu      sync.Mutex
	stop    func()
	checked map[uint32]uint64 // process id → start time, looked at already
}

func newExternalWatch(c *Core) *externalWatch {
	w := &externalWatch{c: c, events: make(chan uint32, 16), checked: map[uint32]uint64{}}
	go w.run()
	return w
}

// set starts or stops watching.
func (w *externalWatch) set(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case on && w.stop == nil:
		stop, err := platform.WatchForeground(func(pid uint32) {
			select {
			case w.events <- pid:
			default: // busy: the next switch comes soon enough
			}
		})
		if err != nil {
			logx.Printf("games started elsewhere: can't watch: %v", err)
			return
		}
		w.stop = stop
	case !on && w.stop != nil:
		w.stop()
		w.stop = nil
	}
}

func (w *externalWatch) run() {
	for {
		select {
		case <-w.c.ctx.Done():
			w.set(false)
			return
		case pid := <-w.events:
			w.check(pid)
		}
	}
}

// check looks at a process that just came to the front, once.
func (w *externalWatch) check(pid uint32) {
	if w.c.Launch.Active() {
		return
	}
	path, started, err := platform.ProcessImage(pid)
	if err != nil {
		return
	}
	w.mu.Lock()
	if w.checked[pid] == started {
		w.mu.Unlock()
		return
	}
	if len(w.checked) > 512 {
		w.checked = map[uint32]uint64{}
	}
	w.checked[pid] = started
	w.mu.Unlock()

	g, ok := gameForPath(w.c.Lib.Games(), path)
	if !ok {
		return
	}
	logx.Printf("noticed %q started outside Seaglass (%s)", g.DisplayTitle(), filepath.Base(path))
	if err := w.c.Launch.Launch(w.c.ctx, w.c.externalPlan(g, pid)); err != nil {
		logx.Printf("following %q: %v", g.DisplayTitle(), err)
	}
}

// gameForPath finds the installed game whose folder holds path (the most
// specific folder when they nest). Hidden games don't count: hiding is how
// you say something isn't a game.
func gameForPath(games []library.Game, path string) (library.Game, bool) {
	var best library.Game
	bestLen := 0
	for _, g := range games {
		if !g.Installed || g.Hidden || g.Dir == "" {
			continue
		}
		for _, d := range launch.UsableDirs([]string{g.Dir}) {
			if len(d) > bestLen && platform.Within(d, path) {
				best, bestLen = g, len(d)
			}
		}
	}
	return best, bestLen > 0 && !strings.EqualFold(filepath.Base(path), "Seaglass.exe")
}

// externalPlan follows a game that is already running.
func (c *Core) externalPlan(g library.Game, pid uint32) launch.Plan {
	title := g.DisplayTitle()
	dirs := []string{g.Dir}
	if g.Exe != "" && !platform.Within(g.Dir, g.Exe) {
		dirs = append(dirs, filepath.Dir(g.Exe))
	}
	// Syncer syncs on its own and backs up every few hours; a backup right
	// after the game closes adds a restore point for that session. (Syncing
	// before playing can't apply: the game is already running.)
	var after []launch.Step
	if _, ok := syncer.Installed(); ok && c.Settings.Get().BackupSavesAfter {
		known := false
		after = append(after, c.savesAfterStep(g, &known, false))
	}
	return launch.Plan{
		GameID: g.ID, Title: title, Dirs: dirs, DetectTimeout: 30 * time.Second, After: after,
		Start: func() (uint32, string, error) {
			c.startedPlaying(g)
			return pid, RouteExternal, nil
		},
		Played: func(secs int64) { c.addPlaytime(g, secs) },
		OnRun: func() {
			heapDiag("playing")
			c.shell.setTrayTooltip("Seaglass · playing " + title)
			if m := c.padManager(); m != nil {
				if c.Settings.Get().PadWhilePlaying == "off" {
					m.SetMode(pad.Off)
				} else {
					m.SetMode(pad.Passive)
				}
			}
		},
	}
}
