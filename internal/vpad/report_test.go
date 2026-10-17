package vpad

import (
	"testing"
	"unsafe"
)

func TestEveryButtonMapsToItsXboxButton(t *testing.T) {
	for sdl, want := range map[int]uint16{
		0: A, 1: B, 2: X, 3: Y, 4: Back, 6: Start, 7: LeftThumb, 8: RightThumb,
		9: LeftShoulder, 10: RightShoulder, 11: DpadUp, 12: DpadDown, 13: DpadLeft, 14: DpadRight,
		20: Back, // the touchpad click
	} {
		if got := FromSDL(1<<sdl, [6]int16{}).Buttons; got != want {
			t.Errorf("SDL button %d: buttons %#04x, want %#04x", sdl, got, want)
		}
	}
}

// The PS button opens Seaglass's overlay; the game must never see it.
func TestGuideIsNotForwarded(t *testing.T) {
	if got := FromSDL(1<<5, [6]int16{}).Buttons; got != 0 {
		t.Errorf("Guide gave buttons %#04x", got)
	}
	if got := FromSDL(0xFFFFFFFF, [6]int16{}).Buttons; got&Guide != 0 {
		t.Errorf("every button down includes Guide: %#04x", got)
	}
}

func TestOtherButtonsAreDropped(t *testing.T) {
	for _, sdl := range []int{15, 16, 17, 18, 19, 21, 31} {
		if got := FromSDL(1<<sdl, [6]int16{}).Buttons; got != 0 {
			t.Errorf("SDL button %d: buttons %#04x", sdl, got)
		}
	}
}

func TestSticksInvertYOnly(t *testing.T) {
	r := FromSDL(0, [6]int16{1000, 2000, -3000, -4000, 0, 0})
	if r.ThumbLX != 1000 || r.ThumbLY != -2000 || r.ThumbRX != -3000 || r.ThumbRY != 4000 {
		t.Errorf("sticks = %+v", r)
	}
}

func TestStickYEdges(t *testing.T) {
	for in, want := range map[int16]int16{-32768: 32767, 32767: -32767, 0: 0, -1: 1, 1: -1} {
		r := FromSDL(0, [6]int16{0, in, 0, in, 0, 0})
		if r.ThumbLY != want || r.ThumbRY != want {
			t.Errorf("Y %d: left %d, right %d, want %d", in, r.ThumbLY, r.ThumbRY, want)
		}
	}
	// X keeps both extremes as they are.
	if r := FromSDL(0, [6]int16{-32768, 0, 32767, 0, 0, 0}); r.ThumbLX != -32768 || r.ThumbRX != 32767 {
		t.Errorf("X extremes = %+v", r)
	}
}

func TestTriggersScaleTo255(t *testing.T) {
	for in, want := range map[int16]uint8{0: 0, -32768: 0, -1: 0, 1: 0, 128: 1, 16384: 128, 32767: 255} {
		r := FromSDL(0, [6]int16{0, 0, 0, 0, in, in})
		if r.LeftTrigger != want || r.RightTrigger != want {
			t.Errorf("trigger %d: %d/%d, want %d", in, r.LeftTrigger, r.RightTrigger, want)
		}
	}
}

// XUSB_REPORT is 12 bytes (no packing pragma; all fields naturally aligned).
func TestReportMatchesXUSBReport(t *testing.T) {
	if s := unsafe.Sizeof(Report{}); s != 12 {
		t.Errorf("sizeof Report = %d, want 12", s)
	}
	if o := unsafe.Offsetof(Report{}.ThumbLX); o != 4 {
		t.Errorf("offset of ThumbLX = %d, want 4", o)
	}
}
