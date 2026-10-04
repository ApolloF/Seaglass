package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestApplyScanKeepsUserChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	found := []Found{
		{Key: `c:\games\hades`, Title: "Hades", SortTitle: "hades", Source: "folder", Dir: `C:\Games\Hades`, Exe: `C:\Games\Hades\Hades.exe`, Confidence: 85},
		{Key: `c:\games\bg3`, Title: "Baldur's Gate 3", SortTitle: "baldur's gate 3", Source: "installer", SteamAppID: 1086940, Dir: `C:\Games\BG3`},
	}
	if added, _ := s.ApplyScan(found, now); added != 2 {
		t.Fatalf("added %d", added)
	}
	games := s.Games()
	if games[0].Title != "Baldur's Gate 3" || games[1].Title != "Hades" {
		t.Fatalf("order: %s, %s", games[0].Title, games[1].Title)
	}
	hades := games[1]
	if _, err := s.Update(hades.ID, func(g *Game) {
		g.Favorite, g.CustomTitle, g.PadMode, g.Playtime = true, "Hades (modded)", "steam", 3600
		g.UserExe, g.Exe = true, `C:\Games\Hades\x64\Hades.exe`
	}); err != nil {
		t.Fatal(err)
	}
	// Rescan: Hades is found again, BG3 is gone.
	found[0].Exe = `C:\Games\Hades\Other.exe`
	if _, removed := s.ApplyScan(found[:1], now.Add(time.Hour)); removed != 1 {
		t.Errorf("removed %d", removed)
	}
	g, _ := s.Get(hades.ID)
	if !g.Favorite || g.CustomTitle != "Hades (modded)" || g.PadMode != "steam" || g.Playtime != 3600 || g.Exe != `C:\Games\Hades\x64\Hades.exe` {
		t.Errorf("user choices lost: %+v", g)
	}
	if g.AddedAt != now.Unix() {
		t.Errorf("addedAt changed")
	}
	bg3, _ := s.Get(games[0].ID)
	if bg3.Installed {
		t.Error("removed game still installed")
	}

	// Round-trip through disk.
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Games()) != 2 {
		t.Fatalf("reloaded %d games", len(s2.Games()))
	}
	if g2, _ := s2.Get(hades.ID); !g2.Favorite || g2.Playtime != 3600 {
		t.Errorf("reloaded = %+v", g2)
	}
	// New ids never reuse old ones.
	s2.ApplyScan([]Found{{Key: `c:\games\new`, Title: "New"}}, now)
	for _, g := range s2.Games() {
		if g.Title == "New" && g.ID <= bg3.ID {
			t.Errorf("id reused: %d", g.ID)
		}
	}
}

func TestBrokenFileStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := writeAtomic(path, []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil || len(s.Games()) != 0 {
		t.Fatalf("got %v, %d games", err, len(s.Games()))
	}
}

func TestBrokenFileFallsBackToBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, _ := Open(path)
	s.ApplyScan([]Found{{Key: `c:\games\hades`, Title: "Hades", SortTitle: "hades", Source: "folder"}}, time.Now())
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if err := writeAtomic(path+".bak", b); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("{cut sh")); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil || len(s.Games()) != 1 {
		t.Fatalf("got %v, %d games; want the backup's one game", err, len(s.Games()))
	}
}

func TestConcurrentFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, _ := Open(path)
	for i := 0; i < 200; i++ {
		s.ApplyScan([]Found{{Key: fmt.Sprintf(`c:\games\%d`, i), Title: "Game", SortTitle: "game", Source: "folder"}}, time.Now())
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Flush()
		}()
	}
	wg.Wait()
	s2, err := Open(path)
	if err != nil || len(s2.Games()) != 200 {
		t.Fatalf("after concurrent saves: %v, %d games", err, len(s2.Games()))
	}
}

func TestConfirmedMatchSurvivesRescan(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	found := []Found{{Key: `d:\games\sable`, Title: "Sable Run", SortTitle: "sable run", Source: "folder", Confidence: 40, NeedsReview: true}}
	s.ApplyScan(found, now)
	id := s.Games()[0].ID
	if _, err := s.Update(id, func(g *Game) {
		g.Confirmed, g.NeedsReview, g.SteamAppID, g.Title, g.SortTitle = true, false, 757310, "Sable", "sable"
		g.Meta = &Meta{Cover: "/art/x.jpg"}
	}); err != nil {
		t.Fatal(err)
	}
	s.ApplyScan(found, now.Add(time.Hour))
	g, _ := s.Get(id)
	if g.Title != "Sable" || g.SteamAppID != 757310 || g.NeedsReview || g.Meta == nil {
		t.Errorf("confirmed match lost: %+v", g)
	}
}

