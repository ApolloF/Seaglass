package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mk creates files (with optional content) below root.
func mk(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func big(n int) string { return strings.Repeat("x", n) }

func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Baldurs.Gate.3-RUNE":                 "Baldurs Gate 3",
		"Hades [FitGirl Repack]":              "Hades",
		"Cyberpunk 2077 v2.12":                "Cyberpunk 2077",
		"Elden_Ring_Build_12345":              "Elden Ring",
		"The Witcher 3 - GOG":                 "The Witcher 3",
		"Hollow Knight (v1.5.78.11833)":       "Hollow Knight",
		"Stardew Valley":                      "Stardew Valley",
		"DOOM Eternal-EMPRESS":                "DOOM Eternal",
		"Sekiro Shadows Die Twice-CODEX":      "Sekiro Shadows Die Twice",
		"It Takes Two [DODI Repack]":          "It Takes Two",
		"Portal 2":                            "Portal 2",
		"Elden.Ring.v1.10-FitGirl":            "Elden Ring",
		"Baldurs.Gate.3.v4.1.1-GOG":           "Baldurs Gate 3",
		"Grand Theft Auto V version 1.0.3179": "Grand Theft Auto V",
	} {
		if got := CleanTitle(in); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooseKey(t *testing.T) {
	same := [][2]string{
		{"Assassin Creed Black Flag Resynced", "Assassin's Creed: Black Flag Resynced"},
		{"Baldurs Gate III", "Baldur's Gate 3"},
		{"AssassinsCreed4", "Assassin's Creed 4"},
		{"Sims 4", "The Sims 4"},
		{"Grand Theft Auto 5", "Grand Theft Auto V"},
		{"Ori & the Blind Forest", "Ori and the Blind Forest"},
		{"Tom Clancy’s Rainbow Six Siege", "Tom Clancys Rainbow Six Siege"},
		{"Spider-Man Miles Morales", "Marvel's Spider-Man: Miles Morales"},
		{"The Division 2", "Tom Clancy's The Division 2"},
	}
	for _, p := range same {
		if a, b := LooseKey(p[0]), LooseKey(p[1]); a != b {
			t.Errorf("LooseKey(%q) = %q, LooseKey(%q) = %q, want equal", p[0], a, p[1], b)
		}
	}
	differ := [][2]string{
		{"Portal", "Portal 2"},
		{"Hades", "Hades II"},
		{"Far Cry 5", "Far Cry 6"},
		{"Metro 2033", "Metro Exodus"},
	}
	for _, p := range differ {
		if a, b := LooseKey(p[0]), LooseKey(p[1]); a == b {
			t.Errorf("LooseKey(%q) = LooseKey(%q) = %q, want different", p[0], p[1], a)
		}
	}
}

func TestLooseKeyBrandOnlyWhenPossessive(t *testing.T) {
	if LooseKey("Marvel Rivals") == LooseKey("Rivals") {
		t.Error("Marvel Rivals lost its name")
	}
}

func TestStripEditionAndAbbrev(t *testing.T) {
	for in, want := range map[string]string{
		"The Witcher 3: Wild Hunt - Complete Edition": "The Witcher 3: Wild Hunt",
		"Hogwarts Legacy Deluxe Edition":              "Hogwarts Legacy",
		"Portal 2":                                    "Portal 2",
	} {
		if got := StripEdition(in); got != want {
			t.Errorf("StripEdition(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"GTA V":    "Grand Theft Auto V",
		"GTAIV":    "Grand Theft Auto IV",
		"RDR2":     "Red Dead Redemption 2",
		"NFS Heat": "Need for Speed Heat",
		"Codename": "Codename",
		"Codex":    "Codex",
		"Gtafun":   "Gtafun",
		"Hades":    "Hades",
	} {
		if got := ExpandAbbrev(in); got != want {
			t.Errorf("ExpandAbbrev(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortTitle(t *testing.T) {
	if SortTitle("The Sims 4") != "sims 4" || SortTitle("A Hat in Time") != "hat in time" || SortTitle("The") != "the" {
		t.Error("articles not dropped")
	}
}

func TestPublisherID(t *testing.T) {
	const ms = "CN=Microsoft Corporation, O=Microsoft Corporation, L=Redmond, S=Washington, C=US"
	if got := publisherID(ms); got != "8wekyb3d8bbwe" {
		t.Errorf("publisherID = %q, want 8wekyb3d8bbwe", got)
	}
}

func TestDetectRUNE(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"bin/bg3.exe":            big(100),
		"bin/steam_api64.dll":    "x",
		"bin/steam_api64.rne":    "x",
		"bin/steam_appid.txt":    "1086940\n",
		"bin/steam_emu.ini":      "[Settings]\r\nAppId=1086940\r\nUserName=RUNE\r\n",
		"Launcher/steam_emu.ini": "AppId=1086940\n",
	})
	e := DetectEmulation(root, func(string) bool { return true })
	if e.Emulator != "RUNE" || e.AppID != 1086940 || e.AppIDFrom != "steam_emu.ini" {
		t.Errorf("got %+v", e)
	}
}

func TestDetectGoldberg(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"Game.exe":                       big(10),
		"steam_api64.dll":                "x",
		"steam_settings/steam_appid.txt": "\xef\xbb\xbf620",
	})
	e := DetectEmulation(root, func(string) bool { return true })
	if e.Emulator != "Goldberg" || e.AppID != 620 {
		t.Errorf("got %+v", e)
	}
}

func TestDetectUnsignedDLL(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{"Game.exe": "x", "steam_api64.dll": "x"})
	if e := DetectEmulation(root, func(string) bool { return false }); e.Emulator != "Steam emulator" {
		t.Errorf("unsigned DLL: got %+v", e)
	}
	if e := DetectEmulation(root, func(string) bool { return true }); e.Emulator != "" {
		t.Errorf("signed DLL: got %+v", e)
	}
}

func TestUnlockerIsNotUnofficial(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{"Game.exe": "x", "steam_api64.dll": "x", "cream_api.ini": "[config]"})
	if e := DetectEmulation(root, func(string) bool { return false }); e.Emulator != "" {
		t.Errorf("DLC unlocker marked unofficial: %+v", e)
	}
}

