package scan

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/ApolloF/Seaglass/internal/platform"
)

const maxJSON = 4 << 20

func readSmall(p string, limit int64) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// ---------- Epic ----------

type epicItem struct {
	DisplayName          string
	InstallLocation      string
	LaunchExecutable     string
	LaunchCommand        string
	CatalogNamespace     string
	CatalogItemID        string `json:"CatalogItemId"`
	AppName              string
	InstallSize          int64
	IncompleteInstall    bool     `json:"bIsIncompleteInstall"`
	AppCategories        []string `json:"AppCategories"`
	MainGameCatalogItem  string   `json:"MainGameCatalogItemId"`
	MainGameAppName      string
	MainGameCatalogSpace string `json:"MainGameCatalogNamespace"`
}

// EpicManifestDir is where the Epic Games Launcher records installed games.
func EpicManifestDir() string {
	return filepath.Join(platform.ProgramData, "Epic", "EpicGamesLauncher", "Data", "Manifests")
}

func epicCandidates() []Candidate {
	files, _ := filepath.Glob(filepath.Join(EpicManifestDir(), "*.item"))
	var out []Candidate
	for _, file := range files {
		b, err := readSmall(file, maxJSON)
		if err != nil {
			continue
		}
		var it epicItem
		if json.Unmarshal(b, &it) != nil || it.IncompleteInstall {
			continue
		}
		if len(it.AppCategories) > 0 && !contains(it.AppCategories, "games") {
			continue
		}
		// DLC shares the base game's folder; skip it.
		if it.MainGameAppName != "" && it.MainGameAppName != it.AppName {
			continue
		}
		dir := filepath.Clean(it.InstallLocation)
		if !filepath.IsAbs(dir) || !platform.IsDir(dir) {
			continue
		}
		c := Candidate{Title: it.DisplayName, TitleTrusted: true, Dir: dir, Source: Epic, How: "Epic Games library", SizeBytes: it.InstallSize}
		if it.CatalogNamespace != "" && it.CatalogItemID != "" && it.AppName != "" {
			c.EpicApp = it.CatalogNamespace + ":" + it.CatalogItemID + ":" + it.AppName
			c.LaunchURI = "com.epicgames.launcher://apps/" + url.PathEscape(c.EpicApp) + "?action=launch&silent=true"
		}
		if exe := filepath.Join(dir, it.LaunchExecutable); it.LaunchExecutable != "" && filepath.IsLocal(it.LaunchExecutable) {
			c.Exe, c.Args = exe, it.LaunchCommand
		}
		if c.Title == "" {
			c.Title = filepath.Base(dir)
		}
		out = append(out, c)
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// ---------- GOG ----------

// gogInfo is a goggame-<id>.info file: GOG writes one into every game it
// installs, and GOG installers and rips carry it too.
type gogInfo struct {
	GameID    string `json:"gameId"`
	Name      string `json:"name"`
	RootID    string `json:"rootGameId"`
	PlayTasks []struct {
		IsPrimary  bool   `json:"isPrimary"`
		Type       string `json:"type"`
		Path       string `json:"path"`
		Arguments  string `json:"arguments"`
		WorkingDir string `json:"workingDir"`
		Category   string `json:"category"`
	} `json:"playTasks"`
}

func readGogInfo(p string) (gogInfo, bool) {
	var g gogInfo
	b, err := readSmall(p, maxJSON)
	if err != nil || json.Unmarshal(b, &g) != nil || g.GameID == "" {
		return g, false
	}
	// DLC info files point at their base game.
	if g.RootID != "" && g.RootID != g.GameID {
		return g, false
	}
	return g, true
}

// primaryTask returns the executable GOG starts for this game, if it lies in dir.
func (g gogInfo) primaryTask(dir string) (exe, args, work string) {
	for _, t := range g.PlayTasks {
		if !t.IsPrimary || (t.Type != "" && t.Type != "FileTask") || !filepath.IsLocal(t.Path) {
			continue
		}
		exe = filepath.Join(dir, t.Path)
		if t.WorkingDir != "" && filepath.IsLocal(t.WorkingDir) {
			work = filepath.Join(dir, t.WorkingDir)
		}
		return exe, t.Arguments, work
	}
	return "", "", ""
}

// ---------- EA and Ubisoft: registry, in stores_windows.go ----------

// ---------- Xbox (moddable XboxGames folders) ----------

func xboxCandidates() []Candidate {
	var out []Candidate
	for _, drive := range platform.FixedDrives() {
		root := filepath.Join(drive, "XboxGames")
		es, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range es {
			if !e.IsDir() || strings.EqualFold(e.Name(), "GameSave") {
				continue
			}
			dir := filepath.Join(root, e.Name())
			content := filepath.Join(dir, "Content")
			if !platform.IsDir(content) {
				content = dir
			}
			c := Candidate{Title: e.Name(), Dir: content, Source: Xbox, How: "Xbox app library"}
			if title, uri := xboxManifest(content); title != "" || uri != "" {
				if title != "" {
					c.Title, c.TitleTrusted = title, true
				}
				c.LaunchURI = uri
			}
			out = append(out, c)
		}
	}
	return out
}

// xboxManifest reads the package's display name and builds its
// shell:AppsFolder\<family name>!<app id> launch link.
func xboxManifest(dir string) (title, uri string) {
	b, err := readSmall(filepath.Join(dir, "appxmanifest.xml"), maxJSON)
	if err != nil {
		return "", ""
	}
	var m struct {
		Identity struct {
			Name      string `xml:"Name,attr"`
			Publisher string `xml:"Publisher,attr"`
		} `xml:"Identity"`
		Properties struct {
			DisplayName string `xml:"DisplayName"`
		} `xml:"Properties"`
		Applications struct {
			Application []struct {
				ID string `xml:"Id,attr"`
			} `xml:"Application"`
		} `xml:"Applications"`
	}
	if xml.Unmarshal(b, &m) != nil {
		return "", ""
	}
	title = m.Properties.DisplayName
	if strings.HasPrefix(title, "ms-resource:") {
		title = ""
	}
	if m.Identity.Name != "" && m.Identity.Publisher != "" && len(m.Applications.Application) > 0 {
		uri = `shell:AppsFolder\` + m.Identity.Name + "_" + publisherID(m.Identity.Publisher) + "!" + m.Applications.Application[0].ID
	}
	return title, uri
}

// publisherID is the 13-character hash Windows puts in a package family
// name: the first 8 bytes of SHA-256 over the UTF-16LE publisher, in a
// Crockford-style base32.
func publisherID(publisher string) string {
	u := utf16.Encode([]rune(publisher))
	raw := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(raw[2*i:], c)
	}
	sum := sha256.Sum256(raw)
	v := binary.BigEndian.Uint64(sum[:8])
	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	out := make([]byte, 13)
	// 64 bits padded with one zero bit make 13 groups of 5.
	for i := 0; i < 13; i++ {
		shift := 64 - 5*(i+1)
		var idx uint64
		if shift >= 0 {
			idx = (v >> uint(shift)) & 31
		} else {
			idx = (v << uint(-shift)) & 31
		}
		out[i] = alphabet[idx]
	}
	return string(out)
}
