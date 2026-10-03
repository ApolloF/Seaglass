package pad

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/ApolloF/Seaglass/internal/platform"
	"golang.org/x/sys/windows"
)

//go:embed sdl3/SDL3.dll
var sdlDLL []byte

// sdlHash pins the embedded SDL3.dll (see sdl3/README.md).
const sdlHash = "1f98969319302a100931f4385e5918a0bd53ab07773040682d22e7edb54858c0"

// sdl holds the SDL3 functions Seaglass calls. Every one takes and
// returns plain integers and pointers, so they are called without cgo.
type sdl struct {
	dll                    *windows.DLL
	init, quit, setHint    *windows.Proc
	getError               *windows.Proc
	pollEvent              *windows.Proc
	openGamepad            *windows.Proc
	closeGamepad           *windows.Proc
	gamepadName            *windows.Proc
	gamepadNameForID       *windows.Proc
	gamepadType            *windows.Proc
	rumbleGamepad          *windows.Proc
	setGamepadLED          *windows.Proc
	gamepadPowerInfo       *windows.Proc
	gamepadConnectionState *windows.Proc
}

// loadSDL writes the embedded DLL to Seaglass's own folder (once),
// checks its hash every time, and loads it from that exact path.
func loadSDL() (*sdl, error) {
	sum := sha256.Sum256(sdlDLL)
	if hex.EncodeToString(sum[:]) != sdlHash {
		return nil, errors.New("embedded SDL3.dll does not match its pinned hash")
	}
	dir := platform.CacheDir("bin")
	p := filepath.Join(dir, "SDL3-"+sdlHash[:12]+".dll")
	if b, err := os.ReadFile(p); err != nil || !bytes.Equal(b, sdlDLL) {
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, sdlDLL, 0o644); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, p); err != nil {
			return nil, err
		}
	}
	// Re-read and check what is actually on disk right before loading it.
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if s := sha256.Sum256(b); hex.EncodeToString(s[:]) != sdlHash {
		return nil, errors.New("SDL3.dll on disk was changed")
	}
	h, err := windows.LoadLibraryEx(p, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, err
	}
	d := &windows.DLL{Name: p, Handle: h}
	s := &sdl{dll: d}
	for _, f := range []struct {
		name string
		dst  **windows.Proc
	}{
		{"SDL_Init", &s.init}, {"SDL_Quit", &s.quit}, {"SDL_SetHint", &s.setHint}, {"SDL_GetError", &s.getError},
		{"SDL_PollEvent", &s.pollEvent}, {"SDL_OpenGamepad", &s.openGamepad}, {"SDL_CloseGamepad", &s.closeGamepad},
		{"SDL_GetGamepadName", &s.gamepadName}, {"SDL_GetGamepadNameForID", &s.gamepadNameForID}, {"SDL_GetGamepadType", &s.gamepadType},
		{"SDL_RumbleGamepad", &s.rumbleGamepad}, {"SDL_SetGamepadLED", &s.setGamepadLED},
		{"SDL_GetGamepadPowerInfo", &s.gamepadPowerInfo}, {"SDL_GetGamepadConnectionState", &s.gamepadConnectionState},
	} {
		p, err := d.FindProc(f.name)
		if err != nil {
			_ = d.Release()
			return nil, err
		}
		*f.dst = p
	}
	return s, nil
}

func cstr(s string) *byte {
	b, _ := syscall.BytePtrFromString(s)
	return b
}

// gostr copies a NUL-terminated C string that SDL returned (SDL owns the memory).
func gostr(p uintptr) string {
	if p == 0 {
		return ""
	}
	return windows.BytePtrToString(*(**byte)(unsafe.Pointer(&p)))
}

func ok(r uintptr) bool { return byte(r) != 0 } // SDL returns C bool in the low byte

func (s *sdl) errorText() string {
	r, _, _ := s.getError.Call()
	return gostr(r)
}
