package vpad

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
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
	ioctlGetBlacklist  = hidHideIoctl(2050)
	ioctlSetBlacklist  = hidHideIoctl(2051)
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

// Hider hides physical controllers from games. A HidHide with the
// per-process (session) block list gets them there: entries live only in
// the driver's memory and go when Seaglass exits, even if it crashes.
// Older ones (every release up to 1.5) only have the block list saved with
// HidHide's settings. Seaglass then notes each controller it adds in a
// ledger file before adding it, and takes exactly those off again when the
// game ends, when Seaglass starts or quits, or from Settings.
type Hider struct {
	mu     sync.Mutex
	h      windows.Handle
	ledger string // the file listing what Seaglass put on the saved block list
}

// OpenHider opens HidHide. It fails with ErrNoHidHide when it isn't there.
// ledger is the file that remembers the controllers Seaglass hid through
// the saved block list.
func OpenHider(ledger string) (*Hider, error) {
	h, err := openHidHide()
	if err != nil {
		return nil, ErrNoHidHide
	}
	return &Hider{h: h, ledger: ledger}, nil
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

// unsupported reports whether HidHide refused a request it doesn't know.
func unsupported(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_INVALID_FUNCTION)
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

// Hide puts controllers (by device instance id) on a block list. Opening
// a hidden device fails for every app not on HidHide's allow list, this
// one included, so Seaglass then puts itself on it (see allowSelf).
// Handles open already keep working meanwhile.
func (d *Hider) Hide(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := syncIoctl(d.h, ioctlAddSessionBL, multiSZ(ids), nil)
	if unsupported(err) {
		err = d.hideSaved(ids)
	}
	if err != nil {
		return err
	}
	if err := d.allowSelf(); err != nil {
		// Hidden from Seaglass too, it would lose the controller the next
		// time SDL looks for devices: better not to hide it at all.
		_ = d.clear()
		return err
	}
	return nil
}

// hideSaved adds controllers to the saved block list. It notes them in the
// ledger first, so that a crash right after still leaves a record to undo.
// Entries the person had on the list already stay out of the ledger, so
// they stay hidden after the game.
func (d *Hider) hideSaved(ids []string) error {
	list, err := d.readList(ioctlGetBlacklist)
	if err != nil {
		return err
	}
	mine := readLedger(d.ledger)
	var add []string
	for _, id := range ids {
		if containsFold(list, id) {
			continue
		}
		add = append(add, id)
		if !containsFold(mine, id) {
			mine = append(mine, id)
		}
	}
	if len(add) == 0 {
		return nil
	}
	if err := writeLedger(d.ledger, mine); err != nil {
		return err
	}
	_, err = syncIoctl(d.h, ioctlSetBlacklist, multiSZ(append(list, add...)), nil)
	return err
}

// Clear shows every controller Seaglass hid again.
func (d *Hider) Clear() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.clear()
}

func (d *Hider) clear() error {
	_, err := syncIoctl(d.h, ioctlClearSessionB, nil, nil)
	if unsupported(err) {
		err = nil
	}
	return errors.Join(err, d.clearSaved())
}

// clearSaved takes the controllers in the ledger off the saved block list.
func (d *Hider) clearSaved() error {
	mine := readLedger(d.ledger)
	if len(mine) == 0 {
		return nil
	}
	list, err := d.readList(ioctlGetBlacklist)
	if err != nil {
		return err
	}
	keep := slices.DeleteFunc(slices.Clone(list), func(e string) bool { return containsFold(mine, e) })
	if len(keep) != len(list) {
		if _, err := syncIoctl(d.h, ioctlSetBlacklist, multiSZ(keep), nil); err != nil {
			return err
		}
	}
	return removeLedger(d.ledger)
}

// Close shows the controllers again and lets go of HidHide.
func (d *Hider) Close() error {
	err := d.Clear()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.h != 0 {
		windows.CloseHandle(d.h)
		d.h = 0
	}
	return err
}

// Unhide shows every controller Seaglass left on HidHide's saved block
// list, for example after a crash. With an empty ledger there is nothing
// to do. Without HidHide nothing can be hidden, so the ledger goes.
func Unhide(ledger string) error {
	if !Hidden(ledger) {
		return nil
	}
	d, err := OpenHider(ledger)
	if err != nil {
		return removeLedger(ledger)
	}
	return d.Close()
}

// Hidden reports whether the ledger lists controllers Seaglass hid.
func Hidden(ledger string) bool { return len(readLedger(ledger)) > 0 }

func containsFold(list []string, s string) bool {
	return slices.ContainsFunc(list, func(e string) bool { return strings.EqualFold(e, s) })
}

// readLedger lists the controllers in the ledger. A missing or unreadable
// file lists none.
func readLedger(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var ids []string
	if json.Unmarshal(b, &ids) != nil {
		return nil
	}
	return ids
}

// writeLedger saves the ledger atomically (tmp, then rename).
func writeLedger(path string, ids []string) error {
	b, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func removeLedger(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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
	list, err := d.readList(ioctlGetWhitelist)
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

// readList reads one of HidHide's lists (allow or block).
func (d *Hider) readList(code uint32) ([]string, error) {
	// Asked without a buffer, HidHide says how many bytes the list takes.
	n, err := syncIoctl(d.h, code, nil, nil)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		n = 4
	}
	buf := make([]byte, n)
	got, err := syncIoctl(d.h, code, nil, buf)
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
