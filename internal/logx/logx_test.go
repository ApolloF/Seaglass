package logx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ApolloF/Seaglass/internal/platform"
)

func TestTestsDontWriteTheRealLog(t *testing.T) {
	if real := filepath.Join(platform.AppDir(), "seaglass.log"); strings.EqualFold(Path(), real) {
		t.Fatalf("tests log to the real log %s", real)
	}
}
