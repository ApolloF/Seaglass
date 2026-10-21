# Releasing

From v1.1, every release is signed with Seaglass's **release key**, an ed25519 key that never goes to GitHub. The updater (v1.1 and later) installs only files whose SHA-256 is listed in a `SHA256SUMS` signed with that key for that exact tag. So even someone who took over the GitHub account, or replaced release files, can't ship an update.

## The key

- It lives on the maintainer's PC, encrypted with Windows DPAPI for that Windows account: `%APPDATA%\Seaglass-release\release-key.dpapi` (outside Seaglass's own data folder, so uninstalling the app can't delete it).
- Its public half is in `internal/update/keys.go`.
- **Backup** (do this once, and keep it offline, apart from its password):

  ```bash
  go run ./tools/release backup E:\seaglass-release-key.json
  ```

  The file is the key encrypted with AES-256-GCM under your password (PBKDF2-SHA256, 600,000 rounds). Without a backup, a lost PC means v1.1+ users can't receive updates until they install a new version by hand.
- On a new PC: `go run ./tools/release restore <file>`.
- Rotating: add the new public key to `keys.go` next to the old one, release that version (signed with the old key), then sign with the new key and remove the old one a release later.

## Steps

1. Merge the release branch into `main` and write `docs/releases/vX.Y.Z.md` (on the branch, or on `main` before the next step).
2. `main` only takes pull requests, so the version bump goes in through one: set the version in `build/windows/info.json` to X.Y.Z on a branch, open a PR "Version X.Y.Z" and let it merge. Then, from a clean `main`, level with GitHub:

   ```bash
   go run ./tools/release cut vX.Y.Z
   ```

   Leave out `--pr` when the PR was squash-merged: the check wants the PR's last commit in `main`, which a squash never leaves there.

   It stops at the first check that fails, before anything that can't be undone:
   - the tag is a version, its notes exist, and it isn't used yet;
   - the working tree is clean, on `main`, level with `origin/main`;
   - with `--pr`, the pull request is merged and its last commit is in `main`;
   - CI passed for `main`'s head (it waits while CI runs).

   Then it commits "Version X.Y.Z" (`build/windows/info.json`) only if the file isn't bumped yet (that commit can't be pushed to `main` any more, so bump it in the PR), pushes, tags, checks the tag on GitHub, waits for CI to build, test (including an install/uninstall smoke test) and code-sign the tag into a **draft** release, and runs `publish` (below). `--dry-run` does the checks only.
3. `publish` downloads the draft's files, checks them against the `.sha256` CI made, prints the hashes, writes `SHA256SUMS` and `SHA256SUMS.sig`, uploads them, publishes the release (as *latest* unless it's a preview like `v1.2.0-beta.1`), and verifies the published signature. It can also be run on its own: `go run ./tools/release publish vX.Y.Z`.
4. Optional: compare the printed hashes with the CI run's *Checksums* step. The signature says "the maintainer approved exactly these files"; it can't tell whether CI built the right thing.
5. Check that `docs/CHANGELOG.md` has the version and `docs/ROADMAP.md` no longer lists what it finished (both belong in the release's pull request).

`go run ./tools/release verify vX.Y.Z` checks any published release.
