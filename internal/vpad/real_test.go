package vpad

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// xinputState is XINPUT_STATE.
type xinputState struct {
	PacketNumber uint32
	Gamepad      Report // XINPUT_GAMEPAD has XUSB_REPORT's layout
}

// Plugs a virtual Xbox 360 controller into the real ViGEmBus, feeds it
// known input and checks Windows sees it through XInput, then unplugs it.
// Needs ViGEmBus installed; no physical controller.
//
//	WL_REAL_VIGEM=1 go test -run RealViGEm -v ./internal/vpad
func TestRealViGEm(t *testing.T) {
	if os.Getenv("WL_REAL_VIGEM") == "" {
		t.Skip("set WL_REAL_VIGEM=1 to plug a virtual controller into this PC's ViGEmBus")
	}
	t.Logf("status: %+v", GetStatus(filepath.Join(t.TempDir(), "hidden-controllers.json")))
	x, err := NewX360()
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if err := x.Plug(); err != nil {
		t.Fatal(err)
	}
	user, err := x.userIndex()
	if err != nil {
		t.Fatalf("user index: %v", err)
	}
	t.Logf("XInput slot %d", user)

	want := FromSDL(1<<0|1<<3|1<<11|1<<20, [6]int16{12000, -20000, -32768, 32767, 32767, 16384})
	if err := x.Send(want); err != nil {
		t.Fatal(err)
	}
	xinput := windows.NewLazySystemDLL("xinput1_4.dll").NewProc("XInputGetState")
	var st xinputState
	deadline := time.Now().Add(3 * time.Second)
	for {
		r, _, _ := xinput.Call(uintptr(user), uintptr(unsafe.Pointer(&st)))
		if r == 0 && st.Gamepad == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("XInputGetState(%d) = %d, %+v; want %+v", user, r, st.Gamepad, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := x.Close(); err != nil {
		t.Fatalf("unplug: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		r, _, _ := xinput.Call(uintptr(user), uintptr(unsafe.Pointer(&st)))
		if r == uintptr(windows.ERROR_DEVICE_NOT_CONNECTED) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after unplugging, XInputGetState(%d) = %d", user, r)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
