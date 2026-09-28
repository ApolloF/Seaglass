package achievements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// Env holds the folders emulators write to. Tests point them at a temporary folder.
type Env struct {
	Roaming, Local, Public, ProgramData, Documents string
}

// DefaultEnv is this Windows account's folders.
func DefaultEnv() Env {
	return Env{Roaming: platform.Roaming, Local: platform.Local, Public: platform.Public, ProgramData: platform.ProgramData, Documents: platform.Documents}
}

// EmuGame is what the emulator lookup needs to know about a game.
type EmuGame struct {
	Dir      string // install folder
	EmuDir   string // relative to Dir: where the emulator sits
	Emulator string // as the scan named it ("Goldberg", "RUNE", …)
	AppID    int    // Steam app id the emulator runs as
	EpicID   string // for Nemirtinga's Epic emulator
}

func (g EmuGame) emuPath() string {
	if g.EmuDir == "" {
		return g.Dir
	}
	p := filepath.Join(g.Dir, g.EmuDir)
	if !platform.Within(g.Dir, p) {
		return g.Dir
	}
	return p
}

// emuSource is one place an emulator keeps its unlocks. In paths,
// {roaming}, {local}, {public}, {programdata} and {documents} are the
// Windows folders, {emu} the folder the emulator sits in, {appid} the Steam
// app id and {epic} the Epic id; a "*" segment matches any one folder.
type emuSource struct {
	name   string
	paths  []string
	parse  parser
	hashed bool // keys are SSEHash(id), not ids
}

// emuSources lists every unlock file format Seaglass reads. Groups whose
// files no public source describes (unknownEmus) aren't guessed at.
var emuSources = []emuSource{
	{name: "Goldberg", parse: parseGoldbergJSON, paths: []string{
		`{roaming}\Goldberg SteamEmu Saves\{appid}\achievements.json`,
		`{roaming}\GSE Saves\{appid}\achievements.json`, // gbe_fork
	}},
	{name: "EMPRESS", parse: parseGoldbergJSON, paths: []string{
		`{roaming}\EMPRESS\remote\{appid}\achievements.json`,
		`{public}\Documents\EMPRESS\{appid}\remote\{appid}\achievements.json`,
	}},
	{name: "CODEX", parse: parseINIUnlocks, paths: []string{
		`{public}\Documents\Steam\CODEX\{appid}\achievements.ini`,
		`{roaming}\Steam\CODEX\{appid}\achievements.ini`,
	}},
	{name: "RUNE", parse: parseINIUnlocks, paths: []string{
		`{public}\Documents\Steam\RUNE\{appid}\achievements.ini`,
	}},
	{name: "OnlineFix", parse: parseINIUnlocks, paths: []string{
		`{public}\Documents\OnlineFix\{appid}\Stats\Achievements.ini`,
		`{public}\Documents\OnlineFix\{appid}\Achievements.ini`,
	}},
	{name: "CreamAPI", parse: parseINIUnlocks, paths: []string{
		`{roaming}\CreamAPI\{appid}\stats\CreamAPI.Achievements.cfg`,
	}},
	{name: "Reloaded", parse: parseRLD, paths: []string{
		`{programdata}\RLD!\{appid}\achievements.ini`,
		`{programdata}\Steam\Player\{appid}\stats\achievements.ini`,
		`{programdata}\Steam\RLD!\{appid}\stats\achievements.ini`,
		`{programdata}\Steam\dodi\{appid}\stats\achievements.ini`,
	}},
	{name: "SKIDROW", parse: parseSkidrow, paths: []string{
		`{documents}\SKIDROW\{appid}\SteamEmu\UserStats\achiev.ini`,
		`{local}\SKIDROW\{appid}\SteamEmu\UserStats\achiev.ini`,
	}},
	{name: "SmartSteamEmu", parse: parseSSE, hashed: true, paths: []string{
		`{roaming}\SmartSteamEmu\{appid}\stats.bin`,
		`{emu}\SmartSteamEmu\{appid}\stats.bin`,
	}},
	{name: "TENOKE", parse: parseTenoke, paths: []string{`{emu}\SteamData\user_stats.ini`}},
	{name: "3DM", parse: parseINIUnlocks, paths: []string{`{emu}\3DMGAME\*\stats\achievements.ini`}},
	{name: "ALI213", parse: parseINIUnlocks, paths: []string{`{emu}\Profile\*\Stats\Achievements.Bin`}},
	{name: "Razor1911", parse: parseRazor, paths: []string{`{roaming}\.1911\{appid}\achievement`}},
	{name: "Epic emulator", parse: parseNemirtinga, paths: []string{`{roaming}\NemirtingasEpicEmu\*\{epic}\achievements.json`}},
}

