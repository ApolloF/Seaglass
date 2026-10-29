package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/emulated"
	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/pad"
	"github.com/ApolloF/Seaglass/internal/platform"
)

func consoleGame() library.Game {
	f := consoleFound(emulated.Game{
		Title: "Skylanders SWAP Force (BLES01966)", BaseTitle: "Skylanders SWAP Force", Platform: "PS3", Emulator: "RPCS3",
		EmuDir: `C:\Emulators\RPCS3`, Exe: `C:\Emulators\RPCS3\rpcs3.exe`, Args: `--no-gui --fullscreen "R:\PS3\SF\PS3_GAME\USRDIR\EBOOT.BIN"`,
		File: `R:\PS3\SF\PS3_GAME\USRDIR\EBOOT.BIN`, Dir: `R:\PS3\SF`, Serial: "BLES01966", How: "RPCS3 game folder", TitleFrom: "PARAM.SFO",
	})
	return library.Game{ID: 1, Key: f.Key, Title: f.Title, Source: f.Source, SourceLabel: f.SourceLabel, Installed: true,
		Dir: f.Dir, Exe: f.Exe, Args: f.Args, WorkDir: f.WorkDir, Console: f.Console, Meta: &library.Meta{Controller: "full"}}
}

func TestConsoleFound(t *testing.T) {
	g := consoleGame()
	if g.Key != `emu:r:\ps3\sf\ps3_game\usrdir\eboot.bin` || g.Source != SourceEmulator || g.SourceLabel != "PS3" {
		t.Fatalf("got %+v", g)
	}
	if g.Console.Title != "Skylanders SWAP Force" || g.Console.Exe != g.Exe || g.WorkDir != g.Console.EmuDir {
		t.Fatalf("console: %+v", g.Console)
	}
}

// The emulator reads the controller itself: Auto never hands a console
// game Seaglass's or Steam's Xbox controller, even when its metadata says
// it only knows Xbox pads. A mode the user chose still applies.
func TestConsoleRoute(t *testing.T) {
	ds := pad.State{Connected: true, Kind: pad.PlayStation, DualSense: true}
	g := consoleGame()
	for _, tr := range []translator{{}, {viGEm: true}, {steam: true}} {
		if r := route(g, ds, tr); r != RouteDirect {
			t.Errorf("%+v: route %s, want direct", tr, r)
		}
		if needsSetup(g, ds, tr) {
			t.Errorf("%+v: asks to set up an Xbox controller", tr)
		}
	}
	g.PadMode = "steam"
	if r := route(g, ds, translator{}); r != RouteSteamInput {
		t.Errorf("chosen Steam Input: %s", r)
	}
}

func TestConsoleExeAndTracking(t *testing.T) {
	g := consoleGame()
	if !consoleExeOK(g) {
		t.Fatal("the scan's emulator should be allowed to start the game")
	}
	other := g
	other.Exe = `C:\Elsewhere\evil.exe`
	if consoleExeOK(other) {
		t.Fatal("a program outside the emulator's folder isn't the game's")
	}
	// Only the emulator's processes (from its folder, or by its program's
	// name from any folder) are the game, whatever started it: not
	// PowerShell running a boot script, not a launcher that stays in the
	// tray (a figure portal's controller).
	c := ctlTestCore(t, false, nil, nil)
	for _, exe := range []string{g.Exe, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, `C:\Emulators\src\portal\portal-controller.exe`} {
		k := g
		k.UserExe, k.Exe = exe != g.Exe, exe
		p := c.plan(k)
		if !slices.Equal(p.Dirs, []string{`C:\Emulators\RPCS3`}) || !p.FolderOnly || !slices.Equal(p.ExeNames, []string{"rpcs3.exe"}) || p.StarterTimeout < 10*time.Minute {
			t.Errorf("%s: dirs %v, folder only %v, exe names %v, starter timeout %v", exe, p.Dirs, p.FolderOnly, p.ExeNames, p.StarterTimeout)
		}
	}
	// A PC game's plan is as it was.
	pc := library.Game{ID: 2, Title: "G", Installed: true, Dir: `C:\G`, Exe: `C:\Tools\g.exe`, UserExe: true}
	if p := c.plan(pc); !slices.Equal(p.Dirs, []string{`C:\G`, `C:\Tools`}) || p.FolderOnly || p.ExeNames != nil || p.StarterTimeout != 0 {
		t.Errorf("pc game: %+v", p)
	}
}

