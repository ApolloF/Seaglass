package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSigner(t *testing.T) {
	// Programs that carry an embedded Authenticode signature on most PCs.
	var found bool
	for _, p := range []string{
		filepath.Join(ProgramFiles, "GitHub CLI", "gh.exe"),
		filepath.Join(ProgramFiles, "Go", "bin", "go.exe"),
		filepath.Join(ProgramFiles, "Git", "cmd", "git.exe"),
		filepath.Join(ProgramFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(ProgramFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
	} {
		if !IsFile(p) {
			continue
		}
		name, err := Signer(p)
		if err != nil {
			t.Logf("%s: %v", p, err)
			continue
		}
		found = true
		t.Logf("%s is signed by %q", p, name)
		if name == "" {
			t.Errorf("%s: empty signer name", p)
		}
	}
	if !found {
		t.Log("no signed program found to check against")
	}
	// A test binary isn't signed.
	self, _ := os.Executable()
	if name, err := Signer(self); err == nil {
		t.Errorf("test binary reported as signed by %q", name)
	}
}

func TestCommandExe(t *testing.T) {
	for in, want := range map[string]string{
		`"C:\Program Files\Seaglass\Seaglass.exe" --tray`: `C:\Program Files\Seaglass\Seaglass.exe`,
		`C:\WL\Seaglass.exe --tray`:                            `C:\WL\Seaglass.exe`,
		`"unterminated`:                                             ``,
	} {
		if got := commandExe(in); got != want {
			t.Errorf("commandExe(%q) = %q, want %q", in, got, want)
		}
	}
	if got := StartupCommand(`C:\Program Files\WL\Seaglass.exe`); got != `"C:\Program Files\WL\Seaglass.exe" --tray` {
		t.Errorf("StartupCommand = %s", got)
	}
}
