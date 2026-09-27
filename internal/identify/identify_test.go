package identify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/scan"
)

const sample = `---
"Baldur's Gate 3":
  cloud:
    steam: true
  files:
    "<winLocalAppData>/Larian Studios/Baldur's Gate 3/PlayerProfiles":
      tags:
        - save
  installDir:
    Baldurs Gate 3: {}
  steam:
    id: 1086940
Hades:
  gog:
    id: 1330167364
  installDir:
    Hades: {}
  steam:
    id: 1145360
The Sims 4:
  installDir:
    The Sims 4: {}
  steam:
    id: 1222670
"-KLAUS-":
  alias: Klaus
Klaus:
  installDir:
    KLAUS: {}
  steam:
    id: 431330
"Ori and the Blind Forest: Definitive Edition":
  steam:
    id: 387290
Stardew Valley:
  installDir:
    Stardew Valley: {}
  steam:
    id: 413150
"Assassin's Creed: Black Flag Resynced":
  installDir:
    Assassin's Creed Black Flag Resynced: {}
  steam:
    id: 3751950
Worm:
  steam:
    id: 1000001
Worms:
  steam:
    id: 1000002
`

func index(t *testing.T) *Index {
	t.Helper()
	es, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	return build(es)
}

func TestParse(t *testing.T) {
	es, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 9 {
		t.Fatalf("got %d entries", len(es))
	}
	if es[0].Name != "Baldur's Gate 3" || es[0].SteamID != 1086940 || es[0].InstallDirs[0] != "Baldurs Gate 3" {
		t.Errorf("bg3 = %+v", es[0])
	}
	if es[1].GogID != "1330167364" {
		t.Errorf("hades = %+v", es[1])
	}
	for _, e := range es {
		if e.Name == "Klaus" && (len(e.Aliases) != 1 || e.Aliases[0] != "-KLAUS-") {
			t.Errorf("klaus aliases = %v", e.Aliases)
		}
	}
}

func TestIdentify(t *testing.T) {
	ix := index(t)
	for _, tc := range []struct {
		name  string
		c     scan.Candidate
		title string
		steam int
		conf  int
	}{
		{"emulator appid wins", scan.Candidate{Title: "Baldurs Gate 3", Source: scan.Installer, SteamAppID: 1086940, AppIDFrom: "steam_emu.ini"}, "Baldur's Gate 3", 1086940, 95},
		{"repack by title", scan.Candidate{Title: "The Sims 4", Source: scan.Installer, TitleTrusted: true}, "The Sims 4", 1222670, 85},
		{"alias", scan.Candidate{Title: "-KLAUS-", Source: scan.Folder}, "Klaus", 431330, 85},
		{"ampersand", scan.Candidate{Title: "Ori & the Blind Forest: Definitive Edition", Source: scan.Folder}, "Ori and the Blind Forest: Definitive Edition", 387290, 85},
		{"edition stripped", scan.Candidate{Title: "Hades Deluxe Edition", Source: scan.Folder}, "Hades", 1145360, 75},
		{"folder name", scan.Candidate{Title: "Kl4us Game", Dir: `D:\Games\KLAUS`, Source: scan.Folder}, "Klaus", 431330, 70},
		{"store title kept", scan.Candidate{Title: "Stardew Valley", Source: scan.Steam, SteamAppID: 413150}, "Stardew Valley", 413150, 100},
		{"unknown folder", scan.Candidate{Title: "Homebrew Thing", Dir: `D:\Games\Homebrew Thing`, Source: scan.Folder}, "Homebrew Thing", 0, 40},
		{"similar title", scan.Candidate{Title: "Assassin Creed Black Flag Resynced", Dir: `D:\Games\Assassin Creed Black Flag Resynced`, Source: scan.Installer}, "Assassin's Creed: Black Flag Resynced", 3751950, 72},
		{"similar folder", scan.Candidate{Title: "AC BFR", Dir: `D:\Games\Assassins Creed Black Flag Resynced [DODI Repack]`, Source: scan.Installer, TitleTrusted: true}, "Assassin's Creed: Black Flag Resynced", 3751950, 72},
		{"roman numerals", scan.Candidate{Title: "Baldurs Gate III", Source: scan.Folder}, "Baldur's Gate 3", 1086940, 72},
		{"similar but ambiguous", scan.Candidate{Title: "The Worm", Source: scan.Folder}, "The Worm", 0, 40},
		{"store title, similar", scan.Candidate{Title: "Assassins Creed Black Flag Resynced", Source: scan.Epic, How: "Epic Games library"}, "Assassins Creed Black Flag Resynced", 3751950, 100},
	} {
		m := ix.Identify(tc.c)
		if m.Title != tc.title || m.SteamAppID != tc.steam || m.Confidence != tc.conf {
			t.Errorf("%s: got %+v", tc.name, m)
		}
	}
}

