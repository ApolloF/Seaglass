package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// How an update is put in place.
const (
	KindInstaller = "installer" // Seaglass was installed: run the new installer silently
	KindExe       = "exe"       // a copy that wasn't installed: swap the exe
)

// Pending is a downloaded, checked update waiting to be installed. It is
// kept in pending.json next to the download.
type Pending struct {
	Tag      string `json:"tag"`
	File     string `json:"file"`
	SHA256   string `json:"sha256"`
	Kind     string `json:"kind"`
	Attempts int    `json:"attempts"` // installs started; one that didn't take isn't retried on its own
}

const pendingFile = "pending.json"

// LoadPending reads the waiting update in dir, if there is one.
func LoadPending(dir string) (Pending, bool) {
	b, err := os.ReadFile(filepath.Join(dir, pendingFile))
	if err != nil {
		return Pending{}, false
	}
	var p Pending
	if json.Unmarshal(b, &p) != nil || !Valid(p.Tag) || (p.Kind != KindInstaller && p.Kind != KindExe) ||
		!platform.Within(dir, p.File) || !isSHA256(p.SHA256) {
		return Pending{}, false
	}
	return p, true
}

// SavePending records the waiting update.
func SavePending(dir string, p Pending) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, pendingFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, pendingFile))
}

// Clear removes downloads in dir except keep (a file path, or "").
func Clear(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() || strings.EqualFold(p, keep) || (keep != "" && e.Name() == pendingFile) {
			continue
		}
		_ = os.Remove(p)
	}
}

// Check makes sure the downloaded file is still the one that was checked,
// and, when running is signed, that it's signed by the same publisher.
func (p Pending) Check(running string) error {
	got, err := FileSHA256(p.File)
	if err != nil {
		return err
	}
	if got != p.SHA256 {
		return errors.New("the downloaded update changed on disk")
	}
	return CheckPublisher(p.File, running)
}

// CheckPublisher refuses a file that isn't signed by the running exe's
// publisher. While Seaglass itself is unsigned, the SHA-256 published
// with the release is the check.
func CheckPublisher(file, running string) error {
	want, err := platform.Signer(running)
	if err != nil {
		return nil // this build isn't signed
	}
	got, err := platform.Signer(file)
	if err != nil {
		return fmt.Errorf("the update isn't signed: %w", err)
	}
	if got != want {
		return fmt.Errorf("the update is signed by %q, not %q", got, want)
	}
	return nil
}
