package platform

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrNotSupported is returned by everything Seaglass only does on Windows
// so far (a Linux port is being explored; see docs/linux-feasibility.md).
var ErrNotSupported = errors.New("not supported on Linux yet")

// Folders on Linux follow the XDG base directories. The Windows-only ones
// (ProgramData, Start menu, …) stay "" so code that joins them finds nothing.
var (
	Roaming         = xdg("XDG_CONFIG_HOME", ".config")
	Local           = xdg("XDG_DATA_HOME", ".local/share")
	ProgramData     = ""
	Public          = ""
	Profile         = home()
	Documents       = ""
	Desktop         = ""
	PublicDesktop   = ""
	StartMenu       = ""
	CommonStartMenu = ""
	ProgramFiles    = ""
	ProgramFilesX86 = ""
	WindowsDir      = ""
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func xdg(env, fallback string) string {
	if d := os.Getenv(env); filepath.IsAbs(d) {
		return filepath.Clean(d)
	}
	if h := home(); h != "" {
		return filepath.Join(h, fallback)
	}
	return ""
}

// FixedDrives returns the file system root; Linux has no drive letters.
func FixedDrives() []string { return []string{"/"} }

// RealCase returns p unchanged: Linux file systems are case-sensitive.
func RealCase(p string) string { return p }

// Proc is one running process.
type Proc struct {
	PID, PPID uint32
	Name      string
}

func Processes() ([]Proc, error)                        { return nil, ErrNotSupported }
func ProcessImage(uint32) (string, uint64, error)       { return "", 0, ErrNotSupported }
func ProcessRunning(string) bool                        { return false }
func EndProcess(uint32, uint64) error                   { return ErrNotSupported }
func Crashed(uint32) bool                               { return false }
func WatchForeground(func(pid uint32)) (func(), error)  { return nil, ErrNotSupported }
func ForegroundPID() uint32                             { return 0 }
func IsForeground(uintptr) bool                         { return false }
func BringToFront(uintptr) bool                         { return false }
func MarkFullscreen(uintptr, bool) error                { return ErrNotSupported }
func InstanceRunning(string) bool                       { return false }
func WaitInstanceGone(string, time.Duration) bool       { return true }
func Protect([]byte) ([]byte, error)                    { return nil, ErrNotSupported }
func Unprotect([]byte) ([]byte, error)                  { return nil, ErrNotSupported }
func ShowInExplorer(string) error                       { return ErrNotSupported }
func StartProcess(string, string, string) (int, error)  { return 0, ErrNotSupported }
func shellExecute(string, string, string, string) error { return ErrNotSupported }
func GetStartup() Startup                               { return Startup{} }
func SetStartup(string, bool) error                     { return ErrNotSupported }
func RepairStartup(string) bool                         { return false }
func MoveOldStartup(string) bool                        { return false }
func StartupCommand(exe string) string                  { return exe + " --tray" }

// ExitWatch holds a process for its exit code; never available on Linux yet.
type ExitWatch struct{}

func WatchExit(uint32, uint64) (*ExitWatch, error) { return nil, ErrNotSupported }
func (*ExitWatch) Code() (uint32, bool)            { return 0, false }
func (*ExitWatch) Close()                          {}

// ErrNotSigned means a file has no valid Authenticode signature.
var ErrNotSigned = errors.New("no valid signature")

// Signer always fails on Linux: there is no Authenticode check there.
func Signer(string) (string, error) { return "", ErrNotSigned }
