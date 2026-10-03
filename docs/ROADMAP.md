# Seaglass roadmap

What Seaglass is for, the decisions that still hold, and what's still open. What each version already did is in [CHANGELOG.md](CHANGELOG.md) (including the original plan, under 0.1 to 0.7).
Design reference: [Design directions](https://claude.ai/artifact/EUqrFQcmgAThrxbm8vAr6i) (A Console, B Orbit, C Deck, D Desktop).

## Maintainer to-dos

These need the maintainer's accounts or decisions; nothing else can do them.

- [ ] **Apply to SignPath Foundation** (signpath.org) for Authenticode signing. Once approved: the project, signing policy and the `binaries` and `installer` artifact configurations in SignPath, then the `SIGNPATH_API_TOKEN` secret and `SIGNPATH_*` variables on GitHub ([SIGNING.md](SIGNING.md)). Until then releases carry the release-key signature but no Authenticode signature, so SmartScreen warns on first run. The CI steps are ready (1.1); once builds are signed, the updater's publisher check switches on by itself.
- [ ] **Turn on GitHub's private vulnerability reporting** (Settings → Security), which [SECURITY.md](../SECURITY.md) points to.

## Open items

- **Tests**: a run of the mock interface in CI (both themes, the target sizes and a narrow window) with Playwright; today the harness in `tools/harness` covers the real app, but only when run by hand.
- **Uninstall tidy-up**: offer to remove the non-Steam shortcuts Seaglass added to Steam (they're in a "Seaglass" collection).
- **Localisation**: the interface is English only, with strings inline in components. Extract them before adding languages.
- **Achievements not read yet**: CPY, PLAZA, FLT, Steamworks Fix, Hoodlum and DARKSiDERS emulators (no public description of their files), and EA, Ubisoft (store copies), Battle.net and Xbox games ([achievements.md](achievements.md)).
- **Add-ons**: the add-on host and DLSS Updater's add-on live on the [`feature/dlss-addon`](https://github.com/ApolloF/Seaglass/tree/feature/dlss-addon) branch, with ideas for bringing them back in its `docs/dlss-addon.md`.
- **Theme packs**: user themes as token sets only (no JavaScript), on top of the per-layout CSS tokens.
- **Memory while playing**: the goal was a Go core under 50 MB with the interface closed. 1.1 got it from 79 MB to 62 MB; a bare Wails v3 app is 46 MB here, so what's left of ours is ~16 MB, nearly all the game database index. An interned or on-disk index would be the next step.
- **Wails**: v3 is still beta; keep the pin, and when upgrading recheck the single-instance mutex name, event payloads and bindings.
- **Syncer** (in Syncer's repo): use `gamekit/steam.Accounts`, and let games the launcher registers feed Syncer's discovery (for now they only help `gameStatus`).

## Goals

- A Windows game launcher that finds games on its own: store installs and external copies (games installed outside a store launcher: standalone and DRM-free installers, Steam API emulators, plain folders), and adds art and metadata the way Playnite does for Steam games.
- Two modes: **Desktop** (mouse, layout D) and **Big picture** (controller). Big picture has three layouts, picked in Settings: **Deck (default)**, Console, Orbit.
- First-class DualSense: native input, glyphs, haptics, lightbar, and the PS button to summon the launcher. Games without DualSense support start through Steam Input automatically. Games that support it start directly.
- Syncer integration: save status per game, sync before launch, back up after exit, conflicts, and each person's playtime, achievements and settings following them between PCs.
- Pretty, responsive, secure and light: the interface unloads while you play.

**Not planned:** downloading or modifying games, or anything that bypasses copy protection (Seaglass only manages what's installed). Also not planned for now: emulators and ROMs, macOS or Linux, importing from Playnite.

## Decisions

| Area | Decision |
|---|---|
| Stack | Go + Wails **v3** (pinned to `v3.0.0-beta.26`, upgraded on purpose, never floating) + Svelte 5 + TypeScript + Vite |
| Why v3 | It can close windows and keep the app alive (the interface unloads while a game runs), has several windows (main and in-game overlay) and a built-in tray. v2 can only hide its one window. |
| Library storage | In memory, saved as one JSON file with atomic writes (changed from SQLite during v0.1: even thousands of games stay a few MB, it loads in milliseconds, and it saves a ~7 MB dependency; search and filters run in the interface) |
| Controller | SDL3 3.4.x (`SDL3.dll`, zlib license) through a small binding of our own over `golang.org/x/sys/windows`. No cgo. |
| Shared code | `github.com/ApolloF/gamekit` (public, MIT): VDF (text and binary), Steam (folder, libraries, installed apps, account, Authenticode, tamper and emulator checks) and the Ludusavi manifest parser. Store detection (Epic, GOG, Xbox, …) stays in each app: Syncer only needs names, Seaglass needs launch details. |
| Syncer | Stays a separate app with a local API (named pipe, current user only). Needs Syncer 0.11+; see [syncer-api.md](syncer-api.md) |
| Store games not installed | Optional, **off by default** |
| Desktop theme | Follows the Windows light or dark setting (Mica). Big picture is always dark. |
| Releases | Signed with the release key outside GitHub ([RELEASING.md](RELEASING.md)); the updater installs only signed releases |
| License | AGPL-3.0 (MIT until 1.4) |

## Security

- **WebView:** strict CSP with no remote scripts. Navigation away from the app is blocked, devtools and the context menu are off in release, only bound services are exposed, and every value from the UI is validated in Go (paths must resolve inside known roots).
- **Network:** allowlisted hosts, HTTPS, size limits, and images re-encoded. The only executables ever downloaded are Seaglass's own update from GitHub Releases (checked against the signed `SHA256SUMS`) and Syncer's installer (checked against the SHA-256 GitHub computed on upload).
- **Secrets:** the SteamGridDB key, Steam Web API key, Epic token and GOG token are stored with DPAPI and never logged.
- **Parsers:** VDF, ini, JSON, lnk and ACF files, and emulators' achievement files, are treated as untrusted, with size limits and fuzz tests.
- **Privileges:** Seaglass never elevates itself, and nothing is ever injected into games. The Syncer pipe is current-user only.

## Performance budget (Ryzen 7 7840HS, 16 GB)

| Metric | Target |
|---|---|
| Cold start to interactive | < 1 s desktop, < 1.5 s big picture |
| Incremental scan / first full scan | < 300 ms / < 3 s |
| Navigation with 2,000 games | 60 fps |
| Idle CPU | about 0 % (event-driven, no polling without a running game) |
| Memory while playing | Go core as small as Wails allows (see *Memory while playing* above) |
| Installer | < 20 MB |

## Risks

| Risk | Mitigation |
|---|---|
| Wails v3 is a beta that ships every few days | Pinned version, deliberate upgrades, thin Wails-specific layer |
| Steam only picks up `shortcuts.vdf` changes after a restart | Batch changes, apply when Steam is closed or after asking; only games that need Steam Input |
| DualSense shared between the launcher and a game (Bluetooth report mode) | Passive listener while playing, no output reports, can be turned off |
| Steam also claims the PS button | Setting explained in Controller settings; detect and warn |
| Overlay over exclusive full-screen games | Recommend borderless windowed; overlay is optional; no injection |
| Metadata endpoints change | One provider per package, cache, fallback chain |
| Losing the release key strands users on their version | Offline backup ([RELEASING.md](RELEASING.md)); a new key is added by a release signed with the old one |
