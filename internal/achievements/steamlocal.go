package achievements

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ApolloF/gamekit/vdf"
)

// SteamIconBase is where Steam's achievement icons are.
const SteamIconBase = "https://cdn.akamai.steamstatic.com/steamcommunity/public/images/apps/"

func validLang(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') {
			return false
		}
	}
	return true
}

// steamBit is where an achievement's unlock lives in Steam's stats: bit
// n of stat stat.
type steamBit struct {
	stat string
	bit  int
}

// steamSchema is a game's schema from Steam's local cache.
type steamSchema struct {
	defs []Def
	bits map[string]steamBit // achievement ID → its bit
}

// SteamSchemaFile is Steam's cached schema for an app.
func SteamSchemaFile(root string, appID int) string {
	return filepath.Join(root, "appcache", "stats", "UserGameStatsSchema_"+strconv.Itoa(appID)+".bin")
}

// SteamStatsFile is an account's cached stats for an app.
func SteamStatsFile(root, account string, appID int) string {
	return filepath.Join(root, "appcache", "stats", "UserGameStats_"+account+"_"+strconv.Itoa(appID)+".bin")
}

// SteamSchema reads the schema Steam keeps for an app it has run.
func SteamSchema(root string, appID int, lang string) ([]Def, error) {
	s, err := readSteamSchema(root, appID, lang)
	if err != nil {
		return nil, err
	}
	return s.defs, nil
}

func readSteamSchema(root string, appID int, lang string) (steamSchema, error) {
	if root == "" || appID <= 0 {
		return steamSchema{}, os.ErrNotExist
	}
	b, err := readSmall(SteamSchemaFile(root, appID), maxFile)
	if err != nil {
		return steamSchema{}, err
	}
	return parseSteamSchema(b, appID, lang)
}

// nodeStr reads a string or number child as text.
func nodeStr(n *vdf.BNode, key string) string {
	c := n.Child(key)
	switch {
	case c == nil:
		return ""
	case c.Type == vdf.BString:
		return c.Str
	case c.Type == vdf.BInt32:
		return strconv.FormatInt(int64(int32(c.Int)), 10)
	}
	return ""
}

func nodeInt(n *vdf.BNode, key string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(nodeStr(n, key)))
	return v, err == nil
}

// localized reads display/name-style maps keyed by language.
func localized(n *vdf.BNode, lang string) string {
	if n == nil {
		return ""
	}
	if n.Type == vdf.BString {
		return n.Str
	}
	for _, l := range []string{lang, "english"} {
		if s := n.String(l); s != "" {
			return s
		}
	}
	for _, k := range n.Kids {
		if k.Type == vdf.BString && k.Str != "" && !strings.EqualFold(k.Key, "token") {
			return k.Str
		}
	}
	return ""
}

// parseSteamSchema reads UserGameStatsSchema_<appid>.bin: stats/<id> with
// type 4 or 5 hold achievements, one per bits/<n>, each with display/name,
// desc (per language), hidden, icon and icon_gray.
func parseSteamSchema(b []byte, appID int, lang string) (steamSchema, error) {
	root, err := vdf.ParseBinary(b)
	if err != nil {
		return steamSchema{}, err
	}
	var stats *vdf.BNode
	if app := root.Child(strconv.Itoa(appID)); app != nil {
		stats = app.Child("stats")
	}
	if stats == nil {
		for _, k := range root.Kids {
			if s := k.Child("stats"); s != nil && s.Type == vdf.BMap {
				stats = s
				break
			}
		}
	}
	if stats == nil {
		return steamSchema{}, errors.New("no stats in Steam's schema")
	}
	out := steamSchema{bits: map[string]steamBit{}}
	for _, st := range stats.Kids {
		if t, _ := nodeInt(st, "type"); t != 4 && t != 5 {
			continue
		}
		bits := st.Child("bits")
		if bits == nil {
			continue
		}
		for _, bn := range bits.Kids {
			id := nodeStr(bn, "name")
			if id == "" {
				continue
			}
			bit, ok := nodeInt(bn, "bit")
			if !ok {
				bit, ok = func() (int, bool) { v, err := strconv.Atoi(bn.Key); return v, err == nil }()
			}
			if !ok || bit < 0 || bit > 31 {
				continue
			}
			disp := bn.Child("display")
			d := Def{ID: id, Name: localized(disp.Child("name"), lang), Desc: localized(disp.Child("desc"), lang), Hidden: truthy(nodeStr(disp, "hidden"))}
			if ic := nodeStr(disp, "icon"); ic != "" && !strings.ContainsAny(ic, `/\?#`) {
				d.Icon = SteamIconBase + strconv.Itoa(appID) + "/" + ic
			}
			if ic := nodeStr(disp, "icon_gray"); ic != "" && !strings.ContainsAny(ic, `/\?#`) {
				d.IconGray = SteamIconBase + strconv.Itoa(appID) + "/" + ic
			}
			out.defs = append(out.defs, d)
			out.bits[id] = steamBit{stat: st.Key, bit: bit}
		}
	}
	return out, nil
}

// SteamLocal reads a Steam game's schema and the account's unlocks from
// Steam's cache. files are the files it depends on, for cache keys.
func SteamLocal(root string, accounts []string, appID int, lang string) (defs []Def, unlocks map[string]Unlock, files []string, err error) {
	files = append(files, SteamSchemaFile(root, appID))
	s, err := readSteamSchema(root, appID, lang)
	if err != nil {
		return nil, nil, files, err
	}
	for _, acc := range accounts {
		p := SteamStatsFile(root, acc, appID)
		files = append(files, p)
		b, err := readSmall(p, maxFile)
		if err != nil {
			continue
		}
		u, err := parseSteamStats(b, s.bits)
		if err != nil {
			continue
		}
		return s.defs, u, files, nil
	}
	return s.defs, nil, files, nil
}

// parseSteamStats reads UserGameStats_<account>_<appid>.bin: achievement
// bit n is set in cache/<stat>/data, its time in AchievementTimes/<n>.
func parseSteamStats(b []byte, bits map[string]steamBit) (map[string]Unlock, error) {
	root, err := vdf.ParseBinary(b)
	if err != nil {
		return nil, err
	}
	cache := root.Child("cache")
	if cache == nil {
		cache = root
	}
	out := make(map[string]Unlock, len(bits))
	for id, sb := range bits {
		st := cache.Child(sb.stat)
		if st == nil {
			out[id] = Unlock{}
			continue
		}
		data, _ := nodeInt(st, "data")
		u := Unlock{Achieved: uint32(data)&(1<<uint(sb.bit)) != 0}
		if u.Achieved {
			times := st.Child("AchievementTimes")
			if times == nil {
				times = cache.Child("AchievementTimes")
			}
			if t, ok := nodeInt(times, strconv.Itoa(sb.bit)); ok {
				u.At = unixTime(float64(uint32(t)))
			}
		}
		out[id] = u
	}
	return out, nil
}
