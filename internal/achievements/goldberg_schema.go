package achievements

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// GoldbergSchema reads the schema Goldberg-style emulators keep next to
// the game (steam_settings\achievements.json). Icons become absolute paths
// of files inside the game folder; lang is Steam's language name
// ("english", "german", …).
func GoldbergSchema(g EmuGame, lang string) ([]Def, string, error) {
	dir := filepath.Join(g.emuPath(), "steam_settings")
	p := filepath.Join(dir, "achievements.json")
	b, err := readSmall(p, maxFile)
	if err != nil {
		return nil, "", err
	}
	defs, err := parseGoldbergSchema(b, lang)
	if err != nil {
		return nil, "", err
	}
	for i := range defs {
		defs[i].Icon = localIcon(g.Dir, dir, defs[i].Icon)
		defs[i].IconGray = localIcon(g.Dir, dir, defs[i].IconGray)
	}
	return defs, p, nil
}

// localIcon finds an icon file named in the schema: relative to
// steam_settings, or in its achievement_images folder. "" when it isn't
// there or lies outside the game folder.
func localIcon(gameDir, settings, name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "://") {
		return ""
	}
	for _, p := range []string{filepath.Join(settings, name), filepath.Join(settings, "achievement_images", name)} {
		if platform.Within(gameDir, p) && platform.IsFile(p) {
			return p
		}
	}
	return ""
}

// parseGoldbergSchema reads an array of
// {name, displayName, description, hidden, icon, icon_gray|icongray}.
// displayName and description are plain strings or objects keyed by language.
func parseGoldbergSchema(b []byte, lang string) ([]Def, error) {
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text(b)), &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", errFormat, err)
	}
	out := make([]Def, 0, len(raw))
	for _, r := range raw {
		d := Def{
			ID:       jsonString(r["name"], lang),
			Name:     jsonString(r["displayName"], lang),
			Desc:     jsonString(r["description"], lang),
			Icon:     jsonString(r["icon"], lang),
			IconGray: jsonString(r["icon_gray"], lang),
			Hidden:   jsonBool(r["hidden"]),
		}
		if d.IconGray == "" {
			d.IconGray = jsonString(r["icongray"], lang)
		}
		if d.ID == "" {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// jsonString reads a string, or from an object keyed by language the
// language asked for, else English, else any.
func jsonString(m json.RawMessage, lang string) string {
	if len(m) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(m, &s) == nil {
		return s
	}
	var byLang map[string]string
	if json.Unmarshal(m, &byLang) != nil {
		return ""
	}
	for _, k := range []string{lang, "english"} {
		if v := byLang[k]; v != "" {
			return v
		}
	}
	for _, k := range []string{"token"} {
		delete(byLang, k)
	}
	best := ""
	for k, v := range byLang { // any language, the same one every time
		if v != "" && (best == "" || k < best) {
			best = k
		}
	}
	return byLang[best]
}

// jsonBool reads true, 1, "1" or "true".
func jsonBool(m json.RawMessage) bool {
	s := strings.Trim(strings.TrimSpace(string(m)), `"`)
	return truthy(s)
}
