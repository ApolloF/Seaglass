package scan

import (
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// uninstallEntry is one program in Windows' installed apps list.
type uninstallEntry struct {
	Key, Name, Publisher, Dir, Icon string
	SizeKB                          int64
}

// repackers maps text found in an installer's publisher or folder to the
// repacker's display name.
var repackers = []struct{ match, name string }{
	{"dodi", "DODI"}, {"fitgirl", "FitGirl"}, {"elamigos", "ElAmigos"}, {"kaos", "KaOs"},
	{"xatab", "xatab"}, {"chovka", "Chovka"}, {"r.g. mechanics", "R.G. Mechanics"},
	{"decepticon", "Decepticon"}, {"masquerade", "Masquerade"}, {"gnarly", "Gnarly"},
	{"darck", "Darck"}, {"empress", "EMPRESS"},
}

func repackerOf(publisher, dir string) string {
	s := strings.ToLower(publisher + "|" + dir)
	for _, r := range repackers {
		if strings.Contains(s, r.match) {
			return r.name
		}
	}
	return ""
}

// launcherNames are store clients and companion apps, never games.
var launcherNames = []string{
	"steam", "epic games launcher", "gog galaxy", "ubisoft connect", "uplay", "ea app", "ea", "origin", "battle.net",
	"xbox", "rockstar games launcher", "riot client", "amazon games", "itch", "playnite", "launchbox", "heroic",
	"nvidia app", "geforce experience", "amd software", "discord", "seaglass", "waterlauncher", "syncer", "dlss updater",
}

// toolWords mark runtimes, drivers and redistributables.
var toolWords = []string{"redistributable", "runtime", "directx", "visual c++", "physx", "vulkan", "openal", "easyanticheat", "battleye", "driver", "sdk"}

func isLauncher(name string) bool {
	l := strings.ToLower(strings.TrimSpace(name))
	for _, n := range launcherNames {
		if l == n || strings.HasPrefix(l, n+" ") {
			return true
		}
	}
	for _, w := range toolWords {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// storePublishers install games through their own store records, which the
// store scanners already read; their uninstall entries only add a name.
var gamePublishers = []string{"blizzard entertainment", "riot games", "gog.com", "electronic arts", "rockstar games", "valve"}

// installerCandidates turns uninstall entries that are games into candidates.
func installerCandidates(entries []uninstallEntry) []Candidate {
	var out []Candidate
	for _, e := range entries {
		if strings.HasPrefix(e.Key, "Steam App ") || isLauncher(e.Name) {
			continue
		}
		dir := filepath.Clean(strings.Trim(e.Dir, `"`))
		if !filepath.IsAbs(dir) || platform.SystemPath(dir) || !platform.IsDir(dir) || filepath.Dir(dir) == dir {
			continue
		}
		rp := repackerOf(e.Publisher, dir)
		pub := strings.ToLower(e.Publisher)
		gamePub := false
		for _, g := range gamePublishers {
			if strings.Contains(pub, g) {
				gamePub = true
			}
		}
		if rp == "" && !gamePub {
			if ok, _ := looksLikeGame(dir, false); !ok {
				continue
			}
		}
		c := Candidate{
			Title: CleanTitle(e.Name), TitleTrusted: true, Dir: dir, Source: Installer,
			Publisher: e.Publisher, Repacker: rp, SizeBytes: e.SizeKB * 1024,
		}
		switch {
		case strings.Contains(pub, "blizzard"):
			c.Source, c.How = BattleNet, "Battle.net library"
		case strings.Contains(pub, "electronic arts"):
			c.Source, c.How = EA, "EA app library"
		case strings.Contains(pub, "gog.com"):
			c.Source, c.How = GOG, "GOG installer"
		case rp != "":
			c.How = "Installed by a " + rp + " repack"
		case e.Publisher != "":
			c.How = "Installer entry (" + e.Publisher + ")"
		default:
			c.How = "Installer entry in Windows"
		}
		if exe := iconExe(e.Icon); exe != "" && platform.Within(dir, exe) && !excludedExe(exe) {
			c.Exe = exe
		}
		out = append(out, c)
	}
	return out
}

// iconExe extracts the executable from a DisplayIcon value ("C:\…\game.exe,0").
func iconExe(icon string) string {
	icon = strings.TrimSpace(icon)
	if i := strings.LastIndex(icon, ","); i > 0 {
		icon = icon[:i]
	}
	icon = strings.Trim(icon, `"`)
	if !strings.EqualFold(filepath.Ext(icon), ".exe") || !filepath.IsAbs(icon) {
		return ""
	}
	return filepath.Clean(icon)
}
