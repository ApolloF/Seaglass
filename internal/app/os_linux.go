package app

import (
	"net"
	"os"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// listenDevPipe: the harness's control pipe is a Windows named pipe.
func listenDevPipe() (net.Listener, error) { return nil, platform.ErrNotSupported }

// privateBytes is unknown on Linux for now (it would read /proc/self/status).
func privateBytes() uint64 { return 0 }

func osVersion() string { return "Linux (unsupported build)" }

// webView2Version: Linux uses WebKitGTK, not WebView2.
func webView2Version() string { return "not used on Linux" }

// machineID is systemd's machine id, the Linux counterpart of MachineGuid.
func machineID() string {
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
