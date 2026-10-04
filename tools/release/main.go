//go:build windows

// Command release signs and publishes Seaglass releases with the
// offline release key (internal/update/signature.go).
//
//	go run ./tools/release keygen            make the key (once), print its public half
//	go run ./tools/release backup <file>     write a password-protected copy for offline keeping
//	go run ./tools/release restore <file>    put a backup on this PC
//	go run ./tools/release pubkey            print the public key
//	go run ./tools/release publish <tag>     sign the draft release <tag> and publish it
//	go run ./tools/release verify <tag>      check a published release's signature
//	go run ./tools/release cut <tag> [--pr N] [--dry-run]
//	                                         the whole release from main, checked step by step (cut.go)
//
// The private key is stored encrypted with Windows DPAPI (this Windows
// account only) in %APPDATA%\Seaglass-release, outside the app's data
// folder so uninstalling Seaglass can't delete it. It never goes to
// GitHub. publish uses the GitHub CLI (gh), signed in as a maintainer.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/update"
	"golang.org/x/sys/windows"
)

// repo is the GitHub repository origin points at: ApolloF/Seaglass, or
// ApolloF/WaterLauncher in a clone from before the rename.
var repo = originRepo()

func originRepo() string {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err == nil {
		u := strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
		if _, r, ok := strings.Cut(u, "github.com/"); ok && strings.Count(r, "/") == 1 {
			return r
		}
		if _, r, ok := strings.Cut(u, "github.com:"); ok && strings.Count(r, "/") == 1 {
			return r
		}
	}
	return "ApolloF/Seaglass"
}

// The files a release signs, besides the signature files themselves.
var signed = []string{update.InstallerAsset, update.ExeAsset}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch arg := func(i int) string {
		if len(os.Args) <= i {
			usage()
		}
		return os.Args[i]
	}; os.Args[1] {
	case "keygen":
		err = keygen()
	case "pubkey":
		var k ed25519.PrivateKey
		if k, err = load(); err == nil {
			fmt.Println(base64.StdEncoding.EncodeToString(k.Public().(ed25519.PublicKey)))
		}
	case "backup":
		err = backup(arg(2))
	case "restore":
		err = restore(arg(2))
	case "publish":
		err = publish(arg(2))
	case "verify":
		err = verify(arg(2))
	case "cut":
		tag, pr, dry := arg(2), "", false
		for i := 3; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--dry-run":
				dry = true
			case "--pr":
				pr = arg(i + 1)
				i++
			default:
				usage()
			}
		}
		err = cut(tag, pr, dry)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: release keygen | pubkey | backup <file> | restore <file> | publish <tag> | verify <tag> | cut <tag> [--pr N] [--dry-run]")
	os.Exit(2)
}

func keyPath() string {
	return filepath.Join(platform.Roaming, "Seaglass-release", "release-key.dpapi")
}

func load() (ed25519.PrivateKey, error) {
	enc, err := os.ReadFile(keyPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("no release key on this PC (keygen, or restore a backup)")
	}
	if err != nil {
		return nil, err
	}
	seed, err := platform.Unprotect(enc)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("the release key can't be read (another Windows account?)")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func store(seed []byte) error {
	if _, err := os.Stat(keyPath()); err == nil {
		return errors.New("a release key already exists at " + keyPath())
	}
	enc, err := platform.Protect(seed)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(keyPath(), enc, 0o600)
}

func keygen() error {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return err
	}
	if err := store(seed); err != nil {
		return err
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fmt.Println("Release key made and stored at", keyPath())
	fmt.Println("Public key (put it in internal/update/keys.go):")
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	fmt.Println("Now make an offline backup: go run ./tools/release backup <file>")
	return nil
}

// A backup: the seed encrypted with AES-256-GCM under a key derived from a
// password (PBKDF2-SHA256).
type backupFile struct {
	Version int    `json:"version"`
	Public  string `json:"public"`
	Iter    int    `json:"iter"`
	Salt    string `json:"salt"`
	Nonce   string `json:"nonce"`
	Box     string `json:"box"`
}

const pbkdfIter = 600_000

func backup(path string) error {
	k, err := load()
	if err != nil {
		return err
	}
	pw, err := password("Backup password: ")
	if err != nil {
		return err
	}
	if len(pw) < 12 {
		return errors.New("use at least 12 characters")
	}
	again, err := password("Again: ")
	if err != nil {
		return err
	}
	if pw != again {
		return errors.New("the passwords differ")
	}
	out, err := sealBackup(k, pw, pbkdfIter)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return err
	}
	fmt.Println("Backup written to", path, "- keep it offline, and the password somewhere else.")
	return nil
}

func restore(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	pw, err := password("Backup password: ")
	if err != nil {
		return err
	}
	seed, err := openBackup(raw, pw)
	if err != nil {
		return err
	}
	if err := store(seed); err != nil {
		return err
	}
	fmt.Println("Release key restored to", keyPath())
	return nil
}

// sealBackup encrypts k's seed under pw.
func sealBackup(k ed25519.PrivateKey, pw string, iter int) ([]byte, error) {
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	gcm, err := aead(pw, salt, iter)
	if err != nil {
		return nil, err
	}
	pub := k.Public().(ed25519.PublicKey)
	return json.MarshalIndent(backupFile{
		Version: 1, Public: base64.StdEncoding.EncodeToString(pub), Iter: iter,
		Salt: base64.StdEncoding.EncodeToString(salt), Nonce: base64.StdEncoding.EncodeToString(nonce),
		Box: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, k.Seed(), pub)),
	}, "", "  ")
}

