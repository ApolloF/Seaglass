// Package logx writes a small, self-rotating log to %APPDATA%\Seaglass\seaglass.log.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

const maxSize = 1 << 20 // rotate at 1 MiB

var mu sync.Mutex

// Path of the current log file. Tests log to a file in Temp: their made-up
// people and games don't belong in the real log.
func Path() string {
	if testing.Testing() && !platform.AppDirOverridden() {
		return filepath.Join(os.TempDir(), "seaglass-test.log")
	}
	return filepath.Join(platform.AppDir(), "seaglass.log")
}

// Printf appends a timestamped line to the log.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	p := Path()
	if fi, err := os.Stat(p); err == nil && fi.Size() > maxSize {
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// Tail returns the last n lines of the log.
func Tail(n int) []string {
	mu.Lock()
	defer mu.Unlock()
	b, err := os.ReadFile(Path())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
