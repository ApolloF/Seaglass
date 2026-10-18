//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/update"
)

// cut releases a version from main, one checked step at a time; the first
// failing check stops it before anything that can't be undone.
//
//	go run ./tools/release cut v1.2.0 [--pr 12] [--dry-run]
//
//  1. This PC's release key is one the updater trusts.
//  2. The tag is a version, its notes exist, it isn't used yet.
//  3. The working tree is clean, on main, level with origin/main.
//  4. With --pr: that pull request is merged and its head is in main.
//  5. CI passed for main's head (waits while it runs).
//  6. "Version X.Y.Z" is committed (build/windows/info.json) and pushed.
//  7. The tag is pushed; CI builds and tests it and makes a draft release.
//  8. publish signs the draft with the release key and makes it public.
func cut(tag string, pr string, dry bool) error {
	num, err := versionNumber(tag)
	if err != nil {
		return err
	}
	step := func(format string, args ...any) { fmt.Printf("• "+format+"\n", args...) }

	step("checking the release key")
	k, err := load()
	if err != nil {
		return err
	}
	if err := trusted(k, update.ReleaseKeys); err != nil {
		return err
	}

	step("checking the working tree")
	if out, err := git("status", "--porcelain"); err != nil || out != "" {
		return errors.New("the working tree has changes; commit or stash them first")
	}
	if b, err := git("rev-parse", "--abbrev-ref", "HEAD"); err != nil || b != "main" {
		return fmt.Errorf("on branch %q; release from main", b)
	}
	if _, err := git("fetch", "--quiet", "origin", "main", "--tags"); err != nil {
		return err
	}
	head, _ := git("rev-parse", "HEAD")
	remote, _ := git("rev-parse", "origin/main")
	if head != remote {
		return errors.New("main isn't level with origin/main; pull (or push) first")
	}
	if _, err := os.Stat("docs/releases/" + tag + ".md"); err != nil {
		return fmt.Errorf("docs/releases/%s.md is missing: write the release notes first", tag)
	}
	if out, _ := git("ls-remote", "--tags", "origin", "refs/tags/"+tag); out != "" {
		return fmt.Errorf("the tag %s exists already", tag)
	}
	if out, _ := git("tag", "--list", tag); out != "" {
		return fmt.Errorf("the tag %s exists locally", tag)
	}

	if pr != "" {
		step("checking pull request #%s", pr)
		var p struct {
			State      string `json:"state"`
			HeadRefOid string `json:"headRefOid"`
		}
		if err := ghJSON(&p, "pr", "view", pr, "--repo", repo, "--json", "state,headRefOid"); err != nil {
			return err
		}
		if p.State != "MERGED" {
			return fmt.Errorf("pull request #%s is %s, not merged", pr, strings.ToLower(p.State))
		}
		if _, err := git("merge-base", "--is-ancestor", p.HeadRefOid, "HEAD"); err != nil {
			return fmt.Errorf("pull request #%s's last commit isn't in main", pr)
		}
	}

	step("waiting for CI on main (%s)", head[:9])
	if err := waitRun("main", head, "push"); err != nil {
		return err
	}
	if dry {
		step("dry run: would commit \"Version %s\", tag and push %s, wait for CI and publish", num, tag)
		return nil
	}

	step("committing Version %s", num)
	changed, err := bumpVersionFile("build/windows/info.json", num)
	if err != nil {
		return err
	}
	if changed {
		if _, err := git("commit", "-q", "-am", "Version "+num); err != nil {
			return err
		}
	}
	if _, err := git("push", "-q", "origin", "main"); err != nil {
		return fmt.Errorf("pushing main: %w", err)
	}
	head, _ = git("rev-parse", "HEAD")

	step("tagging %s", tag)
	if _, err := git("tag", tag); err != nil {
		return err
	}
	if _, err := git("push", "-q", "origin", tag); err != nil {
		return fmt.Errorf("pushing the tag: %w", err)
	}
	if out, _ := git("ls-remote", "--tags", "origin", "refs/tags/"+tag); !strings.HasPrefix(out, head) {
		return fmt.Errorf("the tag on GitHub doesn't point at %s", head[:9])
	}

	step("waiting for CI to build %s", tag)
	if err := waitRun(tag, head, "push"); err != nil {
		return err
	}
	var rel struct {
		IsDraft bool `json:"isDraft"`
	}
	if err := ghJSON(&rel, "release", "view", tag, "--repo", repo, "--json", "isDraft"); err != nil {
		return fmt.Errorf("CI made no release for %s: %w", tag, err)
	}
	if !rel.IsDraft {
		return fmt.Errorf("the release %s is already public", tag)
	}

	step("signing and publishing %s", tag)
	return publish(tag)
}

var reTag = regexp.MustCompile(`^v(\d+\.\d+\.\d+)(-[0-9A-Za-z.]+)?$`)

// versionNumber is the plain version of a tag: "1.2.0" for "v1.2.0-beta.1".
func versionNumber(tag string) (string, error) {
	m := reTag.FindStringSubmatch(tag)
	if m == nil {
		return "", fmt.Errorf("%q isn't a version tag like v1.2.0", tag)
	}
	return m[1], nil
}

var reVersionField = regexp.MustCompile(`("(?:file_version|ProductVersion)"\s*:\s*")[^"]*(")`)

// bumpVersionFile sets the version fields of build/windows/info.json,
// keeping its layout. It reports whether anything changed.
func bumpVersionFile(path, num string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if n := len(reVersionField.FindAll(b, -1)); n != 2 {
		return false, fmt.Errorf("%s has %d version fields, expected 2", path, n)
	}
	out := reVersionField.ReplaceAll(b, []byte("${1}"+num+"${2}"))
	if string(out) == string(b) {
		return false, nil
	}
	return true, os.WriteFile(path, out, 0o644)
}

// waitRun waits for the CI run of commit sha on branch (or tag) and
// requires it to succeed.
func waitRun(branch, sha, event string) error {
	start := time.Now()
	deadline := start.Add(40 * time.Minute)
	seen := false
	for time.Now().Before(deadline) {
		var runs []struct {
			ID         int64  `json:"databaseId"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			SHA        string `json:"headSha"`
			URL        string `json:"url"`
		}
		if err := ghJSON(&runs, "run", "list", "--repo", repo, "--branch", branch, "--event", event,
			"--limit", "10", "--json", "databaseId,status,conclusion,headSha,url"); err != nil {
			return err
		}
		for _, r := range runs {
			if r.SHA != sha {
				continue
			}
			seen = true
			if r.Status != "completed" {
				break
			}
			if r.Conclusion != "success" {
				return fmt.Errorf("CI %s for %s: %s", r.Conclusion, branch, r.URL)
			}
			fmt.Printf("  CI passed: %s\n", r.URL)
			return nil
		}
		if !seen && time.Since(start) > 5*time.Minute {
			return fmt.Errorf("no CI run for %s (%s) showed up", branch, sha[:9])
		}
		time.Sleep(20 * time.Second)
	}
	return fmt.Errorf("CI for %s didn't finish in 40 minutes", branch)
}

// git runs git and returns its trimmed output; a failure includes what
// git said.
func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %v %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
