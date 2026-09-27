// Package steaminput routes games without DualSense support through Steam
// Input. Each such game gets a non-Steam shortcut in the user's
// shortcuts.vdf (in a "Seaglass" collection) and is started with
// steam://rungameid/…, so Steam presents the controller as an Xbox pad.
//
// Steam keeps shortcuts.vdf in memory and writes it back when it exits,
// so the file is only changed while Steam is closed.
package steaminput

import (
	"context"
	"errors"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/gamekit/vdf"
)

// Tag is the Steam collection Seaglass's shortcuts go in.
const Tag = "Seaglass"

// Shortcut is a game as Steam should start it.
type Shortcut struct {
	Name    string
	Exe     string // full path
	WorkDir string
	Args    string
}

func (s Shortcut) quotedExe() string { return `"` + s.Exe + `"` }

// ID is the id Steam gives a non-Steam shortcut: a CRC of the quoted exe
// and the name, with the top bit set.
func (s Shortcut) ID() uint32 {
	return crc32.ChecksumIEEE([]byte(s.quotedExe()+s.Name)) | 0x80000000
}

// URI starts the shortcut through Steam.
func URI(id uint32) string {
	return "steam://rungameid/" + strconv.FormatUint(uint64(id)<<32|0x02000000, 10)
}

// ErrNoSteam means Steam isn't installed or has no signed-in account.
var ErrNoSteam = errors.New("Steam isn't installed or no Steam account has signed in on this PC")

// Files returns the shortcuts.vdf of the Steam account(s) used on this PC.
func Files() ([]string, error) {
	root := scan.SteamDir()
	if root == "" {
		return nil, ErrNoSteam
	}
	var out []string
	for _, acc := range scan.SteamAccounts(root) {
		out = append(out, filepath.Join(root, "userdata", acc, "config", "shortcuts.vdf"))
	}
	if len(out) == 0 {
		return nil, ErrNoSteam
	}
	return out, nil
}

// Find returns the id of an existing shortcut for s in every file, or
// ok=false when any file lacks one.
func Find(files []string, s Shortcut) (id uint32, ok bool) {
	for _, f := range files {
		root, err := read(f)
		if err != nil {
			return 0, false
		}
		got, found := find(root, s)
		if !found {
			return 0, false
		}
		id = got
	}
	return id, len(files) > 0
}

func find(root *vdf.BNode, s Shortcut) (uint32, bool) {
	for _, e := range root.Child("shortcuts").Kids {
		exe := strings.Trim(e.String("exe"), `"`)
		if strings.EqualFold(filepath.Clean(exe), filepath.Clean(s.Exe)) && e.String("appname") == s.Name {
			if id := e.Uint("appid"); id != 0 {
				return id, true
			}
			// Older files have no appid; Steam then derives it the same way.
			return crc32.ChecksumIEEE([]byte(e.String("exe")+e.String("appname"))) | 0x80000000, true
		}
	}
	return 0, false
}

func read(path string) (*vdf.BNode, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &vdf.BNode{Type: vdf.BMap}, nil
	}
	if err != nil {
		return nil, err
	}
	return vdf.ParseBinary(b)
}

// Add puts the shortcuts that are missing into every file. Steam must be
// closed. The first time a file is changed, a copy of the original is kept
// next to it (shortcuts.vdf.seaglass-backup).
func Add(files []string, list []Shortcut) error {
	if Running() {
		return errors.New("Steam is running")
	}
	return add(files, list)
}

func add(files []string, list []Shortcut) error {
	for _, f := range files {
		root, err := read(f)
		if err != nil {
			return err
		}
		scs := root.Child("shortcuts")
		if scs == nil {
			scs = &vdf.BNode{Key: "shortcuts", Type: vdf.BMap}
			root.Kids = append(root.Kids, scs)
		}
		changed := false
		for _, s := range list {
			if _, ok := find(root, s); ok {
				continue
			}
			scs.Kids = append(scs.Kids, entry(nextIndex(scs), s))
			changed = true
		}
		if !changed {
			continue
		}
		b, err := vdf.MarshalBinary(root)
		if err != nil {
			return err
		}
		if err := backupOnce(f); err != nil {
			return err
		}
		if err := writeAtomic(f, b); err != nil {
			return err
		}
	}
	return nil
}

func nextIndex(scs *vdf.BNode) string {
	n := 0
	for _, e := range scs.Kids {
		if i, err := strconv.Atoi(e.Key); err == nil && i >= n {
			n = i + 1
		}
	}
	return strconv.Itoa(n)
}

func entry(key string, s Shortcut) *vdf.BNode {
	str := func(k, v string) *vdf.BNode { return &vdf.BNode{Key: k, Type: vdf.BString, Str: v} }
	num := func(k string, v uint32) *vdf.BNode { return &vdf.BNode{Key: k, Type: vdf.BInt32, Int: v} }
	dir := s.WorkDir
	if dir == "" {
		dir = filepath.Dir(s.Exe)
	}
	return &vdf.BNode{Key: key, Type: vdf.BMap, Kids: []*vdf.BNode{
		num("appid", s.ID()),
		str("AppName", s.Name),
		str("Exe", s.quotedExe()),
		str("StartDir", `"`+dir+`"`),
		str("icon", s.Exe),
		str("ShortcutPath", ""),
		str("LaunchOptions", s.Args),
		num("IsHidden", 0),
		num("AllowDesktopConfig", 1),
		num("AllowOverlay", 1),
		num("OpenVR", 0),
		num("Devkit", 0),
		str("DevkitGameID", ""),
		num("DevkitOverrideAppID", 0),
		num("LastPlayTime", 0),
		str("FlatpakAppID", ""),
		{Key: "tags", Type: vdf.BMap, Kids: []*vdf.BNode{str("0", Tag)}},
	}}
}

func backupOnce(f string) error {
	bak := f + ".seaglass-backup"
	if platform.IsFile(bak) {
		return nil
	}
	b, err := os.ReadFile(f)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(bak, b, 0o644)
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Running reports whether Steam runs.
func Running() bool { return platform.ProcessRunning("steam.exe") }

// Shutdown asks Steam to exit and waits until it has.
func Shutdown(ctx context.Context) error {
	root := scan.SteamDir()
	if root == "" {
		return ErrNoSteam
	}
	if !Running() {
		return nil
	}
	if err := exec.Command(filepath.Join(root, "steam.exe"), "-shutdown").Start(); err != nil {
		return err
	}
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("Steam didn't close in time")
		case <-t.C:
			if !Running() {
				// Steam writes its files on the way out; give it a moment.
				time.Sleep(time.Second)
				return nil
			}
		}
	}
}