func TestDetectGogInfo(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"goggame-1207658924.info": `{"gameId":"1207658924","name":"Quiet Harbor","playTasks":[{"isPrimary":true,"type":"FileTask","path":"bin\\harbor.exe","arguments":"-gog"}]}`,
		"bin/harbor.exe":          big(10),
	})
	e := DetectEmulation(root, nil)
	if e.Gog == nil || e.Gog.GameID != "1207658924" {
		t.Fatalf("got %+v", e)
	}
	exe, args, _ := e.Gog.primaryTask(root)
	if filepath.Base(exe) != "harbor.exe" || args != "-gog" {
		t.Errorf("primary task = %q %q", exe, args)
	}
}

func TestPickExeRepack(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Baldurs Gate 3")
	mk(t, root, map[string]string{
		"bin/bg3.exe":                big(4 << 20),
		"bin/bg3_dx11.exe":           big(4 << 20),
		"Launcher/LariLauncher.exe":  big(1 << 19),
		"Launcher/CrashReporter.exe": big(1 << 19),
		"Uninstall/unins000.exe":     big(1 << 20),
		"DotNetCore/windowsdesktop-runtime-6.0.11-win-x64.exe": big(8 << 20),
	})
	if got := filepath.Base(PickExe(root, "Baldur's Gate 3")); got != "bg3.exe" {
		t.Errorf("picked %s", got)
	}
}

func TestPickExeUnreal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Ember Crown")
	mk(t, root, map[string]string{
		"EmberCrown.exe": big(300 << 10),
		"EmberCrown/Binaries/Win64/EmberCrown-Win64-Shipping.exe": big(8 << 20),
		"Engine/Binaries/Win64/CrashReportClient.exe":             big(2 << 20),
	})
	if got := filepath.Base(PickExe(root, "Ember Crown")); got != "EmberCrown.exe" {
		t.Errorf("picked %s", got)
	}
}

func TestPickExeUnity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Cult of the Lamb")
	mk(t, root, map[string]string{
		"Cult Of The Lamb.exe":    big(600 << 10),
		"Cult Of The Lamb_Data/":  "",
		"UnityCrashHandler64.exe": big(1 << 20),
		"MonoBleedingEdge/x.exe":  big(2 << 20),
	})
	if got := filepath.Base(PickExe(root, "Cult of the Lamb")); got != "Cult Of The Lamb.exe" {
		t.Errorf("picked %s", got)
	}
}

func TestLooksLikeGame(t *testing.T) {
	game := t.TempDir()
	mk(t, game, map[string]string{"Game.exe": "x", "Game_Data/globalgamemanagers": "x"})
	if ok, sign := looksLikeGame(game, true); !ok || sign != "Unity" {
		t.Errorf("unity game: %v %q", ok, sign)
	}
	app := t.TempDir()
	mk(t, app, map[string]string{"app.exe": "x", "resources/app.asar": "x", "d3dcompiler_47.dll": "x"})
	if ok, _ := looksLikeGame(app, true); ok {
		t.Error("an Electron app looked like a game")
	}
}

