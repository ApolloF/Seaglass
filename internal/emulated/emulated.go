// Package emulated finds console games played on emulators installed on
// this PC: the emulators (RPCS3, Dolphin, Cemu), the games their own
// settings and caches list, the game folders those settings point at, and
// how to start each game full screen.
//
// Everything read here is a file anyone could have written: each one is
// read with a size limit, folder walks are bounded, and paths found in
// files are cleaned and checked before they're used.
package emulated

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
)

// Game is one console game an installed emulator can play.
type Game struct {
	Title     string // shown; a second copy of the same game gets its serial added
	BaseTitle string // the title without that, for art lookups
	Platform  string // "PS3", "GameCube", "Wii", "Wii U"
	Emulator  string // "RPCS3", "Dolphin", "Cemu"
	EmuDir    string // the emulator's folder: its processes are the game while it runs
	Exe       string // the emulator's program
	Args      string // starts File full screen, without the emulator's own interface
	File      string // what the emulator boots (EBOOT.BIN, a disc image, an .rpx)
	Dir       string // the game's own folder
	Serial    string // the game's id on its console (BLES01689, RMCE01, 000500001010EC00)
	SizeBytes int64
	How       string // how it was found
	TitleFrom string // where the title came from
	// Pictures that came with the game (PS3 discs carry a 1920×1080
	// background and an icon) or that the emulator downloaded (Dolphin's
	// covers). Paths of PNG files; "" when there's none.
	Cover, Backdrop, Icon string

	listed bool // the emulator's own list has it (not only a folder walk)
	remote bool // on a network drive
}

// Key is the game's identity in the library: what's booted, so two
// copies of a game, or two games in one folder, stay apart.
func (g Game) Key() string { return keyPrefix + platform.Key(g.File) }

const keyPrefix = "emu:"

// IsKey reports whether a library key is a console game's.
func IsKey(k string) bool { return strings.HasPrefix(k, keyPrefix) }

// install is one emulator found on this PC.
type install struct {
	emu *emulator
	dir string // its folder
	exe string // its program
	// titles is the emulator's own title database (Dolphin's GameTDB
	// lists), read once when a game needs it.
	titles map[string]string
	loaded bool
}

// root is a folder an emulator's settings say games are in.
type root struct {
	path  string
	depth int // how many folders down to look: 0 is only its files
}

// emulator is one emulator Seaglass knows. Supporting another one
// (RetroArch, Ryujinx, PCSX2) means adding one of these to emulators.
type emulator struct {
	name string
	exes []string // its program, as named in its folder (lower-case)
	// listed returns the games the emulator's own settings and caches
	// name, and the folders it looks for more in.
	listed func(in *install) ([]Game, []root)
	// exts are the files it plays, for the walk through the folders.
	exts map[string]bool
	// file reads one of those; ok is false when it isn't a game it plays.
	file func(in *install, path string, size int64) (Game, bool)
	// folder recognises a folder that is a game in itself (an unpacked
	// PS3 disc, a Wii U game's code, content and meta folders).
	folder func(in *install, dir string, names map[string]bool) (Game, bool)
	// args are the command-line arguments that boot file full screen.
	args func(file string) string
}

// emulators are the ones Seaglass knows, in the order a file two of them
// could play is offered to them.
var emulators = []*emulator{rpcs3, dolphin, cemu}

// ErrIncomplete means Find didn't get to look everywhere (it was
// cancelled, ran out of time or hit something it couldn't read): games it
// didn't return may still be there.
var ErrIncomplete = errors.New("console games: not all folders were looked through")

// Find returns the console games of the emulators installed on this PC.
// What can't be read is left out; ErrIncomplete comes with the games it
// did find when it didn't finish.
func Find(ctx context.Context) (games []Game, err error) {
	defer recoverLog("find", &err) // finding the emulators, or sorting out copies
	return find(ctx, findInstalls(installDirs()), time.Now().Add(walkBudget))
}

