// Package sqlite reads tables from an SQLite database file, read-only and
// without cgo: enough to list GOG Galaxy's library. It follows the file
// format (https://www.sqlite.org/fileformat2.html) for table b-trees,
// overflow pages, records and the write-ahead log. Indexes, WITHOUT ROWID
// tables and UTF-16 databases aren't supported.
package sqlite

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

// MaxFile caps the size of a database (and its WAL) that is read.
const MaxFile = 512 << 20

// DB is an open database. Pages are read from the file as they're needed,
// so a large database costs little memory; the write-ahead log (small) is
// read whole.
type DB struct {
	r        io.ReaderAt
	size     int64     // bytes in the database file
	closer   io.Closer // the file, for Close
	pageSize int
	usable   int
	wal      map[uint32][]byte // page → newest committed copy in the WAL
	tables   map[string]table
	visits   int // pages left to visit in this walk (a looping b-tree can't run away)
}

type table struct {
	root  uint32
	cols  []string
	rowid int // index of the INTEGER PRIMARY KEY column, -1 if none
}

// Open opens a database file (and reads its -wal file, when there is one).
// Close it when done.
func Open(path string) (*DB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if fi.Size() > MaxFile {
		f.Close()
		return nil, errors.New("database too large")
	}
	db, err := newDB(f, fi.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	db.closer = f
	if wal, err := readCapped(path + "-wal"); err == nil && len(wal) > 32 {
		db.readWAL(wal)
	}
	if err := db.loadSchema(); err != nil {
		f.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the database file.
func (db *DB) Close() error {
	if db.closer == nil {
		return nil
	}
	return db.closer.Close()
}

// pages is how many pages the database file holds.
func (db *DB) pages() int { return int(db.size / int64(db.pageSize)) }

func readCapped(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > MaxFile {
		return nil, errors.New("database too large")
	}
	return os.ReadFile(path)
}

// parse opens a database held in memory.
func parse(data []byte) (*DB, error) { return newDB(bytes.NewReader(data), int64(len(data))) }

func newDB(r io.ReaderAt, size int64) (*DB, error) {
	data := make([]byte, 100)
	if size < 100 {
		return nil, errors.New("not an SQLite database")
	}
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		return nil, errors.New("not an SQLite database")
	}
	ps := int(binary.BigEndian.Uint16(data[16:]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps&(ps-1) != 0 {
		return nil, errors.New("bad page size")
	}
	if enc := binary.BigEndian.Uint32(data[56:]); enc != 0 && enc != 1 {
		return nil, errors.New("only UTF-8 databases are supported")
	}
	return &DB{r: r, size: size, pageSize: ps, usable: ps - int(data[20]), tables: map[string]table{}}, nil
}

// readWAL keeps, for each page, its copy from the last committed
// transaction in the write-ahead log (with salts matching the header).
func (db *DB) readWAL(w []byte) {
	magic := binary.BigEndian.Uint32(w)
	if magic != 0x377f0682 && magic != 0x377f0683 {
		return
	}
	if int(binary.BigEndian.Uint32(w[8:])) != db.pageSize {
		return
	}
	s1, s2 := binary.BigEndian.Uint32(w[16:]), binary.BigEndian.Uint32(w[20:])
	pending := map[uint32][]byte{}
	committed := map[uint32][]byte{}
	for off := 32; off+24+db.pageSize <= len(w); off += 24 + db.pageSize {
		h := w[off : off+24]
		if binary.BigEndian.Uint32(h[8:]) != s1 || binary.BigEndian.Uint32(h[12:]) != s2 {
			break
		}
		pending[binary.BigEndian.Uint32(h)] = w[off+24 : off+24+db.pageSize]
		if binary.BigEndian.Uint32(h[4:]) != 0 { // a commit frame
			for p, d := range pending {
				committed[p] = d
			}
			pending = map[uint32][]byte{}
		}
	}
	if len(committed) > 0 {
		db.wal = committed
	}
}

func (db *DB) page(n uint32) ([]byte, error) {
	if n == 0 {
		return nil, errors.New("page 0")
	}
	if p, ok := db.wal[n]; ok {
		return p, nil
	}
	off := int64(n-1) * int64(db.pageSize)
	if off+int64(db.pageSize) > db.size {
		return nil, fmt.Errorf("page %d past the end", n)
	}
	p := make([]byte, db.pageSize) // callers keep pages while reading others
	if _, err := db.r.ReadAt(p, off); err != nil {
		return nil, err
	}
	return p, nil
}

// Rows returns every row of a table as column name → value (int64,
// float64, string, []byte or nil).
func (db *DB) Rows(name string) ([]map[string]any, error) {
	t, ok := db.tables[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("no table %s", name)
	}
	var out []map[string]any
	db.visits = db.pages() + len(db.wal) + 1
	err := db.walk(t.root, 0, func(rowid int64, rec []any) {
		row := make(map[string]any, len(t.cols))
		for i, c := range t.cols {
			var v any
			if i < len(rec) {
				v = rec[i]
			}
			if i == t.rowid {
				v = rowid
			}
			row[c] = v
		}
		out = append(out, row)
	})
	return out, err
}

// HasTable reports whether the database has a table.
func (db *DB) HasTable(name string) bool {
	_, ok := db.tables[strings.ToLower(name)]
	return ok
}

func (db *DB) loadSchema() error {
	db.visits = db.pages() + len(db.wal) + 1
	return db.walk(1, 0, func(_ int64, rec []any) {
		if len(rec) < 5 || rec[0] != "table" {
			return
		}
		name, _ := rec[1].(string)
		root, _ := rec[3].(int64)
		sql, _ := rec[4].(string)
		if name == "" || root <= 0 || strings.Contains(strings.ToUpper(sql), "WITHOUT ROWID") {
			return
		}
		cols, rowid := columns(sql)
		db.tables[strings.ToLower(name)] = table{root: uint32(root), cols: cols, rowid: rowid}
	})
}

// walk visits the rows of the table b-tree rooted at page n.
func (db *DB) walk(n uint32, depth int, fn func(int64, []any)) error {
	if depth > 40 {
		return errors.New("b-tree too deep")
	}
	if db.visits--; db.visits < 0 {
		return errors.New("b-tree loops")
	}
	p, err := db.page(n)
	if err != nil {
		return err
	}
	h := 0
	if n == 1 {
		h = 100
	}
	if len(p) < h+12 {
		return errors.New("short page")
	}
	kind := p[h]
	cells := int(binary.BigEndian.Uint16(p[h+3:]))
	switch kind {
	case 0x05: // interior table page
		ptrs := p[h+12:]
		if len(ptrs) < cells*2 {
			return errors.New("bad cell pointers")
		}
		for i := 0; i < cells; i++ {
			off := int(binary.BigEndian.Uint16(ptrs[i*2:]))
			if off+4 > len(p) {
				return errors.New("bad cell")
			}
			if err := db.walk(binary.BigEndian.Uint32(p[off:]), depth+1, fn); err != nil {
				return err
			}
		}
		return db.walk(binary.BigEndian.Uint32(p[h+8:]), depth+1, fn)
	case 0x0d: // leaf table page
		ptrs := p[h+8:]
		if len(ptrs) < cells*2 {
			return errors.New("bad cell pointers")
		}
		for i := 0; i < cells; i++ {
			off := int(binary.BigEndian.Uint16(ptrs[i*2:]))
			if off >= len(p) {
				return errors.New("bad cell")
			}
			size, k := varint(p[off:])
			if size > MaxFile {
				return errors.New("bad payload size")
			}
			rowid, k2 := varint(p[off+k:])
			if k == 0 || k2 == 0 {
				return errors.New("bad cell header")
			}
			payload, err := db.payload(p, off+k+k2, int(size))
			if err != nil {
				return err
			}
			rec, err := record(payload)
			if err != nil {
				return err
			}
			fn(int64(rowid), rec)
		}
		return nil
	}
	return fmt.Errorf("page %d isn't a table page", n)
}

// payload collects a cell's payload, following overflow pages.
func (db *DB) payload(p []byte, off, size int) ([]byte, error) {
	if size < 0 || size > MaxFile {
		return nil, errors.New("bad payload size")
	}
	u := db.usable
	x := u - 35
	local := size
	if size > x {
		m := (u-12)*32/255 - 23
		k := m + (size-m)%(u-4)
		if k <= x {
			local = k
		} else {
			local = m
		}
	}
	if off+local > len(p) {
		return nil, errors.New("payload past the page")
	}
	// Every overflow page holds u-4 bytes, so a payload can't be larger
	// than the pages there are: a damaged size can't reserve more memory
	// than the file has.
	if size-local > (db.pages()+len(db.wal))*(u-4) {
		return nil, errors.New("bad payload size")
	}
	out := make([]byte, 0, size)
	out = append(out, p[off:off+local]...)
	if local == size {
		return out, nil
	}
	if off+local+4 > len(p) {
		return nil, errors.New("missing overflow pointer")
	}
	next := binary.BigEndian.Uint32(p[off+local:])
	for hops := 0; len(out) < size; hops++ {
		if next == 0 || hops > db.pages()+len(db.wal)+1 {
			return nil, errors.New("broken overflow chain")
		}
		op, err := db.page(next)
		if err != nil {
			return nil, err
		}
		n := min(size-len(out), u-4)
		out = append(out, op[4:4+n]...)
		next = binary.BigEndian.Uint32(op)
	}
	return out, nil
}

// record decodes a record: a header of serial types, then the values.
func record(b []byte) ([]any, error) {
	hsize, k := varint(b)
	if k == 0 || hsize > uint64(len(b)) || hsize < uint64(k) {
		return nil, errors.New("bad record header")
	}
	var types []uint64
	for i := k; i < int(hsize); {
		t, n := varint(b[i:])
		if n == 0 {
			return nil, errors.New("bad serial type")
		}
		types = append(types, t)
		i += n
	}
	body := b[hsize:]
	out := make([]any, 0, len(types))
	for _, t := range types {
		var n int
		switch {
		case t == 0, t == 8, t == 9:
			n = 0
		case t >= 1 && t <= 4:
			n = int(t)
		case t == 5:
			n = 6
		case t == 6, t == 7:
			n = 8
		case t >= 12:
			if (t-12)/2 > uint64(len(body)) {
				return nil, errors.New("record past its end")
			}
			n = int((t - 12) / 2)
		default:
			return nil, errors.New("reserved serial type")
		}
		if n > len(body) {
			return nil, errors.New("record past its end")
		}
		v := body[:n]
		body = body[n:]
		switch {
		case t == 0:
			out = append(out, nil)
		case t == 8:
			out = append(out, int64(0))
		case t == 9:
			out = append(out, int64(1))
		case t <= 6:
			var x int64
			for _, c := range v {
				x = x<<8 | int64(c)
			}
			shift := 64 - 8*uint(n)
			out = append(out, x<<shift>>shift) // sign-extend
		case t == 7:
			out = append(out, math.Float64frombits(binary.BigEndian.Uint64(v)))
		case t%2 == 0:
			out = append(out, append([]byte(nil), v...))
		default:
			out = append(out, string(v))
		}
	}
	return out, nil
}

// varint reads an SQLite varint; n is 0 when b is too short.
func varint(b []byte) (v uint64, n int) {
	for i := 0; i < 9; i++ {
		if i >= len(b) {
			return 0, 0
		}
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return v, 9
}

// columns lists a CREATE TABLE statement's column names and which one is
// an alias for the rowid (INTEGER PRIMARY KEY), -1 if none.
func columns(sql string) ([]string, int) {
	open := strings.Index(sql, "(")
	close_ := strings.LastIndex(sql, ")")
	if open < 0 || close_ <= open {
		return nil, -1
	}
	var parts []string
	depth, start := 0, open+1
	for i := open + 1; i < close_; i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, sql[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, sql[start:close_])
	var cols []string
	rowid := -1
	for _, p := range parts {
		p = strings.TrimSpace(p)
		up := strings.ToUpper(p)
		if p == "" || strings.HasPrefix(up, "PRIMARY KEY") || strings.HasPrefix(up, "UNIQUE") ||
			strings.HasPrefix(up, "FOREIGN KEY") || strings.HasPrefix(up, "CONSTRAINT") || strings.HasPrefix(up, "CHECK") {
			continue
		}
		name, rest := p, ""
		if q := strings.IndexAny(p[:1], "\"`["); q == 0 {
			end := map[byte]byte{'"': '"', '`': '`', '[': ']'}[p[0]]
			if j := strings.IndexByte(p[1:], end); j >= 0 {
				name, rest = p[1:1+j], p[2+j:]
			}
		} else if i := strings.IndexAny(p, " \t\r\n"); i >= 0 {
			name, rest = p[:i], p[i:]
		}
		fields := strings.Fields(strings.ToUpper(rest))
		if len(fields) >= 3 && fields[0] == "INTEGER" && fields[1] == "PRIMARY" && fields[2] == "KEY" {
			rowid = len(cols)
		}
		cols = append(cols, name)
	}
	return cols, rowid
}
