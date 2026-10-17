package vpad

import (
	"errors"
	"os"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// HidHide's control device and request codes, from HidHide's
// Shared/HidHideIoctlContract.h (MIT, Eric Korff de Gidts and
// Benjamin Höglinger-Stelzer).
const hidHideDevice = `\\.\HidHide`

const hidHideDeviceType = 32769

func hidHideIoctl(n uint32) uint32 {
	return ctlCode(hidHideDeviceType, n, methodBuffered, fileReadData)
}

var (
	ioctlGetWhitelist  = hidHideIoctl(2048)
	ioctlSetWhitelist  = hidHideIoctl(2049)
	ioctlGetActive     = hidHideIoctl(2052)
	ioctlGetInverse    = hidHideIoctl(2054)
	ioctlAddSessionBL  = hidHideIoctl(2056)
	ioctlClearSessionB = hidHideIoctl(2057)
)

// InstanceID turns the device path SDL reports for a HID controller
// (\\?\HID#VID_054C&PID_0CE6&MI_03#7&1a2b&0&0000#{4d1e55b2-…}) into the
// device instance id HidHide lists (HID\VID_054C&PID_0CE6&MI_03\7&1A2B&0&0000).
// ok is false for anything that isn't a HID device path.
func InstanceID(path string) (string, bool) {
	p, ok := strings.CutPrefix(path, `\\?\`)
	if !ok {
		if p, ok = strings.CutPrefix(path, `\\.\`); !ok {
			return "", false
		}
	}
	parts := strings.Split(p, "#")
	// The last part is the interface class GUID; Bluetooth device ids carry
	// a GUID of their own in the second part, which stays.
	if n := len(parts); n > 0 && strings.HasPrefix(parts[n-1], "{") {
		parts = parts[:n-1]
	}
	if len(parts) < 3 || !strings.EqualFold(parts[0], "HID") {
		return "", false
	}
	for _, s := range parts {
		if s == "" || strings.ContainsAny(s, `\/`+"\x00") {
			return "", false
		}
	}
	return strings.ToUpper(strings.Join(parts, `\`)), true
}

// multiSZ encodes strings as a REG_MULTI_SZ: each NUL-terminated UTF-16,
// then one more NUL. An empty list is two NULs, which HidHide requires as
// the least it accepts.
func multiSZ(list []string) []uint16 {
	var out []uint16
	for _, s := range list {
		if s == "" {
			continue // an empty string would end the list early
		}
		out = append(out, utf16.Encode([]rune(s))...)
		out = append(out, 0)
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	return append(out, 0)
}

// parseMultiSZ decodes a REG_MULTI_SZ.
func parseMultiSZ(b []uint16) []string {
	var out []string
	for len(b) > 0 && b[0] != 0 {
		end := 0
		for end < len(b) && b[end] != 0 {
			end++
		}
		out = append(out, string(utf16.Decode(b[:end])))
		if end == len(b) {
			break
		}
		b = b[end+1:]
	}
	return out
}

// Hider hides physical controllers from games through HidHide's session
// block list: entries live only in the driver's memory and belong to this
// process, so they go when Seaglass exits, even if it crashes.
type Hider struct {
	mu sync.Mutex
	h  windows.Handle
}

// OpenHider opens HidHide. It fails with ErrNoHidHide when it isn't there.
func OpenHider() (*Hider, error) {
	h, err := openHidHide()
	if err != nil {
		return nil, ErrNoHidHide
	}
	return &Hider{h: h}, nil
}

func openHidHide() (windows.Handle, error) {
	p, _ := windows.UTF16PtrFromString(hidHideDevice)
	return windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
}

// syncIoctl sends a request on a handle opened without FILE_FLAG_OVERLAPPED.
func syncIoctl(h windows.Handle, code uint32, in []uint16, out []byte) (uint32, error) {
	var inPtr, outPtr *byte
	if len(in) > 0 {
		inPtr = (*byte)(unsafe.Pointer(&in[0]))
	}
	if len(out) > 0 {
		outPtr = &out[0]
	}
	var n uint32
	err := windows.DeviceIoControl(h, code, inPtr, uint32(len(in)*2), outPtr, uint32(len(out)), &n, nil)
	return n, err
}

// Active reports whether HidHide's hiding is switched on. Its block lists
// only take effect while it is.
func (d *Hider) Active() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]byte, 1)
	if _, err := syncIoctl(d.h, ioctlGetActive, nil, out); err != nil {
		return false, err
	}
	return out[0] != 0, nil
}

// Hide adds controllers (by device instance id) to the session block list.
// Opening a hidden device fails for every app not on HidHide's allow list,
// this one included, so Seaglass then puts itself on it (see allowSelf).
// Handles open already keep working meanwhile. An older HidHide refuses
// the request, and its allow list is left alone then.
func (d *Hider) Hide(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := syncIoctl(d.h, ioctlAddSessionBL, multiSZ(ids), nil)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		return ErrOldHidHide
	}
	if err != nil {
		return err
	}
	if err := d.allowSelf(); err != nil {
		// Hidden from Seaglass too, it would lose the controller the next
		// time SDL looks for devices: better not to hide it at all.
		_, _ = syncIoctl(d.h, ioctlClearSessionB, nil, nil)
		return err
	}
	return nil
}

// Clear takes every controller this process hid off the list.
func (d *Hider) Clear() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := syncIoctl(d.h, ioctlClearSessionB, nil, nil)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		return ErrOldHidHide
	}
	return err
}

// Close clears the list and lets go of HidHide.
func (d *Hider) Close() error {
	err := d.Clear()
	if errors.Is(err, ErrOldHidHide) {
		err = nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.h != 0 {
		windows.CloseHandle(d.h)
		d.h = 0
	}
	return err
}

// allowSelf puts Seaglass's exe on HidHide's allow list, the only way for
// it to keep reading a controller it hid: SDL opens the device again when
// devices change (the virtual controller plugging in is one such change),
// and a controller that's unplugged and plugged back in is a new open too.
// It only ever adds its own entry, as HidHide asks of apps, and leaves the
// list alone when HidHide's "inverse" mode turns it into a deny list.
func (d *Hider) allowSelf() error {
	inv := make([]byte, 1)
	if _, err := syncIoctl(d.h, ioctlGetInverse, nil, inv); err == nil && inv[0] != 0 {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	self, err := fullImageName(exe)
	if err != nil {
		return err
	}
	list, err := d.whitelist()
	if err != nil {
		return err
	}
	for _, e := range list {
		if strings.EqualFold(e, self) {
			return nil
		}
	}
	_, err = syncIoctl(d.h, ioctlSetWhitelist, multiSZ(append(list, self)), nil)
	return err
}

func (d *Hider) whitelist() ([]string, error) {
	// Asked without a buffer, HidHide says how many bytes the list takes.
	n, err := syncIoctl(d.h, ioctlGetWhitelist, nil, nil)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		n = 4
	}
	buf := make([]byte, n)
	got, err := syncIoctl(d.h, ioctlGetWhitelist, nil, buf)
	if err != nil {
		return nil, err
	}
	u := make([]uint16, got/2)
	for i := range u {
		u[i] = uint16(buf[2*i]) | uint16(buf[2*i+1])<<8
	}
	return parseMultiSZ(u), nil
}

// fullImageName turns C:\path\app.exe into \Device\HarddiskVolume3\path\app.exe,
// the form HidHide's allow list uses (the kernel's name for the image).
func fullImageName(path string) (string, error) {
	vol := path[:min(2, len(path))]
	if len(vol) != 2 || vol[1] != ':' {
		return "", errors.New("Seaglass isn't on a drive letter")
	}
	buf := make([]uint16, windows.MAX_PATH)
	v, _ := windows.UTF16PtrFromString(vol)
	if _, err := windows.QueryDosDevice(v, &buf[0], uint32(len(buf))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf) + path[2:], nil
}
