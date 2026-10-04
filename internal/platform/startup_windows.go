package platform

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	runValue    = "Seaglass"
)

// StartupCommand is the command Windows runs at sign-in: the exe, in the tray.
func StartupCommand(exe string) string {
	return syscall.EscapeArg(filepath.Clean(exe)) + " --tray"
}

// GetStartup reads the current user's Run entry for Seaglass.
func GetStartup() Startup {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return Startup{}
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(runValue); err != nil {
		return Startup{}
	}
	return Startup{On: true, DisabledByUser: disabledInTaskManager()}
}

// disabledInTaskManager reads the flag Task Manager keeps per startup app:
// the first byte is 2 (or 6) when it's on, 3 (or 7) when it's off.
func disabledInTaskManager() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(runValue)
	return err == nil && len(b) > 0 && b[0]&1 == 1
}

// SetStartup registers (or removes) the exe to start at sign-in, in the tray.
func SetStartup(exe string, on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	if !filepath.IsAbs(exe) || !IsFile(exe) {
		return errors.New("can't find Seaglass's program file")
	}
	return k.SetStringValue(runValue, StartupCommand(exe))
}

// RepairStartup points an existing Run entry at exe when the program it
// names is gone (Seaglass was moved). It reports whether it changed.
func RepairStartup(exe string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	cur, _, err := k.GetStringValue(runValue)
	if err != nil || cur == StartupCommand(exe) {
		return false
	}
	if p := commandExe(cur); p != "" && IsFile(p) {
		return false // points at another copy that still exists; leave it
	}
	return k.SetStringValue(runValue, StartupCommand(exe)) == nil
}

// commandExe is the program a command line starts.
func commandExe(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if i := strings.Index(cmd[1:], `"`); i >= 0 {
			return cmd[1 : i+1]
		}
		return ""
	}
	if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
		return cmd[:i+4]
	}
	return cmd
}

// MoveOldStartup turns WaterLauncher's "start with Windows" entry into
// Seaglass's, keeping whether it's turned off in Task Manager. It reports
// whether there was one.
func MoveOldStartup(exe string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(OldName); err != nil {
		return false
	}
	if filepath.IsAbs(exe) && IsFile(exe) {
		_ = k.SetStringValue(runValue, StartupCommand(exe))
	}
	_ = k.DeleteValue(OldName)
	if a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		if b, _, err := a.GetBinaryValue(OldName); err == nil {
			_ = a.SetBinaryValue(runValue, b)
			_ = a.DeleteValue(OldName)
		}
		a.Close()
	}
	return true
}
