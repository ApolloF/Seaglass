package app

import (
	"strings"
	"testing"

	"github.com/ApolloF/Seaglass/internal/syncer"
)

// Syncer comes only from its own repository's releases, over HTTPS.
func TestSyncerFeedIsPinned(t *testing.T) {
	if syncerFeed.LatestURL != "https://api.github.com/repos/ApolloF/syncer/releases/latest" {
		t.Errorf("latest release asked from %s", syncerFeed.LatestURL)
	}
	if syncerFeed.AssetPrefix != "https://github.com/ApolloF/syncer/releases/download/" {
		t.Errorf("downloads allowed from %s", syncerFeed.AssetPrefix)
	}
	if syncerFeed.Product != "Syncer" {
		t.Errorf("signatures checked for %q", syncerFeed.Product)
	}
}

func TestSyncerInstallRefusesSameOrOlder(t *testing.T) {
	inst := syncer.Install{Exe: `C:\Syncer.exe`, Version: "1.5.0"}
	for _, signed := range []bool{false, true} {
		for _, tag := range []string{"v1.5.0", "v1.4.9", "v0.0.1"} {
			if _, err := syncerInstallPlan(tag, inst, true, signed); err == nil {
				t.Errorf("signed=%v: %s accepted over 1.5.0", signed, tag)
			}
		}
	}
	if _, err := syncerInstallPlan("latest", syncer.Install{}, false, true); err == nil {
		t.Error("a release that isn't a version was accepted")
	}
}

// An unsigned release is never run silently: the person says yes first,
// and Syncer's installer shows its windows.
func TestUnsignedSyncerInstallAsksAndShowsInstaller(t *testing.T) {
	for _, c := range []struct {
		inst      syncer.Install
		installed bool
	}{
		{syncer.Install{}, false},
		{syncer.Install{Exe: `C:\Syncer.exe`, Version: "1.5.0"}, true},
	} {
		p, err := syncerInstallPlan("v1.6.0", c.inst, c.installed, false)
		if err != nil {
			t.Fatal(err)
		}
		if p.ask == "" || !strings.Contains(p.ask, "v1.6.0") {
			t.Errorf("installed=%v: not asked first (%q)", c.installed, p.ask)
		}
		if p.silent() {
			t.Errorf("installed=%v: unsigned installer runs silently: %v", c.installed, p.args)
		}
	}
}

func TestSignedSyncerInstallIsSilent(t *testing.T) {
	p, err := syncerInstallPlan("v1.6.0", syncer.Install{Exe: `C:\Syncer.exe`, Version: "1.5.0"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.ask != "" || !p.silent() {
		t.Errorf("signed update: %+v", p)
	}
	// Over a Syncer whose version is unknown it could be a downgrade: ask.
	p, err = syncerInstallPlan("v1.6.0", syncer.Install{Exe: `C:\Syncer.exe`}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.ask == "" || p.silent() {
		t.Errorf("signed install over an unknown version: %+v", p)
	}
}
