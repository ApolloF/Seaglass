package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// touch moves a file's or folder's time forward, the way a write would
// (tests run faster than the file system's clock ticks).
func touch(t *testing.T, p string) {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	next := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(p, next, next); err != nil {
		t.Fatal(err)
	}
}

func TestEmulationCache(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{"Game.exe": big(10), "Binaries/Win64/steam_api64.dll": "signed", "Data/a/b/c.pak": "x"})
	signed := func(p string) bool {
		b, _ := os.ReadFile(p)
		return string(b) == "signed"
	}
	if e := detectEmulationCached(root, signed); e.Emulator != "" {
		t.Fatalf("clean copy: %+v", e)
	}
	// Nothing changed: the cached answer, without reading the folder.
	if _, ok := emulationCache.get(folderKey(root)); !ok {
		t.Fatal("answer not cached")
	}
	// A crack's config dropped next to the DLL: that folder's time changes.
	ini := filepath.Join(root, "Binaries", "Win64", "steam_emu.ini")
	mk(t, root, map[string]string{"Binaries/Win64/steam_emu.ini": "AppId=620\nUserName=RUNE\n"})
	touch(t, filepath.Dir(ini))
	if e := detectEmulationCached(root, signed); e.Emulator != "RUNE" || e.AppID != 620 {
		t.Fatalf("after adding steam_emu.ini: %+v", e)
	}
	// The DLL replaced in place (no new file): its own size and time change.
	root2 := t.TempDir()
	mk(t, root2, map[string]string{"Game.exe": big(10), "steam_api64.dll": "signed"})
	if e := detectEmulationCached(root2, signed); e.Emulator != "" {
		t.Fatalf("clean copy 2: %+v", e)
	}
	dll := filepath.Join(root2, "steam_api64.dll")
	if err := os.WriteFile(dll, []byte("patched!"), 0o644); err != nil {
		t.Fatal(err)
	}
	touch(t, dll)
	if e := detectEmulationCached(root2, signed); e.Emulator != "Steam emulator" {
		t.Fatalf("after patching the DLL in place: %+v", e)
	}
}

func TestExeCache(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{"Old.exe": big(10)})
	if got := filepath.Base(pickExeCached(root, "Harbor")); got != "Old.exe" {
		t.Fatalf("first pick = %s", got)
	}
	mk(t, root, map[string]string{"Harbor.exe": big(10)})
	touch(t, root)
	if got := filepath.Base(pickExeCached(root, "Harbor")); got != "Harbor.exe" {
		t.Fatalf("after a new exe = %s", got)
	}
}

// BenchmarkEnrichRepeat is a repeat scan's per-game work over 100 games
// of ~1,000 files each, with and without the cache.
func BenchmarkEnrichRepeat(b *testing.B) {
	lib := b.TempDir()
	var dirs []string
	for g := 0; g < 100; g++ {
		dir := filepath.Join(lib, fmt.Sprintf("Game %03d", g))
		for d := 0; d < 20; d++ {
			sub := filepath.Join(dir, "Data", fmt.Sprintf("pack%02d", d), "sub")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				b.Fatal(err)
			}
			for f := 0; f < 50; f++ {
				_ = os.WriteFile(filepath.Join(sub, fmt.Sprintf("f%02d.dat", f)), nil, 0o644)
			}
		}
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("Game%03d.exe", g)), []byte("MZ"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "steam_api64.dll"), []byte("x"), 0o644)
		dirs = append(dirs, dir)
	}
	signed := func(string) bool { return true }
	b.Run("uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, d := range dirs {
				DetectEmulation(d, signed)
				PickExe(d, "x")
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		for _, d := range dirs { // the first scan
			detectEmulationCached(d, signed)
			pickExeCached(d, "x")
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, d := range dirs {
				detectEmulationCached(d, signed)
				pickExeCached(d, "x")
			}
		}
	})
}

func TestTellingAchievementMarkers(t *testing.T) {
	for _, n := range []string{"user_stats.ini", ".1911", "configs.user.ini"} {
		if !telling(n) {
			t.Errorf("%s isn't in the fingerprint", n)
		}
	}
}
