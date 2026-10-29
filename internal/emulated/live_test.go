package emulated

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRealFind lists the emulators and console games on this PC:
// WL_REAL_EMU=1 go test -run RealFind -v ./internal/emulated
func TestRealFind(t *testing.T) {
	if os.Getenv("WL_REAL_EMU") == "" {
		t.Skip("set WL_REAL_EMU=1 to scan this PC")
	}
	start := time.Now()
	ins := findInstalls(installDirs())
	for _, in := range ins {
		t.Logf("emulator %s: %s", in.emu.name, in.exe)
	}
	games, err := find(context.Background(), ins, time.Now().Add(walkBudget))
	t.Logf("%d games in %v (%v)", len(games), time.Since(start).Round(time.Millisecond), err)
	for _, g := range games {
		t.Logf("%-8s %-36q %-16s %s (%s; title: %s)\n           %s %s\n           art: cover %q backdrop %q icon %q",
			g.Platform, g.Title, g.Serial, g.File, g.How, g.TitleFrom, g.Exe, g.Args, g.Cover, g.Backdrop, g.Icon)
	}
}
