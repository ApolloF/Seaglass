package achievements

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Ubisoft games on a Uplay emulator: Goldberg's (and the builds based on
// it, VOICES38's among them) read upc_r2.ini or uplay_r*.ini next to the
// loader DLL and keep their files under a folder per Ubisoft app id,
// %APPDATA%\Goldberg UplayEmu Saves\<id> by default. Builds that save
// achievements write achievements.json there, Goldberg-style
// ({"<prefix><id>": {"earned": 1, "earned_time": …}}), and may ship an
// achievements_schema.json with the names next to the DLL.
//
// The ini doesn't always name the app id, and the ids are Ubisoft's, so
// the right folder is found by what's known: the id when the ini gives it,
// else the key prefix or the schema's keys, else the only folder there is
// when only one game in the library runs on a Uplay emulator.

// UplayEmulator reports whether emulator is one of the Uplay emulators the scan names.
func UplayEmulator(emulator string) bool {
	return emulator == "Uplay emulator" || emulator == "VOICES38"
}

var uplayINIs = []string{"upc_r2.ini", "uplay_r2.ini", "uplay_r1.ini"}

// uplayConfig is what the emulator's ini says.
type uplayConfig struct {
	ini    string
	gameID string   // the Ubisoft app id, when set
	prefix string   // AchKeyPrefix: what achievement keys start with
	bases  []string // folders holding a folder per app id
	achDir string   // AchSavePath: where achievements.json is kept, when set
}

func readUplayConfig(g EmuGame, env Env) uplayConfig {
	emu := g.emuPath()
	c := uplayConfig{}
	kv := map[string]string{}
	for _, name := range uplayINIs {
		p := filepath.Join(emu, name)
		b, err := readSmall(p, 256<<10)
		if err != nil {
			continue
		}
		c.ini = p
		for _, s := range parseINI(b) {
			if s.name == "" || strings.EqualFold(s.name, "settings") {
				for k, v := range s.kv {
					kv[k] = strings.TrimSpace(v)
				}
			}
		}
		break
	}
	if id := kv["gameid"]; isDigits(id) {
		c.gameID = id
	}
	c.prefix = kv["achkeyprefix"]
	// A path from the ini: relative ones are next to the DLL.
	local := func(v string) string {
		if v == "" {
			return ""
		}
		if !filepath.IsAbs(v) {
			v = filepath.Join(emu, v)
		}
		return filepath.Clean(v)
	}
	switch kv["savetype"] {
	case "1":
		sp := kv["savepath"]
		if sp == "" {
			sp = "SAVE_GAMES"
		}
		c.bases = append(c.bases, local(sp))
	case "2":
		if p := local(kv["savepath"]); p != "" {
			c.bases = append(c.bases, p)
		}
	}
	if env.Roaming != "" {
		c.bases = append(c.bases, filepath.Join(env.Roaming, "Goldberg UplayEmu Saves"), filepath.Join(env.Roaming, "R1 UplayEmu Saves"))
	}
	if kv["achsavetype"] == "1" {
		c.achDir = local(kv["achsavepath"])
	}
	return c
}

