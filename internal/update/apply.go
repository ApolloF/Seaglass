package update

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// KindFor says how an update reaches the exe at path: through the
// installer when it was installed (its uninstaller sits next to it), by
// swapping the exe when its folder is writable, or not at all ("": the
// user downloads it by hand).
func KindFor(exe string) string {
	dir := filepath.Dir(exe)
	if platform.IsFile(filepath.Join(dir, "uninstall.exe")) {
		return KindInstaller
	}
	f, err := os.CreateTemp(dir, ".wl-write-test-*")
	if err != nil {
		return ""
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	return KindExe
}

// RunInstaller starts the installer silently, into the folder Seaglass
// is installed in. With relaunch it starts Seaglass again when it's
// done (in the tray with tray). The caller quits right after; the installer
// waits for it to exit.
func RunInstaller(file, installDir string, relaunch, tray bool) error {
	if strings.ContainsAny(installDir, "\"\r\n") || !filepath.IsAbs(installDir) {
		return errors.New("unexpected install folder")
	}
	args := "/S"
	if relaunch {
		args += " /relaunch"
		if tray {
			args += " /tray"
		}
	}
	// NSIS wants /D last and unquoted, even with spaces in it.
	args += " /D=" + filepath.Clean(installDir)
	_, err := platform.StartProcess(file, args, filepath.Dir(file))
	return err
}

// SwapExe puts newExe in the place of the running exe. Windows lets a
// running exe be renamed, so the old one moves aside to "<exe>.old" (removed
// by CleanOld on the next start) and the new one takes its name.
func SwapExe(newExe, exe string) error {
	staged := exe + ".new"
	if err := copyFile(newExe, staged); err != nil {
		_ = os.Remove(staged)
		return err
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		_ = os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, exe); err != nil {
		_ = os.Rename(old, exe)
		_ = os.Remove(staged)
		return err
	}
	return nil
}

// CleanOld removes the exe an earlier swap moved aside.
func CleanOld(exe string) { _ = os.Remove(exe + ".old") }

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
