package app

import (
	"context"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/emulated"
	"github.com/ApolloF/Seaglass/internal/settings"
)

// A console search held up (a network drive that hangs) doesn't hold up
// the PC games, and isn't started again by the next scan while it runs.
func TestConsoleSearchNeverHoldsUpTheScan(t *testing.T) {
	release := make(chan struct{})
	s := consoleSearch{find: func(ctx context.Context) ([]emulated.Game, error) {
		<-release
		return []emulated.Game{{Title: "Flower"}}, nil
	}}
	ch := s.start(context.Background())
	start := time.Now()
	if _, ready := consoleResultNow(ch); ready {
		t.Fatal("a search still going has no result")
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("waited %v for the console search", d)
	}
	if again := s.start(context.Background()); again != nil {
		t.Fatal("a second search started while the first one runs")
	}
	close(release)
	select {
	case r := <-ch:
		if len(r.games) != 1 || r.err != nil {
			t.Fatalf("result %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
	for s.running.Load() {
		time.Sleep(time.Millisecond)
	}
	if s.start(context.Background()) == nil {
		t.Fatal("a search can start again once the last one is done")
	}
}

// The search gets a deadline of its own, whoever started it.
func TestConsoleSearchHasADeadline(t *testing.T) {
	got := make(chan bool, 1)
	s := consoleSearch{find: func(ctx context.Context) ([]emulated.Game, error) {
		_, ok := ctx.Deadline()
		got <- ok
		return nil, nil
	}}
	<-s.start(context.Background())
	if !<-got {
		t.Fatal("no deadline")
	}
}

// Console games a scan didn't get a full answer for in time stay as they
// are; everything else is looked for as before.
func TestLookedFor(t *testing.T) {
	pc, emu := `c:\games\hades`, `emu:r:\ps3\a\eboot.bin`
	if lookedFor(false, false, nil) != nil || lookedFor(true, true, nil) != nil {
		t.Fatal("a scan without console games, or with all of them, looks for everything")
	}
	for _, f := range []func(string) bool{lookedFor(true, false, nil), lookedFor(true, true, emulated.ErrIncomplete)} {
		if !f(pc) || f(emu) {
			t.Fatal("console games left alone, PC games looked for")
		}
	}
	// A walk that didn't get through R:\Games leaves only the games there
	// alone: one elsewhere that's gone is marked so.
	f := lookedFor(true, true, &emulated.WalkError{Dirs: []string{`R:\Games`}})
	if !f(pc) || f(`emu:r:\games\wii\mkw.wbfs`) || !f(`emu:d:\ps3\a\eboot.bin`) {
		t.Fatal("only the games in the folders not walked are left alone")
	}
}

// Turning console games on or off finds them, or marks them gone, right
// away.
func TestDetectEmulatorsRescans(t *testing.T) {
	old := settings.Defaults()
	v := old
	v.DetectEmulators = !old.DetectEmulators
	if !needsRescan(old, v) || !needsRescan(v, old) {
		t.Fatal("toggling console games doesn't rescan")
	}
	v = old
	v.Theme = "light"
	if needsRescan(old, v) {
		t.Fatal("a theme change rescans")
	}
}
