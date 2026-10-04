//go:build windows

// Command fakegame is a stand-in game for testing Seaglass's launch
// and tracking in the real app (tools/harness). It opens a full-screen
// window like a game does, and can pretend to be a launcher that hands
// over to the game, a game that is slow to start, or one that crashes.
//
//	fakegame [--launcher] [--slow=S] [--crash=S] [--run=S] [--title=T]
//
//	--launcher  start a copy of itself as the game, then exit (a launcher
//	            handing over, as many stores and games do)
//	--slow=S    wait S seconds before showing the window
//	--crash=S   crash (exit code 0xC0000005) S seconds after the window shows
//	--run=S     close by itself after S seconds (Esc closes it any time)
//	--title=T   the text shown
//
// Flags also come from fakegame.txt next to the exe (one line), because a
// game found in a folder is started without arguments.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	registerClassEx  = user32.NewProc("RegisterClassExW")
	createWindowEx   = user32.NewProc("CreateWindowExW")
	defWindowProc    = user32.NewProc("DefWindowProcW")
	getMessage       = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessage  = user32.NewProc("DispatchMessageW")
	postQuitMessage  = user32.NewProc("PostQuitMessage")
	destroyWindow    = user32.NewProc("DestroyWindow")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	beginPaint       = user32.NewProc("BeginPaint")
	endPaint         = user32.NewProc("EndPaint")
	drawText         = user32.NewProc("DrawTextW")
	getClientRect    = user32.NewProc("GetClientRect")
	setTimer         = user32.NewProc("SetTimer")
	invalidateRect   = user32.NewProc("InvalidateRect")
	setForeground    = user32.NewProc("SetForegroundWindow")
	showWindow       = user32.NewProc("ShowWindow")
	keybdEvent       = user32.NewProc("keybd_event")
	loadCursor       = user32.NewProc("LoadCursorW")

	createSolidBrush = gdi32.NewProc("CreateSolidBrush")
	setTextColor     = gdi32.NewProc("SetTextColor")
	setBkMode        = gdi32.NewProc("SetBkMode")
	createFont       = gdi32.NewProc("CreateFontW")
	selectObject     = gdi32.NewProc("SelectObject")
)

type wndClassEx struct {
	size, style                uint32
	wndProc                    uintptr
	clsExtra, wndExtra         int32
	instance, icon, cursor, bg uintptr
	menuName, className        *uint16
	iconSm                     uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

type rect struct{ left, top, right, bottom int32 }

type paintStruct struct {
	hdc       uintptr
	erase     int32
	paint     rect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

const (
	wsPopup   = 0x80000000
	wsVisible = 0x10000000

	wmDestroy = 0x0002
	wmPaint   = 0x000F
	wmKeyDown = 0x0100
	wmTimer   = 0x0113

	vkEscape = 0x1B
)

type options struct {
	launcher         bool
	slow, crash, run float64
	title            string
}

func parse(args []string) options {
	o := options{title: "Fake game"}
	for _, a := range args {
		name, val, _ := strings.Cut(a, "=")
		f, _ := strconv.ParseFloat(val, 64)
		switch name {
		case "--launcher":
			o.launcher = true
		case "--slow":
			o.slow = f
		case "--crash":
			o.crash = f
		case "--run":
			o.run = f
		case "--title":
			o.title = val
		}
	}
	return o
}

var (
	opts    options
	started time.Time
	font    uintptr
)

// A window's messages go to the thread that made it: the message loop must
// stay on the main thread.
func init() { runtime.LockOSThread() }

func main() {
	args := os.Args[1:]
	exe, _ := os.Executable()
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "fakegame.txt")); err == nil && len(args) == 0 {
		args = strings.Fields(string(b))
	}
	opts = parse(args)
	logf("start %v", args)
	if opts.launcher {
		// Hand over: the game is a copy of this exe with the same flags
		// minus --launcher, then the launcher leaves.
		var rest []string
		for _, a := range args {
			if a != "--launcher" {
				rest = append(rest, a)
			}
		}
		time.Sleep(1500 * time.Millisecond)
		game := filepath.Join(filepath.Dir(exe), "Game", "FakeGame.exe")
		if _, err := os.Stat(game); err != nil {
			game = exe
		}
		cmd := exec.Command(game, rest...)
		cmd.Dir = filepath.Dir(game)
		if err := cmd.Start(); err != nil {
			logf("launcher: %v", err)
			os.Exit(1)
		}
		logf("launcher handed over to pid %d", cmd.Process.Pid)
		return
	}
	if opts.slow > 0 {
		time.Sleep(time.Duration(opts.slow * float64(time.Second)))
	}
	window()
	logf("closed after %v", time.Since(started).Round(time.Second))
}

func logf(format string, a ...any) {
	exe, _ := os.Executable()
	f, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "fakegame.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s pid %d %s\n", time.Now().Format("15:04:05.000"), os.Getpid(), fmt.Sprintf(format, a...))
}

func window() {
	inst, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	cls, _ := windows.UTF16PtrFromString("SeaglassFakeGame")
	cursor, _, _ := loadCursor.Call(0, 32512)
	bg, _, _ := createSolidBrush.Call(0x3a1a10) // BGR: a dark blue
	wc := wndClassEx{wndProc: windows.NewCallback(wndProc), instance: inst, className: cls, bg: bg, cursor: cursor}
	wc.size = uint32(unsafe.Sizeof(wc))
	registerClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	w, _, _ := getSystemMetrics.Call(0)
	h, _, _ := getSystemMetrics.Call(1)
	title, _ := windows.UTF16PtrFromString(opts.title)
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	font, _, _ = createFont.Call(uintptr(h/14), 0, 0, 0, 600, 0, 0, 0, 0, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	hwnd, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)),
		wsPopup|wsVisible, 0, 0, w, h, 0, 0, inst, 0)
	started = time.Now()
	showWindow.Call(hwnd, 5)
	// A game started by the foreground app may come forward; tap Alt first
	// so Windows lets it when the harness (not a person) started it.
	keybdEvent.Call(0x12, 0, 0, 0)
	keybdEvent.Call(0x12, 0, 2, 0)
	setForeground.Call(hwnd)
	setTimer.Call(hwnd, 1, 250, 0)
	logf("window shown")
	var m msg
	for {
		r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTimer:
		el := time.Since(started).Seconds()
		if opts.crash > 0 && el >= opts.crash {
			logf("crashing")
			os.Exit(-1073741819) // 0xC0000005, as an access violation ends a process
		}
		if opts.run > 0 && el >= opts.run {
			destroyWindow.Call(hwnd)
			return 0
		}
		invalidateRect.Call(hwnd, 0, 1)
		return 0
	case wmKeyDown:
		if wParam == vkEscape {
			destroyWindow.Call(hwnd)
		}
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		selectObject.Call(hdc, font)
		setBkMode.Call(hdc, 1)
		setTextColor.Call(hdc, 0xf0e0d0)
		text := fmt.Sprintf("%s\n%.0f s · pid %d · Esc to quit", opts.title, time.Since(started).Seconds(), os.Getpid())
		t, _ := windows.UTF16FromString(text)
		r.top = r.bottom / 3
		drawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x1) // DT_CENTER
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	}
	r, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}
