// Package addons runs Seaglass's add-ons: separate programs that talk
// JSON-RPC 2.0 over their standard input and output (docs/addon-protocol.md).
// An add-on is off until the user enables it; its program is pinned by
// SHA-256 and has to be approved again when it changes.
package addons

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Protocol is the add-on protocol version Seaglass speaks.
const Protocol = 1

// Hooks an add-on can ask for.
const (
	HookStatus       = "game.status"
	HookActions      = "game.actions"
	HookBeforeLaunch = "game.beforeLaunch"
	HookAfterExit    = "game.afterExit"
	HookGameAdded    = "library.gameAdded"
)

var knownHooks = map[string]bool{HookStatus: true, HookActions: true, HookBeforeLaunch: true, HookAfterExit: true, HookGameAdded: true}

// Permissions an add-on can declare, with how they're explained.
var Permissions = map[string]string{
	"modifyGameFiles": "Changes files in game folders",
	"network":         "Downloads from the internet",
	"readSaves":       "Reads save files",
}

// Manifest is an add-on's addon.json.
type Manifest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Publisher   string   `json:"publisher"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Exe         string   `json:"exe"`
	Args        []string `json:"args,omitempty"`
	Protocol    int      `json:"protocol"`
	Hooks       []string `json:"hooks"`
	Permissions []string `json:"permissions,omitempty"`

	Dir string `json:"-"` // the folder the manifest is in
}

// Has reports whether the add-on asked for a hook.
func (m *Manifest) Has(hook string) bool {
	for _, h := range m.Hooks {
		if h == hook {
			return true
		}
	}
	return false
}

// ExePath is the program's absolute path.
func (m *Manifest) ExePath() string {
	if filepath.IsAbs(m.Exe) {
		return filepath.Clean(m.Exe)
	}
	return filepath.Join(m.Dir, m.Exe)
}

var reID = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,63}$`)

// maxManifest caps addon.json's size.
const maxManifest = 64 << 10

// ReadManifest reads and checks an addon.json.
func ReadManifest(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxManifest+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxManifest {
		return nil, errors.New("addon.json is too large")
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("addon.json isn't valid: %w", err)
	}
	m.Dir = filepath.Dir(path)
	return &m, m.check()
}

func (m *Manifest) check() error {
	m.Name, m.Publisher = strings.TrimSpace(m.Name), strings.TrimSpace(m.Publisher)
	switch {
	case !reID.MatchString(m.ID):
		return errors.New("the add-on id must be 2–64 lower-case letters, digits, dots or dashes")
	case m.Name == "" || len(m.Name) > 64:
		return errors.New("the add-on needs a name (up to 64 characters)")
	case m.Publisher == "" || len(m.Publisher) > 64:
		return errors.New("the add-on needs a publisher (up to 64 characters)")
	case len(m.Version) > 32 || len(m.Description) > 500:
		return errors.New("version or description is too long")
	case m.Homepage != "" && (!strings.HasPrefix(m.Homepage, "https://") || len(m.Homepage) > 300 || strings.ContainsAny(m.Homepage, " \"<>")):
		return errors.New("the homepage must be an https:// address")
	case m.Protocol != Protocol:
		return fmt.Errorf("the add-on speaks protocol %d; Seaglass speaks %d", m.Protocol, Protocol)
	case !strings.EqualFold(filepath.Ext(m.Exe), ".exe"):
		return errors.New("the add-on's program must be an .exe")
	case len(m.Args) > 16:
		return errors.New("too many arguments")
	}
	for _, a := range m.Args {
		if len(a) > 256 || strings.ContainsRune(a, 0) {
			return errors.New("an argument is too long")
		}
	}
	if len(m.Hooks) == 0 {
		return errors.New("the add-on asks for no hooks")
	}
	for _, h := range m.Hooks {
		if !knownHooks[h] {
			return fmt.Errorf("unknown hook %q", h)
		}
	}
	for _, p := range m.Permissions {
		if _, ok := Permissions[p]; !ok {
			return fmt.Errorf("unknown permission %q", p)
		}
	}
	return nil
}

// Discover reads every add-on under root (one folder per add-on). Broken
// manifests are returned with their error, so the interface can show them.
func Discover(root string) (found []*Manifest, broken map[string]error) {
	broken = map[string]error{}
	es, _ := os.ReadDir(root)
	seen := map[string]bool{}
	for _, e := range es {
		if !e.IsDir() {
			continue
		}
		m, err := ReadManifest(filepath.Join(root, e.Name(), "addon.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			broken[e.Name()] = err
			continue
		}
		if seen[m.ID] {
			broken[e.Name()] = fmt.Errorf("another add-on already uses the id %q", m.ID)
			continue
		}
		seen[m.ID] = true
		found = append(found, m)
	}
	sort.Slice(found, func(i, j int) bool { return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name) })
	return found, broken
}

// Install copies a picked addon.json into root\<id>\addon.json, with the
// program's path made absolute (it stays where it is).
func Install(root, manifestPath string) (*Manifest, error) {
	m, err := ReadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	m.Exe = m.ExePath()
	if fi, err := os.Stat(m.Exe); err != nil || !fi.Mode().IsRegular() {
		return nil, errors.New("the add-on's program wasn't found: " + m.Exe)
	}
	dir := filepath.Join(root, m.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "addon.json"), b, 0o644); err != nil {
		return nil, err
	}
	m.Dir = dir
	return m, nil
}

// HashFile returns a file's SHA-256 (hex).
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
