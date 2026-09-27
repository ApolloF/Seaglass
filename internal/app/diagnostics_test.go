package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// A crashing run leaves crash.log behind; the next start keeps it as
// crash-previous.log and says so.
func TestCrashCapture(t *testing.T) {
	if os.Getenv("WL_CRASH_DIR") != "" {
		captureCrashes(os.Getenv("WL_CRASH_DIR"))
		go func() { // outside the test's own recover
			var m map[string]int
			m["boom"] = 1 // nil map: an unhandled panic
		}()
		// Wait for the crash to end the process. Nothing may signal back
		// from the panicking goroutine: its deferred calls run while the
		// panic unwinds, so the test could return (and the process exit 0)
		// before the runtime ends it.
		time.Sleep(time.Minute)
		t.Fatal("the panic didn't end the process")
	}
	dir := t.TempDir()
	if captureCrashes(dir) {
		t.Fatal("a fresh folder reported a crash")
	}
	// Let go of crash.log, as a crashed process would have.
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	cmd := exec.Command(os.Args[0], "-test.run=TestCrashCapture")
	cmd.Env = append(os.Environ(), "WL_CRASH_DIR="+dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("the crashing child didn't crash")
	}
	crashed := captureCrashes(dir)
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	if !crashed {
		t.Fatal("the crash wasn't noticed")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "crash-previous.log"))
	if !strings.Contains(string(b), "assignment to entry in nil map") {
		t.Errorf("crash output: %q", b)
	}
	crashed = captureCrashes(dir)
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	if crashed {
		t.Error("a clean run was reported as a crash")
	}
}

func TestRedact(t *testing.T) {
	if platform.Profile == "" {
		t.Skip("no profile folder")
	}
	in := "log: " + platform.Profile + `\AppData` + "\njson: " + strings.ReplaceAll(strings.ToUpper(platform.Profile), `\`, `\`) + `\Games`
	out := redact(in)
	if strings.Contains(strings.ToLower(out), strings.ToLower(filepath.Base(platform.Profile))) {
		t.Errorf("user name left in %q", out)
	}
	if !strings.Contains(out, `%USERPROFILE%\AppData`) || !strings.Contains(out, `%USERPROFILE%\Games`) {
		t.Errorf("redacted = %q", out)
	}
}
