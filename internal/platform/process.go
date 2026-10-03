package platform

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Proc is one running process.
type Proc struct {
	PID, PPID uint32
	Name      string // exe file name, as Windows lists it
}

// Processes lists the running processes.
func Processes() ([]Proc, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []Proc
	for err = windows.Process32First(h, &e); err == nil; err = windows.Process32Next(h, &e) {
		out = append(out, Proc{PID: e.ProcessID, PPID: e.ParentProcessID, Name: windows.UTF16ToString(e.ExeFile[:])})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return out, err
	}
	return out, nil
}

// ProcessImage returns the full path of a process's exe and when it
// started (a FILETIME, to tell a reused process id apart). It works for
// elevated processes of the same user too.
func ProcessImage(pid uint32) (path string, started uint64, err error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", 0, err
	}
	defer windows.CloseHandle(h)
	// Most paths fit in MAX_PATH; a long path asks again with room for
	// the longest, rather than every lookup taking 64 KB.
	var buf []uint16
	var n uint32
	for _, size := range []int{windows.MAX_PATH + 1, windows.MAX_LONG_PATH} {
		buf = make([]uint16, size)
		n = uint32(len(buf))
		if err = windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != windows.ERROR_INSUFFICIENT_BUFFER {
			break
		}
	}
	if err != nil {
		return "", 0, err
	}
	var c, x, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &x, &k, &u); err == nil {
		started = uint64(c.HighDateTime)<<32 | uint64(c.LowDateTime)
	}
	return windows.UTF16ToString(buf[:n]), started, nil
}

// ProcessRunning reports whether a process with this exe name runs.
func ProcessRunning(name string) bool {
	ps, _ := Processes()
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

// EndProcess stops a process right away (like Task Manager's End task).
// started must match, so a process id Windows handed out again is left alone.
func EndProcess(pid uint32, started uint64) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var c, x, k, u windows.Filetime
	if started != 0 {
		if err := windows.GetProcessTimes(h, &c, &x, &k, &u); err != nil {
			return err
		}
		if uint64(c.HighDateTime)<<32|uint64(c.LowDateTime) != started {
			return errors.New("process ended already")
		}
	}
	return windows.TerminateProcess(h, 1)
}

// ExitWatch holds a process open so its exit code can be read after it
// ends (Windows keeps it for as long as a handle is open).
type ExitWatch struct{ h windows.Handle }

// WatchExit opens a process for its exit code. started must match, so a
// process id Windows handed out again isn't watched instead.
func WatchExit(pid uint32, started uint64) (*ExitWatch, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, err
	}
	var c, x, k, u windows.Filetime
	if started != 0 && (windows.GetProcessTimes(h, &c, &x, &k, &u) != nil || uint64(c.HighDateTime)<<32|uint64(c.LowDateTime) != started) {
		windows.CloseHandle(h)
		return nil, errors.New("process ended already")
	}
	return &ExitWatch{h: h}, nil
}

// Code returns the exit code once the process has ended.
func (w *ExitWatch) Code() (uint32, bool) {
	var code uint32
	if windows.GetExitCodeProcess(w.h, &code) != nil || code == 259 { // STILL_ACTIVE
		return 0, false
	}
	return code, true
}

// Close lets go of the process.
func (w *ExitWatch) Close() { windows.CloseHandle(w.h) }

// Crashed reports whether an exit code is one Windows gives a process
// that crashed (an NTSTATUS error: an access violation, a stack overrun,
// heap corruption, an unhandled exception, …) rather than one that quit.
func Crashed(code uint32) bool {
	switch code {
	case 0xFFFFFFFF, 0xC000013A: // exit(-1); STATUS_CONTROL_C_EXIT (console closed or Ctrl+C)
		return false
	}
	return code >= 0xC0000000 || code == 0x80000003
}
