package scan

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/lnk"
	"github.com/ApolloF/Seaglass/internal/platform"
)

// shortcut is a .lnk that starts an executable.
type shortcut struct {
	Path, Name        string
	Target, Args, Dir string
}

// shortcutDirs are where installers put their shortcuts.
func shortcutDirs() []string {
	var out []string
	for _, d := range []string{platform.Desktop, platform.PublicDesktop, platform.StartMenu, platform.CommonStartMenu} {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

var shortcutSkip = []string{"uninstall", "readme", "read me", "manual", "help", "website", "web site", "support", "config", "settings", "setup", "license", "changelog", "editor", "server", "benchmark", "crash"}

// shortcuts lists .lnk files in dirs whose target is an existing .exe.
func shortcuts(dirs []string) []shortcut {
	var out []shortcut
	n := 0
	for _, root := range dirs {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if n++; n > 20000 {
				return filepath.SkipAll
			}
			if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".lnk") {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			l := strings.ToLower(name)
			for _, s := range shortcutSkip {
				if strings.Contains(l, s) {
					return nil
				}
			}
			link, err := lnk.ReadFile(p)
			if err != nil || !strings.EqualFold(filepath.Ext(link.Target), ".exe") || platform.SystemPath(link.Target) || excludedExe(link.Target) {
				return nil
			}
			if !platform.IsFile(link.Target) {
				return nil
			}
			out = append(out, shortcut{Path: p, Name: name, Target: link.Target, Args: link.Args, Dir: link.WorkDir})
			return nil
		})
	}
	return out
}

// bestShortcut picks the shortcut that most likely starts the game in dir.
func bestShortcut(scs []shortcut, dir, title string) (shortcut, bool) {
	best, bestScore := shortcut{}, -1
	nt := Normalize(title)
	for _, s := range scs {
		if !platform.Within(dir, s.Target) {
			continue
		}
		score := 1
		nn := Normalize(s.Name)
		switch {
		case strings.HasPrefix(strings.ToLower(s.Name), "play "):
			score += 30
		case nt != "" && nn == nt:
			score += 25
		case nt != "" && strings.Contains(nn, nt):
			score += 15
		}
		if strings.Contains(strings.ToLower(s.Path), `\desktop\`) {
			score += 5
		}
		if score > bestScore {
			best, bestScore = s, score
		}
	}
	return best, bestScore >= 0
}