// openBackup decrypts a backup's seed with pw.
func openBackup(raw []byte, pw string) ([]byte, error) {
	var b backupFile
	if err := json.Unmarshal(raw, &b); err != nil || b.Version != 1 || b.Iter < 100_000 {
		return nil, errors.New("not a release key backup")
	}
	salt, _ := base64.StdEncoding.DecodeString(b.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(b.Nonce)
	box, _ := base64.StdEncoding.DecodeString(b.Box)
	pub, _ := base64.StdEncoding.DecodeString(b.Public)
	gcm, err := aead(pw, salt, b.Iter)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("the backup is damaged")
	}
	seed, err := gcm.Open(nil, nonce, box, pub)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("wrong password, or the backup is damaged")
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey), pub) {
		return nil, errors.New("the backup is damaged")
	}
	return seed, nil
}

func aead(pw string, salt []byte, iter int) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, pw, salt, iter, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// password reads a line from the console without echoing it.
func password(prompt string) (string, error) {
	h := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return "", errors.New("run this in a terminal: it asks for a password")
	}
	fmt.Fprint(os.Stderr, prompt)
	_ = windows.SetConsoleMode(h, mode&^windows.ENABLE_ECHO_INPUT)
	defer func() {
		_ = windows.SetConsoleMode(h, mode)
		fmt.Fprintln(os.Stderr)
	}()
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if n == 0 || buf[0] == '\n' {
			break
		}
		if buf[0] != '\r' {
			line = append(line, buf[0])
		}
	}
	return string(line), nil
}

// publish signs a draft release and makes it public.
func publish(tag string) error {
	k, err := load()
	if err != nil {
		return err
	}
	if err := trusted(k, update.ReleaseKeys); err != nil {
		return err
	}
	var info struct {
		IsDraft bool `json:"isDraft"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := ghJSON(&info, "release", "view", tag, "--repo", repo, "--json", "isDraft,assets"); err != nil {
		return err
	}
	if !info.IsDraft {
		return errors.New(tag + " is already published")
	}
	dir, err := os.MkdirTemp("", "wl-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	args := []string{"release", "download", tag, "--repo", repo, "--dir", dir}
	for _, n := range signed {
		args = append(args, "--pattern", n, "--pattern", n+".sha256")
	}
	if err := gh(args...); err != nil {
		return err
	}
	sums := map[string]string{}
	for _, n := range signed {
		sum, err := update.FileSHA256(filepath.Join(dir, n))
		if err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
		pub, err := os.ReadFile(filepath.Join(dir, n+".sha256"))
		if err != nil {
			return err
		}
		if f := strings.Fields(string(pub)); len(f) == 0 || !strings.EqualFold(f[0], sum) {
			return fmt.Errorf("%s doesn't match the SHA-256 CI published for it", n)
		}
		sums[n] = sum
		fmt.Printf("%s  %s\n", sum, n)
	}
	list := update.FormatSums(sums)
	sig := update.EncodeSig(ed25519.Sign(k, update.SignedMessage(tag, list)))
	if _, err := update.VerifySums([]ed25519.PublicKey{k.Public().(ed25519.PublicKey)}, tag, list, sig); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, update.SumsAsset), list, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, update.SigAsset), sig, 0o644); err != nil {
		return err
	}
	if err := gh("release", "upload", tag, "--repo", repo, "--clobber",
		filepath.Join(dir, update.SumsAsset), filepath.Join(dir, update.SigAsset)); err != nil {
		return err
	}
	edit := []string{"release", "edit", tag, "--repo", repo, "--draft=false"}
	if !strings.Contains(tag, "-") && !strings.HasPrefix(tag, "v0.") {
		edit = append(edit, "--latest")
	}
	if err := gh(edit...); err != nil {
		return err
	}
	fmt.Println("Signed and published", tag)
	return verify(tag)
}

// trusted fails when keys (the updater's release keys) is set but doesn't
// hold k's public half: updaters would reject everything k signs.
func trusted(k ed25519.PrivateKey, keys []ed25519.PublicKey) error {
	pub := k.Public().(ed25519.PublicKey)
	if len(keys) == 0 || slices.ContainsFunc(keys, func(r ed25519.PublicKey) bool { return r.Equal(pub) }) {
		return nil
	}
	return fmt.Errorf("this PC's release key (%s) isn't in internal/update/keys.go; updaters would reject the release", base64.StdEncoding.EncodeToString(pub))
}

// verify checks a published release against the keys the updater knows.
func verify(tag string) error {
	dir, err := os.MkdirTemp("", "wl-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := gh("release", "download", tag, "--repo", repo, "--dir", dir, "--pattern", update.SumsAsset, "--pattern", update.SigAsset); err != nil {
		return err
	}
	list, _ := os.ReadFile(filepath.Join(dir, update.SumsAsset))
	sig, _ := os.ReadFile(filepath.Join(dir, update.SigAsset))
	// Check exactly what installed updaters check; only while keys.go has
	// no key yet does this PC's key stand in for it.
	keys := append([]ed25519.PublicKey(nil), update.ReleaseKeys...)
	if len(keys) == 0 {
		if k, err := load(); err == nil {
			keys = append(keys, k.Public().(ed25519.PublicKey))
		}
	}
	sums, err := update.VerifySums(keys, tag, list, sig)
	if err != nil {
		return err
	}
	for n, s := range sums {
		fmt.Printf("ok  %s  %s\n", s[:16], n)
	}
	if len(update.ReleaseKeys) == 0 {
		fmt.Println("note: internal/update/keys.go has no release key yet; this build's updater doesn't check signatures")
	}
	return nil
}

func gh(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func ghJSON(v any, args ...string) error {
	var out bytes.Buffer
	cmd := exec.Command("gh", args...)
	cmd.Stdout, cmd.Stderr = &out, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return json.Unmarshal(out.Bytes(), v)
}
