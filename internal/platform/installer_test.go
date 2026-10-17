package platform

import (
	"testing"
	"unsafe"
)

// SHELLEXECUTEINFOW is 112 bytes on x64, with the process handle last.
func TestShellExecuteInfoLayout(t *testing.T) {
	if s := unsafe.Sizeof(shellExecuteInfo{}); s != 112 {
		t.Errorf("sizeof = %d, want 112", s)
	}
	if o := unsafe.Offsetof(shellExecuteInfo{}.process); o != 104 {
		t.Errorf("hProcess at %d, want 104", o)
	}
}
