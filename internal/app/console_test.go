package app

import (
	"slices"
	"testing"

	"github.com/ApolloF/Seaglass/internal/emulated"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/pad"
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
	if d := consoleDirs(g); !slices.Equal(d, []string{`C:\Emulators\RPCS3`}) {
		t.Fatalf("dirs %v", d)
	}
	// Started through PowerShell (a boot script): PowerShell's folder
	// doesn't make every PowerShell window the game.
	ps := g
	ps.UserExe, ps.Exe = true, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if d := consoleDirs(ps); len(d) != 1 {
		t.Fatalf("dirs %v", d)
	}
	tool := g
	tool.UserExe, tool.Exe = true, `C:\Tools\Launcher\launch.exe`
	if d := consoleDirs(tool); len(d) != 2 || d[1] != `C:\Tools\Launcher` {
		t.Fatalf("dirs %v", d)
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
