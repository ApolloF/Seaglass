// Package platform wraps the OS specifics the rest of Seaglass
// needs: known folders, the app's own data folders, drives and path checks.
package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// AppDir is Seaglass's settings folder (%APPDATA%\Seaglass), created on demand.
func AppDir() string {
	if appDirOverride != "" {
		return ensure(appDirOverride)
	}
	return ensure(filepath.Join(Roaming, "Seaglass"))
}

var appDirOverride string

// UseAppDir puts the library, settings and log in dir instead (a test
// harness's own data). Call it before anything is opened.
func UseAppDir(dir string) { appDirOverride = filepath.Clean(dir) }

// AppDirOverridden reports whether UseAppDir moved the data elsewhere (a
// test harness's own data): caches tied to that library belong with it.
func AppDirOverridden() bool { return appDirOverride != "" }

// CacheDir is a folder under %LOCALAPPDATA%\Seaglass, created on demand.
func CacheDir(sub ...string) string {
	base := filepath.Join(Local, "Seaglass")
	if localDirOverride != "" {
		base = localDirOverride
	}
	return ensure(filepath.Join(append([]string{base}, sub...)...))
}

// localDirOverride is WaterLauncher's folder when it couldn't be moved.
var localDirOverride string

func ensure(dir string) string {
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// Within reports whether child is parent or lies below it (case-insensitive).
func Within(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	p := strings.ToLower(filepath.Clean(parent))
	c := strings.ToLower(filepath.Clean(child))
	if p == c {
		return true
	}
	if sep := string(filepath.Separator); !strings.HasSuffix(p, sep) {
		p += sep
	}
	return strings.HasPrefix(c, p)
}

// Key is a folder's identity: cleaned and lower-cased.
func Key(dir string) string { return strings.ToLower(filepath.Clean(dir)) }

// IsDir reports whether p is an existing folder.
func IsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// IsFile reports whether p is an existing regular file.
func IsFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// SystemPath reports whether p lies in a folder no game installs into
// (Windows itself, Common Files, WindowsApps).
func SystemPath(p string) bool {
	for _, root := range []string{
		WindowsDir,
		filepath.Join(ProgramFiles, "Common Files"),
		filepath.Join(ProgramFilesX86, "Common Files"),
		filepath.Join(ProgramFiles, "WindowsApps"),
	} {
		if root != "" && Within(root, p) {
			return true
		}
	}
	return false
}
