package platform

import (
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
