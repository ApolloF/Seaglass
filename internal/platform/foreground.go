package platform

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procSetWinEventHook          = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent           = user32.NewProc("UnhookWinEvent")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procPeekMessageW             = user32.NewProc("PeekMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
)

// ForegroundPID is the process whose window is in front, 0 when none is.
func ForegroundPID() uint32 {
	h, _, _ := procGetForegroundWindow.Call()
	if h == 0 {
		return 0
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

const (
	eventSystemForeground  = 0x0003
	winEventOutOfContext   = 0x0000
	winEventSkipOwnProcess = 0x0002
	wmQuit                 = 0x0012
)

// One watcher at a time; the callback (a scarce resource) is made once.
var (
	fgMu       sync.Mutex
	fgFn       func(pid uint32)
	fgCallback uintptr
	fgOnce     sync.Once
)

func foregroundCallback(hook, event, hwnd, idObject, idChild, thread, when uintptr) uintptr {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	fgMu.Lock()
	fn := fgFn
	fgMu.Unlock()
	if fn != nil && pid != 0 {
		fn(pid)
	}
	return 0
}

// WatchForeground calls fn with the process id of each window that comes
// to the front, other than Seaglass's own. Windows tells it (a
// WinEvent hook, out of context): nothing is polled and nothing is
// injected into other processes. fn runs on the watcher's thread and must
// return quickly. stop ends the watch.
func WatchForeground(fn func(pid uint32)) (stop func(), err error) {
	fgOnce.Do(func() { fgCallback = syscall.NewCallback(foregroundCallback) })
	fgMu.Lock()
	if fgFn != nil {
		fgMu.Unlock()
		return nil, errors.New("already watching")
	}
	fgFn = fn
	fgMu.Unlock()

	started := make(chan error, 1)
	var tid uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The hook belongs to this thread, which must pump messages for it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid = windows.GetCurrentThreadId()
		var msg [48]byte // MSG
		// Make sure the thread has a message queue before anyone posts to it.
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 0)
		h, _, herr := procSetWinEventHook.Call(eventSystemForeground, eventSystemForeground, 0, fgCallback, 0, 0,
			winEventOutOfContext|winEventSkipOwnProcess)
		if h == 0 {
			started <- herr
			return
		}
		started <- nil
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
		procUnhookWinEvent.Call(h)
	}()
	if err := <-started; err != nil {
		fgMu.Lock()
		fgFn = nil
		fgMu.Unlock()
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			fgMu.Lock()
			fgFn = nil
			fgMu.Unlock()
			procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
			<-done
		})
	}, nil
}
