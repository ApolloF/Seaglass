package emulated

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// write creates files under root; a value of nil bytes makes a folder.
func write(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for rel, b := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if b == nil {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// sfo builds a PARAM.SFO with text values.
func sfo(vals map[string]string) []byte {
	var keys, data bytes.Buffer
	type entry struct{ k, d, n uint32 }
	var es []entry
	for _, k := range []string{"CATEGORY", "TITLE", "TITLE_ID"} {
		v, ok := vals[k]
		if !ok {
			continue
		}
		es = append(es, entry{uint32(keys.Len()), uint32(data.Len()), uint32(len(v) + 1)})
		keys.WriteString(k + "\x00")
		data.WriteString(v + "\x00")
	}
	le := binary.LittleEndian
	hdr := 20 + 16*len(es)
	out := make([]byte, hdr)
	copy(out, "\x00PSF")
	le.PutUint32(out[4:], 0x101)
	le.PutUint32(out[8:], uint32(hdr))
	le.PutUint32(out[12:], uint32(hdr+keys.Len()))
	le.PutUint32(out[16:], uint32(len(es)))
	for i, e := range es {
		b := out[20+16*i:]
		le.PutUint16(b[0:], uint16(e.k))
		le.PutUint16(b[2:], 0x0204)
		le.PutUint32(b[4:], e.n)
		le.PutUint32(b[8:], e.n)
		le.PutUint32(b[12:], e.d)
	}
	return append(append(out, keys.Bytes()...), data.Bytes()...)
}

// gcDisc is the start of a GameCube (or, with wii, a Wii) disc image.
func gcDisc(id, name string, wii bool, size int) []byte {
	b := make([]byte, max(size, 0x400))
	copy(b, id)
	if wii {
		binary.BigEndian.PutUint32(b[0x18:], wiiMagic)
	} else {
		binary.BigEndian.PutUint32(b[0x1C:], gcMagic)
	}
	copy(b[0x20:], name)
	return b
}

func ps3Disc(serial, title, category string) map[string][]byte {
	return map[string][]byte{
		"PS3_GAME/PARAM.SFO":        sfo(map[string]string{"CATEGORY": category, "TITLE": title, "TITLE_ID": serial}),
		"PS3_GAME/USRDIR/EBOOT.BIN": []byte("eboot"),
		"PS3_GAME/PIC1.PNG":         []byte("png"),
	}
}

func under(prefix string, files map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range files {
		out[prefix+"/"+k] = v
	}
	return out
}

func findIn(t *testing.T, dirs ...string) []Game {
	t.Helper()
	return find(context.Background(), findInstalls(dirs), time.Now().Add(time.Minute))
}

func byTitle(games []Game) map[string]Game {
	m := map[string]Game{}
	for _, g := range games {
		m[g.Title] = g
	}
	return m
}

func TestRPCS3GameListAndCopies(t *testing.T) {
	root := t.TempDir()
	emu := filepath.Join(root, "Emulators")
	games := filepath.Join(root, "Games", "PS3")
	write(t, emu, map[string][]byte{"RPCS3/rpcs3.exe": []byte("x")})
	write(t, games, under("Swap Force [BLES01860]", ps3Disc("BLES01860", "Skylanders SWAP Force", "DG")))
	// A second region's copy RPCS3 doesn't list, next to the one it does.
	write(t, games, under("Swap Force [BLES01966]", ps3Disc("BLES01966", "Skylanders SWAP Force", "DG")))
	write(t, games, under("Super [BLES02172]", ps3Disc("BLES02172", "Skylanders™\nSuperChargers", "DG")))
	gy := "BLES01860: \"" + filepath.ToSlash(filepath.Join(games, "Swap Force [BLES01860]")) + "/\"\n" +
		"BLES02172: " + filepath.ToSlash(filepath.Join(games, "Super [BLES02172]")) + "/\n" +
		"BLUS00000: \"Z:/gone/\"\n"
	write(t, emu, map[string][]byte{"RPCS3/config/games.yml": []byte(gy)})
	// Installed: a game (HG) and an update (GD), which can't be booted.
	write(t, emu, map[string][]byte{
		"RPCS3/dev_hdd0/game/NPEB00001/PARAM.SFO":        sfo(map[string]string{"CATEGORY": "HG", "TITLE": "Flower", "TITLE_ID": "NPEB00001"}),
		"RPCS3/dev_hdd0/game/NPEB00001/USRDIR/EBOOT.BIN": []byte("x"),
		"RPCS3/dev_hdd0/game/BLES01860/PARAM.SFO":        sfo(map[string]string{"CATEGORY": "GD", "TITLE": "Update", "TITLE_ID": "BLES01860"}),
		"RPCS3/dev_hdd0/game/BLES01860/USRDIR/EBOOT.BIN": []byte("x"),
	})

	got := byTitle(findIn(t, emu))
	if len(got) != 4 {
		t.Fatalf("want 4 games, got %v", got)
	}
	a, b := got["Skylanders SWAP Force (BLES01860)"], got["Skylanders SWAP Force (BLES01966)"]
	if a.File == "" || b.File == "" {
		t.Fatalf("both copies should be there, told apart by serial: %v", got)
	}
	if a.BaseTitle != "Skylanders SWAP Force" || !a.listed || b.listed {
		t.Fatalf("copies: %+v %+v", a, b)
	}
	s := got["Skylanders SuperChargers"]
	if s.Platform != "PS3" || s.Serial != "BLES02172" || !strings.HasSuffix(s.File, `PS3_GAME\USRDIR\EBOOT.BIN`) || s.Backdrop == "" {
		t.Fatalf("superchargers: %+v", s)
	}
	if want := "--no-gui --fullscreen " + quote(s.File); s.Args != want {
		t.Fatalf("args %q, want %q", s.Args, want)
	}
	if got["Flower"].How != "RPCS3 installed games" {
		t.Fatalf("installed game: %+v", got["Flower"])
	}
	if _, ok := got["Update"]; ok {
		t.Fatal("game data isn't a game")
	}
}

func TestDolphinFoldersAndTitles(t *testing.T) {
	root := t.TempDir()
	emu := filepath.Join(root, "Emulators", "Dolphin-x64")
	lib := filepath.Join(root, "Games")
	ini := "[General]\nISOPaths = 1\nISOPath0 = " + filepath.ToSlash(lib) + "\nRecursiveISOPaths = True\n"
	write(t, emu, map[string][]byte{
		"Dolphin.exe":                      []byte("x"),
		"portable.txt":                     []byte(""),
		"User/Config/Dolphin.ini":          []byte(ini),
		"Sys/wiitdb-en.txt":                []byte("TITLES = https://www.gametdb.com\nGM4E01 = Mario Kart: Double Dash!!\n"),
		"User/Cache/GameCovers/GM4E01.png": []byte("png"),
	})
	wbfs := make([]byte, 0x400)
	copy(wbfs, "WBFS")
	wbfs[8] = 9
	copy(wbfs[0x200:], gcDisc("RMCE01", "MarioKartWii", true, 0x60)[:0x60])
	write(t, lib, map[string][]byte{
		"GameCube/Double Dash/Double Dash (USA).iso": gcDisc("GM4E01", "Mario Kart Double Dash!", false, 0),
		"Wii/RMCE01 Mario Kart Wii/RMCE01.wbfs":      wbfs,
		"Wii/Flat/Zelda Twilight Princess.iso":       gcDisc("RZDE01", "ZeldaTP", true, 0),
		"Wii/Flat/Other Game.iso":                    gcDisc("RXXE01", "Other Game Disc", true, 0),
		"PS2/Ratchet.iso":                            make([]byte, 0x400), // not a GameCube or Wii disc
		// A PC game's folder: its files aren't looked at.
		"Some PC Game/game.exe":      []byte("x"),
		"Some PC Game/extra.iso":     gcDisc("GXXE01", "Hidden", false, 0),
		"a/b/c/d/e/f/g/too deep.iso": gcDisc("GDPE01", "Deep", false, 0),
	})

	got := byTitle(findIn(t, filepath.Join(root, "Emulators")))
	dd := got["Mario Kart: Double Dash!!"]
	if dd.Platform != "GameCube" || dd.Serial != "GM4E01" || dd.TitleFrom != "Dolphin game database" || dd.Cover == "" {
		t.Fatalf("double dash: %+v (all: %v)", dd, got)
	}
	if want := "-b -C Main.Display.Fullscreen=True -e " + quote(dd.File); dd.Args != want {
		t.Fatalf("args %q", dd.Args)
	}
	if w := got["Mario Kart Wii"]; w.Platform != "Wii" || w.Serial != "RMCE01" || w.TitleFrom != "folder name" {
		t.Fatalf("wii (folder named after the game, file by its id): %+v", w)
	}
	// Two games in one folder are named by their files, or their disc's
	// own name when it reads like a title.
	if g := got["Zelda Twilight Princess"]; g.TitleFrom != "file name" {
		t.Fatalf("zelda: %+v", g)
	}
	if g := got["Other Game Disc"]; g.TitleFrom != "disc" {
		t.Fatalf("other: %+v", g)
	}
	for _, bad := range []string{"Ratchet", "Hidden", "Deep"} {
		if _, ok := got[bad]; ok {
			t.Fatalf("%s shouldn't be found: %v", bad, got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("want 4 games, got %v", got)
	}
}

func TestCemuListAndCopies(t *testing.T) {
	root := t.TempDir()
	emu := filepath.Join(root, "Emu", "Cemu")
	local := filepath.Join(root, "Games", "Wii U")
	elsewhere := filepath.Join(root, "Share", "Wii U")
	wud := bytes.Repeat([]byte{1}, 2<<20)
	write(t, local, map[string][]byte{"Mario Kart 8/fk-mk8.wud": wud})
	write(t, elsewhere, map[string][]byte{"Mario Kart 8/fk-mk8.wud": wud})
	cache := `<?xml version="1.0" encoding="UTF-8"?>
<title_list>
 <title titleId="000500001010ec00"><name>MARIO KART 8</name><path>` + filepath.ToSlash(filepath.Join(elsewhere, "Mario Kart 8", "fk-mk8.wud")) + `</path></title>
 <title titleId="0005000010000000"><name>Gone</name><path>C:/nowhere/gone.wud</path></title>
</title_list>`
	settings := `<?xml version="1.0" encoding="UTF-8"?><content><GamePaths><Entry>` + local + `</Entry></GamePaths></content>`
	write(t, emu, map[string][]byte{
		"Cemu.exe":                      []byte("x"),
		"portable/settings.xml":         []byte(settings),
		"portable/title_list_cache.xml": []byte(cache),
		// Installed to Cemu's storage.
		"mlc01/usr/title/00050000/10144f00/code/Zelda.rpx":   []byte("rpx"),
		"mlc01/usr/title/00050000/10144f00/meta/meta.xml":    []byte(`<menu><title_id type="hexBinary" length="8">0005000010144F00</title_id><longname_en type="string">The Legend of Zelda&#xA;Breath of the Wild</longname_en></menu>`),
		"mlc01/usr/title/00050000/10144f00/content/data.bin": []byte("x"),
	})

	games := findIn(t, filepath.Join(root, "Emu"))
	got := byTitle(games)
	if len(games) != 2 {
		t.Fatalf("the two copies of Mario Kart 8 are one game: %v", got)
	}
	mk := got["Mario Kart 8"]
	if mk.Serial != "000500001010EC00" || mk.TitleFrom != "Cemu game list" || mk.Platform != "Wii U" {
		t.Fatalf("mario kart 8: %+v", mk)
	}
	if want := "-f -g " + quote(mk.File); mk.Args != want {
		t.Fatalf("args %q", mk.Args)
	}
	z := got["The Legend of Zelda Breath of the Wild"]
	if z.Serial != "0005000010144F00" || !strings.HasSuffix(z.File, "Zelda.rpx") {
		t.Fatalf("zelda: %+v (all %v)", z, got)
	}
}

func TestCollapsePrefersLocalListedCopy(t *testing.T) {
	a := Game{Platform: "Wii U", File: `Z:\a\mk8.wud`, SizeBytes: 5, Serial: "X", Title: "Mario Kart 8", BaseTitle: "Mario Kart 8", TitleFrom: "Cemu game list", listed: true, remote: true}
	b := Game{Platform: "Wii U", File: `R:\Games\mk8.wud`, SizeBytes: 5, Title: "mk8", BaseTitle: "mk8"}
	got := collapse([]Game{a, b})
	if len(got) != 1 || got[0].File != b.File || got[0].Title != "Mario Kart 8" || got[0].Serial != "X" {
		t.Fatalf("got %+v", got)
	}
	// Same name and size on another console isn't the same game.
	c := b
	c.Platform = "Wii"
	if n := len(collapse([]Game{a, c})); n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"RMCE01 Mario Kart Wii":                 "Mario Kart Wii",
		"Skylanders Giants [BLES01689]":         "Skylanders Giants",
		"Mario Kart Double Dash (USA).nkit.iso": "Mario Kart Double Dash",
		"MARIO KART 8.wud":                      "Mario Kart 8",
		"Metroid Prime (Europe) (En,Fr,De).rvz": "Metroid Prime",
	} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDiscFormats(t *testing.T) {
	at := func(b []byte) func(int64) ([]byte, error) {
		return func(off int64) ([]byte, error) {
			if off+0x60 > int64(len(b)) {
				return nil, os.ErrNotExist
			}
			return b[off : off+0x60], nil
		}
	}
	rvz := make([]byte, 0x100)
	copy(rvz, "RVZ\x01")
	copy(rvz[0x58:], gcDisc("GALE01", "Super Smash Bros Melee", false, 0)[:0x80])
	gcz := make([]byte, 0x100)
	binary.LittleEndian.PutUint32(gcz, 0xB10BC001)
	binary.LittleEndian.PutUint32(gcz[4:], 1)
	wbfsBad := make([]byte, 0x100)
	copy(wbfsBad, "WBFS")
	wbfsBad[8] = 40 // a sector size no WBFS has
	for name, c := range map[string]struct {
		b    []byte
		want disc
		ok   bool
	}{
		"rvz":      {rvz, disc{id: "GALE01", title: "Super Smash Bros Melee", platform: "GameCube"}, true},
		"gcz":      {gcz, disc{platform: "Wii"}, true},
		"bad wbfs": {wbfsBad, disc{}, false},
		"zeros":    {make([]byte, 0x100), disc{}, false},
		"short":    {[]byte("WBFS"), disc{}, false},
	} {
		got, ok := parseDisc(c.b, at(c.b))
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v %v", name, got, ok, c.want, c.ok)
		}
	}
}

func TestParseSFORejectsBrokenTables(t *testing.T) {
	good := sfo(map[string]string{"CATEGORY": "DG", "TITLE": "Game", "TITLE_ID": "BLES00001"})
	if m, err := parseSFO(good); err != nil || m["TITLE"] != "Game" {
		t.Fatalf("good file: %v %v", m, err)
	}
	huge := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(huge[16:], 1<<30) // more entries than the file holds
	if _, err := parseSFO(huge); err == nil {
		t.Fatal("entry count past the end accepted")
	}
	off := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(off[20+12:], 1<<31) // value past the end
	if _, err := parseSFO(off); err == nil {
		t.Fatal("value past the end accepted")
	}
}

func TestOversizedFilesAreIgnored(t *testing.T) {
	root := t.TempDir()
	emu := filepath.Join(root, "Emulators")
	games := filepath.Join(root, "PS3")
	write(t, games, under("A", ps3Disc("BLES00001", "Game A", "DG")))
	line := "BLES00001: " + filepath.ToSlash(filepath.Join(games, "A")) + "\n"
	big := strings.Repeat("# padding\n", (1<<20)/10+1) + line
	write(t, emu, map[string][]byte{"RPCS3/rpcs3.exe": []byte("x"), "RPCS3/config/games.yml": []byte(big)})
	if got := findIn(t, emu); len(got) != 0 {
		t.Fatalf("a games.yml over 1 MB is not read: %v", got)
	}
}

func TestPathFrom(t *testing.T) {
	for _, c := range []struct{ base, in, want string }{
		{`C:\Emu`, "R:/Games/", `R:\Games`},
		{`C:\Emu`, "games", `C:\Emu\games`},
		{`C:\Emu`, `\\?\C:\x`, ""},
		{"", "relative", ""},
		{`C:\Emu`, "a\x00b", ""},
		{`C:\Emu`, `\rooted`, ""},
	} {
		if got := pathFrom(c.base, c.in); got != c.want {
			t.Errorf("pathFrom(%q, %q) = %q, want %q", c.base, c.in, got, c.want)
		}
	}
}

func TestWalkStopsAtItsBounds(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string][]byte{"a/x.iso": gcDisc("GM4E01", "Mario Kart", false, 0)})
	in := &install{emu: dolphin, dir: dir, exe: filepath.Join(dir, "Dolphin.exe")}
	w := &walker{ctx: context.Background(), ins: []*install{in}, deadline: time.Now().Add(-time.Second), seen: map[string]bool{}}
	w.walk(root{path: dir, depth: maxDepth})
	if w.stopped == "" || len(w.found) != 0 {
		t.Fatalf("a walk past its deadline should stop: %q %v", w.stopped, w.found)
	}
}
