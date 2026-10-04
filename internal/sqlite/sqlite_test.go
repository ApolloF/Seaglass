package sqlite

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
)

// The fixtures come from Python's sqlite3 (see the commit that added them):
// types.db has 512-byte pages, 1,201 rows (interior pages), overflowing
// text and every value type; wal.db has changes only in its -wal file.

func TestReadTable(t *testing.T) {
	db, err := Open("testdata/types.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Rows("T")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1201 {
		t.Fatalf("%d rows, want 1201", len(rows))
	}
	byID := map[int64]map[string]any{}
	for _, r := range rows {
		byID[r["id"].(int64)] = r
	}
	r := byID[7]
	if r["name"] != "row 7" || r["n"] != int64(-7000021) || r["f"] != 1.75 || !bytes.Equal(r["b"].([]byte), []byte{7, 7, 7}) || r["big"] != nil {
		t.Errorf("row 7 = %v", r)
	}
	if big := byID[1150]["big"].(string); big != strings.Repeat("x", 1150%7*300) {
		t.Errorf("overflowing text has %d characters", len(big))
	}
	e := byID[5000]
	if e["n"] != int64(math.MinInt64) || e["f"] != 1e300 || len(e["b"].([]byte)) != 0 || e["big"] != "é漢字" {
		t.Errorf("extremes = %v", e)
	}
	if !db.HasTable("t") || db.HasTable("sqlite_sequence_nope") {
		t.Error("HasTable")
	}
	if _, err := db.Rows("missing"); err == nil {
		t.Error("a missing table must fail")
	}
}

func TestWAL(t *testing.T) {
	db, err := Open("testdata/wal.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Rows("k")
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]any{}
	for _, r := range rows {
		got[r["id"].(int64)] = r["v"]
	}
	if len(got) != 2 || got[1] != "new" || got[2] != "added" {
		t.Errorf("rows = %v, want the WAL's version", got)
	}
}

func TestColumns(t *testing.T) {
	cols, rowid := columns(`CREATE TABLE x ("a b" TEXT, [c] INT, d INTEGER PRIMARY KEY AUTOINCREMENT, e, PRIMARY KEY(a), FOREIGN KEY (c) REFERENCES y(z))`)
	if strings.Join(cols, "|") != "a b|c|d|e" || rowid != 2 {
		t.Errorf("columns = %q, rowid %d", cols, rowid)
	}
}

func TestGarbage(t *testing.T) {
	good, _ := os.ReadFile("testdata/types.db")
	if _, err := parse([]byte("not a database at all, clearly not one, no")); err == nil {
		t.Error("garbage must fail")
	}
	// Truncated or scribbled files must fail cleanly, never panic.
	for _, n := range []int{100, 600, 5000, len(good) / 2} {
		dir := t.TempDir()
		p := dir + "/x.db"
		os.WriteFile(p, good[:n], 0o644)
		if db, err := Open(p); err == nil {
			_, _ = db.Rows("t")
			db.Close()
		}
	}
}

func FuzzRecord(f *testing.F) {
	f.Add([]byte{3, 1, 13, 42, 'h'})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = record(b) })
}

func FuzzDB(f *testing.F) {
	good, _ := os.ReadFile("testdata/types.db")
	f.Add(good[:4096])
	f.Fuzz(func(t *testing.T, b []byte) {
		db, err := parse(b)
		if err != nil {
			return
		}
		if db.loadSchema() == nil {
			for name := range db.tables {
				_, _ = db.Rows(name)
			}
		}
	})
}

// sqliteVarint encodes v as SQLite's big-endian varint (v < 2^56).
func sqliteVarint(v uint64) []byte {
	b := []byte{byte(v & 0x7f)}
	for v >>= 7; v > 0; v >>= 7 {
		b = append([]byte{byte(v&0x7f) | 0x80}, b...)
	}
	return b
}

// A tiny damaged file whose one cell claims a huge payload must not make
// the reader reserve that much memory before it finds the chain is broken.
func TestHugePayloadSizeInTinyFile(t *testing.T) {
	db := make([]byte, 512)
	copy(db, "SQLite format 3\x00")
	binary.BigEndian.PutUint16(db[16:], 512)
	db[100] = 0x0d                            // leaf table page
	binary.BigEndian.PutUint16(db[103:], 1)   // one cell
	binary.BigEndian.PutUint16(db[108:], 200) // at 200
	cell := append(sqliteVarint(500<<20), sqliteVarint(1)...)
	copy(db[200:], cell)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	d, err := parse(db)
	if err != nil {
		t.Fatal(err)
	}
	if d.loadSchema() == nil {
		t.Error("a cell claiming 500 MB in a 512-byte file was accepted")
	}
	runtime.ReadMemStats(&after)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<20 {
		t.Fatalf("allocated %d MB reading a 512-byte file", grew>>20)
	}
}