func find(ctx context.Context, ins []*install, deadline time.Time) ([]Game, error) {
	var games []Game
	var roots []root
	var err error
	for _, in := range ins {
		func() {
			defer recoverLog(in.emu.name, &err)
			gs, rs := in.emu.listed(in)
			for i := range gs {
				gs[i].listed = true
				games = append(games, in.fill(gs[i]))
			}
			roots = append(roots, rs...)
		}()
	}
	w := &walker{ctx: ctx, ins: ins, deadline: deadline, seen: map[string]bool{}}
	for _, g := range games {
		w.seen[platform.Key(g.File)] = true
	}
	for _, r := range mergeRoots(roots) {
		func() {
			defer recoverLog("walk "+r.path, &err)
			w.walk(r)
		}()
	}
	if w.stopped != "" {
		logx.Printf("emulated games: folder walk stopped early (%s)", w.stopped)
		err = ErrIncomplete
	}
	games = append(games, w.found...)
	games = collapse(games)
	nameCopies(games)
	sort.Slice(games, func(i, j int) bool {
		if a, b := scan.SortTitle(games[i].Title), scan.SortTitle(games[j].Title); a != b {
			return a < b
		}
		return platform.Key(games[i].File) < platform.Key(games[j].File)
	})
	if err == nil && ctx.Err() != nil {
		err = ErrIncomplete
	}
	return games, err
}

// fill completes a game an emulator found with what's the same for all
// of its games.
func (in *install) fill(g Game) Game {
	g.Emulator, g.EmuDir, g.Exe = in.emu.name, in.dir, in.exe
	g.Args = in.emu.args(g.File)
	if g.Dir == "" {
		g.Dir = filepath.Dir(g.File)
	}
	if g.BaseTitle == "" {
		g.BaseTitle = g.Title
	}
	g.remote = remote(g.File)
	return g
}

// collapse keeps one of several copies of the same game: two files with
// the same serial, or with the same name and size (a game copied from a
// network drive to a local one). A local copy beats one on the network,
// then a copy the emulator lists beats one only the walk found. What the
// kept copy lacks (its serial, a title from the emulator, pictures) comes
// from the others.
func collapse(games []Game) []Game {
	var out []Game
	byFile := map[string]bool{}
	for _, g := range games {
		k := platform.Key(g.File)
		if byFile[k] {
			continue // two installs of an emulator listing the same file
		}
		byFile[k] = true
		i := -1
		for j := range out {
			if sameGame(out[j], g) {
				i = j
				break
			}
		}
		if i < 0 {
			out = append(out, g)
			continue
		}
		keep, other := out[i], g
		if better(other, keep) {
			keep, other = other, keep
		}
		if keep.Serial == "" {
			keep.Serial = other.Serial
		}
		if !keep.listed && other.listed && other.TitleFrom != "" {
			keep.Title, keep.BaseTitle, keep.TitleFrom = other.Title, other.BaseTitle, other.TitleFrom
		}
		if keep.Cover == "" {
			keep.Cover = other.Cover
		}
		if keep.Backdrop == "" {
			keep.Backdrop = other.Backdrop
		}
		if keep.Icon == "" {
			keep.Icon = other.Icon
		}
		out[i] = keep
	}
	return out
}

func sameGame(a, b Game) bool {
	if a.Platform != b.Platform {
		return false
	}
	if a.Serial != "" && b.Serial != "" {
		return strings.EqualFold(a.Serial, b.Serial)
	}
	if !strings.EqualFold(filepath.Base(a.File), filepath.Base(b.File)) {
		return false
	}
	// A file on a network drive isn't opened, so its size is unknown.
	if a.remote && a.SizeBytes == 0 || b.remote && b.SizeBytes == 0 {
		return true
	}
	return a.SizeBytes > 0 && a.SizeBytes == b.SizeBytes
}

// better says whether a is the copy to keep rather than b.
func better(a, b Game) bool {
	if a.remote != b.remote {
		return !a.remote
	}
	if a.listed != b.listed {
		return a.listed
	}
	return len(a.File) < len(b.File)
}

// nameCopies adds the serial to the titles of different copies of one
// game that are still both there (two regions' discs), so they can be
// told apart.
func nameCopies(games []Game) {
	n := map[string]int{}
	key := func(g Game) string { return g.Platform + "|" + scan.LooseKey(g.BaseTitle) }
	for _, g := range games {
		n[key(g)]++
	}
	for i, g := range games {
		if n[key(g)] > 1 && g.Serial != "" {
			games[i].Title = g.BaseTitle + " (" + g.Serial + ")"
		}
	}
}

