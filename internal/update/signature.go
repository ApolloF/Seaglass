package update

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
)

// Release signatures. Every release carries SHA256SUMS (the SHA-256 of each
// file) and SHA256SUMS.sig, an ed25519 signature made with a key that is
// kept offline, away from GitHub (tools/release). The updater only installs
// a download whose hash is in a SHA256SUMS signed by one of ReleaseKeys, so
// someone who takes over the GitHub account can't ship an update.
const (
	SumsAsset = "SHA256SUMS"
	SigAsset  = "SHA256SUMS.sig"
)

// SignedMessage is what a release signature covers: the tag as well as the
// hashes, so an old release can't be passed off under a newer tag.
func SignedMessage(tag string, sums []byte) []byte {
	return ProductMessage(defaultProduct, tag, sums)
}

// defaultProduct names Seaglass's own releases in their signatures.
const defaultProduct = "Seaglass"

// ProductMessage is what a signature of one of product's releases covers.
// The name keeps a signed release of one product from passing for
// another's under the same tag.
func ProductMessage(product, tag string, sums []byte) []byte {
	return append([]byte(product+" release "+tag+"\n"), sums...)
}

// FormatSums writes name → hash as sha256sum does, sorted by name.
func FormatSums(sums map[string]string) []byte {
	names := make([]string, 0, len(sums))
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var b bytes.Buffer
	for _, n := range names {
		b.WriteString(sums[n] + "  " + n + "\n")
	}
	return b.Bytes()
}

// ParseSums reads a SHA256SUMS file.
func ParseSums(b []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 || !isSHA256(f[0]) {
			return nil, errors.New("SHA256SUMS is unreadable")
		}
		out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
	}
	if len(out) == 0 {
		return nil, errors.New("SHA256SUMS is empty")
	}
	return out, sc.Err()
}

// EncodeSig and DecodeSig turn a signature into the .sig file's one line.
func EncodeSig(sig []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(sig) + "\n") }

func DecodeSig(b []byte) ([]byte, error) {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, errors.New("the release signature is unreadable")
	}
	return sig, nil
}

// VerifySums checks a release's SHA256SUMS against its signature with any
// of keys, and returns the hashes it lists.
func VerifySums(keys []ed25519.PublicKey, tag string, sums, sigFile []byte) (map[string]string, error) {
	return VerifyProductSums(defaultProduct, keys, tag, sums, sigFile)
}

// VerifyProductSums is VerifySums for product's releases.
func VerifyProductSums(product string, keys []ed25519.PublicKey, tag string, sums, sigFile []byte) (map[string]string, error) {
	sig, err := DecodeSig(sigFile)
	if err != nil {
		return nil, err
	}
	msg := ProductMessage(product, tag, sums)
	for _, k := range keys {
		if len(k) == ed25519.PublicKeySize && ed25519.Verify(k, msg, sig) {
			return ParseSums(sums)
		}
	}
	return nil, errors.New("the release isn't signed with " + product + "'s release key")
}

// ParseKey reads a base64 public key.
func ParseKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("not an ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

func mustKeys(ss ...string) []ed25519.PublicKey {
	var out []ed25519.PublicKey
	for _, s := range ss {
		k, err := ParseKey(s)
		if err != nil {
			panic(err)
		}
		out = append(out, k)
	}
	return out
}