func isDigits(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// uplayFiles lists every achievements.json the game's could be, and the
// folders they're found in (for cache stamps: a new app id folder changes
// its parent's time).
func uplayFiles(c uplayConfig) (files, dirs []string) {
	add := func(base string) {
		dirs = append(dirs, base)
		if c.gameID != "" {
			files = append(files, filepath.Join(base, c.gameID, "achievements.json"))
			return
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return
		}
		n := 0
		for _, e := range entries {
			if !e.IsDir() || !isDigits(e.Name()) {
				continue
			}
			if n++; n > 256 {
				break
			}
			files = append(files, filepath.Join(base, e.Name(), "achievements.json"))
		}
	}
	if c.achDir != "" {
		files = append(files, filepath.Join(c.achDir, "achievements.json"))
		add(c.achDir)
	}
	for _, b := range c.bases {
		add(b)
	}
	return files, dirs
}

// UplayFiles lists the files and folders a Uplay game's achievements are read from.
func UplayFiles(g EmuGame, env Env) []string {
	c := readUplayConfig(g, env)
	files, dirs := uplayFiles(c)
	files = append(files, dirs...)
	files = append(files, filepath.Join(g.emuPath(), "achievements_schema.json"))
	for _, name := range uplayINIs {
		files = append(files, filepath.Join(g.emuPath(), name))
	}
	return files
}

// UplaySchema reads achievements_schema.json next to the emulator:
// {"<key>": {"displayName": …, "description": …, "earned": 0}}.
func UplaySchema(g EmuGame) ([]Def, string) {
	p := filepath.Join(g.emuPath(), "achievements_schema.json")
	b, err := readSmall(p, maxFile)
	if err != nil {
		return nil, ""
	}
	defs, err := parseUplaySchema(b)
	if err != nil {
		return nil, ""
	}
	return defs, p
}

func parseUplaySchema(b []byte) ([]Def, error) {
	var m map[string]struct {
		DisplayName string          `json:"displayName"`
		Description string          `json:"description"`
		Hidden      json.RawMessage `json:"hidden"`
	}
	if err := json.Unmarshal([]byte(text(b)), &m); err != nil {
		return nil, fmt.Errorf("%w: %v", errFormat, err)
	}
	out := make([]Def, 0, len(m))
	for k, v := range m {
		if k == "" {
			continue
		}
		out = append(out, Def{ID: k, Name: v.DisplayName, Desc: v.Description, Hidden: jsonBool(v.Hidden)})
	}
	sortDefsByNumber(out)
	return out, nil
}

var reTrailingNum = regexp.MustCompile(`(\d+)$`)

// trailingNum is the number an achievement key ends in ("AFOP_Ach_12" →
// 12): Ubisoft's achievement id, which the Steam version's API names end
// in too. -1 when there's none.
func trailingNum(s string) int {
	m := reTrailingNum.FindString(s)
	if m == "" || len(m) > 9 {
		return -1
	}
	n, _ := strconv.Atoi(m)
	return n
}

func sortDefsByNumber(d []Def) {
	for i := 1; i < len(d); i++ { // small lists: insertion sort, by number then key
		for j := i; j > 0 && defLess(d[j], d[j-1]); j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

func defLess(a, b Def) bool {
	na, nb := trailingNum(a.ID), trailingNum(b.ID)
	if na != nb {
		return na < nb
	}
	return a.ID < b.ID
}

// ReadUplay finds and reads the game's achievements.json. known are keys
// the game's achievements have (from a schema), to tell its folder from
// other games'; soleGame says this is the only game on a Uplay emulator,
// so a lone folder must be its. files lists what was looked at; seen
// counts the unlock files found, the game's or not.
func ReadUplay(g EmuGame, env Env, known []string, soleGame bool) (res EmuResult, files []string, seen int, ok bool) {
	c := readUplayConfig(g, env)
	cands, dirs := uplayFiles(c)
	files = append(append(files, cands...), dirs...)
	if c.ini != "" {
		files = append(files, c.ini)
	}
	knownSet := map[string]bool{}
	for _, k := range known {
		knownSet[strings.ToLower(k)] = true
	}
	type found struct {
		path    string
		unlocks map[string]Unlock
		score   int
		exact   bool
	}
	var all []found
	for _, p := range cands {
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxFile {
			continue
		}
		b, err := readSmall(p, maxFile)
		if err != nil {
			continue
		}
		u, err := safeParse(parseGoldbergJSON, b)
		if err != nil {
			continue
		}
		f := found{path: p, unlocks: u}
		f.exact = c.gameID != "" || (c.achDir != "" && filepath.Dir(p) == c.achDir)
		for k := range u {
			if c.prefix != "" && strings.HasPrefix(strings.ToLower(k), strings.ToLower(c.prefix)) {
				f.score++
			} else if knownSet[strings.ToLower(k)] {
				f.score++
			}
		}
		all = append(all, f)
	}
	pick := -1
	for i, f := range all {
		if f.exact {
			pick = i
			break
		}
	}
	if pick < 0 {
		best, tie := 0, false
		for i, f := range all {
			switch {
			case f.score > best:
				pick, best, tie = i, f.score, false
			case f.score == best && best > 0:
				tie = true
			}
		}
		if tie {
			pick = -1
		}
	}
	if pick < 0 && len(all) == 1 && soleGame {
		pick = 0
	}
	if pick < 0 {
		return EmuResult{}, files, len(all), false
	}
	return EmuResult{Source: "Uplay emulator", File: all[pick].path, Unlocks: all[pick].unlocks}, files, len(all), true
}

// matchByNumber renames Uplay unlock keys to the schema's IDs that end in
// the same number ("AFOP_Ach_12" and Steam's "ACH_12"), when that's
// unambiguous. Keys already in the schema stay.
func matchByNumber(defs []Def, unlocks map[string]Unlock) map[string]Unlock {
	ids := map[string]bool{}
	byNum := map[int]string{}
	dup := map[int]bool{}
	for _, d := range defs {
		ids[strings.ToLower(d.ID)] = true
		n := trailingNum(d.ID)
		if n < 0 {
			continue
		}
		if _, ok := byNum[n]; ok {
			dup[n] = true
		}
		byNum[n] = d.ID
	}
	out := make(map[string]Unlock, len(unlocks))
	for k, u := range unlocks {
		if ids[strings.ToLower(k)] {
			out[k] = u
			continue
		}
		if n := trailingNum(k); n >= 0 && !dup[n] && byNum[n] != "" {
			k = byNum[n]
		}
		if old, ok := out[k]; ok && old.Achieved && !u.Achieved {
			continue
		}
		out[k] = u
	}
	return out
}
