package achievements

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// maxFile caps every file this package reads; unlock files are a few KB.
const maxFile = 4 << 20

// readSmall reads a file of at most limit bytes.
func readSmall(p string, limit int64) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file too large")
	}
	return b, nil
}

// A parser turns an unlock file into unlocks by achievement ID.
type parser func([]byte) (map[string]Unlock, error)

var errFormat = errors.New("unknown unlock file format")

// text decodes a text file: UTF-8 (with or without BOM) or UTF-16LE with a BOM.
func text(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xff, 0xfe}):
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
		return string(utf16.Decode(u))
	}
	return string(b)
}

// iniSection is one [section] of an INI file, keys lower-cased.
type iniSection struct {
	name string
	kv   map[string]string
}

// parseINI reads sections in file order. Keys before the first section
// land in a section named "".
func parseINI(b []byte) []iniSection {
	var out []iniSection
	cur := -1
	sc := bufio.NewScanner(strings.NewReader(text(b)))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			out = append(out, iniSection{name: name, kv: map[string]string{}})
			cur = len(out) - 1
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if cur < 0 {
			out = append(out, iniSection{kv: map[string]string{}})
			cur = 0
		}
		out[cur].kv[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func num(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// unixTime cleans up an unlock time: 0 for nonsense, seconds for milliseconds.
func unixTime(v float64) int64 {
	switch {
	case v <= 0 || v > 1e15:
		return 0
	case v > 1e11: // milliseconds
		return int64(v / 1000)
	}
	return int64(v)
}

// first returns the first key of keys that kv has.
func first(kv map[string]string, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := kv[k]; ok {
			return v, true
		}
	}
	return "", false
}

// ---- Goldberg, gbe_fork, EMPRESS ----

// parseGoldbergJSON reads {"ID":{"earned":true,"earned_time":…,"progress":…,"max_progress":…}}.
func parseGoldbergJSON(b []byte) (map[string]Unlock, error) {
	var m map[string]struct {
		Earned      json.RawMessage `json:"earned"`
		EarnedTime  float64         `json:"earned_time"`
		Progress    float64         `json:"progress"`
		MaxProgress float64         `json:"max_progress"`
	}
	if err := json.Unmarshal([]byte(text(b)), &m); err != nil {
		return nil, fmt.Errorf("%w: %v", errFormat, err)
	}
	out := make(map[string]Unlock, len(m))
	for id, v := range m {
		out[id] = Unlock{Achieved: truthy(strings.Trim(string(v.Earned), `"`)), At: unixTime(v.EarnedTime), Progress: v.Progress, Max: v.MaxProgress}
	}
	return out, nil
}

// ---- INI files: CODEX, RUNE, OnlineFix, CreamAPI, 3DMGAME, ALI213 ----

// skipSections hold settings, not achievements.
var skipSections = map[string]bool{"steamachievements": true, "steam": true, "settings": true, "stats": true, "achievementsunlocked": true}

// parseINIUnlocks reads one [ID] section per achievement, with the key
// names the different groups use:
//
//	CODEX, RUNE, 3DMGAME: Achieved=1, UnlockTime=…, CurProgress, MaxProgress
//	OnlineFix:            achieved=true, timestamp=…
//	CreamAPI:             achieved=true, unlocktime=… (seven digits: ×1000)
//	ALI213:               HaveAchieved=1, HaveAchievedTime=…
//
// CODEX and RUNE also list what's unlocked, in unlock order, in
// [SteamAchievements] (00000=ID, …, Count=N); an ID listed there without a
// section of its own counts as unlocked. An achievement whose progress
// reached its maximum is unlocked too (some games only report progress).
func parseINIUnlocks(b []byte) (map[string]Unlock, error) {
	out := map[string]Unlock{}
	var listed []string
	for _, s := range parseINI(b) {
		if strings.EqualFold(s.name, "steamachievements") {
			for k, v := range s.kv {
				if k != "count" && isDigits(k) && strings.TrimSpace(v) != "" {
					listed = append(listed, strings.TrimSpace(v))
				}
			}
			continue
		}
		if s.name == "" || skipSections[strings.ToLower(s.name)] {
			continue
		}
		a, ok := first(s.kv, "achieved", "haveachieved", "unlocked", "earned")
		if !ok {
			continue
		}
		u := Unlock{Achieved: truthy(a)}
		if t, ok := first(s.kv, "unlocktime", "timestamp", "haveachievedtime", "time", "earned_time"); ok {
			v := num(t)
			if v >= 1e6 && v < 1e7 {
				v *= 1000 // CreamAPI drops the last digits
			}
			u.At = unixTime(v)
		}
		if p, ok := first(s.kv, "curprogress", "progress"); ok {
			u.Progress = num(p)
		}
		if m, ok := first(s.kv, "maxprogress", "max_progress"); ok {
			u.Max = num(m)
		}
		if u.Max > 0 && u.Progress >= u.Max {
			u.Achieved = true
		}
		out[s.name] = u
	}
	for _, id := range listed {
		if _, ok := out[id]; !ok {
			out[id] = Unlock{Achieved: true}
		}
	}
	if len(out) == 0 && !bytes.Contains(b, []byte("[")) && len(bytes.TrimSpace(b)) > 0 {
		return nil, errFormat
	}
	return out, nil
}

// ---- Reloaded, dodi ----

// hexLE reads a little-endian hex number ("01000000" → 1).
func hexLE(s string) (uint32, bool) {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) == 0 || len(b) > 4 {
		return 0, false
	}
	var v [4]byte
	copy(v[:], b)
	return binary.LittleEndian.Uint32(v[:]), true
}

// parseRLD reads [ID] sections with hex little-endian State, Time,
// CurProgress and MaxProgress; the [Steam] section is settings.
func parseRLD(b []byte) (map[string]Unlock, error) {
	out := map[string]Unlock{}
	for _, s := range parseINI(b) {
		if s.name == "" || skipSections[strings.ToLower(s.name)] {
			continue
		}
		st, ok := s.kv["state"]
		if !ok {
			continue
		}
		state, _ := hexLE(st)
		u := Unlock{Achieved: state != 0}
		if t, ok := hexLE(s.kv["time"]); ok {
			u.At = unixTime(float64(t))
		}
		if p, ok := hexLE(s.kv["curprogress"]); ok {
			u.Progress = float64(p)
		}
		if m, ok := hexLE(s.kv["maxprogress"]); ok {
			u.Max = float64(m)
		}
		out[s.name] = u
	}
	return out, nil
}

// ---- SKIDROW ----

// parseSkidrow reads achiev.ini lines "ID=1@…@time" (in any section).
func parseSkidrow(b []byte) (map[string]Unlock, error) {
	out := map[string]Unlock{}
	for _, s := range parseINI(b) {
		for k, v := range s.kv {
			parts := strings.Split(v, "@")
			if len(parts) < 2 {
				continue
			}
			u := Unlock{Achieved: truthy(parts[0])}
			if u.Achieved {
				u.At = unixTime(num(parts[len(parts)-1]))
			}
			out[k] = u
		}
	}
	return out, nil
}

// ---- TENOKE ----

// parseTenoke reads SteamData\user_stats.ini: an [ACHIEVEMENTS] section
// whose lines are ID = {unlocked = true, time = …}.
func parseTenoke(b []byte) (map[string]Unlock, error) {
	out := map[string]Unlock{}
	for _, s := range parseINI(b) {
		if !strings.EqualFold(s.name, "achievements") {
			continue
		}
		for k, v := range s.kv {
			v = strings.TrimSpace(v)
			if !strings.HasPrefix(v, "{") {
				continue
			}
			fields := map[string]string{}
			for _, f := range strings.Split(strings.Trim(v, "{}"), ",") {
				fk, fv, ok := strings.Cut(f, "=")
				if ok {
					fields[strings.ToLower(strings.TrimSpace(fk))] = strings.Trim(strings.TrimSpace(fv), `"`)
				}
			}
			u := Unlock{Achieved: truthy(fields["unlocked"])}
			u.At = unixTime(num(fields["time"]))
			out[strings.Trim(k, `"`)] = u
		}
	}
	return out, nil
}

// ---- Razor1911 ----

// parseRazor reads lines "ID achieved time".
func parseRazor(b []byte) (map[string]Unlock, error) {
	out := map[string]Unlock{}
	sc := bufio.NewScanner(strings.NewReader(text(b)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		u := Unlock{Achieved: truthy(f[1])}
		if len(f) > 2 {
			u.At = unixTime(num(f[2]))
		}
		out[f[0]] = u
	}
	return out, nil
}

// ---- SmartSteamEmu ----

// sseKey is how parseSSE keys an achievement: the CRC32 of its ID.
func sseKey(crc uint32) string { return fmt.Sprintf("crc:%08x", crc) }

// SSEHash is the key SmartSteamEmu's stats.bin files an achievement ID under.
func SSEHash(id string) string { return sseKey(crc32.ChecksumIEEE([]byte(id))) }

// parseSSE reads stats.bin: a little-endian record count, then 24-byte
// records: the ID's CRC32 (stored byte-reversed), …, the unlock time at 8
// and the state in the last four bytes. The keys are hashes (sseKey); the
// caller matches them to the schema's IDs.
func parseSSE(b []byte) (map[string]Unlock, error) {
	const stride = 24
	if len(b) < 4 {
		return nil, errFormat
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < 0 || n > 10000 || 4+n*stride > len(b) {
		return nil, errFormat
	}
	out := make(map[string]Unlock, n)
	for i := 0; i < n; i++ {
		r := b[4+i*stride : 4+(i+1)*stride]
		crc := binary.LittleEndian.Uint32(r[0:4])
		u := Unlock{Achieved: binary.LittleEndian.Uint32(r[stride-4:]) == 1}
		if u.Achieved {
			u.At = unixTime(float64(binary.LittleEndian.Uint32(r[8:12])))
		}
		out[sseKey(crc)] = u
	}
	return out, nil
}

// ---- Nemirtinga's Epic emulator ----

// parseNemirtinga reads achievements.json: a list of objects naming the
// achievement and whether it's unlocked. Its times aren't wall-clock
// times, so they're left out ("unlocked, time unknown").
func parseNemirtinga(b []byte) (map[string]Unlock, error) {
	var list []map[string]any
	if err := json.Unmarshal([]byte(text(b)), &list); err != nil {
		var wrap map[string][]map[string]any
		if json.Unmarshal([]byte(text(b)), &wrap) != nil {
			return nil, fmt.Errorf("%w: %v", errFormat, err)
		}
		for _, v := range wrap {
			list = append(list, v...)
		}
	}
	out := map[string]Unlock{}
	for _, o := range list {
		lo := make(map[string]any, len(o))
		for k, v := range o {
			lo[strings.ToLower(k)] = v
		}
		id := ""
		for _, k := range []string{"achievementid", "achievement_id", "id", "name"} {
			if s, ok := lo[k].(string); ok && s != "" {
				id = s
				break
			}
		}
		if id == "" {
			continue
		}
		u := Unlock{}
		for _, k := range []string{"unlocked", "achieved", "earned"} {
			if v, ok := lo[k].(bool); ok {
				u.Achieved = v
			}
		}
		if p, ok := lo["progress"].(float64); ok && p >= 1 {
			u.Achieved = true
		}
		out[id] = u
	}
	return out, nil
}
