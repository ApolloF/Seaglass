package platform

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// ShowInExplorer opens Explorer at dir.
func ShowInExplorer(dir string) error {
	if !filepath.IsAbs(dir) || !IsDir(dir) {
		return errors.New("folder not found")
	}
	cmd := exec.Command(filepath.Join(WindowsDir, "explorer.exe"), filepath.Clean(dir))
	return cmd.Start()
}

// StartProcess starts exe with a raw argument string in workDir, detached
// from Seaglass. It never goes through a shell. When the game asks for
// administrator rights, Windows shows its UAC prompt.
func StartProcess(exe, args, workDir string) (int, error) {
	if !filepath.IsAbs(exe) || !IsFile(exe) {
		return 0, errors.New("game executable not found")
	}
	if workDir == "" || !IsDir(workDir) {
		workDir = filepath.Dir(exe)
	}
	cmd := exec.Command(exe)
	cmd.Dir = workDir
	cmdLine := syscall.EscapeArg(exe)
	if args = strings.TrimSpace(args); args != "" {
		cmdLine += " " + args
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       cmdLine,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB,
	}
	err := cmd.Start()
	if errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
		return 0, shellExecute("open", exe, args, workDir)
	}
	if err != nil {
		// Breaking away from a job isn't allowed everywhere; try once without.
		cmd = exec.Command(exe)
		cmd.Dir = workDir
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdLine, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
		if err2 := cmd.Start(); err2 != nil {
			if errors.Is(err2, windows.ERROR_ELEVATION_REQUIRED) {
				return 0, shellExecute("open", exe, args, workDir)
			}
			return 0, err2
		}
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

func shellExecute(verb, file, args, dir string) error {
	v, _ := windows.UTF16PtrFromString(verb)
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	var a, d *uint16
	if args != "" {
		a, _ = windows.UTF16PtrFromString(args)
	}
	if dir != "" {
		d, _ = windows.UTF16PtrFromString(dir)
	}
	return windows.ShellExecute(0, v, f, a, d, windows.SW_SHOWNORMAL)
}
