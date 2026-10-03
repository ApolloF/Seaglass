package scan

import (
	"path/filepath"
	"strings"
	"testing"
)

// Steam installs that were never migrated keep libraryfolders.vdf in the
// pre-2021 format, with each library as a plain value.
func TestSteamCandidatesOldLibraryFormat(t *testing.T) {
	root := t.TempDir()
	lib := t.TempDir()
	vdfPath := strings.ReplaceAll(lib, `\`, `\\`)
	mk(t, root, map[string]string{
		"steamapps/libraryfolders.vdf": `"LibraryFolders" { "TimeNextStatsReport" "1" "1" "` + vdfPath + `" }`,
	})
	mk(t, lib, map[string]string{
		"steamapps/appmanifest_413150.acf": `"AppState" { "appid" "413150" "name" "Stardew Valley" "StateFlags" "4" "installdir" "Stardew Valley" }`,
		"steamapps/common/Stardew Valley/Stardew Valley.exe": "x",
	})
	libs := SteamLibraries(root)
	if len(libs) != 2 || !strings.EqualFold(libs[1], filepath.Clean(lib)) {
		t.Fatalf("libraries = %q", libs)
	}
	cs := steamCandidates(root)
	if len(cs) != 1 || cs[0].SteamAppID != 413150 || cs[0].Title != "Stardew Valley" {
		t.Errorf("candidates = %+v", cs)
	}
}