func TestMerge(t *testing.T) {
	dir := `C:\Games\Hades`
	got := merge([]Candidate{
		{Title: "Hades", Dir: dir, Source: Folder, Exe: `C:\Games\Hades\x64\Hades.exe`},
		{Title: "Hades", Dir: dir + `\`, Source: Installer, TitleTrusted: true, Repacker: "FitGirl", SizeBytes: 100},
		{Title: "Other", Dir: `C:\Games\Other`, Source: Folder},
	})
	if len(got) != 2 {
		t.Fatalf("got %d games: %+v", len(got), got)
	}
	h := got[0]
	if h.Source != Installer || h.Repacker != "FitGirl" || h.Exe == "" || !h.TitleTrusted {
		t.Errorf("merged = %+v", h)
	}
}

func TestInstallerCandidates(t *testing.T) {
	root := t.TempDir()
	repack := filepath.Join(root, "DODI-Repacks", "The Sims 4")
	mk(t, repack, map[string]string{"Game/Bin/TS4_x64.exe": big(1 << 20), "Uninstall/unins000.exe": "x"})
	tool := filepath.Join(root, "Some Tool")
	mk(t, tool, map[string]string{"tool.exe": "x"})
	got := installerCandidates([]uninstallEntry{
		{Key: "The Sims 4_is1", Name: "The Sims 4", Publisher: "DODI-Repacks", Dir: repack + `\`, Icon: filepath.Join(repack, `Uninstall\unins000.exe`)},
		{Key: "tool", Name: "Some Tool", Publisher: "Tools Inc", Dir: tool},
		{Key: "Steam App 413150", Name: "Stardew Valley", Dir: repack},
		{Key: "steam", Name: "Steam", Publisher: "Valve Corporation", Dir: tool},
	})
	if len(got) != 1 || got[0].Repacker != "DODI" || got[0].Exe != "" || got[0].Title != "The Sims 4" {
		t.Errorf("got %+v", got)
	}
}

// TestRealScan scans this PC. Run with WL_REAL_SCAN=1 go test -run RealScan -v ./internal/scan
func TestRealScan(t *testing.T) {
	if os.Getenv("WL_REAL_SCAN") == "" {
		t.Skip("set WL_REAL_SCAN=1 to scan this PC")
	}
	r := Run(context.Background(), Options{AutoFolders: true, DetectUnofficial: true})
	t.Logf("%d games in %v", len(r.Games), r.Took)
	for _, g := range r.Games {
		t.Logf("%-28s %-9s app=%-8d emu=%-14s repack=%-6s exe=%s uri=%s\n    how=%s dir=%s", g.Title, g.Source, g.SteamAppID, g.Emulator, g.Repacker, g.Exe, g.LaunchURI, g.How, g.Dir)
	}
}

func TestCleanTitleKeepsShortNumbers(t *testing.T) {
	for in, want := range map[string]string{
		"DiRT Rally 2.0":                     "DiRT Rally 2.0",
		"Octopath.Traveler.II.v1.9.7-DOGE":   "Octopath Traveler II",
		"Dying.Light.2.Stay.Human.v1.17-P2P": "Dying Light 2 Stay Human",
		"Some Game 1.0.3":                    "Some Game",
	} {
		if got := CleanTitle(in); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAliasesAndNotAGame(t *testing.T) {
	for in, want := range map[string]string{
		"SkyrimSE":       "The Elder Scrolls V: Skyrim Special Edition",
		"AC Valhalla":    "Assassin's Creed Valhalla",
		"ACOdyssey":      "Assassin's Creed Odyssey",
		"MHWilds":        "Monster Hunter Wilds",
		"MK1":            "Mortal Kombat 1",
		"Mafia 2 DE":     "Mafia 2 Definitive Edition",
		"C&C Remastered": "Command & Conquer Remastered",
	} {
		if got := Aliases(in); len(got) == 0 || got[0] != want {
			t.Errorf("Aliases(%q) = %q, want %q first", in, got, want)
		}
	}
	if got := Aliases("Hades"); len(got) != 0 {
		t.Errorf("Aliases(Hades) = %q", got)
	}
	for _, n := range []string{"Tools", "_CommonRedist", "Mod Organizer 2", "Steam", "My Games"} {
		if !NotAGame(n) {
			t.Errorf("%q should not be a game", n)
		}
	}
	for _, n := range []string{"Hades", "Portal 2", "Steamworld Dig"} {
		if NotAGame(n) {
			t.Errorf("%q is a game", n)
		}
	}
}

func TestEmuDir(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		emu   string
		dir   string
	}{
		{"goldberg in bin", map[string]string{"Game.exe": "x", "bin/steam_api64.dll": "x", "bin/steam_settings/steam_appid.txt": "620"}, "Goldberg", "bin"},
		{"gbe_fork configs", map[string]string{"Game.exe": "x", "steam_settings/configs.user.ini": "[user::saves]\n"}, "Goldberg", "."},
		{"rune ini", map[string]string{"Game.exe": "x", "Binaries/Win64/rune.ini": "AppId=10\n"}, "RUNE", `Binaries\Win64`},
		{"tenoke steamdata", map[string]string{"Game.exe": "x", "Game/SteamData/user_stats.ini": "[ACHIEVEMENTS]\n"}, "TENOKE", "Game"},
		{"razor1911", map[string]string{"Game.exe": "x", ".1911": "x"}, "Razor1911", "."},
		{"stray user_stats", map[string]string{"Game.exe": "x", "cfg/user_stats.ini": "x"}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mk(t, root, tc.files)
			e := DetectEmulation(root, func(string) bool { return true })
			if e.Emulator != tc.emu || e.EmuDir != tc.dir {
				t.Errorf("got emulator %q in %q, want %q in %q", e.Emulator, e.EmuDir, tc.emu, tc.dir)
			}
		})
	}
}
