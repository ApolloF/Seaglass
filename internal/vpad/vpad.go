// Package vpad shows games a virtual Xbox 360 controller and hides the
// physical one from them, so a PlayStation (or any other) controller works
// in games that only understand Xbox controllers, without Steam.
//
// The virtual controller comes from the ViGEmBus driver and the hiding from
// the HidHide driver. Neither is bundled: both are installed on request from
// their official releases (see Installers). The code here talks to them
// through DeviceIoControl; the request codes and structures are ported from
// ViGEmClient and HidHide (both MIT, credited in THIRD_PARTY_NOTICES.txt).
package vpad

import "errors"

// Bridge is a virtual controller a game sees. ViGEmBus is the only backend
// today; the interface leaves room for another (VIIPER over usbip-win2)
// should ViGEmBus stop working on a future Windows.
type Bridge interface {
	// Plug connects the virtual controller and returns once games can see it.
	Plug() error
	// Send sets what the virtual controller reports.
	Send(Report) error
	// Rumble sets where the game's rumble goes (strengths 0–255 for the
	// large, slow motor and the small, quick one); nil drops it.
	Rumble(fn func(large, small uint8))
	// Close unplugs the virtual controller and lets go of the driver.
	Close() error
}

// ErrNoViGEm means ViGEmBus isn't installed (or doesn't answer).
var ErrNoViGEm = errors.New("ViGEmBus isn't installed")

// ErrNoHidHide means HidHide isn't installed (or doesn't answer).
var ErrNoHidHide = errors.New("HidHide isn't installed")

// ErrOldHidHide means HidHide is too old for the per-process list
// Seaglass hides controllers with (it came in 1.7.333).
var ErrOldHidHide = errors.New("HidHide is too old to hide a controller only while a game runs")