// unknownEmus write unlock files no public source describes yet; samples
// would let Seaglass read them. Hoodlum and DARKSiDERS use the CODEX-style
// INI format, but where they keep it isn't known.
var unknownEmus = map[string]bool{"CPY": true, "PLAZA": true, "FLT": true, "Steamworks Fix": true, "HOODLUM": true, "DARKSiDERS": true}

// EmuResult is the unlock file found for a game.
type EmuResult struct {
	Source  string            // emuSource name
	File    string            // the file read
	Unlocks map[string]Unlock // by ID, or by SSEHash(ID) when Hashed
	Hashed  bool
}

// Unknown reports whether emulator is a group whose unlock files Seaglass can't read yet.
func Unknown(emulator string) bool { return unknownEmus[emulator] }

// ReadEmu finds the game's unlock file and reads it. Every place is tried;
// when several have one (an earlier emulator setup's, or the launcher's and the
// game's), the newest file names the source and what any of them has
// unlocked counts. Files lists every candidate path looked at, for cache keys.
func ReadEmu(g EmuGame, env Env) (res EmuResult, files []string, ok bool) {
	var best time.Time
	var others []EmuResult
	defer func() {
		if ok {
			res.Unlocks = mergeUnlocks(res, others)
		}
	}()
	for _, c := range emuCandidates(g, env) {
		files = append(files, c.path)
		fi, err := os.Stat(c.path)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxFile {
			continue
		}
		b, err := readSmall(c.path, maxFile)
		if err != nil {
			continue
		}
		u, err := safeParse(c.src.parse, b)
		if err != nil {
			continue
		}
		r := EmuResult{Source: c.src.name, File: c.path, Unlocks: u, Hashed: c.src.hashed}
		if ok && !fi.ModTime().After(best) {
			others = append(others, r)
			continue
		}
		if ok {
			others = append(others, res)
		}
		res, best, ok = r, fi.ModTime(), true
	}
	return res, files, ok
}

// mergeUnlocks adds to the newest file's unlocks what the others have
// unlocked (keyed the same way: SmartSteamEmu's hashes stay apart).
func mergeUnlocks(res EmuResult, others []EmuResult) map[string]Unlock {
	if len(others) == 0 {
		return res.Unlocks
	}
	out := make(map[string]Unlock, len(res.Unlocks))
	lower := map[string]string{}
	for k, u := range res.Unlocks {
		out[k] = u
		lower[strings.ToLower(k)] = k
	}
	for _, o := range others {
		if o.Hashed != res.Hashed {
			continue
		}
		for k, u := range o.Unlocks {
			if !u.Achieved {
				continue
			}
			if have, ok := lower[strings.ToLower(k)]; ok {
				if !out[have].Achieved {
					out[have] = u
				}
				continue
			}
			out[k] = u
			lower[strings.ToLower(k)] = k
		}
	}
	return out
}

type emuCandidate struct {
	src  *emuSource
	path string
}

// emuCandidates lists every place the game's unlock file could be.
func emuCandidates(g EmuGame, env Env) []emuCandidate {
	var out []emuCandidate
	for i := range emuSources {
		src := &emuSources[i]
		paths := src.paths
		if src.name == "Goldberg" {
			paths = append(append([]string(nil), paths...), localSaves(g)...)
		}
		for _, tmpl := range paths {
			for _, p := range expand(tmpl, g, env) {
				out = append(out, emuCandidate{src, p})
			}
		}
	}
	return out
}