// mergeRoots drops folders another one already covers.
func mergeRoots(rs []root) []root {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].depth != rs[j].depth {
			return rs[i].depth > rs[j].depth
		}
		return len(rs[i].path) < len(rs[j].path)
	})
	var out []root
	for _, r := range rs {
		if r.path == "" || !filepath.IsAbs(r.path) {
			continue
		}
		r.path = filepath.Clean(r.path)
		covered := false
		for i, o := range out {
			if platform.Key(o.path) == platform.Key(r.path) {
				out[i].depth = max(o.depth, r.depth)
				covered = true
				break
			}
			if o.depth >= maxDepth && platform.Within(o.path, r.path) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, r)
		}
	}
	return out
}

// findInstalls looks for each emulator's program in the folders given,
// and in their subfolders (C:\Emulators\RPCS3\rpcs3.exe). The first
// folder an emulator is found in is the one used.
func findInstalls(dirs []string) []*install {
	var out []*install
	have := map[*emulator]bool{}
	check := func(dir string) {
		names := lowerNames(dir, 400)
		for _, e := range emulators {
			if have[e] {
				continue
			}
			for _, x := range e.exes {
				if real, ok := names[x]; ok {
					exe := filepath.Join(dir, real)
					if isFile(exe) {
						have[e] = true
						out = append(out, &install{emu: e, dir: filepath.Clean(dir), exe: exe})
						break
					}
				}
			}
		}
	}
	for _, d := range dirs {
		if !isDir(d) {
			continue
		}
		check(d)
		ents, err := readDir(d, 400)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() && looksLikeEmulator(e.Name()) {
				check(filepath.Join(d, e.Name()))
			}
		}
	}
	return out
}

// looksLikeEmulator keeps the walk for emulators away from folders that
// are clearly something else (Program Files holds hundreds).
func looksLikeEmulator(name string) bool {
	n := strings.ToLower(name)
	for _, e := range emulators {
		if strings.Contains(n, strings.ToLower(e.name)) {
			return true
		}
	}
	return strings.Contains(n, "emu")
}

// installDirs are the folders emulators are usually unpacked or installed
// in: an Emulators (or Emulation, Emu) folder on any fixed drive, also
// inside a Games folder, Program Files, the user's programs and Scoop.
func installDirs() []string {
	var out []string
	for _, d := range platform.FixedDrives() {
		for _, rel := range []string{"Emulators", "Emulation", "Emu", `Games\Emulators`, `Games\Emulation`} {
			out = append(out, filepath.Join(d, rel))
		}
	}
	for _, d := range []string{platform.ProgramFiles, platform.ProgramFilesX86} {
		if d != "" {
			out = append(out, d)
		}
	}
	if platform.Local != "" {
		out = append(out, filepath.Join(platform.Local, "Programs"))
	}
	if platform.Profile != "" {
		scoop := filepath.Join(platform.Profile, "scoop", "apps")
		for _, e := range []string{"rpcs3", "dolphin", "dolphin-dev", "cemu"} {
			out = append(out, filepath.Join(scoop, e, "current"))
		}
	}
	return out
}

// lowerNames lists a folder's entries by lower-cased name.
func lowerNames(dir string, limit int) map[string]string {
	ents, err := readDir(dir, limit)
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(ents))
	for _, e := range ents {
		m[strings.ToLower(e.Name())] = e.Name()
	}
	return m
}

// readDir lists at most limit entries of a folder.
func readDir(dir string, limit int) ([]os.DirEntry, error) {
	if remote(dir) {
		return nil, errRemote
	}
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ents, err := f.ReadDir(limit)
	if err == io.EOF {
		err = nil
	}
	return ents, err
}

// readSmall reads a whole file of at most limit bytes; larger files are
// refused rather than cut short.
func readSmall(p string, limit int64) ([]byte, error) {
	if remote(p) {
		return nil, errRemote
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errTooLarge
	}
	return b, nil
}

