package scan

import (
	"bufio"
	"bytes"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// emuFiles are files Steam emulators and cracks drop next to the game,
// mapped to the name Seaglass shows.
var emuFiles = map[string]string{
	"steam_emu.ini":           "", // CODEX, RUNE and relatives; the group is worked out below
	"coldclientloader.ini":    "Goldberg",
	"steam_interfaces.txt":    "Goldberg",
	"local_save.txt":          "Goldberg",
	"configs.user.ini":        "Goldberg", // gbe_fork
	"smartsteamemu.ini":       "SmartSteamEmu",
	"onlinefix.ini":           "OnlineFix",
	"onlinefix64.dll":         "OnlineFix",
	"codex.ini":               "CODEX",
	"rune.ini":                "RUNE",
	"tenoke.ini":              "TENOKE",
	"empress.ini":             "EMPRESS",
	"3dmgame.ini":             "3DM",
	"ali213.ini":              "ALI213",
	"cpy.ini":                 "CPY",
	"steam_api.cdx":           "CODEX",
	"steam_api64.cdx":         "CODEX",
	"steam_api.rne":           "RUNE",
	"steam_api64.rne":         "RUNE",
	"nemirtingasepicemu.json": "Epic emulator",
	".1911":                   "Razor1911",
}

// unlockers wrap the real steam_api DLL (CreamAPI, SmokeAPI): the game
// still runs through Steam, so they don't make a copy unofficial.
var unlockers = map[string]bool{"cream_api.ini": true, "smokeapi.json": true, "smokeapi.config.json": true, "steam_api_o.dll": true, "steam_api64_o.dll": true}

var reAppID = regexp.MustCompile(`(?im)^\s*(?:appid|realappid|app_id)\s*=\s*(\d{1,10})\s*$`)

// Emulation is what unofficial-copy detection found in a game folder.
type Emulation struct {
	Emulator  string // "RUNE", "Goldberg", …; "" when the copy looks official
	Marker    string // the file that gave it away
	AppID     int    // Steam app id the emulator is set up as
	AppIDFrom string
	Gog       *gogInfo // a GOG game info file, when present
	GogFile   string
	PadHint   string // "libScePad" (Sony's DualSense library) or "SDL" when the game ships one
	// EmuDir is the folder, relative to the game folder, where the emulator
	// marker (or steam_settings) was found; "." for the game folder itself.
	EmuDir string
}

// DetectEmulation looks through a game folder for Steam emulators, cracks
// and GOG game info files. signed reports whether a DLL carries a valid
// signature; nil skips that check.
func DetectEmulation(dir string, signed func(string) bool) Emulation {
	return detectEmulation(dir, signed, nil)
}

// detectEmulation is DetectEmulation; fp (may be nil) records what the
// answer depends on.
func detectEmulation(dir string, signed func(string) bool, fp *fingerprint) Emulation {
	var e Emulation
	root := filepath.Clean(dir)
	depth0 := strings.Count(root, `\`)
	var dlls []string
	unlocker := false
	appIDs := map[string]int{}        // file → app id, first found per kind
	var markerDir, settingsDir string // where the marker and steam_settings were found
	n := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if n++; n > 30000 {
			return filepath.SkipAll
		}
		name := strings.ToLower(d.Name())
		if d.IsDir() {
			if p == root || strings.Count(filepath.Clean(p), `\`)-depth0 == 1 {
				fp.entry(p, d) // the folder and its direct subfolders
			}
			if p == root {
				return nil
			}
			if skipFolders[name] {
				return filepath.SkipDir
			}
			if name == "steam_settings" {
				if settingsDir == "" {
					settingsDir = filepath.Dir(p)
				}
				if e.Emulator == "" {
					e.Emulator, e.Marker = "Goldberg", d.Name()
					markerDir = filepath.Dir(p)
				}
				if id := readAppIDTxt(filepath.Join(p, "steam_appid.txt")); id > 0 {
					appIDs["steam_settings"] = id
				}
			}
			if strings.Count(filepath.Clean(p), `\`)-depth0 >= 7 {
				return filepath.SkipDir
			}
			return nil
		}
		if telling(name) {
			fp.entry(p, d)
			fp.dir(filepath.Dir(p))
		}
		switch {
		case name == "user_stats.ini" && strings.EqualFold(filepath.Base(filepath.Dir(p)), "steamdata"):
			// TENOKE keeps its settings and unlocks in SteamData.
			if e.Emulator == "" || e.Emulator == "Steam emulator" {
				e.Emulator, e.Marker, markerDir = "TENOKE", "SteamData\\"+d.Name(), filepath.Dir(filepath.Dir(p))
			}
		case name == "steam_appid.txt":
			if id := readAppIDTxt(p); id > 0 {
				if _, ok := appIDs["steam_appid.txt"]; !ok {
					appIDs["steam_appid.txt"] = id
				}
			}
		case emuFiles[name] != "" || name == "steam_emu.ini":
			group := emuFiles[name]
			if name == "steam_emu.ini" || strings.HasSuffix(name, ".ini") {
				if id := readINIAppID(p); id > 0 {
					if _, ok := appIDs[name]; !ok {
						appIDs[name] = id
					}
				}
			}
			if name == "steam_emu.ini" {
				group = emuGroup(p)
			}
			// A specific group beats the generic "Steam emulator".
			if e.Emulator == "" || e.Emulator == "Steam emulator" {
				e.Emulator, e.Marker = group, d.Name()
				markerDir = filepath.Dir(p)
			}
		case unlockers[name]:
			unlocker = true
		case name == "libscepad.dll" || name == "libscepad_x64.dll":
			e.PadHint = "libScePad"
		case (name == "sdl2.dll" || name == "sdl3.dll") && e.PadHint == "":
			e.PadHint = "SDL"
		case name == "steam_api.dll" || name == "steam_api64.dll":
			dlls = append(dlls, p)
		case strings.HasPrefix(name, "goggame-") && strings.HasSuffix(name, ".info") && e.Gog == nil:
			if g, ok := readGogInfo(p); ok && filepath.Dir(p) == root {
				e.Gog, e.GogFile = &g, p
			}
		}
		return nil
	})
	if e.Emulator == "" && !unlocker && signed != nil {
		for _, p := range dlls {
			if !signed(p) {
				e.Emulator, e.Marker = "Steam emulator", filepath.Base(p)+" (unsigned)"
				markerDir = filepath.Dir(p)
				break
			}
		}
	}
	if settingsDir != "" && e.Emulator != "" {
		markerDir = settingsDir // where the schema and settings are
	}
	if markerDir != "" {
		// gbe_fork keeps configs.user.ini inside steam_settings.
		if strings.EqualFold(filepath.Base(markerDir), "steam_settings") {
			markerDir = filepath.Dir(markerDir)
		}
		if rel, err := filepath.Rel(root, markerDir); err == nil && !strings.HasPrefix(rel, "..") {
			e.EmuDir = rel
		}
	}
	// Which file's app id to trust, most specific first.
	for _, f := range []string{"steam_emu.ini", "steam_settings", "onlinefix.ini", "smartsteamemu.ini", "codex.ini", "rune.ini", "tenoke.ini", "empress.ini", "cpy.ini", "3dmgame.ini", "ali213.ini", "steam_appid.txt"} {
		if id, ok := appIDs[f]; ok {
			e.AppID, e.AppIDFrom = id, f
			break
		}
	}
	return e
}

// telling reports whether a file name matters to DetectEmulation.
func telling(name string) bool {
	_, emu := emuFiles[name]
	return emu || unlockers[name] || name == "steam_appid.txt" || name == "user_stats.ini" || name == "steam_api.dll" || name == "steam_api64.dll" ||
		name == "libscepad.dll" || name == "libscepad_x64.dll" || name == "sdl2.dll" || name == "sdl3.dll" ||
		(strings.HasPrefix(name, "goggame-") && strings.HasSuffix(name, ".info"))
}

// emuGroup works out which group's steam_emu.ini this is: RUNE and CODEX
// leave renamed originals next to it, and name themselves in UserName.
func emuGroup(ini string) string {
	dir := filepath.Dir(ini)
	for _, g := range []struct{ file, name string }{
		{"steam_api64.rne", "RUNE"}, {"steam_api.rne", "RUNE"},
		{"steam_api64.cdx", "CODEX"}, {"steam_api.cdx", "CODEX"},
	} {
		if platform.IsFile(filepath.Join(dir, g.file)) {
			return g.name
		}
	}
	b, err := readSmall(ini, 256<<10)
	if err == nil {
		sc := bufio.NewScanner(bytes.NewReader(b))
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(k), "UserName") {
				continue
			}
			switch u := strings.ToUpper(strings.TrimSpace(v)); u {
			case "RUNE", "CODEX", "EMPRESS", "TENOKE", "PLAZA", "SKIDROW", "CPY", "FLT", "HOODLUM":
				return u
			}
		}
	}
	return "Steam emulator"
}

func readAppIDTxt(p string) int {
	b, err := readSmall(p, 64)
	if err != nil {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func readINIAppID(p string) int {
	b, err := readSmall(p, 256<<10)
	if err != nil {
		return 0
	}
	if m := reAppID.FindSubmatch(b); m != nil {
		if id, err := strconv.Atoi(string(m[1])); err == nil && id > 0 {
			return id
		}
	}
	return 0
}
