package platform

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var reSecretName = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

func secretPath(name string) (string, error) {
	if !reSecretName.MatchString(name) {
		return "", errors.New("bad secret name")
	}
	return filepath.Join(AppDir(), "secrets", name+".bin"), nil
}

// SaveSecret stores a secret (an API key) encrypted with Windows DPAPI,
// so only this Windows user on this PC can read it back. An empty value
// deletes it.
func SaveSecret(name, value string) error {
	p, err := secretPath(name)
	if err != nil {
		return err
	}
	if value == "" {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	enc, err := Protect([]byte(value))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	// Through a flushed temporary file: a crash mid-write mustn't leave a
	// half secret, which reads as signed out.
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(enc); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// LoadSecret reads a secret stored with SaveSecret; "" when there is none.
func LoadSecret(name string) string {
	p, err := secretPath(name)
	if err != nil {
		return ""
	}
	enc, err := os.ReadFile(p)
	if err != nil || len(enc) == 0 {
		return ""
	}
	b, err := Unprotect(enc)
	if err != nil {
		return ""
	}
	return string(b)
}