func TestNilIndex(t *testing.T) {
	var ix *Index
	m := ix.Identify(scan.Candidate{Title: "Baldurs Gate 3", SteamAppID: 1086940, AppIDFrom: "steam_emu.ini"})
	if m.Title != "Baldurs Gate 3" || m.SteamAppID != 1086940 {
		t.Errorf("got %+v", m)
	}
}

// TestRealManifest parses a downloaded manifest: WL_MANIFEST=path go test -run RealManifest ./internal/identify
func TestRealManifest(t *testing.T) {
	p := os.Getenv("WL_MANIFEST")
	if p == "" {
		t.Skip("set WL_MANIFEST to a manifest.yaml")
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	es, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	ix := build(es)
	t.Logf("%d entries, %d names, %d steam ids", len(es), ix.Len(), len(ix.bySteam))
	for _, c := range []scan.Candidate{
		{Title: "Baldurs Gate 3", SteamAppID: 1086940, AppIDFrom: "steam_emu.ini", Source: scan.Installer},
		{Title: "The Sims 4", Source: scan.Installer, TitleTrusted: true},
	} {
		t.Logf("%q → %+v", c.Title, ix.Identify(c))
	}
}

func TestIndexLetGoWhenUnused(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.keep = 20 * time.Millisecond
	if err := writeIndex(m.indexFile(), []Entry{{Name: "Portal", SteamID: 400}}); err != nil {
		t.Fatal(err)
	}
	if ix := m.Index(); ix == nil || ix.Len() != 1 {
		t.Fatalf("index = %v", ix)
	}
	time.Sleep(100 * time.Millisecond)
	m.mu.Lock()
	held := m.idx != nil
	m.mu.Unlock()
	if held {
		t.Error("index still held after it went unused")
	}
	if m.Len() != 1 {
		t.Errorf("Len = %d after letting go, want 1", m.Len())
	}
	if ix := m.Index(); ix == nil || ix.Len() != 1 {
		t.Errorf("index not read again: %v", ix)
	}
}

// Folder names people and repackers give games: shortened, run together,
// without "Edition", without little words, by subtitle; and folders that
// aren't games.
func TestIdentifyFolderNames(t *testing.T) {
	es, err := Parse(strings.NewReader(`---
"The Elder Scrolls V: Skyrim Special Edition":
  steam:
    id: 489830
Fallout 3:
  steam:
    id: 22300
"Like a Dragon: Infinite Wealth":
  steam:
    id: 2072450
Indiana Jones and the Great Circle:
  steam:
    id: 2677660
Mass Effect Legendary Edition:
  steam:
    id: 1328670
Mass Effect:
  steam:
    id: 17460
Command & Conquer:
  steam:
    id: 1
Assassin's Creed Valhalla:
  steam:
    id: 2208920
Vortex:
  steam:
    id: 2
"Batman: Arkham City":
  steam:
    id: 200260
"LEGO Batman: Arkham City":
  steam:
    id: 3
`))
	if err != nil {
		t.Fatal(err)
	}
	ix := build(es)
	for _, tc := range []struct {
		dir   string
		steam int
	}{
		{`D:\Games\SkyrimSE`, 489830},
		{`D:\Games\Fallout3GameoftheYearEdition`, 22300},
		{`D:\Games\Infinite Wealth`, 2072450},
		{`D:\Games\Indiana Jones Great Circle`, 2677660},
		{`D:\Games\Mass Effect Legendary`, 1328670},
		{`D:\Games\AC Valhalla`, 2208920},
		{`D:\Games\ACValhalla`, 2208920},
		{`D:\Games\C&C Remastered`, 0}, // not Command & Conquer (1995)
		{`D:\Games\Arkham City`, 0},    // two games have that subtitle
		{`D:\Games\Vortex`, 0},         // the mod manager, not the game
		{`D:\Games\Steam`, 0},
	} {
		m := ix.Identify(scan.Candidate{Title: scan.CleanTitle(filepath.Base(tc.dir)), Dir: tc.dir, Source: scan.Folder})
		if m.SteamAppID != tc.steam {
			t.Errorf("%s: got %q (%d, %s), want %d", tc.dir, m.Title, m.SteamAppID, m.How, tc.steam)
		}
	}
}