// With "ask who plays" on, a console game asks like any other; Syncer's
// save steps are only for PC games.
func TestConsoleAsksWhoPlays(t *testing.T) {
	old := syncerInstalled
	syncerInstalled = func() (string, bool) { return `C:\Syncer\syncer.exe`, true }
	t.Cleanup(func() { syncerInstalled = old })
	c := ctlTestCore(t, false, nil, nil)
	cfg := c.Settings.Get()
	cfg.AskWhoPlays, cfg.SyncSavesBefore, cfg.BackupSavesAfter = true, true, true
	if _, err := c.Settings.Set(cfg); err != nil {
		t.Fatal(err)
	}
	steps := func(p launch.Plan) (ids []string) {
		for _, st := range append(p.Before, p.After...) {
			ids = append(ids, st.ID)
		}
		return ids
	}
	pc := library.Game{ID: 2, Title: "G", Installed: true, Dir: `C:\G`, Exe: `C:\G\g.exe`}
	pcSteps := steps(c.plan(pc))
	if !slices.Contains(pcSteps, "whoPlays") || len(pcSteps) != 3 {
		t.Errorf("pc game: %v", pcSteps)
	}
	if got := steps(c.plan(consoleGame())); !slices.Equal(got, []string{"whoPlays"}) {
		t.Errorf("console game: %v", got)
	}
}

type fakeSessions struct {
	mu    sync.Mutex
	cur   launch.Session
	quits int
}

func (f *fakeSessions) Current() launch.Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cur
}

func (f *fakeSessions) Quit() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quits++
	return nil
}

func (f *fakeSessions) quitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quits
}

