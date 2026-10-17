package vpad

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The ViGEmBus client protocol, ported from ViGEmClient
// (include/ViGEm/km/BusShared.h, include/ViGEm/Common.h and
// src/ViGEmClient.cpp; MIT, Nefarius Software Solutions e.U. and
// Contributors).

// viGEmInterface is GUID_DEVINTERFACE_BUSENUM_VIGEM, the bus's device
// interface.
var viGEmInterface = windows.GUID{Data1: 0x96E42B22, Data2: 0xF5E9, Data3: 0x42F8, Data4: [8]byte{0xB0, 0x43, 0xED, 0x0F, 0x93, 0x2F, 0x01, 0x4F}}

// ctlCode is the CTL_CODE macro.
func ctlCode(deviceType, function, method, access uint32) uint32 {
	return deviceType<<16 | access<<14 | function<<2 | method
}

const (
	fileDeviceBusExtender = 0x2A // FILE_DEVICE_BUS_EXTENDER
	methodBuffered        = 0
	fileReadData          = 1
	fileWriteData         = 2

	viGEmBase = 0x801

	viGEmCommonVersion = 0x0001 // VIGEM_COMMON_VERSION
	viGEmTargetsMax    = 16     // VIGEM_TARGETS_MAX
	xbox360Wired       = 0      // VIGEM_TARGET_TYPE Xbox360Wired
)

var (
	ioctlPlugIn       = ctlCode(fileDeviceBusExtender, viGEmBase+0x000, methodBuffered, fileWriteData)
	ioctlUnplug       = ctlCode(fileDeviceBusExtender, viGEmBase+0x001, methodBuffered, fileWriteData)
	ioctlCheckVersion = ctlCode(fileDeviceBusExtender, viGEmBase+0x002, methodBuffered, fileWriteData)
	ioctlWaitReady    = ctlCode(fileDeviceBusExtender, viGEmBase+0x003, methodBuffered, fileWriteData)
	ioctlXUSBNotify   = ctlCode(fileDeviceBusExtender, viGEmBase+0x200, methodBuffered, fileWriteData|fileReadData)
	ioctlXUSBSubmit   = ctlCode(fileDeviceBusExtender, viGEmBase+0x201, methodBuffered, fileWriteData)
	ioctlXUSBUser     = ctlCode(fileDeviceBusExtender, viGEmBase+0x206, methodBuffered, fileWriteData|fileReadData)
)

// VIGEM_PLUGIN_TARGET
type pluginTarget struct {
	Size       uint32
	SerialNo   uint32
	TargetType int32
	VendorID   uint16
	ProductID  uint16
}

// VIGEM_UNPLUG_TARGET, VIGEM_CHECK_VERSION and VIGEM_WAIT_DEVICE_READY
// share this layout (the second field is the serial or the version).
type sizeAndSerial struct {
	Size     uint32
	SerialNo uint32
}

// XUSB_REQUEST_NOTIFICATION
type xusbNotification struct {
	Size       uint32
	SerialNo   uint32
	LargeMotor uint8
	SmallMotor uint8
	LedNumber  uint8
}

// XUSB_SUBMIT_REPORT
type xusbSubmit struct {
	Size     uint32
	SerialNo uint32
	Report   Report
}

// XUSB_GET_USER_INDEX
type xusbUserIndex struct {
	Size      uint32
	SerialNo  uint32
	UserIndex uint32
}

// viGEmBusPaths lists the device paths of the ViGEm buses present.
func viGEmBusPaths() ([]string, error) {
	return windows.CM_Get_Device_Interface_List("", &viGEmInterface, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
}

// X360 is a virtual Xbox 360 controller on ViGEmBus.
type X360 struct {
	h windows.Handle

	mu     sync.Mutex // one request at a time on send and the fields below
	serial uint32     // 0 until plugged in
	submit *xusbSubmit
	ev     windows.Handle // for Send's requests

	rumbleMu sync.Mutex
	rumble   func(large, small uint8)
	notify   chan struct{} // closed when the rumble loop has ended
	closing  atomic.Bool   // Close has begun; the rumble loop stops
}

var _ Bridge = (*X360)(nil)

// NewX360 opens ViGEmBus for a virtual Xbox 360 controller (not yet
// plugged in). It fails with ErrNoViGEm when the bus isn't there.
func NewX360() (*X360, error) {
	paths, err := viGEmBusPaths()
	if err != nil || len(paths) == 0 {
		return nil, ErrNoViGEm
	}
	var last error = ErrNoViGEm
	for _, p := range paths {
		h, err := openBus(p)
		if err != nil {
			last = fmt.Errorf("couldn't open ViGEmBus: %w", err)
			continue
		}
		// The bus only serves clients built for its protocol version.
		v := &sizeAndSerial{Size: uint32(unsafe.Sizeof(sizeAndSerial{})), SerialNo: viGEmCommonVersion}
		if err := ioctl(h, ioctlCheckVersion, unsafe.Pointer(v), v.Size, nil, 0); err != nil {
			windows.CloseHandle(h)
			last = errors.New("ViGEmBus speaks another protocol version")
			continue
		}
		ev, err := windows.CreateEvent(nil, 0, 0, nil)
		if err != nil {
			windows.CloseHandle(h)
			return nil, err
		}
		return &X360{h: h, ev: ev, submit: &xusbSubmit{Size: uint32(unsafe.Sizeof(xusbSubmit{}))}}, nil
	}
	return nil, last
}

func openBus(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_NO_BUFFERING|windows.FILE_FLAG_WRITE_THROUGH|windows.FILE_FLAG_OVERLAPPED, 0)
}

