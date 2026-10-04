package syncer

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/ApolloF/Seaglass/internal/platform"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\ApolloFSyncer`

func dialPipe(ctx context.Context) (*Client, error) {
	dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(dctx, Pipe)
	if err != nil {
		return nil, err
	}
	if err := checkServer(conn); err != nil {
		conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, pending: map[int64]chan response{}, closed: make(chan struct{})}
	go c.read()
	return c, nil
}

// checkServer makes sure the pipe is served by a process of this Windows
// user, so another account can't pose as Syncer.
func checkServer(conn net.Conn) error {
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("can't check who serves the Syncer pipe")
	}
	var pid uint32
	if err := getNamedPipeServerProcessID(windows.Handle(f.Fd()), &pid); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var tok windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	theirs, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	mine, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !windows.EqualSid(theirs.User.Sid, mine.User.Sid) {
		return errors.New("the Syncer pipe belongs to another user")
	}
	return nil
}

var procGetNamedPipeServerProcessId = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeServerProcessId")

func getNamedPipeServerProcessID(h windows.Handle, pid *uint32) error {
	r, _, err := procGetNamedPipeServerProcessId.Call(uintptr(h), uintptr(unsafe.Pointer(pid)))
	if r == 0 {
		return err
	}
	return nil
}

// Find returns the installed Syncer.
func Find() (Install, bool) {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, uninstallKey, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		icon, _, _ := k.GetStringValue("DisplayIcon")
		ver, _, _ := k.GetStringValue("DisplayVersion")
		k.Close()
		exe := strings.Trim(strings.SplitN(icon, ",", 2)[0], "\" ")
		if strings.EqualFold(filepath.Base(exe), "Syncer.exe") && filepath.IsAbs(exe) && platform.IsFile(exe) {
			return Install{Exe: exe, Version: strings.TrimPrefix(strings.TrimSpace(ver), "v")}, true
		}
	}
	if exe := filepath.Join(platform.Local, "Programs", "Syncer", "Syncer.exe"); platform.IsFile(exe) {
		return Install{Exe: exe}, true
	}
	return Install{}, false
}
