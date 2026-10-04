package platform

import (
	"errors"
	"os"
	"path/filepath"
)

// OldName is what Seaglass was called before 1.5.
const OldName = "WaterLauncher"

// MoveOldData moves WaterLauncher's data (%APPDATA%\WaterLauncher and
// %LOCALAPPDATA%\WaterLauncher) into Seaglass's folders: whatever Seaglass
// doesn't have yet, so its own data wins, and nothing is deleted. While
// WaterLauncher itself still runs (oldRunning), its folders are used where
// they are and moved another time. Call it before anything is opened;
// what it did is returned for the log.
func MoveOldData(oldRunning bool) []string {
	var notes []string
	move := func(base string, override *string) {
		if base == "" {
			return
		}
		old, cur := filepath.Join(base, OldName), filepath.Join(base, "Seaglass")
		entries, err := os.ReadDir(old)
		if err != nil {
			return // nothing to move
		}
		if oldRunning {
			*override = old
			notes = append(notes, "WaterLauncher is running: using "+old+" for now")
			return
		}
		if err := os.Rename(old, cur); err == nil {
			notes = append(notes, "moved "+old+" to "+cur)
			return
		}
		// Seaglass has a folder already: move in what it doesn't have.
		if err := os.MkdirAll(cur, 0o755); err != nil {
			return
		}
		for _, e := range entries {
			from, to := filepath.Join(old, e.Name()), filepath.Join(cur, e.Name())
			if _, err := os.Lstat(to); !errors.Is(err, os.ErrNotExist) {
				notes = append(notes, "kept Seaglass's "+e.Name()+"; WaterLauncher's stays in "+old)
				continue
			}
			if err := os.Rename(from, to); err != nil {
				notes = append(notes, "couldn't move "+from+": "+err.Error())
				continue
			}
			notes = append(notes, "moved "+from+" to "+to)
		}
		_ = os.Remove(old) // only when it's empty now
	}
	if appDirOverride == "" {
		move(Roaming, &appDirOverride)
	}
	move(Local, &localDirOverride)
	return notes
}
