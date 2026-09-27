package scan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// containers are folder names that hold many games rather than one.
var containers = map[string]bool{
	"games": true, "gog games": true, "epic games": true, "repacks": true, "dodi-repacks": true,
	"fitgirl repacks": true, "fitgirl-repacks": true, "elamigos": true, "steamlibrary": true,
}

// skipFolders never hold a game of their own.
var skipFolders = map[string]bool{
	"steamapps": true, "xboxgames": true, "modifiablewindowsapps": true, "windowsapps": true,
	"$recycle.bin": true, "system volume information": true, "_commonredist": true, "commonredist": true,
	"redist": true, "directx": true, "__installer": true, "steam": true,
}

// AutoFolders are common places repacks and standalone games install to,
// on every fixed drive, that exist on this PC.
func AutoFolders() []string {
	var out []string
	for _, drive := range platform.FixedDrives() {
		for _, rel := range []string{
			"Games",
			`Program Files (x86)\DODI-Repacks`,
			`Program Files\DODI-Repacks`,
			`Program Files (x86)\FitGirl Repacks`,
			`Program Files\FitGirl Repacks`,
			"GOG Games",
			"Repacks",
		} {
			if d := filepath.Join(drive, rel); platform.IsDir(d) {
				out = append(out, d)
			}
		}
	}
	return out
}

// folderCandidates treats every subfolder of roots that looks like a game
// as one. A subfolder that is itself a container (…\Games\GOG Games) is
// looked into one more level.
func folderCandidates(roots []string) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	var visit func(root string, depth int)
	visit = func(root string, depth int) {
		es, err := os.ReadDir(root)
		if err != nil {
			return
		}
		for _, e := range es {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			l := strings.ToLower(name)
			if skipFolders[l] || strings.HasPrefix(name, ".") {
				continue
			}
			dir := filepath.Join(root, name)
			if seen[platform.Key(dir)] {
				continue
			}
			if containers[l] && depth == 0 {
				visit(dir, depth+1)
				continue
			}
			ok, sign := looksLikeGame(dir, true)
			if !ok {
				continue
			}
			seen[platform.Key(dir)] = true
			out = append(out, Candidate{
				Title: CleanTitle(name), Dir: dir, Source: Folder,
				How: "Game folder in " + root + " (" + sign + ")",
			})
		}
	}
	for _, r := range roots {
		if filepath.IsAbs(r) && platform.IsDir(r) {
			visit(filepath.Clean(r), 0)
		}
	}
	return out
}

func isFile(p string) bool { return platform.IsFile(p) }

func isDir(p string) bool { return platform.IsDir(p) }
