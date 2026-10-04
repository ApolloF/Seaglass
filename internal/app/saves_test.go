package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

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
		if p.limit() != 0 {
			t.Errorf("installed=%v: the installer with windows is stopped after %v", c.installed, p.limit())
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
	if p.ask != "" || !p.silent() || p.limit() <= 0 {
		t.Errorf("signed update: %+v, limit %v", p, p.limit())
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

// TestFakeSyncerInstaller stands in for Syncer's installer when run by the
// tests below: it waits, then exits with the code they asked for.
func TestFakeSyncerInstaller(t *testing.T) {
	spec := os.Getenv("WL_FAKE_INSTALLER")
	if spec == "" {
		return
	}
	wait, code, _ := strings.Cut(spec, ",")
	d, _ := time.ParseDuration(wait)
	n, _ := strconv.Atoi(code)
	time.Sleep(d)
	os.Exit(n)
}

// runFakeInstaller runs the test binary as an installer that takes wait
// and exits with code.
func runFakeInstaller(t *testing.T, wait time.Duration, code int, limit time.Duration) error {
	t.Helper()
	t.Setenv("WL_FAKE_INSTALLER", wait.String()+","+strconv.Itoa(code))
	return runSyncerInstaller(context.Background(), os.Args[0], []string{"-test.run=^TestFakeSyncerInstaller$"}, t.TempDir(), limit)
}

// An installer with windows is waited for however long the person takes:
// no timer stops it.
func TestInteractiveSyncerInstallerIsNotStopped(t *testing.T) {
	if err := runFakeInstaller(t, 2*time.Second, 0, 0); err != nil {
		t.Fatalf("an installer still open was stopped: %v", err)
	}
}

// A silent install has nobody to finish it, so it's stopped at its limit.
func TestSilentSyncerInstallerIsStoppedAtLimit(t *testing.T) {
	start := time.Now()
	err := runFakeInstaller(t, time.Minute, 0, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "didn't finish") {
		t.Fatalf("err = %v", err)
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("stopped only after %v", d)
	}
}

// Cancelling or closing the installer is the person's choice, not a
// failure; any other exit still is, and a silent install never "closes".
func TestClosedSyncerInstallerIsNotAnError(t *testing.T) {
	if err := runFakeInstaller(t, 0, nsisCancelled, 0); !errors.Is(err, errInstallerClosed) {
		t.Errorf("closed installer: %v", err)
	}
	var exit *exec.ExitError
	if err := runFakeInstaller(t, 0, 2, 0); err == nil || errors.Is(err, errInstallerClosed) || !errors.As(err, &exit) {
		t.Errorf("installer that stopped with an error: %v", err)
	}
	if err := runFakeInstaller(t, 0, nsisCancelled, time.Minute); err == nil || errors.Is(err, errInstallerClosed) {
		t.Errorf("silent installer exiting %d: %v", nsisCancelled, err)
	}
}
