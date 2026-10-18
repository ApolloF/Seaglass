package pad

import (
	"github.com/ApolloF/Seaglass/internal/platform"
)

// proc stands in for an SDL3 function. On Linux, SDL3 would come from the
// system (libSDL3.so.0) and could be loaded without cgo through purego's
// dlopen; until then every call fails and the controller layer stays off.
type proc struct{}

func (*proc) Call(...uintptr) (uintptr, uintptr, error) { return 0, 0, platform.ErrNotSupported }

type library struct{}

func (*library) FindProc(string) (*proc, error) { return nil, platform.ErrNotSupported }

// sdl mirrors the Windows binding's function table (dll_windows.go).
type sdl struct {
	dll                    *library
	init, quit, setHint    *proc
	getError               *proc
	pollEvent              *proc
	openGamepad            *proc
	closeGamepad           *proc
	gamepadName            *proc
	gamepadNameForID       *proc
	gamepadType            *proc
	rumbleGamepad          *proc
	setGamepadLED          *proc
	gamepadPowerInfo       *proc
	gamepadConnectionState *proc
}

func loadSDL() (*sdl, error) { return nil, platform.ErrNotSupported }

func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

func gostr(uintptr) string { return "" }

func ok(r uintptr) bool { return byte(r) != 0 }

func (s *sdl) errorText() string { return platform.ErrNotSupported.Error() }
