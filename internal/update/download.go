package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxSize is the largest download accepted (the installer is ~15 MB).
const MaxSize = 128 << 20

// Download fetches the release asset called name into dir and checks it:
// against the release's signed SHA256SUMS when the feed has release keys,
// else against its published "<name>.sha256", or else the SHA-256 GitHub
// computed when the file was uploaded. It returns the file's path
// and SHA-256. progress (optional) hears how far it got.
func (f Feed) Download(ctx context.Context, rel Release, name, dir string, progress func(done, total int64)) (string, string, error) {
	a, ok := rel.Asset(name)
	if !ok {
		return "", "", fmt.Errorf("%s has no %s", rel.Tag, name)
	}
	if a.Size > MaxSize {
		return "", "", fmt.Errorf("%s is too large", name)
	}
	want, err := f.expected(ctx, rel, name)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	dst := filepath.Join(dir, safeName(rel.Tag)+"-"+name)
	part := dst + ".part"
	got, err := f.fetch(ctx, a, part, progress)
	if err != nil {
		_ = os.Remove(part)
		return "", "", err
	}
	if got != want {
		_ = os.Remove(part)
		return "", "", fmt.Errorf("%s doesn't match its published SHA-256", name)
	}
	if err := os.Rename(part, dst); err != nil {
		_ = os.Remove(part)
		return "", "", err
	}
	return dst, got, nil
}

// expected is the SHA-256 the release promises for name.
func (f Feed) expected(ctx context.Context, rel Release, name string) (string, error) {
	if len(f.Keys) == 0 {
		if s, ok := rel.Asset(name + ".sha256"); ok {
			return f.checksum(ctx, s, name)
		}
		a, _ := rel.Asset(name)
		if d, ok := strings.CutPrefix(a.Digest, "sha256:"); ok && isSHA256(d) {
			return strings.ToLower(d), nil
		}
		return "", fmt.Errorf("%s has no checksum for %s", rel.Tag, name)
	}
	sa, ok1 := rel.Asset(SumsAsset)
	ga, ok2 := rel.Asset(SigAsset)
	if !ok1 || !ok2 {
		return "", fmt.Errorf("%s isn't signed", rel.Tag)
	}
	sums, err := f.small(ctx, sa)
	if err != nil {
		return "", err
	}
	sig, err := f.small(ctx, ga)
	if err != nil {
		return "", err
	}
	list, err := VerifyProductSums(f.product(), f.Keys, rel.Tag, sums, sig)
	if err != nil {
		return "", err
	}
	want, ok := list[name]
	if !ok {
		return "", fmt.Errorf("%s isn't in the signed list of %s", name, rel.Tag)
	}
	return want, nil
}

// small downloads a small text asset whole.
func (f Feed) small(ctx context.Context, a Asset) ([]byte, error) {
	resp, err := f.get(ctx, a.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<10 {
		return nil, errors.New(a.Name + " is too large")
	}
	return b, nil
}

// checksum reads a "<hex>  <name>" file (sha256sum's format).
func (f Feed) checksum(ctx context.Context, a Asset, name string) (string, error) {
	resp, err := f.get(ctx, a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(io.LimitReader(resp.Body, 1024)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	fields := strings.Fields(line)
	if len(fields) == 0 || !isSHA256(fields[0]) {
		return "", errors.New("the published checksum is unreadable")
	}
	if len(fields) > 1 && !strings.EqualFold(strings.TrimPrefix(fields[1], "*"), name) {
		return "", errors.New("the published checksum is for another file")
	}
	return strings.ToLower(fields[0]), nil
}

func (f Feed) fetch(ctx context.Context, a Asset, dst string, progress func(done, total int64)) (string, error) {
	resp, err := f.get(ctx, a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.ContentLength > MaxSize {
		return "", errors.New("the download is too large")
	}
	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	w := &counter{w: io.MultiWriter(out, h), total: total, progress: progress}
	n, err := io.Copy(w, io.LimitReader(resp.Body, MaxSize+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if n > MaxSize {
		return "", errors.New("the download is too large")
	}
	if a.Size > 0 && n != a.Size {
		return "", errors.New("the download was cut short")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (f Feed) get(ctx context.Context, raw string) (*http.Response, error) {
	if !f.inRelease(raw) {
		return nil, errors.New("refusing to download from outside the release")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Seaglass (+https://github.com/ApolloF/Seaglass)")
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, errors.New("download failed: " + resp.Status)
	}
	return resp, nil
}

type counter struct {
	w        io.Writer
	done     int64
	total    int64
	progress func(done, total int64)
	last     int64
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.done += int64(n)
	// Report every 256 KiB or so, not every packet.
	if c.progress != nil && (c.done-c.last >= 256<<10 || c.done == c.total) {
		c.last = c.done
		c.progress(c.done, c.total)
	}
	return n, err
}

// FileSHA256 hashes a file.
func FileSHA256(p string) (string, error) {
	f, err := os.Open(p)
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

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// safeName keeps a tag usable as part of a file name.
func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, s)
}
