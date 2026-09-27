package achievements_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/owned"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
)

// TestRealLibrary reads the achievements of every installed game in this
// PC's Seaglass library, from the real Steam, emulator files, GOG Galaxy
// and the stores' servers (with the keys and sign-ins Seaglass has).
// Nothing is written but a cache in a temporary folder.
//
//	WL_REAL_ACH=1 go test ./internal/achievements -run Real -v
func TestRealLibrary(t *testing.T) {
	if os.Getenv("WL_REAL_ACH") == "" {
		t.Skip("set WL_REAL_ACH=1 to read this PC's real achievements")
	}
	lib, err := library.Open(filepath.Join(platform.AppDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := owned.NewClient()
	d := achievements.Deps{
		Env: achievements.DefaultEnv(), Lang: achievements.SteamLanguage(), SteamRoot: scan.SteamDir(),
		SteamKey: platform.LoadSecret("steam-webapi"), Net: client,
		Cache: &achievements.Cache{Dir: t.TempDir()},
	}
	d.SteamAccounts = scan.SteamAccounts(d.SteamRoot)
	d.EpicLocale = owned.EpicLocale(d.Lang)
	if d.SteamKey != "" {
		d.SteamID, _ = owned.SteamID()
	}
	if db := owned.GalaxyDB(); db != "" {
		d.GalaxyDB = db
		d.GOGUnlocks = func(id string) (map[string]achievements.Unlock, error) { return owned.GOGUnlocks(db, id) }
	}
	t.Logf("Steam %q (accounts %v), language %s, key %v, Galaxy %q", d.SteamRoot, d.SteamAccounts, d.Lang, d.SteamKey != "", d.GalaxyDB)
	n := 0
	for _, g := range lib.Games() {
		if !g.Installed {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		l, net := achievements.Resolve(ctx, g, d)
		cancel()
		n++
		named := 0
		for _, a := range l.Items {
			if a.Name != a.ID {
				named++
			}
		}
		t.Logf("%-40.40s %-14s %3d / %3d  named %3d  net %-5v %s", g.DisplayTitle(), l.Source, l.Unlocked, l.Total, named, net, l.Hint)
	}
	t.Logf("%d installed games", n)
}
