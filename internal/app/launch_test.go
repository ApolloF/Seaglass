package app

import (
	"testing"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/pad"
)

func TestRoute(t *testing.T) {
	ds := pad.State{Connected: true, Kind: pad.PlayStation, DualSense: true}
	xbox := pad.State{Connected: true, Kind: pad.Xbox}
	xinputOnly := &library.Meta{Controller: "full"}
	for _, tc := range []struct {
		name string
		g    library.Game
		pad  pad.State
		want string
	}{
		{"store game", library.Game{LaunchURI: "steam://rungameid/1", Meta: xinputOnly}, ds, RouteStore},
		{"xinput game, DualSense", library.Game{Exe: `C:\g.exe`, Meta: xinputOnly}, ds, RouteSteamInput},
		{"xinput game, Xbox pad", library.Game{Exe: `C:\g.exe`, Meta: xinputOnly}, xbox, RouteDirect},
		{"xinput game, no pad", library.Game{Exe: `C:\g.exe`, Meta: xinputOnly}, pad.State{}, RouteDirect},
		{"DualSense game", library.Game{Exe: `C:\g.exe`, Meta: &library.Meta{Controller: "full", DualSense: "yes"}}, ds, RouteDirect},
		{"ships libScePad", library.Game{Exe: `C:\g.exe`, PadHint: "libScePad", Meta: xinputOnly}, ds, RouteDirect},
		{"unknown support", library.Game{Exe: `C:\g.exe`}, ds, RouteDirect},
		{"no controller support", library.Game{Exe: `C:\g.exe`, Meta: &library.Meta{}}, ds, RouteDirect},
		{"forced native", library.Game{Exe: `C:\g.exe`, PadMode: "native", Meta: xinputOnly}, ds, RouteDirect},
		{"forced Steam Input", library.Game{Exe: `C:\g.exe`, PadMode: "steam"}, xbox, RouteSteamInput},
	} {
		// The Steam route for Auto is the one a person who chose Steam gets.
		if got := route(tc.g, tc.pad, translator{steam: true}); got != tc.want {
			t.Errorf("%s: route = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// Auto picks the translator the person chose: Steam Input, or Seaglass's
// own virtual controller once ViGEmBus is there, else it starts directly.
// The xbox mode needs ViGEmBus; native and steam ignore the drivers.
func TestRouteTranslator(t *testing.T) {
	ds := pad.State{Connected: true, Kind: pad.PlayStation, DualSense: true}
	xbox := pad.State{Connected: true, Kind: pad.Xbox}
	xinputOnly := &library.Meta{Controller: "partial"}
	auto := library.Game{Exe: `C:\g.exe`, Meta: xinputOnly}
	forced := library.Game{Exe: `C:\g.exe`, PadMode: "xbox"}
	none, vigem := translator{}, translator{viGEm: true}
	steam, both := translator{steam: true}, translator{viGEm: true, steam: true}
	for _, tc := range []struct {
		name  string
		g     library.Game
		pad   pad.State
		tr    translator
		want  string
		setup bool
	}{
		{"auto, ViGEmBus", auto, ds, vigem, RouteBridge, false},
		{"auto, nothing set up", auto, ds, none, RouteDirect, true},
		{"auto, Steam chosen", auto, ds, steam, RouteSteamInput, false},
		{"auto, Steam chosen over ViGEmBus", auto, ds, both, RouteSteamInput, false},
		{"auto, Xbox pad", auto, xbox, vigem, RouteDirect, false},
		{"auto, Xbox pad, nothing set up", auto, xbox, none, RouteDirect, false},
		{"auto, DualSense game", library.Game{Exe: `C:\g.exe`, Meta: &library.Meta{Controller: "full", DualSense: "yes"}}, ds, none, RouteDirect, false},
		{"xbox mode, ViGEmBus", forced, xbox, vigem, RouteBridge, false},
		{"xbox mode, no pad", forced, pad.State{}, both, RouteBridge, false},
		{"xbox mode, nothing set up", forced, ds, steam, RouteDirect, true},
		{"native mode", library.Game{Exe: `C:\g.exe`, PadMode: "native", Meta: xinputOnly}, ds, vigem, RouteDirect, false},
		{"steam mode", library.Game{Exe: `C:\g.exe`, PadMode: "steam", Meta: xinputOnly}, ds, vigem, RouteSteamInput, false},
		{"store game", library.Game{LaunchURI: "steam://rungameid/1", PadMode: "xbox"}, ds, none, RouteStore, false},
	} {
		if got := route(tc.g, tc.pad, tc.tr); got != tc.want {
			t.Errorf("%s: route = %s, want %s", tc.name, got, tc.want)
		}
		if got := needsSetup(tc.g, tc.pad, tc.tr); got != tc.setup {
			t.Errorf("%s: needsSetup = %v, want %v", tc.name, got, tc.setup)
		}
	}
}

func TestPlayArg(t *testing.T) {
	if id, ok := PlayArg([]string{`C:\Seaglass.exe`, "--play", "42"}); !ok || id != 42 {
		t.Errorf("PlayArg = %d, %v", id, ok)
	}
	for _, args := range [][]string{nil, {"--play"}, {"--play", "x"}, {"--play", "-1"}} {
		if _, ok := PlayArg(args); ok {
			t.Errorf("PlayArg(%q) accepted", args)
		}
	}
}

func TestParseArgs(t *testing.T) {
	a := ParseArgs([]string{`C:\Seaglass.exe`, "--tray", "--updated"})
	if !a.Tray || !a.Updated || a.Quit || a.Play != 0 {
		t.Errorf("ParseArgs = %+v", a)
	}
	if a := ParseArgs([]string{"--play", "7"}); a.Play != 7 || a.Tray {
		t.Errorf("ParseArgs(--play 7) = %+v", a)
	}
	if a := ParseArgs([]string{"--quit"}); !a.Quit {
		t.Errorf("ParseArgs(--quit) = %+v", a)
	}
}

func TestGameForPath(t *testing.T) {
	games := []library.Game{
		{ID: 1, Title: "Repacks", Installed: true, Dir: `D:\Games`},
		{ID: 2, Title: "Hades", Installed: true, Dir: `D:\Games\Hades`},
		{ID: 3, Title: "Gone", Installed: false, Dir: `D:\Games\Gone`},
		{ID: 4, Title: "Not a game", Installed: true, Hidden: true, Dir: `C:\Tools\Editor`},
		{ID: 5, Title: "Too broad", Installed: true, Dir: `C:\Program Files`},
	}
	for path, want := range map[string]int64{
		`D:\Games\Hades\x64\Hades.exe`:     2, // the most specific folder
		`D:\Games\Other\Other.exe`:         1,
		`D:\Games\Gone\Gone.exe`:           1, // not installed: only the folder around it counts
		`C:\Tools\Editor\editor.exe`:       0, // hidden
		`C:\Program Files\Some\app.exe`:    0, // too broad a folder
		`D:\Games\Hades\Seaglass.exe`: 0,
		`C:\Windows\System32\notepad.exe`:  0,
	} {
		g, ok := gameForPath(games, path)
		if got := map[bool]int64{true: g.ID, false: 0}[ok]; got != want {
			t.Errorf("%s: game %d, want %d", path, got, want)
		}
	}
}
