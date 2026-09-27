package platform

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procSendInput           = user32.NewProc("SendInput")
	procIsWindow            = user32.NewProc("IsWindow")

	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

// IsForeground reports whether hwnd is the window in front.
func IsForeground(hwnd uintptr) bool {
	h, _, _ := procGetForegroundWindow.Call()
	return hwnd != 0 && h == hwnd
}

// BringToFront makes hwnd the window in front. Windows only lets the
// process the user last gave input to do that, and after a game closes
// that's the game, not Seaglass (a controller doesn't count): a plain
// SetForegroundWindow then only flashes the taskbar button, and the
// taskbar stays on top of a full-screen window that isn't in front. So
// it first sends an empty input (nothing moves or is typed), which makes
// Seaglass the process with the last input, and failing that joins the
// input of the window in front for a moment.
func BringToFront(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	if ok, _, _ := procIsWindow.Call(hwnd); ok == 0 {
		return false
	}
	procSetForegroundWindow.Call(hwnd)
	if IsForeground(hwnd) {
		return true
	}
	// An empty mouse input: INPUT{type: INPUT_MOUSE} with nothing set.
	var in [40]byte // sizeof(INPUT) on 64-bit Windows
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in))
	procSetForegroundWindow.Call(hwnd)
	if IsForeground(hwnd) {
		return true
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == 0 {
		return false
	}
	fgThread, _, _ := procGetWindowThreadProcessId.Call(fg, 0)
	self := uintptr(windows.GetCurrentThreadId())
	if fgThread != 0 && fgThread != self {
		procAttachThreadInput.Call(self, fgThread, 1)
		defer procAttachThreadInput.Call(self, fgThread, 0)
	}
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	return IsForeground(hwnd)
}

// ITaskbarList2
var (
	clsidTaskbarList = windows.GUID{Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11d0, Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90}}
	iidTaskbarList2  = windows.GUID{Data1: 0x602D4995, Data2: 0xB13A, Data3: 0x429b, Data4: [8]byte{0xA6, 0x6E, 0x19, 0x35, 0xE4, 0x4F, 0x43, 0x17}}
)

// MarkFullscreen tells the taskbar hwnd is a full-screen window, so it
// steps behind it whenever it's in front, rather than going by the
// window's size alone (which it checks only now and then).
func MarkFullscreen(hwnd uintptr, on bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	var tl *struct{ vtbl *[9]uintptr } // ITaskbarList2: IUnknown(3), HrInit, AddTab, DeleteTab, ActivateTab, SetActiveAlt, MarkFullscreenWindow
	const clsctxInprocServer = 1
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidTaskbarList2)), uintptr(unsafe.Pointer(&tl))); int32(hr) < 0 || tl == nil {
		return syscall.Errno(hr)
	}
	self := uintptr(unsafe.Pointer(tl))
	const release, hrInit, markFullscreenWindow = 2, 3, 8 // vtable slots
	defer syscall.SyscallN(tl.vtbl[release], self)
	if hr, _, _ := syscall.SyscallN(tl.vtbl[hrInit], self); int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	flag := uintptr(0)
	if on {
		flag = 1
	}
	if hr, _, _ := syscall.SyscallN(tl.vtbl[markFullscreenWindow], self, hwnd, flag); int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}
