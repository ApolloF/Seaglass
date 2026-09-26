package library

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCleanCollections(t *testing.T) {
	got := CleanCollections([]string{"  Co-op ", "co-op", "", "Finished", strings.Repeat("x", 60), "  Couch   games "})
	want := []string{"Co-op", "Finished", strings.Repeat("x", 40), "Couch games"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenameCollection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.ApplyScan([]Found{
		{Key: `c:\a`, Title: "A", SortTitle: "a", Dir: `C:\a`},
		{Key: `c:\b`, Title: "B", SortTitle: "b", Dir: `C:\b`},
	}, time.Now())
	gs := s.Games()
	s.Update(gs[0].ID, func(g *Game) { g.Collections = []string{"Co-op", "Racing"} })
	s.Update(gs[1].ID, func(g *Game) { g.Collections = []string{"co-op"} })
	if ids := s.RenameCollection("CO-OP", "Couch"); len(ids) != 2 {
		t.Fatalf("renamed in %d games, want 2", len(ids))
	}
	a, _ := s.Get(gs[0].ID)
	if !slices.Equal(a.Collections, []string{"Couch", "Racing"}) {
		t.Errorf("a: %q", a.Collections)
	}
	s.RenameCollection("racing", "")
	a, _ = s.Get(gs[0].ID)
	if !slices.Equal(a.Collections, []string{"Couch"}) {
		t.Errorf("after removing: %q", a.Collections)
	}
	// A rescan keeps them.
	s.ApplyScan([]Found{{Key: `c:\a`, Title: "A", SortTitle: "a", Dir: `C:\a`}}, time.Now())
	if a, _ = s.Get(gs[0].ID); !slices.Equal(a.Collections, []string{"Couch"}) {
		t.Errorf("after a scan: %q", a.Collections)
	}
}
