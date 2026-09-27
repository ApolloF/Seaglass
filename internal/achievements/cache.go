package achievements

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Cache keeps results per game and schemas fetched from the stores.
//
//	<Dir>\games\<gameID>.json            the last result and what it was made from
//	<Dir>\schema\<source>-<id>-<lang>.json  a store's schema, kept SchemaTTL
type Cache struct {
	Dir string
}

// SchemaTTL is how long a downloaded schema (or rarity) is used before it's fetched again.
const SchemaTTL = 30 * 24 * time.Hour

// RarityTTL is how long global unlock percentages are kept.
const RarityTTL = 7 * 24 * time.Hour

// Entry is a cached result.
type Entry struct {
	Stamp string `json:"stamp"` // what the result was made from; see Stamp
	Net   bool   `json:"net"`   // part of it came from a store's servers
	List  List   `json:"list"`
}

func writeJSON(p string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

func (c *Cache) gamePath(id int64) string {
	return filepath.Join(c.Dir, "games", strconv.FormatInt(id, 10)+".json")
}

// Load returns a game's cached result.
func (c *Cache) Load(id int64) (Entry, bool) {
	var e Entry
	if c == nil || c.Dir == "" {
		return e, false
	}
	b, err := readSmall(c.gamePath(id), 32<<20)
	if err != nil || json.Unmarshal(b, &e) != nil {
		return e, false
	}
	return e, true
}

// Save stores a game's result.
func (c *Cache) Save(id int64, e Entry) error {
	if c == nil || c.Dir == "" {
		return nil
	}
	return writeJSON(c.gamePath(id), e)
}

// Clear forgets every cached game result (schemas stay).
func (c *Cache) Clear() {
	if c != nil && c.Dir != "" {
		_ = os.RemoveAll(filepath.Join(c.Dir, "games"))
	}
}

var reSafe = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

func (c *Cache) schemaPath(source, id, lang string) string {
	name := reSafe.ReplaceAllString(source+"-"+id+"-"+lang, "_")
	return filepath.Join(c.Dir, "schema", name+".json")
}

type schemaFile struct {
	At     int64              `json:"at"`
	Defs   []Def              `json:"defs,omitempty"`
	Rarity map[string]float64 `json:"rarity,omitempty"`
	None   bool               `json:"none,omitempty"` // the store said the game has none
}

// schema returns a cached schema younger than ttl.
func (c *Cache) schema(source, id, lang string, ttl time.Duration) (schemaFile, bool) {
	var s schemaFile
	if c == nil || c.Dir == "" {
		return s, false
	}
	b, err := readSmall(c.schemaPath(source, id, lang), 32<<20)
	if err != nil || json.Unmarshal(b, &s) != nil || time.Since(time.Unix(s.At, 0)) > ttl {
		return s, false
	}
	return s, true
}

func (c *Cache) putSchema(source, id, lang string, s schemaFile) {
	if c == nil || c.Dir == "" {
		return
	}
	s.At = time.Now().Unix()
	_ = writeJSON(c.schemaPath(source, id, lang), s)
}

// Stamp sums up what a result depends on: the size and time of each file
// that exists (so a result is reused while nothing changed), and extra
// words (the language, whether a key is set, …).
func Stamp(files []string, extra ...string) string {
	h := sha256.New()
	fs := append([]string(nil), files...)
	sort.Strings(fs)
	for _, f := range fs {
		if fi, err := os.Stat(f); err == nil {
			fmt.Fprintf(h, "%s|%d|%d\n", f, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	for _, x := range extra {
		fmt.Fprintf(h, "x|%s\n", x)
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}
