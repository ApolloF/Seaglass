//go:build windows

package main

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupRoundTrip(t *testing.T) {
	_, k, _ := ed25519.GenerateKey(nil)
	raw, err := sealBackup(k, "correct horse battery staple", 100_000)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := openBackup(raw, "correct horse battery staple")
	if err != nil || !bytes.Equal(seed, k.Seed()) {
		t.Fatalf("restore: %v", err)
	}
	if _, err := openBackup(raw, "wrong password here"); err == nil {
		t.Error("a wrong password opened the backup")
	}
	bad := bytes.Replace(raw, []byte(`"box": "`), []byte(`"box": "AA`), 1)
	if _, err := openBackup(bad, "correct horse battery staple"); err == nil {
		t.Error("a damaged backup opened")
	}
}

func TestVersionNumber(t *testing.T) {
	for tag, want := range map[string]string{"v1.2.0": "1.2.0", "v1.2.0-beta.1": "1.2.0", "v10.0.3": "10.0.3"} {
		if got, err := versionNumber(tag); err != nil || got != want {
			t.Errorf("versionNumber(%q) = %q, %v", tag, got, err)
		}
	}
	for _, bad := range []string{"1.2.0", "v1.2", "v1.2.0 ", "v1.2.0;rm", "main"} {
		if _, err := versionNumber(bad); err == nil {
			t.Errorf("versionNumber(%q) accepted", bad)
		}
	}
}

func TestBumpVersionFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "info.json")
	in := "{\n\t\"fixed\": {\n\t\t\"file_version\": \"1.1.0\"\n\t},\n\t\"info\": {\n\t\t\"0409\": {\n\t\t\t\"ProductVersion\": \"1.1.0\",\n\t\t\t\"CompanyName\": \"ApolloF\"\n\t\t}\n\t}\n}"
	os.WriteFile(p, []byte(in), 0o644)
	changed, err := bumpVersionFile(p, "1.2.0")
	if err != nil || !changed {
		t.Fatalf("bump: %v, %v", changed, err)
	}
	b, _ := os.ReadFile(p)
	if want := strings.ReplaceAll(in, "1.1.0", "1.2.0"); string(b) != want {
		t.Errorf("got\n%s", b)
	}
	if changed, _ := bumpVersionFile(p, "1.2.0"); changed {
		t.Error("an unchanged version reported a change")
	}
}

func TestTrusted(t *testing.T) {
	pub, k, _ := ed25519.GenerateKey(nil)
	other, _, _ := ed25519.GenerateKey(nil)
	if err := trusted(k, nil); err != nil {
		t.Errorf("no keys yet: %v", err)
	}
	if err := trusted(k, []ed25519.PublicKey{other, pub}); err != nil {
		t.Errorf("listed key: %v", err)
	}
	if err := trusted(k, []ed25519.PublicKey{other}); err == nil {
		t.Error("a key missing from keys.go was trusted")
	}
}
