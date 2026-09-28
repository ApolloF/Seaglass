package achievements

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
)

func TestSetINIKey(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"replaced":         {"[Settings]\r\nUsername = Me\r\nachievements=0\r\n[DLC]\r\n5490\r\n", "[Settings]\r\nUsername = Me\r\nAchievements = 1\r\n[DLC]\r\n5490\r\n"},
		"added to section": {"[Settings]\nUsername = Me\n\n[DLC]\nAchievements=0\n", "[Settings]\nAchievements = 1\nUsername = Me\n\n[DLC]\nAchievements=0\n"},
		"commented out":    {"[Settings]\n;Achievements = 1\n", "[Settings]\nAchievements = 1\n;Achievements = 1\n"},
		"no section":       {"\xef\xbb\xbf[DLC]\r\n5490", "\xef\xbb\xbf[DLC]\r\n5490\r\n\r\n[Settings]\r\nAchievements = 1\r\n"},
		"empty":            {"", "[Settings]\r\nAchievements = 1\r\n"},
		"key prefix kept":  {"[settings]\nAchKeyPrefix = AFOP_Ach_\n", "[settings]\nAchievements = 1\nAchKeyPrefix = AFOP_Ach_\n"},
	} {
		if got := string(setINIKey([]byte(c.in), "Settings", "Achievements", "1")); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

// A VOICES38 game with achievements off offers to turn them on; once on,
// the card waits for a play, and after one that saved nothing it goes away.
func TestResolveUplayTurnOn(t *testing.T) {
	root := t.TempDir()
	ini := "[Settings]\r\nUsername = Me\r\nSaveType=0\r\n"
	mk(t, root, map[string]string{
		"game/voices38.dll":          "x",
		"game/upc_r2_loader64.dll":   "x",
		"game/upc_r2.ini":            ini,
		"bare/uplay_r1_loader64.dll": "x",
	})
	d := testDeps(t, root, nil)
	g := library.Game{ID: 1, Source: "installer", External: true, Emulator: "VOICES38", EmuDir: ".", Dir: filepath.Join(root, "game"), LastPlayed: time.Now().Add(-time.Hour).Unix()}
	l, _ := Resolve(context.Background(), g, d)
	if l.Source != "VOICES38" || l.Fix != FixUplayINI || !strings.Contains(l.Hint, "Achievements = 1") {
		t.Fatalf("off: %+v", l)
	}

	p, err := EnableUplay(g, d.Env)
	if err != nil || p != filepath.Join(g.Dir, "upc_r2.ini") {
		t.Fatalf("EnableUplay: %q %v", p, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "[Settings]\r\nAchievements = 1\r\nUsername = Me\r\nSaveType=0\r\n" {
		t.Errorf("ini: %q", b)
	}
	if b, _ := os.ReadFile(p + ".seaglass.bak"); string(b) != ini {
		t.Errorf("backup: %q", b)
	}
	l, _ = Resolve(context.Background(), g, d)
	if l.Source != "VOICES38" || l.Fix != "" || !strings.Contains(l.Hint, "Play the game") {
		t.Fatalf("on, not played since: %+v", l)
	}
	// Turning it on again changes nothing, the backup included.
	if _, err := EnableUplay(g, d.Env); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p + ".seaglass.bak"); string(b) != ini {
		t.Errorf("backup overwritten: %q", b)
	}

	g.LastPlayed = time.Now().Add(time.Minute).Unix()
	l, _ = Resolve(context.Background(), g, d)
	if l.Source != "" || l.Hint != "" || l.Total != 0 || l.Fix != "" {
		t.Fatalf("played since, nothing saved: %+v", l)
	}
	// Once it does save some, they show.
	mk(t, root, map[string]string{"Roaming/Goldberg UplayEmu Saves/6200/achievements.json": `{"AFOP_Ach_1":{"earned":1,"earned_time":1700000000}}`})
	d.UplayGames = 1
	if l, _ = Resolve(context.Background(), g, d); l.Source != "VOICES38" || l.Unlocked != 1 {
		t.Fatalf("saved: %+v", l)
	}

	// No ini yet: one named after the loader is made.
	d.UplayGames = 2
	bare := library.Game{ID: 2, Source: "installer", External: true, Emulator: "Uplay emulator", EmuDir: ".", Dir: filepath.Join(root, "bare")}
	if l, _ = Resolve(context.Background(), bare, d); l.Fix != FixUplayINI {
		t.Fatalf("bare: %+v", l)
	}
	if p, err = EnableUplay(bare, d.Env); err != nil || filepath.Base(p) != "uplay_r1.ini" {
		t.Fatalf("bare: %q %v", p, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "[Settings]\r\nAchievements = 1\r\n" {
		t.Errorf("bare ini: %q", b)
	}
	if _, err := os.Stat(p + ".seaglass.bak"); err == nil {
		t.Error("backed up a file that wasn't there")
	}

	// Nothing to change: no fix offered, and EnableUplay says why.
	none := library.Game{ID: 3, Source: "installer", External: true, Emulator: "VOICES38", EmuDir: ".", Dir: filepath.Join(root, "none")}
	_ = os.MkdirAll(none.Dir, 0o755)
	if l, _ = Resolve(context.Background(), none, d); l.Fix != "" {
		t.Errorf("none: %+v", l)
	}
	if _, err := EnableUplay(none, d.Env); err == nil {
		t.Error("none: no error")
	}
}
