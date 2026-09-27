package scan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/gamekit/steam"
)

// steamTools are app ids Steam installs that aren't games.
var steamTools = map[int]bool{
	228980:  true, // Steamworks Common Redistributables
	250820:  true, // SteamVR
	431960:  true, // Wallpaper Engine
	1070560: true, // Steam Linux Runtime
	1391110: true, // Steam Linux Runtime - Soldier
	1628350: true, // Steam Linux Runtime - Sniper
	1826330: true, // Proton EasyAntiCheat Runtime
	1493710: true, // Proton Experimental
	1161040: true, // Proton BattlEye Runtime
}

var steamToolNames = []string{"proton", "steam linux runtime", "steamworks", "redistributable", "steamvr"}

// SteamDir returns Steam's install folder ("" if Steam isn't installed),
// with the capitalisation the file system uses (Steam keeps it lower-cased
// in the registry).
func SteamDir() string {
	if d := steam.Dir(); d != "" {
		return platform.RealCase(d)
	}
	return ""
}

// SteamLibraries returns Steam's own folder plus every library folder listed
// in its libraryfolders.vdf.
func SteamLibraries(root string) []string { return steam.Libraries(root) }

// steamCandidates lists the games Steam installed, across all libraries.
func steamCandidates(root string) []Candidate {
	var out []Candidate
	seen := map[int]bool{}
	stats := steamStats(root)
	for _, lib := range SteamLibraries(root) {
		for _, app := range steam.LibraryApps(lib) {
			if !app.Installed || seen[app.ID] || steamTools[app.ID] || isSteamTool(app.Name) {
				continue
			}
			name := app.Name
			if name == "" {
				name = filepath.Base(app.Dir)
			}
			seen[app.ID] = true
			out = append(out, Candidate{
				Title: name, TitleTrusted: true, Dir: app.Dir, Source: Steam, SteamAppID: app.ID,
				LaunchURI: "steam://rungameid/" + strconv.Itoa(app.ID), How: "Steam library", SizeBytes: app.Size,
				StorePlaytime: stats[app.ID].Playtime, StoreLastPlayed: stats[app.ID].LastPlayed,
			})
		}
	}
	return out
}

func isSteamTool(name string) bool {
	l := strings.ToLower(name)
	for _, t := range steamToolNames {
		if strings.Contains(l, t) {
			return true
		}
	}
	return false
}

// SteamWatchDirs are the folders whose changes mean Steam installed or removed something.
func SteamWatchDirs() []string {
	var out []string
	for _, lib := range SteamLibraries(SteamDir()) {
		if d := filepath.Join(lib, "steamapps"); platform.IsDir(d) {
			out = append(out, d)
		}
	}
	return out
}

// sizes remembers folder sizes for a while: walking a big game folder on
// every scan (they run on every install and every half hour) adds up.
var sizes = struct {
	sync.Mutex
	m map[string]sizeEntry
}{m: map[string]sizeEntry{}}

type sizeEntry struct {
	size int64
	at   time.Time
}

const sizeMaxAge = 6 * time.Hour

// dirSize totals file sizes below dir, stopping after limit entries. A size
// worked out in the last few hours is reused.
func dirSize(dir string, limit int) int64 {
	key := strings.ToLower(filepath.Clean(dir))
	sizes.Lock()
	e, ok := sizes.m[key]
	sizes.Unlock()
	if ok && time.Since(e.at) < sizeMaxAge {
		return e.size
	}
	total := walkSize(dir, limit)
	sizes.Lock()
	sizes.m[key] = sizeEntry{size: total, at: time.Now()}
	sizes.Unlock()
	return total
}

func walkSize(dir string, limit int) int64 {
	var total int64
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if n++; n > limit {
			return filepath.SkipAll
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	return total
}
