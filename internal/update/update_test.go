package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ApolloF/Seaglass/internal/platform"
)

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"v1.0.0", "v0.7.0", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0", "v0.99.99", true},
		{"1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.9.9", "v1.0.0", false},
		{"v1.1.0-beta", "v1.0.0", true},
		{"v1.0.0", "dev", false},
		{"dev", "v0.1.0", false},
		{"v1.2.3.4", "v1.0.0", false},
		{"", "v1.0.0", false},
	} {
		if got := Newer(tt.a, tt.b); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// fakeGitHub serves a latest-release answer and its assets over TLS. The
// feed's prefix and host allowlist point at it.
type fakeGitHub struct {
	srv     *httptest.Server
	release map[string]any
	files   map[string][]byte
}

func newFake(t *testing.T) (*fakeGitHub, Feed) {
	f := &fakeGitHub{files: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if f.release == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.release)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/download/redirect-") {
			http.Redirect(w, r, "https://evil.example/x", http.StatusFound)
			return
		}
		b, ok := f.files[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	u, _ := url.Parse(f.srv.URL)
	return f, Feed{
		LatestURL:   f.srv.URL + "/latest",
		AssetPrefix: f.srv.URL + "/download/",
		Hosts:       []string{u.Hostname()},
		Client:      f.srv.Client(),
	}
}

func (f *fakeGitHub) publish(tag string, files map[string][]byte, extra ...map[string]any) {
	var assets []map[string]any
	for name, b := range files {
		f.files[name] = b
		assets = append(assets, map[string]any{"name": name, "size": len(b), "browser_download_url": f.srv.URL + "/download/" + name})
	}
	for _, e := range extra {
		assets = append(assets, e)
	}
	f.release = map[string]any{"tag_name": tag, "body": "notes", "html_url": "https://github.com/ApolloF/Seaglass/releases/tag/" + tag, "assets": assets}
}

func sumLine(b []byte, name string) []byte {
	h := sha256.Sum256(b)
	return []byte(hex.EncodeToString(h[:]) + "  " + name + "\n")
}

func TestLatestAndDownload(t *testing.T) {
	f, feed := newFake(t)
	ctx := context.Background()
	if _, err := feed.Latest(ctx); err != ErrNoRelease {
		t.Fatalf("no release: err = %v", err)
	}
	body := []byte("MZ pretend installer")
	f.publish("v1.2.0", map[string][]byte{
		InstallerAsset:             body,
		InstallerAsset + ".sha256": sumLine(body, InstallerAsset),
	}, map[string]any{"name": "elsewhere.exe", "size": 1, "browser_download_url": "https://example.com/elsewhere.exe"})
	rel, err := feed.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v1.2.0" || rel.Notes != "notes" {
		t.Errorf("release = %+v", rel)
	}
	if _, ok := rel.Asset("elsewhere.exe"); ok {
		t.Error("an asset outside the release prefix was accepted")
	}
	dir := t.TempDir()
	var last int64
	p, sum, err := feed.Download(ctx, rel, InstallerAsset, dir, func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != string(body) {
		t.Errorf("downloaded %q", b)
	}
	if want, _ := FileSHA256(p); sum != want || last != int64(len(body)) {
		t.Errorf("sum %s want %s, progress %d", sum, want, last)
	}
	if filepath.Base(p) != "v1.2.0-"+InstallerAsset {
		t.Errorf("saved as %s", p)
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	f, feed := newFake(t)
	body := []byte("MZ tampered")
	f.publish("v1.2.0", map[string][]byte{
		ExeAsset:             body,
		ExeAsset + ".sha256": sumLine([]byte("MZ original"), ExeAsset),
	})
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, _, err := feed.Download(context.Background(), rel, ExeAsset, dir, nil); err == nil {
		t.Fatal("a file that doesn't match its checksum was accepted")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left behind %d file(s)", len(left))
	}
	// A checksum file for another file is refused too.
	f.publish("v1.2.0", map[string][]byte{ExeAsset: body, ExeAsset + ".sha256": sumLine(body, "other.exe")})
	rel, _ = feed.Latest(context.Background())
	if _, _, err := feed.Download(context.Background(), rel, ExeAsset, dir, nil); err == nil {
		t.Fatal("a checksum naming another file was accepted")
	}
}

func TestDownloadRefusesForeignRedirect(t *testing.T) {
	f, feed := newFake(t)
	f.publish("v1.2.0", nil,
		map[string]any{"name": ExeAsset, "size": 3, "browser_download_url": f.srv.URL + "/download/redirect-x"},
		map[string]any{"name": ExeAsset + ".sha256", "size": 3, "browser_download_url": f.srv.URL + "/download/redirect-y"},
		map[string]any{"name": "sneaky.exe", "size": 3, "browser_download_url": f.srv.URL + "/download/../elsewhere/x"})
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rel.Asset("sneaky.exe"); ok {
		t.Error("an asset URL climbing out of the release prefix was accepted")
	}
	if _, _, err := feed.Download(context.Background(), rel, ExeAsset, t.TempDir(), nil); err == nil ||
		!strings.Contains(err.Error(), "isn't allowed") {
		t.Fatalf("redirect to another host: err = %v", err)
	}
}

func TestPrereleaseIgnored(t *testing.T) {
	f, feed := newFake(t)
	f.publish("v2.0.0-beta", nil)
	f.release["prerelease"] = true
	if _, err := feed.Latest(context.Background()); err != ErrNoRelease {
		t.Fatalf("prerelease: err = %v", err)
	}
}

func TestPendingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "v1.2.0-"+ExeAsset)
	if err := os.WriteFile(file, []byte("MZ new"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, _ := FileSHA256(file)
	if err := SavePending(dir, Pending{Tag: "v1.2.0", File: file, SHA256: sum, Kind: KindExe}); err != nil {
		t.Fatal(err)
	}
	p, ok := LoadPending(dir)
	if !ok || p.Tag != "v1.2.0" {
		t.Fatalf("LoadPending = %+v, %v", p, ok)
	}
	// This test binary isn't signed, so only the hash counts.
	self, _ := os.Executable()
	if err := p.Check(self); err != nil {
		t.Errorf("Check: %v", err)
	}
	_ = os.WriteFile(file, []byte("MZ changed"), 0o644)
	if err := p.Check(self); err == nil {
		t.Error("a changed download passed")
	}
	// A pending file pointing outside its folder is ignored.
	_ = SavePending(dir, Pending{Tag: "v1.2.0", File: `C:\Windows\notepad.exe`, SHA256: sum, Kind: KindExe})
	if _, ok := LoadPending(dir); ok {
		t.Error("pending.json pointing outside the updates folder was accepted")
	}
	Clear(dir, "")
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("Clear left %d file(s)", len(left))
	}
}

func TestSwapExe(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Seaglass.exe")
	next := filepath.Join(dir, "next.exe")
	_ = os.WriteFile(exe, []byte("old"), 0o755)
	_ = os.WriteFile(next, []byte("new"), 0o755)
	if KindFor(exe) != KindExe {
		t.Errorf("KindFor(writable folder) = %q", KindFor(exe))
	}
	if err := SwapExe(next, exe); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("exe holds %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old" {
		t.Errorf("old exe holds %q", b)
	}
	CleanOld(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Error("CleanOld left the old exe")
	}
	_ = os.WriteFile(filepath.Join(dir, "uninstall.exe"), nil, 0o755)
	if KindFor(exe) != KindInstaller {
		t.Errorf("KindFor(installed) = %q", KindFor(exe))
	}
}

func TestCheckPublisher(t *testing.T) {
	gh := filepath.Join(platform.ProgramFiles, "GitHub CLI", "gh.exe")
	git := filepath.Join(platform.ProgramFiles, "Git", "cmd", "git.exe")
	if _, err := platform.Signer(gh); err != nil {
		t.Skip("no signed gh.exe to test with")
	}
	self, _ := os.Executable() // unsigned
	if err := CheckPublisher(gh, gh); err != nil {
		t.Errorf("same publisher refused: %v", err)
	}
	if err := CheckPublisher(self, gh); err == nil {
		t.Error("an unsigned update was accepted by a signed build")
	}
	if _, err := platform.Signer(git); err == nil {
		if err := CheckPublisher(git, gh); err == nil {
			t.Error("an update from another publisher was accepted")
		}
	}
	if err := CheckPublisher(gh, self); err != nil {
		t.Errorf("an unsigned build refused a signed update: %v", err)
	}
}

func TestSignedRelease(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	f, feed := newFake(t)
	feed.Keys = []ed25519.PublicKey{pub}
	body := []byte("MZ signed installer")
	h := sha256.Sum256(body)
	sums := FormatSums(map[string]string{InstallerAsset: hex.EncodeToString(h[:])})
	sign := func(tag string, s []byte) []byte { return EncodeSig(ed25519.Sign(priv, SignedMessage(tag, s))) }
	ctx := context.Background()
	try := func(tag string, files map[string][]byte) error {
		f.publish(tag, files)
		rel, err := feed.Latest(ctx)
		if err != nil {
			return err
		}
		_, _, err = feed.Download(ctx, rel, InstallerAsset, t.TempDir(), nil)
		return err
	}
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: sign("v1.2.0", sums)}); err != nil {
		t.Fatalf("signed release refused: %v", err)
	}
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums}); err == nil {
		t.Error("a release without a signature was accepted")
	}
	// An old signed release republished under a newer tag.
	if err := try("v9.0.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: sign("v1.2.0", sums)}); err == nil {
		t.Error("a signature for another tag was accepted")
	}
	// The file swapped, with its hash in a list that's no longer signed.
	other := []byte("MZ evil")
	h2 := sha256.Sum256(other)
	sums2 := FormatSums(map[string]string{InstallerAsset: hex.EncodeToString(h2[:])})
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: other, SumsAsset: sums2, SigAsset: sign("v1.2.0", sums)}); err == nil {
		t.Error("a changed SHA256SUMS was accepted")
	}
	// Signed by someone else.
	_, priv2, _ := ed25519.GenerateKey(nil)
	sig2 := EncodeSig(ed25519.Sign(priv2, SignedMessage("v1.2.0", sums)))
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: body, SumsAsset: sums, SigAsset: sig2}); err == nil {
		t.Error("a signature by another key was accepted")
	}
	// Right list, wrong file.
	if err := try("v1.2.0", map[string][]byte{InstallerAsset: other, SumsAsset: sums, SigAsset: sign("v1.2.0", sums)}); err == nil {
		t.Error("a file not matching the signed list was accepted")
	}
}

