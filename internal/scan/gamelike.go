package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// strongFiles are files that (almost) only games ship, with the sign shown to the user.
var strongFiles = map[string]string{
	"steam_api.dll":                 "Steamworks",
	"steam_api64.dll":               "Steamworks",
	"steamworks.net.dll":            "Steamworks",
	"steam_emu.ini":                 "Steam emulator",
	"steam_appid.txt":               "Steam app id",
	"onlinefix64.dll":               "Steam emulator",
	"smartsteamemu.ini":             "Steam emulator",
	"coldclientloader.ini":          "Steam emulator",
	"steam_interfaces.txt":          "Steam emulator",
	"unityplayer.dll":               "Unity",
	"gameassembly.dll":              "Unity",
	"eossdk-win64-shipping.dll":     "Epic Online Services",
	"eossdk-win32-shipping.dll":     "Epic Online Services",
	"galaxy64.dll":                  "GOG Galaxy",
	"galaxy.dll":                    "GOG Galaxy",
	"binkw32.dll":                   "Bink video",
	"binkw64.dll":                   "Bink video",
	"bink2w64.dll":                  "Bink video",
	"fmod.dll":                      "FMOD",
	"fmod64.dll":                    "FMOD",
	"fmodex64.dll":                  "FMOD",
	"fmodstudio.dll":                "FMOD",
	"fmodstudio64.dll":              "FMOD",
	"wwise.dll":                     "Wwise",
	"data.win":                      "GameMaker",
	"nvngx_dlss.dll":                "DLSS",
	"amd_fidelityfx_dx12.dll":       "FSR",
	"discord_game_sdk.dll":          "Discord Game SDK",
	"easyanticheat_x64.dll":         "EasyAntiCheat",
	"rpg_rt.exe":                    "RPG Maker",
	"renpy.exe":                     "Ren'Py",
	"thirdparty_notices_unreal.txt": "Unreal Engine",
	"crashreportclient.exe":         "Unreal Engine",
	"sl.interposer.dll":             "Streamline",
	"libscepad.dll":                 "DualSense",
}

// weakFiles are common in games but also in other software.
var weakFiles = map[string]bool{
	"d3dcompiler_47.dll": true, "openal32.dll": true, "sdl2.dll": true, "sdl3.dll": true, "xinput1_3.dll": true,
	"physxdevice64.dll": true, "physx3_x64.dll": true, "gfsdk_aftermath_lib.x64.dll": true, "d3dx9_43.dll": true,
}

// appFiles mark Electron, Chromium and CEF applications: browsers, editors and chat apps.
var appFiles = map[string]bool{
	"app.asar": true, "resources.pak": true, "chrome_100_percent.pak": true, "icudtl.dat": true,
	"msedge.dll": true, "chrome.dll": true, "libcef.dll": true, "ffmpeg.dll": true,
}

// strongDirs are folder names games use.
var strongDirs = map[string]string{
	"steam_settings": "Steam emulator",
	"paks":           "Unreal Engine",
	"renpy":          "Ren'Py",
}

// gameLike remembers looksLikeGame's answers for this run of Seaglass.
// A folder is looked at again when its own modification time changes
// (files added or removed at its top), so later scans only stat it.
var gameLike = struct {
	sync.Mutex
	m map[string]gameLikeEntry
}{m: map[string]gameLikeEntry{}}

type gameLikeEntry struct {
	mod  time.Time
	ok   bool
	sign string
}

// looksLikeGame reports whether dir holds a game, and the telling sign.
// With allowWeak, a folder with several weak signs and no application
// markers also counts (for folders the user said hold games). Only the top
// few levels are looked at, with a budget, so a huge folder stays cheap.
func looksLikeGame(dir string, allowWeak bool) (bool, string) {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false, ""
	}
	key := strconv.FormatBool(allowWeak) + "|" + strings.ToLower(filepath.Clean(dir))
	gameLike.Lock()
	e, ok := gameLike.m[key]
	gameLike.Unlock()
	if ok && e.mod.Equal(fi.ModTime()) {
		return e.ok, e.sign
	}
	yes, sign := walkGameLike(dir, allowWeak)
	gameLike.Lock()
	gameLike.m[key] = gameLikeEntry{mod: fi.ModTime(), ok: yes, sign: sign}
	gameLike.Unlock()
	return yes, sign
}

func walkGameLike(dir string, allowWeak bool) (bool, string) {
	root := filepath.Clean(dir)
	depth0 := strings.Count(root, `\`)
	sign := ""
	weak, app := 0, false
	n := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if n++; n > 4000 {
			return filepath.SkipAll
		}
		name := strings.ToLower(d.Name())
		if d.IsDir() {
			if p != root {
				if s, ok := strongDirs[name]; ok {
					sign = s
					return filepath.SkipAll
				}
				if strings.HasSuffix(name, "_data") && isFile(filepath.Join(p, "globalgamemanagers")) {
					sign = "Unity"
					return filepath.SkipAll
				}
				if strings.Count(filepath.Clean(p), `\`)-depth0 >= 4 {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if s, ok := strongFiles[name]; ok {
			sign = s
			return filepath.SkipAll
		}
		if strings.HasPrefix(name, "goggame-") && strings.HasSuffix(name, ".info") {
			sign = "GOG game"
			return filepath.SkipAll
		}
		switch {
		case weakFiles[name]:
			weak++
		case appFiles[name]:
			app = true
		case strings.HasSuffix(name, ".rpa") || strings.HasSuffix(name, ".forge") || strings.HasSuffix(name, ".bik") || strings.HasSuffix(name, ".bk2"):
			weak++
		}
		return nil
	})
	if sign != "" {
		return true, sign
	}
	return allowWeak && !app && weak >= 2, "game files"
}
