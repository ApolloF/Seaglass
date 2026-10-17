package vpad

// Report is what an Xbox 360 controller reports: XUSB_REPORT in
// ViGEmClient, the same layout as XInput's XINPUT_GAMEPAD.
type Report struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

// XUSB button bits (XUSB_BUTTON in ViGEmClient, XINPUT_GAMEPAD_* in XInput).
const (
	DpadUp        = 0x0001
	DpadDown      = 0x0002
	DpadLeft      = 0x0004
	DpadRight     = 0x0008
	Start         = 0x0010
	Back          = 0x0020
	LeftThumb     = 0x0040
	RightThumb    = 0x0080
	LeftShoulder  = 0x0100
	RightShoulder = 0x0200
	Guide         = 0x0400
	A             = 0x1000
	B             = 0x2000
	X             = 0x4000
	Y             = 0x8000
)

// sdlToXUSB maps SDL gamepad buttons (by position, so ✕ is A) to XUSB bits.
// Guide (5) is left out on purpose: the PS button stays Seaglass's overlay
// button and never reaches the game. The touchpad click (20) is Back, like
// Create (4): it's the big button next to it, and the one people press when
// a game shows the View glyph.
var sdlToXUSB = [...]uint16{
	0: A, 1: B, 2: X, 3: Y,
	4: Back, 6: Start,
	7: LeftThumb, 8: RightThumb,
	9: LeftShoulder, 10: RightShoulder,
	11: DpadUp, 12: DpadDown, 13: DpadLeft, 14: DpadRight,
	20: Back,
}

// FromSDL turns an SDL gamepad's state into an Xbox 360 report. buttons has
// bit n set while SDL gamepad button n is down; axes are SDL's left stick
// x and y, right stick x and y, left and right trigger.
func FromSDL(buttons uint32, axes [6]int16) Report {
	var r Report
	for b, bit := range sdlToXUSB {
		if bit != 0 && buttons&(1<<b) != 0 {
			r.Buttons |= bit
		}
	}
	r.ThumbLX, r.ThumbLY = axes[0], invert(axes[1])
	r.ThumbRX, r.ThumbRY = axes[2], invert(axes[3])
	r.LeftTrigger, r.RightTrigger = trigger(axes[4]), trigger(axes[5])
	return r
}

// invert flips a stick's Y axis: SDL counts down as positive, XInput up.
// -32768 has no positive twin in an int16, so it becomes 32767.
func invert(v int16) int16 {
	if v == -32768 {
		return 32767
	}
	return -v
}

// trigger scales SDL's 0..32767 to XInput's 0..255, rounding to nearest.
func trigger(v int16) uint8 {
	if v <= 0 {
		return 0
	}
	return uint8((int32(v)*255 + 32767/2) / 32767)
}
