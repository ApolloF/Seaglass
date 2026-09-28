package scan

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/gamekit/steam"
)

// Options control a scan.
type Options struct {
	Folders        []string // watched folders from Settings
	AutoFolders    bool     // also look in common game folders on every drive
	DetectExternal bool     // look for Steam API emulators in game folders
	Signed         func(path string) bool
}

// Result is one scan's outcome.
type Result struct {
	Games    []Candidate
	Took     time.Duration
	PerStore map[Source]int
}

// Run scans this PC. Every source is best-effort: one failing never stops the others.
func Run(ctx context.Context, o Options) Result {
	start := time.Now()
	if o.Signed == nil {
		o.Signed = steam.Signed
	}
	steamRoot := SteamDir()
	folders := append([]string(nil), o.Folders...)
	if o.AutoFolders {
		folders = append(folders, AutoFolders()...)
	}

	type part struct {
		name string
		run  func() []Candidate
	}
	parts := []part{
		{"steam", func() []Candidate { return steamCandidates(steamRoot) }},
		{"epic", epicCandidates},
		{"gog", gogCandidates},
		{"ea", eaCandidates},
		{"ubisoft", ubisoftCandidates},
		{"xbox", xboxCandidates},
		{"installers", func() []Candidate { return installerCandidates(uninstallEntries()) }},
		{"folders", func() []Candidate { return folderCandidates(folders) }},
	}
	results := make([][]Candidate, len(parts))
	var scs []shortcut
	var wg sync.WaitGroup
	for i, p := range parts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer recoverLog(p.name)
			results[i] = p.run()
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer recoverLog("shortcuts")
		scs = shortcuts(shortcutDirs())
	}()
	wg.Wait()

	var all []Candidate
	for _, r := range results {
		all = append(all, r...)
	}
	games := merge(all)
	games = append(games, shortcutOnly(scs, games)...)
	if ctx.Err() != nil {
		return Result{Took: time.Since(start)}
	}

	// Per-game work reads each folder once: emulator markers, GOG info,
	// the shortcut and the executable. Bounded, it stays gentle on disks.
	sem := make(chan struct{}, 4)
	for i := range games {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *Candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			defer recoverLog("enrich " + c.Dir)
			if ctx.Err() != nil {
				return
			}
			enrich(c, scs, o)
		}(&games[i])
	}
	wg.Wait()

	sort.Slice(games, func(i, j int) bool { return SortTitle(games[i].Title) < SortTitle(games[j].Title) })
	per := map[Source]int{}
	for _, g := range games {
		per[g.Source]++
	}
	return Result{Games: games, Took: time.Since(start), PerStore: per}
}

// enrich fills in what the source didn't know: emulator, AppID, GOG
// details, the shortcut's launch target and a fallback executable.
func enrich(c *Candidate, scs []shortcut, o Options) {
	if o.DetectExternal && c.Source != Xbox {
		em := detectEmulationCached(c.Dir, o.Signed)
		c.Emulator, c.EmuMarker, c.EmuDir, c.PadHint = em.Emulator, em.Marker, em.EmuDir, em.PadHint
		if em.AppID > 0 && c.SteamAppID == 0 {
			c.SteamAppID, c.AppIDFrom = em.AppID, em.AppIDFrom
		}
		if em.Gog != nil {
			if c.GogID == "" {
				c.GogID = em.Gog.GameID
			}
			if c.Source != GOG {
				c.DRMFree = "GOG"
				if !c.TitleTrusted && em.Gog.Name != "" {
					c.Title, c.TitleTrusted = em.Gog.Name, true
				}
			}
			if c.Exe == "" {
				if exe, args, work := em.Gog.primaryTask(c.Dir); exe != "" && platform.IsFile(exe) {
					c.Exe, c.Args, c.WorkDir = exe, args, work
				}
			}
		}
	}
	// A store that launches through its own link keeps it; the executable is
	// still recorded for the icon and, later, for tracking.
	if s, ok := bestShortcut(scs, c.Dir, c.Title); ok && (c.Exe == "" || c.Source == Installer || c.Source == Folder) {
		c.Exe, c.Args, c.WorkDir = s.Target, s.Args, s.Dir
	}
	if c.Exe != "" && !platform.IsFile(c.Exe) {
		c.Exe, c.Args, c.WorkDir = "", "", ""
	}
	if c.Exe == "" {
		c.Exe = pickExeCached(c.Dir, c.Title)
	}
	if c.Emulator != "" && c.Source == Steam {
		c.How = "Steam library, files modified (" + c.EmuMarker + ")"
	}
	if c.Source == Steam && c.LaunchURI == "" && c.SteamAppID > 0 {
		c.LaunchURI = "steam://rungameid/" + strconv.Itoa(c.SteamAppID)
	}
	if c.SizeBytes == 0 && c.Source != Steam {
		c.SizeBytes = dirSize(c.Dir, 50000)
	}
}

