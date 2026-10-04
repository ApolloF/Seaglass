package platform

import (
	"os"
	"strings"
	"testing"
)

// The test's own process reads back as its own exe, with a start time.
func TestProcessImageOfItself(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path, started, err := ProcessImage(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(path, exe) || started == 0 {
		t.Errorf("got %q started %d, want %q", path, started, exe)
	}
}