// ioctl sends one request on an overlapped handle and waits for it. in and
// out must point to heap memory: the driver may fill out after
// DeviceIoControl returns, and a goroutine's stack can move meanwhile.
func ioctl(h windows.Handle, code uint32, in unsafe.Pointer, inLen uint32, out unsafe.Pointer, outLen uint32) error {
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ev)
	return ioctlWith(h, ev, code, in, inLen, out, outLen)
}

func ioctlWith(h, ev windows.Handle, code uint32, in unsafe.Pointer, inLen uint32, out unsafe.Pointer, outLen uint32) error {
	ov := &windows.Overlapped{HEvent: ev}
	var n uint32
	err := windows.DeviceIoControl(h, code, (*byte)(in), inLen, (*byte)(out), outLen, &n, ov)
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		return err
	}
	err = windows.GetOverlappedResult(h, ov, &n, true)
	runtime.KeepAlive(in)
	runtime.KeepAlive(out)
	runtime.KeepAlive(ov)
	return err
}

// Plug connects the controller on the first free slot of the bus and waits
// until it can take reports.
func (x *X360) Plug() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.serial != 0 {
		return nil
	}
	in := &pluginTarget{Size: uint32(unsafe.Sizeof(pluginTarget{})), TargetType: xbox360Wired}
	var err error
	for serial := uint32(1); serial <= viGEmTargetsMax; serial++ {
		in.SerialNo = serial
		if err = ioctl(x.h, ioctlPlugIn, unsafe.Pointer(in), in.Size, nil, 0); err != nil {
			continue // that slot is taken (by another app's controller)
		}
		w := &sizeAndSerial{Size: uint32(unsafe.Sizeof(sizeAndSerial{})), SerialNo: serial}
		// Older buses don't know the wait request (ViGEmClient carries on then).
		if err = ioctl(x.h, ioctlWaitReady, unsafe.Pointer(w), w.Size, nil, 0); err != nil && !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			u := &sizeAndSerial{Size: uint32(unsafe.Sizeof(sizeAndSerial{})), SerialNo: serial}
			_ = ioctl(x.h, ioctlUnplug, unsafe.Pointer(u), u.Size, nil, 0)
			return fmt.Errorf("the virtual controller didn't start: %w", err)
		}
		x.serial = serial
		x.submit.SerialNo = serial
		x.notify = make(chan struct{})
		go x.rumbleLoop(serial, x.notify)
		return nil
	}
	return fmt.Errorf("couldn't plug in a virtual controller: %w", err)
}

// Send sets the controller's buttons, sticks and triggers.
func (x *X360) Send(r Report) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.serial == 0 {
		return errors.New("the virtual controller isn't plugged in")
	}
	x.submit.Report = r
	// The bus takes the report even when it ends the request with another
	// error (for instance while no game reads the controller yet), so only
	// "access denied", a controller that's gone, counts, as in ViGEmClient.
	err := ioctlWith(x.h, x.ev, ioctlXUSBSubmit, unsafe.Pointer(x.submit), x.submit.Size, nil, 0)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return errors.New("the virtual controller is gone")
	}
	return nil
}

// Rumble sets where the game's rumble goes.
func (x *X360) Rumble(fn func(large, small uint8)) {
	x.rumbleMu.Lock()
	x.rumble = fn
	x.rumbleMu.Unlock()
}

// rumbleLoop waits for the game to change the rumble, until the controller
// is unplugged or Close cancels the request. Other errors only mean asking
// again, as in ViGEmClient.
func (x *X360) rumbleLoop(serial uint32, done chan struct{}) {
	defer close(done)
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return
	}
	defer windows.CloseHandle(ev)
	n := &xusbNotification{Size: uint32(unsafe.Sizeof(xusbNotification{})), SerialNo: serial}
	for {
		err := ioctlWith(x.h, ev, ioctlXUSBNotify, unsafe.Pointer(n), n.Size, unsafe.Pointer(n), n.Size)
		if x.closing.Load() || errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
			return
		}
		if err != nil {
			n.Size, n.SerialNo = uint32(unsafe.Sizeof(xusbNotification{})), serial
			time.Sleep(10 * time.Millisecond) // no busy loop if the bus keeps refusing
			continue
		}
		x.rumbleMu.Lock()
		fn := x.rumble
		x.rumbleMu.Unlock()
		if fn != nil {
			fn(n.LargeMotor, n.SmallMotor)
		}
		n.Size, n.SerialNo = uint32(unsafe.Sizeof(xusbNotification{})), serial
	}
}

// userIndex is the XInput slot (0–3) Windows gave the controller.
func (x *X360) userIndex() (uint32, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	u := &xusbUserIndex{Size: uint32(unsafe.Sizeof(xusbUserIndex{})), SerialNo: x.serial}
	if err := ioctl(x.h, ioctlXUSBUser, unsafe.Pointer(u), u.Size, unsafe.Pointer(u), u.Size); err != nil {
		return 0, err
	}
	return u.UserIndex, nil
}

// Close unplugs the controller and closes the bus. Closing the handle alone
// would also remove it (the bus drops a closed client's controllers), but
// unplugging first makes the game see it go right away.
func (x *X360) Close() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.h == 0 {
		return nil
	}
	x.closing.Store(true)
	var err error
	if x.serial != 0 {
		u := &sizeAndSerial{Size: uint32(unsafe.Sizeof(sizeAndSerial{})), SerialNo: x.serial}
		err = ioctl(x.h, ioctlUnplug, unsafe.Pointer(u), u.Size, nil, 0)
		x.serial = 0
	}
	// The rumble request may still be pending if the unplug failed.
	_ = windows.CancelIoEx(x.h, nil)
	if x.notify != nil {
		<-x.notify
		x.notify = nil
	}
	windows.CloseHandle(x.ev)
	windows.CloseHandle(x.h)
	x.h = 0
	return err
}
