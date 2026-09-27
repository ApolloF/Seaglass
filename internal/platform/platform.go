// Package platform wraps the Windows specifics the rest of Seaglass
// needs: known folders, the app's own data folders, drives and path checks.
package platform

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Known folders, resolved once at start. "" when Windows doesn't know one.
var (
	Roaming         = known(windows.FOLDERID_RoamingAppData)
	Local           = known(windows.FOLDERID_LocalAppData)
	ProgramData     = known(windows.FOLDERID_ProgramData)
	Public          = known(windows.FOLDERID_Public)
	Profile         = known(windows.FOLDERID_Profile)
	Documents       = known(windows.FOLDERID_Documents)
	Desktop         = known(windows.FOLDERID_Desktop)
	PublicDesktop   = known(windows.FOLDERID_PublicDesktop)
	StartMenu       = known(windows.FOLDERID_StartMenu)
	CommonStartMenu = known(windows.FOLDERID_CommonStartMenu)
	ProgramFiles    = known(windows.FOLDERID_ProgramFiles)
	ProgramFilesX86 = known(windows.FOLDERID_ProgramFilesX86)
	WindowsDir      = known(windows.FOLDERID_Windows)
)

func known(id *windows.KNOWNFOLDERID) string {
	p, err := windows.KnownFolderPath(id, 0)
	if err != nil {
		return ""
	}
	return filepath.Clean(p)
}

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
	if !strings.HasSuffix(p, `\`) {
		p += `\`
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

// FixedDrives returns the roots of local fixed drives ("C:\", "D:\", …).
func FixedDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{`C:\`}
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		if windows.GetDriveType(p) == windows.DRIVE_FIXED {
			out = append(out, root)
		}
	}
	return out
}

// RealCase returns p with the capitalisation the file system uses (Steam
// keeps its path lower-cased in the registry). p is returned unchanged
// when it can't be opened or resolves somewhere else (a junction).
func RealCase(p string) string {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	h, err := windows.CreateFile(u, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return p
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || int(n) >= len(buf) {
		return p
	}
	s := strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`)
	if strings.EqualFold(filepath.Clean(s), filepath.Clean(p)) {
		return s
	}
	return p
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
