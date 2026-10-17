package platform

import (
	"errors"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RunInstaller runs an installer from dir and waits for it to exit,
// returning its exit code. An installer that asks for administrator rights
// in its manifest is started through the shell, so Windows shows its UAC
// prompt; declining that prompt counts as cancelling (exit code 1602).
func RunInstaller(exe, dir string) (uint32, error) {
	cmd := exec.Command(exe)
	cmd.Dir = dir
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		return uint32(exit.ExitCode()), nil
	case !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED):
		return 0, err
	}
	return runElevated(exe, dir)
}

// shellExecuteInfo is SHELLEXECUTEINFOW (x64).
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	verb         *uint16
	file         *uint16
	parameters   *uint16
	directory    *uint16
	show         int32
	instApp      uintptr
	idList       uintptr
	class        *uint16
	keyClass     uintptr
	hotKey       uint32
	iconOrMonitr uintptr
	process      windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	exitInstallUserExit   = 1602
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

func runElevated(exe, dir string) (uint32, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	d, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	info := &shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync, verb: verb, file: file, directory: d, show: windows.SW_SHOWNORMAL}
	info.cbSize = uint32(unsafe.Sizeof(*info))
	if r, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(info))); r == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return exitInstallUserExit, nil
		}
		return 0, err
	}
	if info.process == 0 {
		return 0, errors.New("the installer didn't start")
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return 0, err
	}
	return code, nil
}
