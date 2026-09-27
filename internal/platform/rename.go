package platform

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// OldName is what Seaglass was called before 1.5.
const OldName = "WaterLauncher"

// MoveOldData moves WaterLauncher's data folders (%APPDATA%\WaterLauncher
// and %LOCALAPPDATA%\WaterLauncher) to Seaglass's, when Seaglass has none
// yet. A folder that can't be moved is used where it is. Call it before
// anything is opened; what it did is returned for the log.
func MoveOldData() []string {
	var notes []string
	move := func(base string, override *string) {
		if base == "" {
			return
		}
		old, cur := filepath.Join(base, OldName), filepath.Join(base, "Seaglass")
		if !IsDir(old) {
			return
		}
		if _, err := os.Stat(cur); !errors.Is(err, os.ErrNotExist) {
			return // moved already, or Seaglass started fresh
		}
		if err := os.Rename(old, cur); err != nil {
			*override = old
			notes = append(notes, "couldn't move "+old+", using it where it is: "+err.Error())
			return
		}
		notes = append(notes, "moved "+old+" to "+cur)
	}
	if appDirOverride == "" {
		move(Roaming, &appDirOverride)
	}
	move(Local, &localDirOverride)
	return notes
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