// A feed for another product (Syncer) checks signatures made for that
// product, so a signed release of one can't pass for the other's.
func TestSignatureIsBoundToProduct(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	f, feed := newFake(t)
	feed.Keys, feed.Product = []ed25519.PublicKey{pub}, "Syncer"
	const name = "Syncer-amd64-installer.exe"
	body := []byte("MZ syncer installer")
	h := sha256.Sum256(body)
	sums := FormatSums(map[string]string{name: hex.EncodeToString(h[:])})
	ctx := context.Background()
	try := func(msg []byte) error {
		f.publish("v1.3.0", map[string][]byte{name: body, SumsAsset: sums, SigAsset: EncodeSig(ed25519.Sign(priv, msg))})
		rel, err := feed.Latest(ctx)
		if err != nil {
			return err
		}
		_, _, err = feed.Download(ctx, rel, name, t.TempDir(), nil)
		return err
	}
	if err := try(ProductMessage("Syncer", "v1.3.0", sums)); err != nil {
		t.Fatalf("Syncer-signed release refused: %v", err)
	}
	if err := try(SignedMessage("v1.3.0", sums)); err == nil {
		t.Error("a signature over a Seaglass release was accepted for Syncer")
	}
}

func TestLatestRefusesPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.0.0"})
	}))
	t.Cleanup(srv.Close)
	feed := Feed{LatestURL: srv.URL + "/latest", AssetPrefix: srv.URL + "/download/"}
	if _, err := feed.Latest(context.Background()); err == nil {
		t.Fatal("asked for releases over plain HTTP")
	}
}

