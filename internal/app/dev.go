package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/pad"
)

// Dev flags drive the real app from a test harness (tools/harness). They
// only work in dev builds: a release build ignores them.
//
//	--virtual-pad[=ps|xbox]  plug in a virtual controller (SDL's own)
//	--remote-debugging=PORT  WebView2's DevTools protocol on 127.0.0.1:PORT
//	--dev-data=DIR           library, settings and log in DIR; no scans,
//	                         metadata, store accounts or update checks
//
// Any of them opens the control pipe DevPipe, one command per line:
//
//	press <button> [ms]   down, wait (default 80 ms), up
//	down|up <button>      buttons: south east west north back guide start
//	                      lb rb up down left right touchpad leftstick …
//	axis <axis> <value>   lx ly rx ry lt rt, -32768…32767
//	plug ps|xbox, unplug
//	state                 controller, session, windows (JSON)
//	mem                   the core's memory (JSON)
//	quit
type DevArgs struct {
	VirtualPad string // "", "ps" or "xbox"
	CDPPort    int
	DataDir    string
}

// Any reports whether a dev flag was given.
func (d DevArgs) Any() bool { return d.VirtualPad != "" || d.CDPPort != 0 || d.DataDir != "" }

// DevPipe is the control pipe's name.
const DevPipe = `\\.\pipe\seaglass-dev`

func parseDevArg(a *DevArgs, s string) {
	name, val, _ := strings.Cut(s, "=")
	switch name {
	case "--virtual-pad":
		a.VirtualPad = "ps"
		if val == "xbox" {
			a.VirtualPad = "xbox"
		}
	case "--remote-debugging":
		a.CDPPort, _ = strconv.Atoi(val)
	case "--dev-data":
		a.DataDir = val
	}
}

// DevAllowed reports whether dev flags work in this build.
func DevAllowed(version string) bool { return version == "dev" }

// StartDev opens the control pipe and plugs in the virtual controller.
func StartDev(c *Core, d DevArgs) {
	logx.Printf("dev flags: %+v", d)
	l, err := listenDevPipe()
	if err != nil {
		logx.Printf("dev pipe: %v", err)
		return
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go devServe(c, conn)
		}
	}()
	if d.VirtualPad != "" {
		go func() {
			// The controller layer starts with the pad service.
			for k := 0; k < 100 && c.padManager() == nil; k++ {
				time.Sleep(50 * time.Millisecond)
			}
			if err := devPlug(c, d.VirtualPad); err != nil {
				logx.Printf("virtual pad: %v", err)
			}
		}()
	}
}

func devPlug(c *Core, kind string) error {
	m := c.padManager()
	if m == nil {
		return fmt.Errorf("no controller layer")
	}
	k := pad.PlayStation
	if kind == "xbox" {
		k = pad.Xbox
	}
	return m.PlugVirtual(k)
}

func devServe(c *Core, conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		out, err := devCommand(c, strings.Fields(sc.Text()))
		if err != nil {
			out = "error: " + err.Error()
		}
		if _, err := conn.Write([]byte(out + "\n")); err != nil {
			return
		}
	}
}

func devCommand(c *Core, f []string) (string, error) {
	if len(f) == 0 {
		return "", fmt.Errorf("empty command")
	}
	m := c.padManager()
	button := func() (int, error) {
		if len(f) < 2 {
			return 0, fmt.Errorf("which button?")
		}
		b, ok := pad.VirtualButtons[f[1]]
		if !ok {
			return 0, fmt.Errorf("unknown button %q", f[1])
		}
		if m == nil {
			return 0, fmt.Errorf("no controller layer")
		}
		return b, nil
	}
	switch f[0] {
	case "press":
		b, err := button()
		if err != nil {
			return "", err
		}
		ms := 80
		if len(f) > 2 {
			ms, _ = strconv.Atoi(f[2])
		}
		if err := m.VirtualButton(b, true); err != nil {
			return "", err
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return "ok", m.VirtualButton(b, false)
	case "down", "up":
		b, err := button()
		if err != nil {
			return "", err
		}
		return "ok", m.VirtualButton(b, f[0] == "down")
	case "axis":
		if len(f) < 3 || m == nil {
			return "", fmt.Errorf("axis <name> <value>")
		}
		a, ok := pad.VirtualAxes[f[1]]
		v, err := strconv.Atoi(f[2])
		if !ok || err != nil || v < -32768 || v > 32767 {
			return "", fmt.Errorf("bad axis or value")
		}
		return "ok", m.VirtualAxis(a, int16(v))
	case "plug":
		kind := "ps"
		if len(f) > 1 {
			kind = f[1]
		}
		return "ok", devPlug(c, kind)
	case "unplug":
		if m == nil {
			return "", fmt.Errorf("no controller layer")
		}
		return "ok", m.UnplugVirtual()
	case "state":
		sh := c.shell
		sh.mu.Lock()
		st := map[string]any{
			"pad": c.padState(), "session": c.Launch.Current(), "mainOpen": sh.main != nil,
			"overlayOpen": sh.overlay != nil, "uiMode": sh.uiMode, "gameMode": sh.gameMode,
		}
		sh.mu.Unlock()
		if m != nil {
			st["padMode"] = m.Mode()
		}
		b, err := json.Marshal(st)
		return string(b), err
	case "mem":
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		b, err := json.Marshal(map[string]any{
			"privateBytes": privateBytes(), "heapAlloc": ms.HeapAlloc, "heapHeld": ms.HeapSys - ms.HeapReleased,
			"goroutines": runtime.NumGoroutine(), "games": len(c.Lib.Games()),
		})
		return string(b), err
	case "quit":
		go c.quitForUpdate()
		return "ok", nil
	}
	return "", fmt.Errorf("unknown command %q", f[0])
}