// EmuFiles lists every file the game's emulator unlocks could be in.
func EmuFiles(g EmuGame, env Env) []string {
	var out []string
	for _, c := range emuCandidates(g, env) {
		out = append(out, c.path)
	}
	return out
}

// safeParse runs a parser; a panic on a hostile file is a parse error.
func safeParse(p parser, b []byte) (m map[string]Unlock, err error) {
	defer func() {
		if r := recover(); r != nil {
			m, err = nil, errFormat
		}
	}()
	return p(b)
}

// localSaves are Goldberg's saves kept in the game folder: local_save.txt
// names the folder (next to the emulator), gbe_fork's configs.user.ini
// sets local_save_path. Only folders inside the game's own count.
func localSaves(g EmuGame) []string {
	if g.AppID <= 0 {
		return nil
	}
	emu := g.emuPath()
	var dirs []string
	if b, err := readSmall(filepath.Join(emu, "local_save.txt"), 4096); err == nil {
		name := strings.TrimSpace(text(b))
		if name == "" {
			name = "SAVE"
		}
		dirs = append(dirs, filepath.Join(emu, name))
	}
	for _, ini := range []string{filepath.Join(emu, "steam_settings", "configs.user.ini"), filepath.Join(emu, "configs.user.ini")} {
		b, err := readSmall(ini, 256<<10)
		if err != nil {
			continue
		}
		for _, s := range parseINI(b) {
			if v := strings.TrimSpace(s.kv["local_save_path"]); v != "" && strings.EqualFold(s.name, "user::saves") {
				if !filepath.IsAbs(v) {
					v = filepath.Join(emu, v)
				}
				dirs = append(dirs, v)
			}
		}
	}
	var out []string
	for _, d := range dirs {
		p := filepath.Join(d, strconv.Itoa(g.AppID), "achievements.json")
		if platform.Within(g.Dir, p) {
			out = append(out, p)
		}
	}
	return out
}

// expand fills in a path template. Templates needing an id the game
// doesn't have, or a Windows folder that's unknown, expand to nothing.
func expand(tmpl string, g EmuGame, env Env) []string {
	vars := map[string]string{
		"{roaming}": env.Roaming, "{local}": env.Local, "{public}": env.Public,
		"{programdata}": env.ProgramData, "{documents}": env.Documents,
		"{emu}": "", "{appid}": "", "{epic}": "",
	}
	if g.Dir != "" {
		vars["{emu}"] = g.emuPath()
	}
	if g.AppID > 0 {
		vars["{appid}"] = strconv.Itoa(g.AppID)
	}
	if g.EpicID != "" && !strings.ContainsAny(g.EpicID, `\/.:*?"<>|`) {
		vars["{epic}"] = g.EpicID
	}
	p := tmpl
	for k, v := range vars {
		if strings.Contains(p, k) {
			if v == "" {
				return nil
			}
			p = strings.ReplaceAll(p, k, v)
		}
	}
	// Only whole "*" segments are wildcards; game folders may hold [ or ?.
	segs := strings.Split(p, `\`)
	star := -1
	for i, s := range segs {
		if s == "*" {
			star = i
			break
		}
	}
	if star < 0 {
		return []string{filepath.Clean(p)}
	}
	base := strings.Join(segs[:star], `\`)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for i, e := range entries {
		if i >= 64 || !e.IsDir() {
			continue
		}
		rest := strings.Join(append([]string{base, e.Name()}, segs[star+1:]...), `\`)
		if strings.Contains(rest, `\*\`) || strings.HasSuffix(rest, `\*`) {
			continue // one wildcard per template is all the table needs
		}
		out = append(out, filepath.Clean(rest))
	}
	return out
}

// EpicEmuID finds the id Nemirtinga's emulator files its saves under, from
// its settings file next to the game.
func EpicEmuID(g EmuGame) string {
	b, err := readSmall(filepath.Join(g.emuPath(), "nemirtingasepicemu.json"), 256<<10)
	if err != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(text(b)), &m) != nil {
		return ""
	}
	for k, v := range m {
		switch strings.ToLower(k) {
		case "appid", "app_id", "productid", "product_id", "sandboxid", "namespace":
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
