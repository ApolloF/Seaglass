# Security

## Reporting a problem

Please report security problems privately. Don't put details in a public issue.

- **GitHub:** on this repository, *Security → Report a vulnerability*.
- **Email:** me@apollof.nl.

Fixes ship in the latest release only. Seaglass updates itself, so with automatic updates on (the default, *Settings → General*) a fix reaches installed copies on their next start.

## What Seaglass does to stay safe

- **No elevation.** Seaglass, its installer and its updates run with your normal rights. A game that asks for administrator rights gets Windows' own prompt.
- **Nothing runs through a shell.** Games start with an explicit program path, working folder and arguments. Links handed to Windows are limited to store schemes (`steam://`, `com.epicgames.launcher://`, …) and `https://` pages.
- **Network.** Metadata, art and store accounts use allowlisted hosts over HTTPS, with size and time limits. Every image is decoded and re-encoded before it's stored, so only pixels reach the interface.
- **Updates** come only from this repository's GitHub releases (redirects are limited to GitHub's download hosts). From v1.1 each one must be listed in the release's `SHA256SUMS`, signed for that exact version with a release key kept offline, away from GitHub ([docs/RELEASING.md](docs/RELEASING.md)); a hijacked GitHub account can't ship an update. Once builds are Authenticode-signed, updates must also carry the same publisher's signature ([docs/SIGNING.md](docs/SIGNING.md)).
- **Secrets** (SteamGridDB key, Steam Web API key, Epic and GOG sign-ins) are encrypted with Windows DPAPI for your account and never logged.
- **Installing Syncer** downloads only from Syncer's own GitHub releases and checks the installer against the SHA-256 GitHub published for it before running it, for your account only.
- **Syncer's pipe** is checked to be served by a process of your own Windows account.
- **Interface.** A strict Content Security Policy (no remote code, no inline scripts), and every value from the interface is checked again in Go.
- **Parsers** for VDF, `.lnk`, SQLite and the rest treat files as untrusted, with size limits; several are fuzz-tested.