func TestStoreMatchSurvivesRescan(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	found := []Found{{Key: `d:\games\ac`, Title: "Assassin Creed Black Flag Resynced", SortTitle: "assassin creed black flag resynced", Source: "installer", Confidence: 40, NeedsReview: true, MatchHow: "Not matched to a known game"}}
	s.ApplyScan(found, now)
	id := s.Games()[0].ID
	// What the metadata worker does after a Steam store search found it.
	if _, err := s.Update(id, func(g *Game) {
		g.MetaAppID, g.Title, g.SortTitle = 3751950, "Assassin's Creed Black Flag Resynced", "assassin's creed black flag resynced"
		g.MatchHow, g.Confidence, g.NeedsReview = "Matched on the Steam store", 80, false
	}); err != nil {
		t.Fatal(err)
	}
	s.ApplyScan(found, now.Add(time.Hour))
	g, _ := s.Get(id)
	if g.Title != "Assassin's Creed Black Flag Resynced" || g.NeedsReview || g.Confidence != 80 {
		t.Errorf("store match lost on rescan: %+v", g)
	}
	// A scan that knows the game (a newer game database) takes over.
	found[0].Title, found[0].SteamAppID, found[0].Confidence, found[0].NeedsReview, found[0].MatchHow = "Assassin's Creed: Black Flag Resynced", 3751950, 72, false, "Matched by a similar title"
	s.ApplyScan(found, now.Add(2*time.Hour))
	g, _ = s.Get(id)
	if g.Title != "Assassin's Creed: Black Flag Resynced" || g.SteamAppID != 3751950 || g.MatchHow != "Matched by a similar title" {
		t.Errorf("scan's identity not taken: %+v", g)
	}
}

func TestOwnedGames(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "library.json"))
	now := time.Now()
	// A Steam game installed here, and two owned elsewhere.
	s.ApplyScan([]Found{{Key: `c:\steam\common\portal`, Title: "Portal", SortTitle: "portal", Source: "steam", SteamAppID: 400, Dir: `C:\Steam\common\Portal`}}, now)
	added, _ := s.ApplyOwned("steam", []Owned{
		{Store: "steam", ID: "400", Title: "Portal", InstallURI: "steam://install/400", Playtime: 3600},
		{Store: "steam", ID: "620", Title: "Portal 2", InstallURI: "steam://install/620", Playtime: 7200},
		{Store: "steam", ID: "", Title: "No id"},
	}, now)
	if added != 1 {
		t.Fatalf("added = %d, want 1 (Portal is already here)", added)
	}
	var portal, portal2 Game
	for _, g := range s.Games() {
		switch g.Title {
		case "Portal":
			portal = g
		case "Portal 2":
			portal2 = g
		}
	}
	if !portal.Owned || !portal.Installed || portal.InstallURI == "" || portal.StorePlaytime != 3600 {
		t.Errorf("installed + owned = %+v", portal)
	}
	if portal2.Installed || !portal2.Owned || !portal2.IsOwnedOnly() || portal2.SteamAppID != 620 || !portal2.Initial {
		t.Errorf("owned only = %+v", portal2)
	}

	// The user favourites Portal 2, then installs it: one game remains, favourite kept.
	s.Update(portal2.ID, func(g *Game) { g.Favorite = true })
	s.ApplyScan([]Found{
		{Key: `c:\steam\common\portal`, Title: "Portal", SortTitle: "portal", Source: "steam", SteamAppID: 400},
		{Key: `c:\steam\common\portal 2`, Title: "Portal 2", SortTitle: "portal 2", Source: "steam", SteamAppID: 620},
	}, now)
	var both []Game
	for _, g := range s.Games() {
		if g.SteamAppID == 620 {
			both = append(both, g)
		}
	}
	if len(both) != 1 || !both[0].Installed || !both[0].Favorite || !both[0].Owned || both[0].IsOwnedOnly() {
		t.Errorf("after install = %+v", both)
	}

	// The account stops listing a game: owned-only games go, found ones stay.
	s.ApplyOwned("steam", []Owned{{Store: "steam", ID: "730", Title: "CS2"}}, now)
	_, removed := s.ApplyOwned("steam", nil, now)
	if removed != 1 || len(s.Games()) != 2 {
		t.Errorf("removed %d, games %d", removed, len(s.Games()))
	}
	s.ForgetOwned("steam")
	for _, g := range s.Games() {
		if g.Owned {
			t.Errorf("%s still owned after disconnecting", g.Title)
		}
	}
}

