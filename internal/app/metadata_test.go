package app

import (
	"testing"

	"github.com/ApolloF/WaterLauncher/internal/library"
	"github.com/ApolloF/WaterLauncher/internal/meta"
)

func TestPickStoreHit(t *testing.T) {
	// What the Steam store search returned for the folder name.
	hits := []meta.StoreHit{
		{AppID: 3751950, Name: "Assassin's Creed Black Flag Resynced"},
		{AppID: 4496580, Name: "Assassin's Creed Black Flag Resynced - Sea Serpent Character Pack"},
		{AppID: 4496530, Name: "Assassin's Creed Black Flag Resynced - Hellfire Character Pack"},
	}
	for _, tc := range []struct {
		title string
		hits  []meta.StoreHit
		want  int
	}{
		{"Assassin Creed Black Flag Resynced", hits, 3751950},
		{"Assassin's Creed Black Flag Resynced", hits, 3751950},
		{"Assassin Creed Black Flag", hits, 0},
		{"Portal", []meta.StoreHit{{AppID: 620, Name: "Portal 2"}}, 0},
		{"Hades", []meta.StoreHit{{AppID: 1, Name: "Hades II"}, {AppID: 2, Name: "HADES"}}, 2},
		{"Worm", []meta.StoreHit{{AppID: 1, Name: "Worms"}, {AppID: 2, Name: "The Worms"}}, 0}, // two games: no guess
		// The store's name with its edition, when only one game has it.
		{"Tomb Raider", []meta.StoreHit{{AppID: 203160, Name: "Tomb Raider Game of the Year"}, {AppID: 8000, Name: "Tomb Raider: Anniversary"}, {AppID: 224960, Name: "Tomb Raider I (1996)"}}, 203160},
	} {
		h, ok := pickStoreHit(tc.title, tc.hits)
		if got := map[bool]int{true: h.AppID}[ok]; got != tc.want {
			t.Errorf("pickStoreHit(%q) = %d, want %d", tc.title, got, tc.want)
		}
	}
}

func TestAdoptStoreMatch(t *testing.T) {
	g := library.Game{Title: "Assassin Creed Black Flag Resynced", MatchHow: "Not matched to a known game", Confidence: 40, NeedsReview: true}
	adoptStoreMatch(&g, meta.StoreHit{AppID: 3751950, Name: "Assassin's Creed® Black Flag Resynced"})
	if g.MetaAppID != 3751950 || g.Title != "Assassin's Creed Black Flag Resynced" || g.NeedsReview || g.MatchHow != storeMatchHow || g.Confidence != storeMatchConfidence {
		t.Errorf("not adopted: %+v", g)
	}

	// A title the game database knew (without store ids) stays; so does
	// the user's choice. The store only adds art.
	k := library.Game{Title: "Known Game", MatchHow: "Matched by title", Confidence: 85}
	adoptStoreMatch(&k, meta.StoreHit{AppID: 7, Name: "Known Game™"})
	if k.MetaAppID != 7 || k.Title != "Known Game" || k.MatchHow != "Matched by title" {
		t.Errorf("known game changed: %+v", k)
	}

	c := library.Game{Title: "My Name", Confirmed: true, MatchHow: "Chosen by you", Confidence: 100}
	adoptStoreMatch(&c, meta.StoreHit{AppID: 5, Name: "Other"})
	if c.MetaAppID != 5 || c.Title != "My Name" || c.MatchHow != "Chosen by you" {
		t.Errorf("confirmed game changed: %+v", c)
	}
}
