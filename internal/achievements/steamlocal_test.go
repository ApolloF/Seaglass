package achievements

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ApolloF/gamekit/vdf"
)

func bm(key string, kids ...*vdf.BNode) *vdf.BNode {
	return &vdf.BNode{Key: key, Type: vdf.BMap, Kids: kids}
}
func bs(key, v string) *vdf.BNode { return &vdf.BNode{Key: key, Type: vdf.BString, Str: v} }
func bi(key string, v uint32) *vdf.BNode {
	return &vdf.BNode{Key: key, Type: vdf.BInt32, Int: v}
}

func bin(t testing.TB, root *vdf.BNode) []byte {
	t.Helper()
	b, err := vdf.MarshalBinary(root)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// schemaBin is a schema like Steam writes: one achievement stat with two
// bits (names in two languages, one hidden) and an ordinary stat.
func schemaBin(t testing.TB) []byte {
	ach := func(n uint32, id, en, de, hidden string) *vdf.BNode {
		return bm(string(rune('0'+n)), bs("name", id), bi("bit", n),
			bm("display",
				bm("name", bs("english", en), bs("german", de), bs("token", "NEW_ACHIEVEMENT_1_"+id)),
				bm("desc", bs("english", en+" desc")),
				bs("hidden", hidden), bs("icon", id+".jpg"), bs("icon_gray", id+"_g.jpg")))
	}
	return bin(t, bm("", bm("620",
		bs("gamename", "Portal 2"), bi("version", 3),
		bm("stats",
			bm("1", bs("type", "4"), bs("id", "1"), bm("bits", ach(0, "ACH_A", "Alpha", "Alfa", "0"), ach(1, "ACH_B", "Beta", "", "1"))),
			bm("2", bi("type", 1), bs("name", "kills")),
		))))
}

func TestSteamSchema(t *testing.T) {
	s, err := parseSteamSchema(schemaBin(t), 620, "german")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.defs) != 2 {
		t.Fatalf("defs: %+v", s.defs)
	}
	a, b := s.defs[0], s.defs[1]
	if a.ID != "ACH_A" || a.Name != "Alfa" || a.Desc != "Alpha desc" || a.Hidden || a.Icon != SteamIconBase+"620/ACH_A.jpg" || a.IconGray != SteamIconBase+"620/ACH_A_g.jpg" {
		t.Errorf("a: %+v", a)
	}
	if b.Name != "Beta" || !b.Hidden {
		t.Errorf("b falls back to English and is hidden: %+v", b)
	}
	if s.bits["ACH_B"] != (steamBit{"1", 1}) {
		t.Errorf("bits: %+v", s.bits)
	}
}

func TestSteamLocal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "appcache", "stats")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SteamSchemaFile(root, 620), schemaBin(t), 0o644); err != nil {
		t.Fatal(err)
	}
	stats := bin(t, bm("", bm("cache", bi("crc", 1), bi("PendingChanges", 0),
		bm("1", bi("data", 0b10), bm("AchievementTimes", bi("1", 1_700_000_000))))))
	if err := os.WriteFile(SteamStatsFile(root, "22", 620), stats, 0o644); err != nil {
		t.Fatal(err)
	}
	defs, unlocks, files, err := SteamLocal(root, []string{"11", "22"}, 620, "english")
	if err != nil || len(defs) != 2 || len(files) != 3 {
		t.Fatalf("defs %d files %v err %v", len(defs), files, err)
	}
	if unlocks["ACH_A"].Achieved || !unlocks["ACH_B"].Achieved || unlocks["ACH_B"].At != 1_700_000_000 {
		t.Errorf("unlocks: %+v", unlocks)
	}
}

func FuzzSteamSchema(f *testing.F) {
	f.Add(schemaBin(f))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseSteamSchema(b, 620, "english") })
}

func FuzzSteamStats(f *testing.F) {
	f.Add(bin(f, bm("", bm("cache", bm("1", bi("data", 3), bm("AchievementTimes", bi("0", 5)))))))
	bits := map[string]steamBit{"A": {"1", 0}, "B": {"1", 1}, "C": {"9", 31}}
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseSteamStats(b, bits) })
}