func TestOwnedMarkAcrossSources(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "library.json"))
	now := time.Now()
	found := []Found{
		{Key: `c:\steam\common\portal`, Title: "Portal", Source: "steam", SteamAppID: 400},
		{Key: `d:\repacks\portal`, Title: "Portal", Source: "installer", SteamAppID: 400},
		{Key: `d:\games\hl2`, Title: "Half-Life 2", Source: "folder", SteamAppID: 220},
		{Key: `d:\gog\witcher`, Title: "The Witcher", Source: "gog", GogID: "1207658924"},
	}
	s.ApplyScan(found, now)
	byKey := func() map[string]Game {
		m := map[string]Game{}
		for _, g := range s.Games() {
			m[g.Key] = g
		}
		return m
	}
	s.ApplyOwned("steam", []Owned{
		{Store: "steam", ID: "400", Title: "Portal", InstallURI: "steam://install/400"},
		{Store: "steam", ID: "220", Title: "Half-Life 2", InstallURI: "steam://install/220"},
	}, now)
	s.ApplyOwned("gog", []Owned{{Store: "gog", ID: "1207658924", Title: "The Witcher", InstallURI: "goggalaxy://openGameView/1207658924", Playtime: 3600}}, now)
	g := byKey()
	for _, k := range []string{`c:\steam\common\portal`, `d:\repacks\portal`, `d:\games\hl2`} {
		if !g[k].Owned || g[k].InstallURI == "" {
			t.Errorf("%s not marked owned: %+v", k, g[k])
		}
	}

	// A rescan keeps Galaxy's playtime.
	s.ApplyScan(found, now)
	if w := byKey()[`d:\gog\witcher`]; w.StorePlaytime != 3600 || !w.Owned {
		t.Errorf("GOG playtime lost on rescan: %+v", w)
	}

	// The account stops listing Half-Life 2: its mark goes, Portal keeps it.
	s.ApplyOwned("steam", []Owned{{Store: "steam", ID: "400", Title: "Portal", InstallURI: "steam://install/400"}}, now)
	g = byKey()
	if hl := g[`d:\games\hl2`]; hl.Owned || hl.InstallURI != "" {
		t.Errorf("unlisted game still owned: %+v", hl)
	}
	if !g[`d:\repacks\portal`].Owned || !g[`c:\steam\common\portal`].Owned {
		t.Error("listed copies lost the mark")
	}

	// Disconnecting Steam clears the repack too, but not GOG's mark.
	s.ForgetOwned("steam")
	g = byKey()
	if r := g[`d:\repacks\portal`]; r.Owned || r.InstallURI != "" {
		t.Errorf("repack still owned after disconnecting Steam: %+v", r)
	}
	if !g[`d:\gog\witcher`].Owned {
		t.Error("disconnecting Steam cleared GOG's mark")
	}
}

func TestApplyScanEmuDir(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := Found{Key: `c:\games\x`, Title: "X", Source: "folder", Dir: `C:\Games\X`, Emulator: "Goldberg", EmuDir: "bin", External: true}
	s.ApplyScan([]Found{f}, time.Now())
	if g := s.Games()[0]; g.EmuDir != "bin" {
		t.Fatalf("EmuDir = %q", g.EmuDir)
	}
	f.Emulator, f.EmuDir, f.External = "", "", false
	s.ApplyScan([]Found{f}, time.Now())
	if g := s.Games()[0]; g.EmuDir != "" {
		t.Fatalf("EmuDir kept after the emulator went: %q", g.EmuDir)
	}
}

// Libraries from before v1.9 called external copies unofficial.
func TestLegacyUnofficial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	old := `{"version":1,"nextId":3,"games":[
		{"id":1,"key":"c:\\a","title":"A","source":"folder","sourceLabel":"Unofficial · RUNE","unofficial":true,"emulator":"RUNE"},
		{"id":2,"key":"c:\\b","title":"B","source":"steam","sourceLabel":"Steam"}]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Get(1)
	b, _ := s.Get(2)
	if !a.External || a.SourceLabel != "External copy" {
		t.Errorf("a: external %v, label %q", a.External, a.SourceLabel)
	}
	if b.External || b.SourceLabel != "Steam" {
		t.Errorf("b: external %v, label %q", b.External, b.SourceLabel)
	}
}
