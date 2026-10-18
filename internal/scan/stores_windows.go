package scan

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
	"golang.org/x/sys/windows/registry"
)

// ---------- GOG ----------

func gogCandidates() []Candidate {
	var out []Candidate
	registryEach(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\GOG.com\Games`, func(k registry.Key, _ string) {
		dir := filepath.Clean(regString(k, "path"))
		if !filepath.IsAbs(dir) || !platform.IsDir(dir) {
			return
		}
		c := Candidate{Title: regString(k, "gameName"), TitleTrusted: true, Dir: dir, Source: GOG, How: "GOG Galaxy library", GogID: regString(k, "gameID")}
		if exe := regString(k, "exe"); filepath.IsAbs(exe) && platform.Within(dir, exe) {
			c.Exe, c.Args, c.WorkDir = filepath.Clean(exe), regString(k, "launchParam"), regString(k, "workingDir")
		}
		if c.Title == "" {
			c.Title = filepath.Base(dir)
		}
		out = append(out, c)
	})
	return out
}

// ---------- EA ----------

var eaSkip = map[string]bool{"ea desktop": true, "ea core": true, "eadm": true, "origin": true, "ea app": true}

func eaCandidates() []Candidate {
	var out []Candidate
	for _, hive := range []string{`SOFTWARE\WOW6432Node\EA Games`, `SOFTWARE\WOW6432Node\Origin Games`, `SOFTWARE\WOW6432Node\Electronic Arts`, `SOFTWARE\EA Games`} {
		registryEach(registry.LOCAL_MACHINE, hive, func(k registry.Key, name string) {
			if eaSkip[strings.ToLower(name)] {
				return
			}
			dir := regString(k, "Install Dir")
			if dir == "" {
				dir = regString(k, "InstallDir")
			}
			dir = filepath.Clean(dir)
			if !filepath.IsAbs(dir) || !platform.IsDir(dir) {
				return
			}
			title := regString(k, "DisplayName")
			if title == "" {
				title = name
			}
			out = append(out, Candidate{Title: title, TitleTrusted: true, Dir: dir, Source: EA, How: "EA app library"})
		})
	}
	return out
}

// ---------- Ubisoft ----------

func ubisoftCandidates() []Candidate {
	var out []Candidate
	registryEach(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Ubisoft\Launcher\Installs`, func(k registry.Key, id string) {
		dir := filepath.Clean(filepath.FromSlash(regString(k, "InstallDir")))
		if !filepath.IsAbs(dir) || !platform.IsDir(dir) {
			return
		}
		out = append(out, Candidate{
			Title: filepath.Base(dir), Dir: dir, Source: Ubisoft, UbisoftID: id,
			LaunchURI: "uplay://launch/" + url.PathEscape(id) + "/0", How: "Ubisoft Connect library",
		})
	})
	return out
}

// ---------- registry helpers ----------

func regString(k registry.Key, name string) string {
	s, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func regInt(k registry.Key, name string) int64 {
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0
	}
	return int64(v)
}

// registryEach visits every subkey of root\path.
func registryEach(root registry.Key, path string, visit func(k registry.Key, name string)) {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	for _, name := range names {
		sub, err := registry.OpenKey(k, name, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		visit(sub, name)
		sub.Close()
	}
}