// readAt reads n bytes at off (a disc image's header).
func readAt(p string, off int64, n int) ([]byte, error) {
	if remote(p) {
		return nil, errRemote
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := make([]byte, n)
	if _, err := f.ReadAt(b, off); err != nil {
		return nil, err
	}
	return b, nil
}

type tooLarge struct{}

func (tooLarge) Error() string { return "file too large" }

var errTooLarge error = tooLarge{}

// pathFrom turns a path an emulator's settings hold (often with forward
// slashes, sometimes relative to the emulator) into a clean absolute one,
// or "" when it isn't one.
func pathFrom(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsAny(p, "\x00\r\n") || len(p) > 1024 {
		return ""
	}
	p = filepath.FromSlash(p)
	if !filepath.IsAbs(p) {
		if base == "" || strings.HasPrefix(p, `\`) {
			return ""
		}
		p = filepath.Join(base, p)
	}
	p = filepath.Clean(p)
	if strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`) {
		return "" // device paths
	}
	return p
}

// quote makes one command-line argument of a path.
func quote(p string) string { return syscall.EscapeArg(p) }

// titleCase turns an all-capitals title ("MARIO KART 8", as discs and
// Cemu's cache often have them) into "Mario Kart 8". Titles with any
// lower-case letter are left as they are.
func titleCase(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	for _, r := range s {
		if unicode.IsLower(r) {
			return s
		}
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		rs := []rune(w)
		rs[0] = unicode.ToUpper(rs[0])
		words[i] = string(rs)
	}
	return strings.Join(words, " ")
}

// cleanName makes a title of a file or folder name: "RMCE01 Mario Kart
// Wii" → "Mario Kart Wii", "Skylanders Giants [BLES01689]" → "Skylanders
// Giants", "Mario Kart Double Dash (USA).nkit.iso" → "Mario Kart Double Dash".
func cleanName(name string) string {
	low := strings.ToLower(name)
	for _, ext := range []string{".nkit.iso", ".nkit.gcz", ".nkit.rvz"} {
		if strings.HasSuffix(low, ext) {
			name = name[:len(name)-len(ext)]
		}
	}
	if ext := filepath.Ext(name); ext != "" && len(ext) <= 5 && !strings.Contains(ext, " ") {
		name = strings.TrimSuffix(name, ext)
	}
	if f := strings.Fields(name); len(f) > 1 && isDiscID(f[0]) {
		name = strings.Join(f[1:], " ")
	}
	return titleCase(scan.CleanTitle(name))
}

// isDiscID reports whether s looks like a GameCube or Wii game id (RMCE01).
func isDiscID(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// cleanTitle tidies a title read from a game's own files: no control
// characters or trademark signs, single spaces (a PS3 title can have a
// line break in it).
func cleanTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '™' || r == '®' || r == '©':
			return -1
		case unicode.IsControl(r) || r == unicode.ReplacementChar:
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return strings.ToValidUTF8(s, "")
}

// remote reports whether a path is on a network drive; tests stand in
// for one with it.
var remote = isRemote

var errRemote = errors.New("on a network drive")

// isDir and isFile never touch a path on a network drive: one whose
// server is away can hang a scan for minutes. Every file this package
// reads goes through them or readDir, readSmall and readAt.
func isDir(p string) bool  { return !remote(p) && platform.IsDir(p) }
func isFile(p string) bool { return !remote(p) && platform.IsFile(p) }

func isRemote(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	vol := filepath.VolumeName(p)
	if vol == "" {
		return false
	}
	return platform.DriveRemote(vol + `\`)
}

// recoverLog, deferred, logs a panic of a step and makes *err
// ErrIncomplete: the games the step would have found are missing.
func recoverLog(what string, err *error) {
	if r := recover(); r != nil {
		logx.Printf("emulated games %s: panic: %v", what, r)
		*err = ErrIncomplete
	}
}

// nameSet is the set of a folder's entries, by lower-cased name.
func nameSet(dir string, limit int) map[string]bool {
	m := map[string]bool{}
	for k := range lowerNames(dir, limit) {
		m[k] = true
	}
	return m
}
