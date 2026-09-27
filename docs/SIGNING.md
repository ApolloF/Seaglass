# Code signing

Two kinds of signature protect Seaglass's downloads:

- **Release signature** (from v1.1, always on): `SHA256SUMS.sig`, made with an offline ed25519 key. The updater requires it. See [RELEASING.md](RELEASING.md).
- **Authenticode** (this document): Windows' own code signature on `Seaglass.exe`, the uninstaller and `Seaglass-setup.exe`. It needs a certificate, which Seaglass doesn't have yet.

Unsigned (Authenticode), everything works; the differences are:

- Windows SmartScreen warns on first run of a new download ("Windows protected your PC") until the file has built up reputation.
- Once builds are signed, the updater also requires every update to carry the same publisher's signature as the running copy (`internal/update.CheckPublisher`).

## Route 1: SignPath Foundation (chosen)

[SignPath Foundation](https://signpath.org) signs open-source projects for free, in its HSM, from GitHub Actions. CI is ready for it; it switches on for tagged builds once these exist.

**To set up (maintainer):**

1. Apply at signpath.org with the repository (AGPL-3.0 licensed, public, builds on GitHub-hosted runners: all true). They review the project.
2. In SignPath, once approved:
   - a project (for example `seaglass`), with GitHub as its trusted build system;
   - a signing policy (for example `release-signing`);
   - two **artifact configurations**:
     - `binaries`: a zip holding `Seaglass.exe` and `uninstall.exe`, both signed as PE files;
     - `installer`: a zip holding `Seaglass-setup.exe`, signed as a PE file.
3. In the GitHub repository's settings:
   - secret `SIGNPATH_API_TOKEN` (a SignPath CI user's API token);
   - variables `SIGNPATH_ORGANIZATION_ID`, `SIGNPATH_PROJECT_SLUG`, `SIGNPATH_POLICY_SLUG`.

```bash
gh secret set SIGNPATH_API_TOKEN
gh variable set SIGNPATH_ORGANIZATION_ID --body "<organization id>"
gh variable set SIGNPATH_PROJECT_SLUG --body "seaglass"
gh variable set SIGNPATH_POLICY_SLUG --body "release-signing"
```

**What CI then does on a tag:**

1. Builds `Seaglass.exe`, and the uninstaller on its own (`makensis -DINNER` makes `uninstaller-maker.exe`, which writes `uninstall.exe`), since NSIS can't call a remote signer while it builds.
2. Sends both to SignPath (`binaries`) and waits for the signed files.
3. Builds the installer around them (`-DSIGNED_UNINSTALLER=bin\uninstall.exe`) and sends it to SignPath (`installer`).
4. Makes the checksums from the signed files and creates the draft release.

Builds of branches and pull requests stay unsigned.

## Route 2: a certificate of your own

`build/windows/sign.ps1 <file>` signs a file with `signtool` (SHA-256, RFC 3161 timestamp) and verifies the result. With no certificate configured it prints a note and succeeds. It reads:

| Variable | Meaning |
|---|---|
| `SIGN_PFX_BASE64`, `SIGN_PFX_PASSWORD` | A `.pfx` certificate, base64-encoded, and its password |
| `SIGN_THUMBPRINT` | Or: the SHA-1 thumbprint of a certificate in the current user's store (a hardware token, a cloud HSM's local provider) |
| `SIGN_TIMESTAMP_URL` | Timestamp server (default `http://timestamp.digicert.com`) |

With the repository secrets `SIGN_PFX_BASE64` and `SIGN_PFX_PASSWORD`, CI signs `Seaglass.exe`, the uninstaller (while NSIS builds it, through `-DSIGN_CMD`) and `Seaglass-setup.exe` on every build. Since June 2023 publicly trusted code-signing keys must live in hardware, so a plain `.pfx` from a CA is rare; Azure Artifact Signing (a monthly fee) works through `signtool` with Microsoft's dlib instead.

## Either way

Keep the publisher name stable: the updater compares the signer's name, and an update signed under a different name is refused. When the name has to change, ship one release that's installed by hand.
