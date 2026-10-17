package vpad

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

// The structures match the C ones in ViGEmClient's BusShared.h (x64, no
// packing pragmas): the driver rejects a request whose size is off.
func TestViGEmStructSizes(t *testing.T) {
	for name, c := range map[string]struct{ got, want uintptr }{
		"VIGEM_PLUGIN_TARGET":       {unsafe.Sizeof(pluginTarget{}), 16},
		"VIGEM_UNPLUG_TARGET":       {unsafe.Sizeof(sizeAndSerial{}), 8},
		"XUSB_REQUEST_NOTIFICATION": {unsafe.Sizeof(xusbNotification{}), 12},
		"XUSB_SUBMIT_REPORT":        {unsafe.Sizeof(xusbSubmit{}), 20},
		"XUSB_GET_USER_INDEX":       {unsafe.Sizeof(xusbUserIndex{}), 12},
	} {
		if c.got != c.want {
			t.Errorf("sizeof %s = %d, want %d", name, c.got, c.want)
		}
	}
	if o := unsafe.Offsetof(xusbSubmit{}.Report); o != 8 {
		t.Errorf("XUSB_SUBMIT_REPORT.Report at %d, want 8", o)
	}
	if o := unsafe.Offsetof(xusbNotification{}.LargeMotor); o != 8 {
		t.Errorf("XUSB_REQUEST_NOTIFICATION.LargeMotor at %d, want 8", o)
	}
}

// The codes CTL_CODE gives for ViGEmBus's and HidHide's requests.
func TestIoctlCodes(t *testing.T) {
	for name, c := range map[string]struct{ got, want uint32 }{
		"IOCTL_VIGEM_PLUGIN_TARGET":       {ioctlPlugIn, 0x2AA004},
		"IOCTL_VIGEM_UNPLUG_TARGET":       {ioctlUnplug, 0x2AA008},
		"IOCTL_VIGEM_CHECK_VERSION":       {ioctlCheckVersion, 0x2AA00C},
		"IOCTL_VIGEM_WAIT_DEVICE_READY":   {ioctlWaitReady, 0x2AA010},
		"IOCTL_XUSB_REQUEST_NOTIFICATION": {ioctlXUSBNotify, 0x2AE804},
		"IOCTL_XUSB_SUBMIT_REPORT":        {ioctlXUSBSubmit, 0x2AA808},
		"IOCTL_XUSB_GET_USER_INDEX":       {ioctlXUSBUser, 0x2AE81C},
		"IOCTL_GET_WHITELIST":             {ioctlGetWhitelist, 0x80016000},
		"IOCTL_SET_WHITELIST":             {ioctlSetWhitelist, 0x80016004},
		"IOCTL_GET_ACTIVE":                {ioctlGetActive, 0x80016010},
		"IOCTL_GET_WLINVERSE":             {ioctlGetInverse, 0x80016018},
		"IOCTL_ADD_SESSION_BLACKLIST":     {ioctlAddSessionBL, 0x80016020},
		"IOCTL_CLR_SESSION_BLACKLIST":     {ioctlClearSessionB, 0x80016024},
	} {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", name, c.got, c.want)
		}
	}
}

func TestInstanceIDFromSDLPath(t *testing.T) {
	for path, want := range map[string]string{
		// DualSense over USB
		`\\?\HID#VID_054C&PID_0CE6&MI_03#7&1a2b3c4d&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`: `HID\VID_054C&PID_0CE6&MI_03\7&1A2B3C4D&0&0000`,
		// lower case, as Windows often hands it out
		`\\?\hid#vid_054c&pid_09cc&mi_03#8&2f0e&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`: `HID\VID_054C&PID_09CC&MI_03\8&2F0E&0&0000`,
		// Bluetooth: the device id carries a GUID of its own
		`\\?\HID#{00001124-0000-1000-8000-00805f9b34fb}_VID&0002054c_PID&0ce6#9&3a1c&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}`: `HID\{00001124-0000-1000-8000-00805F9B34FB}_VID&0002054C_PID&0CE6\9&3A1C&0&0000`,
	} {
		got, ok := InstanceID(path)
		if !ok || got != want {
			t.Errorf("InstanceID(%s) = %q, %v; want %q", path, got, ok, want)
		}
	}
}

// Only HID device paths can be hidden; other paths SDL reports aren't.
func TestInstanceIDRejectsOtherPaths(t *testing.T) {
	for _, path := range []string{
		"", "XInput#0", `\\?\USB#VID_054C&PID_0CE6#5&1#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`,
		`\\?\HID#VID_054C#{4d1e55b2-f16f-11cf-88cb-001111000030}`, `HID\VID_054C&PID_0CE6\7&1`,
		`\\?\HID##7&1#{4d1e55b2-f16f-11cf-88cb-001111000030}`,
	} {
		if got, ok := InstanceID(path); ok {
			t.Errorf("InstanceID(%q) = %q, accepted", path, got)
		}
	}
}

func TestMultiSZ(t *testing.T) {
	got := multiSZ([]string{"AB", "", "C"})
	want := []uint16{'A', 'B', 0, 'C', 0, 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("multiSZ = %v, want %v", got, want)
	}
	// HidHide takes no less than two NULs, which is also the empty list.
	if got := multiSZ(nil); !reflect.DeepEqual(got, []uint16{0, 0}) {
		t.Errorf("empty multiSZ = %v", got)
	}
	in := []string{`\Device\HarddiskVolume3\Games\Seaglass.exe`, "ü€"}
	if back := parseMultiSZ(multiSZ(in)); !reflect.DeepEqual(back, in) {
		t.Errorf("round trip = %q", back)
	}
	if back := parseMultiSZ([]uint16{0, 0}); len(back) != 0 {
		t.Errorf("empty list parsed as %q", back)
	}
}

func TestInstallersArePinnedToOfficialReleases(t *testing.T) {
	if len(Installers) != 2 {
		t.Fatalf("%d installers", len(Installers))
	}
	for _, in := range Installers {
		if !strings.HasPrefix(in.URL, in.Repo+"/releases/download/") || !strings.HasPrefix(in.Repo, "https://github.com/nefarius/") {
			t.Errorf("%s: %s isn't a release of %s", in.Name, in.URL, in.Repo)
		}
		if len(in.SHA256) != 64 {
			t.Errorf("%s: hash %q", in.Name, in.SHA256)
		}
	}
	if n := Needed(Status{ViGEm: true, HidHide: true}); len(n) != 0 {
		t.Errorf("both installed, still needs %v", n)
	}
	if n := Needed(Status{HidHideVersion: "1.5.230.0"}); len(n) != 1 || n[0].Name != "ViGEmBus" {
		t.Errorf("HidHide waiting for a restart, needs %v", n)
	}
}