// Quit game runs a boot script's quit arguments and ends the game itself
// when it's still running after that; with the emulator itself as the
// program, it never starts a second emulator to quit the first.
func TestQuitGameWithQuitArgs(t *testing.T) {
	type start struct{ exe, args string }
	var started []start
	var mu sync.Mutex
	oldStart, oldWait := startProcess, quitWait
	startProcess = func(exe, args, _ string) (int, error) {
		mu.Lock()
		started = append(started, start{exe, args})
		mu.Unlock()
		return 1, nil
	}
	quitWait = 20 * time.Millisecond
	t.Cleanup(func() { startProcess, quitWait = oldStart, oldWait })

	c := ctlTestCore(t, false, nil, nil)
	const ps = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	script := consoleGame()
	script.Exe, script.Args, script.QuitArgs, script.UserExe = ps, `-File C:\Emulators\tools\stream-skylanders.ps1 -Serial BLES01966`, `-File C:\Emulators\tools\stream-skylanders.ps1 -Quit`, true
	emu := consoleGame()
	emu.QuitArgs, emu.UserArgs = "--quit", true
	ids := map[string]int64{}
	for name, g := range map[string]library.Game{"script": script, "emu": emu} {
		f := consoleFound(emulated.Game{File: `R:\PS3\` + name + `\EBOOT.BIN`, Dir: `R:\PS3\` + name, Title: name, EmuDir: `C:\Emulators\RPCS3`, Exe: g.Console.Exe})
		c.Lib.ApplyScan([]library.Found{f}, time.Now())
		var id int64
		for _, x := range c.Lib.Games() {
			if x.Key == f.Key {
				id = x.ID
			}
		}
		if _, err := c.Lib.Update(id, func(x *library.Game) {
			x.Exe, x.Args, x.QuitArgs, x.UserExe, x.UserArgs = g.Exe, g.Args, g.QuitArgs, g.UserExe, g.UserArgs
		}); err != nil {
			t.Fatal(err)
		}
		ids[name] = id
	}

	// The script quits the game; Seaglass ends it after the wait.
	m := &fakeSessions{cur: launch.Session{ID: 7, GameID: ids["script"], Phase: launch.Running}}
	if err := c.quitGame(m); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(started) != 1 || started[0].exe != ps || started[0].args != script.QuitArgs {
		t.Fatalf("started %+v", started)
	}
	mu.Unlock()
	if m.quitCount() != 0 {
		t.Fatal("ended right away instead of letting the script quit it")
	}
	deadline := time.Now().Add(5 * time.Second)
	for m.quitCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("not ended after the wait")
		}
		time.Sleep(time.Millisecond)
	}

	// The game closed after its quit arguments: nothing more to end.
	m = &fakeSessions{cur: launch.Session{ID: 8, GameID: ids["script"], Phase: launch.Running}}
	if err := c.quitGame(m); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.cur.Phase = launch.Ended
	m.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	if m.quitCount() != 0 {
		t.Fatal("ended a session that was over")
	}

	// The emulator as the program: Seaglass ends it, nothing is started.
	mu.Lock()
	started = nil
	mu.Unlock()
	m = &fakeSessions{cur: launch.Session{ID: 9, GameID: ids["emu"], Phase: launch.Running}}
	if err := c.quitGame(m); err != nil || m.quitCount() != 1 {
		t.Fatalf("err %v, quits %d", err, m.quitCount())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(started) != 0 {
		t.Fatalf("started a second emulator: %+v", started)
	}
}

func TestQuitsWithArgs(t *testing.T) {
	g := consoleGame()
	g.QuitArgs = "-Quit"
	if quitsWithArgs(g) {
		t.Error("the emulator itself")
	}
	g.Exe = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if !quitsWithArgs(g) {
		t.Error("a boot script")
	}
	pc := library.Game{Dir: `C:\G`, Exe: `C:\G\g.exe`, QuitArgs: "-quit"}
	if quitsWithArgs(pc) {
		t.Error("a PC game's own program")
	}
	pc.Exe = `C:\Launcher\launcher.exe`
	if !quitsWithArgs(pc) {
		t.Error("a PC game's launcher")
	}
	pc.LaunchURI = "steam://run/1"
	if quitsWithArgs(pc) {
		t.Error("a store game")
	}
}

// Launch options are saved only once the person confirms the exact
// command lines in a Windows dialog. Arguments alone keep the program,
// so its folder guard stays; a program is only taken from the picker.
func TestSetLaunch(t *testing.T) {
	var asked []string
	answer := true
	old := confirmLaunch
	confirmLaunch = func(_, msg string) bool { asked = append(asked, msg); return answer }
	t.Cleanup(func() { confirmLaunch = old })
	c := ctlTestCore(t, false, nil, nil)
	s := NewLibraryService(c)
	dir := t.TempDir()
	game := filepath.Join(dir, "game.exe")
	tool := filepath.Join(t.TempDir(), "launcher.exe")
	for _, p := range []string{game, tool} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c.Lib.ApplyScan([]library.Found{{Key: platform.Key(dir), Title: "G", SortTitle: "g", Source: "folder", Dir: dir, Exe: game, WorkDir: dir}}, time.Now())
	id := c.Lib.Games()[0].ID

	answer = false
	if _, err := s.SetLaunch(id, "", "-windowed", ""); err == nil {
		t.Fatal("saved without being confirmed")
	}
	if g, _ := c.Lib.Get(id); g.Args != "" {
		t.Fatalf("args %q", g.Args)
	}
	answer = true
	g, err := s.SetLaunch(id, game, "-windowed", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Args != "-windowed" || g.UserExe || !g.UserArgs || g.Exe != game {
		t.Fatalf("game %+v", g)
	}
	if want := platform.CommandLine(game, "-windowed"); len(asked) != 2 || !strings.Contains(asked[1], want) || strings.Contains(asked[1], "Quit") {
		t.Fatalf("asked %q, want %q in it", asked, want)
	}
	// The game's own program would start a second copy with quit
	// arguments, so Quit game never runs them: they're refused, unasked.
	if _, err := s.SetLaunch(id, game, "-windowed", "-quit"); err == nil || len(asked) != 2 {
		t.Fatalf("quit arguments for the game's own program: err %v, asked %d", err, len(asked))
	}
	// A rescan keeps the arguments, with the program it found.
	c.Lib.ApplyScan([]library.Found{{Key: platform.Key(dir), Title: "G", SortTitle: "g", Source: "folder", Dir: dir, Exe: game, WorkDir: dir}}, time.Now())
	if g, _ := c.Lib.Get(id); g.Args != "-windowed" {
		t.Fatalf("rescan: args %q", g.Args)
	}
	// Nothing changed: nothing to confirm.
	if _, err := s.SetLaunch(id, game, "-windowed", ""); err != nil || len(asked) != 2 {
		t.Fatalf("err %v, asked %d", err, len(asked))
	}
	// A program the interface names, but the person didn't pick.
	if _, err := s.SetLaunch(id, tool, "", ""); err == nil {
		t.Fatal("took a program that wasn't picked")
	}
	c.picked.Store(id, tool)
	g, err = s.SetLaunch(id, tool, "--go", "--stop")
	if err != nil || g.Exe != tool || !g.UserExe || g.WorkDir != filepath.Dir(tool) || g.QuitArgs != "--stop" {
		t.Fatalf("err %v, game %+v", err, g)
	}
	if last := asked[len(asked)-1]; !strings.Contains(last, platform.CommandLine(tool, "--go")) || !strings.Contains(last, platform.CommandLine(tool, "--stop")) {
		t.Fatalf("asked %q", last)
	}
	if _, ok := c.picked.Load(id); ok {
		t.Fatal("the pick is used up")
	}
	// Back to the program found: arguments and quit arguments are gone.
	g, err = s.ResetLaunch(id)
	if err != nil || g.UserExe || g.UserArgs || g.QuitArgs != "" {
		t.Fatalf("reset: err %v, game %+v", err, g)
	}
}

// Arguments that would show differently in the confirmation than they
// run (a line break, a right-to-left override, an invisible character,
// broken UTF-8) are refused before anything is asked.
func TestSetLaunchRefusesHiddenCharacters(t *testing.T) {
	asked := 0
	old := confirmLaunch
	confirmLaunch = func(_, _ string) bool { asked++; return true }
	t.Cleanup(func() { confirmLaunch = old })
	c := ctlTestCore(t, false, nil, nil)
	s := NewLibraryService(c)
	dir := t.TempDir()
	game := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(game, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Lib.ApplyScan([]library.Found{{Key: platform.Key(dir), Title: "G", SortTitle: "g", Source: "folder", Dir: dir, Exe: game, WorkDir: dir}}, time.Now())
	id := c.Lib.Games()[0].ID
	for _, a := range []string{"-a\n-b", "-a\x7f", "-a\u0085b", "-file \u202egpj.exe", "-a\u200b-b", "-a\u2066x", "-a\xff"} {
		if _, err := s.SetLaunch(id, "", a, ""); err == nil {
			t.Errorf("%q taken", a)
		}
	}
	if asked != 0 {
		t.Fatalf("asked %d times", asked)
	}
	if _, err := s.SetLaunch(id, "", `-name "Zoë" -lang 日本語`, ""); err != nil || asked != 1 {
		t.Fatalf("plain text refused: %v", err)
	}
}

// Resetting a console game starts it with its emulator again, the way
// the scan found.
func TestResetLaunchConsole(t *testing.T) {
	c := ctlTestCore(t, false, nil, nil)
	g := consoleGame()
	f := consoleFound(emulated.Game{File: g.Console.File, Dir: g.Dir, Title: g.Title, EmuDir: g.Console.EmuDir, Exe: g.Console.Exe, Args: g.Console.Args})
	c.Lib.ApplyScan([]library.Found{f}, time.Now())
	id := c.Lib.Games()[0].ID
	if _, err := c.Lib.Update(id, func(x *library.Game) {
		x.Exe, x.Args, x.QuitArgs, x.WorkDir, x.UserExe, x.UserArgs = `C:\Windows\powershell.exe`, "-File boot.ps1", "-File boot.ps1 -Quit", `C:\`, true, true
	}); err != nil {
		t.Fatal(err)
	}
	got, err := NewLibraryService(c).ResetLaunch(id)
	if err != nil || got.Exe != g.Console.Exe || got.Args != g.Console.Args || got.WorkDir != g.Console.EmuDir || got.UserExe || got.UserArgs || got.QuitArgs != "" {
		t.Fatalf("err %v, game %+v", err, got)
	}
}

// Playtime of a console game is its own: by its serial, never a PC game
// of the same name.
func TestConsoleProfileKey(t *testing.T) {
	g := consoleGame()
	if ks := profileKeys(&g); !slices.Equal(ks, []string{"console:ps3:bles01966"}) {
		t.Fatalf("keys %v", ks)
	}
	g.Console.Serial, g.Console.Platform = "", "Wii U"
	if k := profileKey(&g); k != "console:wiiu:title:skylanderswapforce" {
		t.Fatalf("key %q", k)
	}
}