// Releases without checksum files (like Syncer's) are checked against the
// SHA-256 GitHub computed on upload.
func TestDownloadUsesGitHubDigest(t *testing.T) {
	f, feed := newFake(t)
	ctx := context.Background()
	body := []byte("MZ pretend Syncer installer")
	h := sha256.Sum256(body)
	f.files["Syncer-setup.exe"] = body
	f.publish("v0.12.0", nil, map[string]any{
		"name": "Syncer-setup.exe", "size": len(body), "digest": "sha256:" + hex.EncodeToString(h[:]),
		"browser_download_url": f.srv.URL + "/download/Syncer-setup.exe",
	})
	rel, err := feed.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := feed.Download(ctx, rel, "Syncer-setup.exe", t.TempDir(), nil); err != nil {
		t.Fatalf("download with a digest: %v", err)
	}
	f.publish("v0.12.1", nil, map[string]any{
		"name": "Syncer-setup.exe", "size": len(body), "digest": "sha256:" + strings.Repeat("0", 64),
		"browser_download_url": f.srv.URL + "/download/Syncer-setup.exe",
	})
	if rel, err = feed.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := feed.Download(ctx, rel, "Syncer-setup.exe", t.TempDir(), nil); err == nil {
		t.Error("a file that doesn't match its digest was accepted")
	}
}

func TestLongNotesStayValidUTF8(t *testing.T) {
	f, feed := newFake(t)
	f.publish("v2.0.0", nil)
	f.release["body"] = strings.Repeat("a", maxNotes-1) + "—more"
	rel, err := feed.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(rel.Notes) || !strings.HasSuffix(rel.Notes, "a…") {
		t.Errorf("notes end in %q", rel.Notes[len(rel.Notes)-8:])
	}
}