// merge combines candidates that point at the same folder, keeping the
// most trusted source's record and filling gaps from the others. A
// candidate inside another's folder (an installer entry for a subfolder)
// is folded into the outer one.
func merge(all []Candidate) []Candidate {
	sort.SliceStable(all, func(i, j int) bool { return rank[all[i].Source] > rank[all[j].Source] })
	var out []Candidate
	byKey := map[string]int{}
	for _, c := range all {
		c.Dir = filepath.Clean(c.Dir)
		k := platform.Key(c.Dir)
		i, ok := byKey[k]
		if !ok {
			for j := range out {
				if platform.Within(out[j].Dir, c.Dir) || platform.Within(c.Dir, out[j].Dir) {
					i, ok = j, true
					break
				}
			}
		}
		if !ok {
			byKey[k] = len(out)
			out = append(out, c)
			continue
		}
		m := &out[i]
		if m.Exe == "" && c.Exe != "" && platform.Within(m.Dir, c.Exe) {
			m.Exe, m.Args, m.WorkDir = c.Exe, c.Args, c.WorkDir
		}
		if !m.TitleTrusted && c.TitleTrusted {
			m.Title, m.TitleTrusted = c.Title, true
		}
		if m.Publisher == "" {
			m.Publisher = c.Publisher
		}
		if m.Repacker == "" {
			m.Repacker = c.Repacker
		}
		if m.SizeBytes < c.SizeBytes {
			m.SizeBytes = c.SizeBytes
		}
		if m.GogID == "" {
			m.GogID = c.GogID
		}
		if m.SteamAppID == 0 {
			m.SteamAppID = c.SteamAppID
		}
		// A repack's installer entry beats a bare folder match.
		if m.Source == Folder && c.Source == Installer {
			m.Source, m.How = Installer, c.How
		}
	}
	return out
}

// shortcutOnly adds games known only from a shortcut whose target folder
// clearly holds a game (copied-in games often only get a desktop shortcut).
func shortcutOnly(scs []shortcut, known []Candidate) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	for _, s := range scs {
		dir := filepath.Dir(s.Target)
		// Games often keep their exe in bin\ or Binaries\Win64\; walk up to
		// the folder the shortcut's own name matches, at most two levels.
		for up := 0; up < 2; up++ {
			parent := filepath.Dir(dir)
			if parent == dir || Normalize(filepath.Base(dir)) == Normalize(s.Name) {
				break
			}
			if isGameSubdir(filepath.Base(dir)) {
				dir = parent
			}
		}
		k := platform.Key(dir)
		if seen[k] || filepath.Dir(dir) == dir || k == platform.Key(platform.ProgramFiles) || k == platform.Key(platform.ProgramFilesX86) {
			continue
		}
		covered := false
		for _, c := range known {
			if platform.Within(c.Dir, s.Target) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		if ok, sign := looksLikeGame(dir, false); ok && !isLauncher(s.Name) {
			seen[k] = true
			title := CleanTitle(s.Name)
			if len(title) > 5 && (title[:5] == "Play " || title[:5] == "play ") {
				title = title[5:]
			}
			out = append(out, Candidate{
				Title: title, Dir: dir, Exe: s.Target, Args: s.Args, WorkDir: s.Dir, Source: Shortcut,
				How: "Shortcut " + filepath.Base(s.Path) + " (" + sign + ")",
			})
		}
	}
	return out
}

func isGameSubdir(name string) bool {
	switch strings.ToLower(name) {
	case "bin", "bin64", "binaries", "win64", "x64", "game", "retail", "shipping":
		return true
	}
	return false
}

func recoverLog(what string) {
	if r := recover(); r != nil {
		logx.Printf("scan %s: panic: %v", what, r)
	}
}
