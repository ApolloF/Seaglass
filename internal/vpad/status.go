package vpad

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Status is what the two drivers can do on this PC.
type Status struct {
	ViGEm        bool   `json:"viGEm"`                  // ViGEmBus answers: games can get a virtual Xbox controller
	ViGEmVersion string `json:"viGEmVersion,omitempty"` // its driver's version, when known
	HidHide      bool   `json:"hidHide"`                // HidHide answers
	// HidHideVersion is its driver's version, when known.
	HidHideVersion string `json:"hidHideVersion,omitempty"`
	HidHideActive  bool   `json:"hidHideActive"` // its hiding is switched on (block lists only work then)
	// CanHide: HidHide answers and its hiding is switched on.
	CanHide bool `json:"canHide"`
	// Hidden: Seaglass left controllers on HidHide's saved block list (a
	// game is running, or Seaglass didn't get to show them again).
	Hidden bool `json:"hidden"`
	// Restart: a driver is installed but doesn't answer yet; Windows has to
	// restart first.
	Restart bool `json:"restart"`
}

// GetStatus looks at both drivers and the ledger of controllers Seaglass
// hid (see Hider). It only reads; nothing changes.
func GetStatus(ledger string) Status {
	var s Status
	if paths, err := viGEmBusPaths(); err == nil && len(paths) > 0 {
		s.ViGEm = true
	}
	s.ViGEmVersion = driverVersion("ViGEmBus.sys")
	if h, err := openHidHide(); err == nil {
		s.HidHide = true
		d := &Hider{h: h}
		s.HidHideActive, _ = d.Active()
		windows.CloseHandle(h)
	}
	s.HidHideVersion = driverVersion("HidHide.sys")
	s.CanHide = s.HidHide && s.HidHideActive
	s.Hidden = Hidden(ledger)
	s.Restart = !s.ViGEm && serviceInstalled("ViGEmBus") || !s.HidHide && serviceInstalled("HidHide")
	return s
}

// serviceInstalled reports whether a driver's service is registered.
func serviceInstalled(name string) bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+name, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

// driverVersion reads the file version of a driver in System32\drivers,
// "" when it isn't there.
func driverVersion(file string) string {
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return ""
	}
	p := filepath.Join(sys, "drivers", file)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	v, err := fileVersion(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

func fileVersion(path string) ([4]uint16, error) {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil {
		return [4]uint16{}, err
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return [4]uint16{}, err
	}
	var fixed *windows.VS_FIXEDFILEINFO
	var n uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &n); err != nil {
		return [4]uint16{}, err
	}
	if fixed == nil || n < uint32(unsafe.Sizeof(*fixed)) {
		return [4]uint16{}, errors.New("no version")
	}
	return [4]uint16{
		uint16(fixed.FileVersionMS >> 16), uint16(fixed.FileVersionMS),
		uint16(fixed.FileVersionLS >> 16), uint16(fixed.FileVersionLS),
	}, nil
}
