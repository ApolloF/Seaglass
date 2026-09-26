package app

import "testing"

func TestDevArgs(t *testing.T) {
	a := ParseArgs([]string{"--virtual-pad=xbox", "--remote-debugging=9333", "--dev-data=C:/wl test"})
	if a.Dev != (DevArgs{VirtualPad: "xbox", CDPPort: 9333, DataDir: "C:/wl test"}) {
		t.Errorf("dev args %+v", a.Dev)
	}
	if b := ParseArgs([]string{"--virtual-pad"}); b.Dev.VirtualPad != "ps" || !b.Dev.Any() {
		t.Errorf("--virtual-pad alone: %+v", b.Dev)
	}
	if c := ParseArgs([]string{"--tray"}); c.Dev.Any() {
		t.Errorf("no dev flags expected: %+v", c.Dev)
	}
	// Release builds ignore them.
	if DevAllowed("v1.3.0") || !DevAllowed("dev") {
		t.Error("dev flags must only work in dev builds")
	}
}
