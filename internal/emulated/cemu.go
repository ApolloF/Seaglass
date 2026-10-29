package emulated

import (
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// cemu is the Wii U emulator. Its settings.xml (in portable\ next to it,
// else in %APPDATA%\Cemu) lists its game folders and where its internal
// storage (mlc01) is; title_list_cache.xml beside it names the games it
// has seen. Games are disc images (.wud, .wux, .wua) or unpacked games
// (code\*.rpx with meta\meta.xml), and those installed to mlc01.
var cemu = &emulator{
	name: "Cemu",
	exes: []string{"cemu.exe"},
	listed: func(in *install) ([]Game, []root) {
		dir := cemuSettingsDir(in.dir)
		var games []Game
		var roots []root
		mlc := filepath.Join(in.dir, "mlc01")
		if b, err := readSmall(filepath.Join(dir, "settings.xml"), 1<<20); err == nil {
			s := parseCemuSettings(b)
			for _, p := range s.paths {
				if p = pathFrom(in.dir, p); p != "" {
					roots = append(roots, root{path: p, depth: maxDepth})
				}
			}
			if p := pathFrom(in.dir, s.mlc); p != "" {
				mlc = p
			}
		}
		if b, err := readSmall(filepath.Join(dir, "title_list_cache.xml"), 4<<20); err == nil {
			for _, t := range parseCemuTitles(b) {
				p := pathFrom(in.dir, t.path)
				if p == "" || !platform.IsFile(p) {
					continue
				}
				g, ok := wiiUFile(p, fileSize(p))
				if !ok {
					continue
				}
				if name := cleanTitle(t.name); name != "" {
					g.Title, g.BaseTitle, g.TitleFrom = titleCase(name), titleCase(name), "Cemu game list"
				}
				if isTitleID(t.id) {
					g.Serial = strings.ToUpper(t.id)
				}
				g.How = "Cemu game list"
				games = append(games, g)
			}
		}
		// Games installed to Cemu's storage: usr\title\00050000\<id>.
		base := filepath.Join(mlc, "usr", "title", "00050000")
		if ents, err := readDir(base, 2000); err == nil {
			for _, e := range ents {
				if !e.IsDir() {
					continue
				}
				d := filepath.Join(base, e.Name())
				if g, ok := wiiUFolder(d, nameSet(d, 100)); ok {
					g.How = "Cemu installed games"
					games = append(games, g)
				}
			}
		}
		return games, roots
	},
	exts: map[string]bool{".wud": true, ".wux": true, ".wua": true},
	file: func(in *install, path string, size int64) (Game, bool) { return wiiUFile(path, size) },
	folder: func(in *install, dir string, names map[string]bool) (Game, bool) {
		return wiiUFolder(dir, names)
	},
	args: func(file string) string { return "-f -g " + quote(file) },
}

// cemuSettingsDir is where Cemu keeps settings.xml: a portable folder
// beside it (2.0 and later), beside it (1.x), or in AppData.
func cemuSettingsDir(dir string) string {
	if p := filepath.Join(dir, "portable"); platform.IsFile(filepath.Join(p, "settings.xml")) {
		return p
	}
	if platform.IsFile(filepath.Join(dir, "settings.xml")) {
		return dir
	}
	return filepath.Join(platform.Roaming, "Cemu")
}

// wiiUFile is a Wii U disc image. Its contents are encrypted, so its
// title comes from Cemu's list or the file and folder names.
func wiiUFile(path string, size int64) (Game, bool) {
	if size < 1<<20 {
		return Game{}, false
	}
	return Game{Platform: "Wii U", File: path, SizeBytes: size, Title: cleanName(filepath.Base(path))}, true
}

// wiiUFolder is an unpacked or installed Wii U game: code\<name>.rpx, and
// meta\meta.xml with its name and title id.
func wiiUFolder(dir string, names map[string]bool) (Game, bool) {
	if !names["code"] || !names["meta"] {
		return Game{}, false
	}
	b, err := readSmall(filepath.Join(dir, "meta", "meta.xml"), 256<<10)
	if err != nil {
		return Game{}, false
	}
	m := parseWiiUMeta(b)
	rpx := ""
	if ents, err := readDir(filepath.Join(dir, "code"), 500); err == nil {
		for _, e := range ents {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".rpx") {
				rpx = filepath.Join(dir, "code", e.Name())
				break
			}
		}
	}
	if rpx == "" {
		return Game{}, false
	}
	g := Game{Platform: "Wii U", File: rpx, Dir: dir, Title: cleanTitle(m.name), TitleFrom: "meta.xml"}
	if isTitleID(m.id) {
		g.Serial = strings.ToUpper(m.id)
	}
	if g.Title == "" {
		g.Title, g.TitleFrom = cleanName(filepath.Base(dir)), "folder name"
	}
	g.BaseTitle = g.Title
	return g, g.Title != ""
}

func isTitleID(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, r := range strings.ToLower(s) {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

type cemuSettings struct {
	paths []string
	mlc   string
}

// parseCemuSettings reads the game folders (GamePaths) and the storage
// folder (mlc_path) from Cemu's settings.xml.
func parseCemuSettings(b []byte) cemuSettings {
	var s struct {
		MLC   string   `xml:"mlc_path"`
		Paths []string `xml:"GamePaths>Entry"`
		// Cemu 1.x kept them under content.
		Content struct {
			MLC   string   `xml:"mlc_path"`
			Paths []string `xml:"GamePaths>Entry"`
		} `xml:"content"`
	}
	if decodeXML(b, &s) != nil {
		return cemuSettings{}
	}
	out := cemuSettings{paths: append(s.Paths, s.Content.Paths...), mlc: s.MLC}
	if out.mlc == "" {
		out.mlc = s.Content.MLC
	}
	if len(out.paths) > 64 {
		out.paths = out.paths[:64]
	}
	return out
}

type cemuTitle struct{ id, name, path string }

// parseCemuTitles reads title_list_cache.xml: each game Cemu has seen,
// with its title id, name and file.
func parseCemuTitles(b []byte) []cemuTitle {
	var l struct {
		Titles []struct {
			ID   string `xml:"titleId,attr"`
			Name string `xml:"name"`
			Path string `xml:"path"`
		} `xml:"title"`
	}
	if decodeXML(b, &l) != nil {
		return nil
	}
	var out []cemuTitle
	for _, t := range l.Titles[:min(len(l.Titles), 2000)] {
		out = append(out, cemuTitle{id: t.ID, name: t.Name, path: t.Path})
	}
	return out
}

type wiiUMeta struct{ id, name string }

// parseWiiUMeta reads a Wii U game's meta.xml (longname_en, title_id).
func parseWiiUMeta(b []byte) wiiUMeta {
	var m struct {
		ID   string `xml:"title_id"`
		Name string `xml:"longname_en"`
	}
	if decodeXML(b, &m) != nil {
		return wiiUMeta{}
	}
	return wiiUMeta{id: strings.TrimSpace(m.ID), name: m.Name}
}

// decodeXML decodes a small XML file; entities and other character sets
// than UTF-8 aren't followed anywhere.
func decodeXML(b []byte, v any) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d.Decode(v)
}
