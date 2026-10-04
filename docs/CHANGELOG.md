# Changelog

What each version of Seaglass (WaterLauncher until 1.4) changed and why, newest first. The notes published with each release are in [releases/](releases/); what's still open is in [ROADMAP.md](ROADMAP.md). This file was split out of the old `docs/PLAN.md`, which kept the plan and the release log together.

Alongside Seaglass: [gamekit](https://github.com/ApolloF/gamekit) v0.1.0, Syncer 0.15.0 and DLSS Updater 1.4.1.

## 1.11.0 (unreleased): a controller slow to answer

The DualSense was found 25 to 300 s late after Seaglass started, and again after games, from 2026-10-03 on (it had been instant every time before, and `internal/pad` hadn't changed). It reproduced with SDL alone: with HIDAPI on, `SDL_Init` took 17 to 30 s and the pad was added up to 61 s later; with HIDAPI off, the RawInput and GameInput backends took 30 s or never reported it. After a replug, both took 0.15 s, 48 times out of 48. So the controller (or Windows' HID stack for it) answered every request only after a timeout, and Seaglass's controller thread waited through each SDL restart: at start, when a game starts and when it ends. Switching HIDAPI at runtime instead of restarting doesn't work: SDL removes the pad, and no other backend picks it up, so the PS button would stop working while a game runs.

- `pad.Manager` times `SDL_Init`, `SDL_Quit` and each `SDL_PollEvent` (SDL looks for new controllers inside it). A call over 5 s is logged with its duration and sets `State.Slow` until SDL next starts. The interface shows a note telling the person to reconnect the controller (`lib/padstate.ts`), and diagnostics flag it.

## 1.10.0 (2026-10-03): controller and audit fixes

A DualSense stopped being detected, and the mouse wheel did nothing in big picture. No cause could be reproduced without the controller, but the controller layer had three ways to stay released: mode switches from two places at once reaching the SDL thread in the wrong order (after which `SetMode` ignored the fix-up), an SDL start that failed and was never tried again, and the controller coming back only after the steps after a game. `pad.Manager` now signals a mode change and the SDL thread reads the newest mode itself; failed starts are retried every 30 s; the controller is active again at `Finishing`. An `SDL_OpenGamepad` failure is logged with SDL's error, and diagnostics show the mode SDL is really in (`Manager.InMode`).

- **Mouse wheel** (`lib/wheel.ts`): big picture's screens move a selection, so the wheel dispatches direction steps; a list that really scrolls keeps the wheel.
- **Audit**: settings saved through a flushed file with a backup (a damaged file falls back to it); exe lookups retried for processes that couldn't be read yet; Epic/GOG refreshes without a new token keep the old one; secrets written through a flushed file; art temp files unique per fetch; a Syncer write cut short closes the pipe; interface fixes (settings saves applied at once and ordered, the welcome keeps the controller, Quick access closes the game page, overlay pad state, shrinking lists, Esc in dialogs) and library search without re-sorting.

## 1.9.0 (2026-09-28) and 1.9.1 (2026-10-02): external copies

Games from outside a store launcher were called *unofficial copies*; they are now *external copies*, so the wording describes where a game came from rather than suggesting what it is. The README gained an *Intended use* section: Seaglass manages installed games only, and doesn't download, modify or unlock them.

- `Game.unofficial` → `external`, `settings.detectUnofficial` → `detectExternal`, the hidden library `unofficial` → `external`, labels `Unofficial · X` → `External · X`, `internal/scan/unofficial.go` → `emulation.go`.
- Older files keep working: settings read `detectUnofficial` when `detectExternal` is missing (also from another PC's older Seaglass through the profile) and turn a hidden `unofficial` into `external`; libraries saved before (file version 1) carry the flag and label over, and are saved as version 2.
- Uplay emulator inis with achievements turned off get a *Turn on* button, and the card goes away when nothing is saved.
- 1.9.1 changes how Seaglass is tested, not how it behaves: tests and the end-to-end harness keep away from the real `%APPDATA%\Seaglass` and `seaglass.log`.

## 1.8.0 (2026-09-28): Syncer accounts and the profile

Syncer can give each person their own saves (accounts). Seaglass follows the person playing, and has Syncer sync what goes with them. Details in [syncer-api.md](syncer-api.md).

- **Who's playing**: a launch plays as the account Syncer has on this PC, without asking (read in the saves step). Switched from the sidebar, big picture's quick access or Settings → Saves (`switchAccount` puts their saves in place). Asking before each launch is optional (*Ask who's playing*, off, per PC) for PCs people take turns on.
- **Profile** (`internal/profile`): playtime, last plays, unlocked achievements and portable settings per person, in `%APPDATA%\Seaglass\Profile\<account>\<pc>.json`. One file per PC and person: PCs never write the same file, totals are sums and unions, settings the newest. Syncer syncs and backs up the folder through a new launcher API method, `launcherData` (Syncer's `ApolloF/LauncherData` branch).
- **Library** playtime and last play now come from the profile (for the person playing); what this PC had goes into its first file.
- New settings: *Sync them with Syncer*, *Same settings on every PC* (on), *Ask who's playing* (off).
- **Audit**: a game's totals add every key it was recorded under (title before a store match, Steam and metadata ids), so a match doesn't hide earlier play; the library is carried over once per install (`profile-seeded.txt`), not again after a PC rename; settings saved here are stamped newer than any seen (clocks ahead elsewhere); settings changes are serialised (`Core.setMu`); switching people waits for a running apply; the shared file is moved aside before it's merged, so it's merged once.

## 1.7.0 and 1.7.1 (2026-09-27): tester feedback after 1.6

- **Resident Evil 4 (2023) wasn't matched**: Steam sells it as "Resident Evil 4" (the original is "Resident Evil 4 (2005)"), and the store search finds nothing for "Resident Evil 4 Remake". Matching and *Change game…* now also try the title without "Remake" (last, after the edition and aliases), and RE2/RE3/RE4 short names (`RE4R`) are written out. *Change game…* retries a search that found nothing the same way.
- **Typing in *Change game…*** selected the whole title on every key: the effect that focuses the field also read the query. It now runs once, and results follow the title as it's typed.
- **RUNE showed one achievement for a finished game**: the INI reader now counts IDs listed in `[SteamAchievements]` (the unlocked ones, in order) and progress at its maximum, and unlocks in older files (left behind by an earlier emulator setup) still count instead of only the newest file's.
- **Ubisoft achievements**: Uplay emulators (`upc_r2.ini`, `uplay_r*.ini`) and VOICES38 (`voices38.dll`) are detected; their `Goldberg UplayEmu Saves` files are read (see [achievements.md](achievements.md)).
- **Launch sequence and mode**: sessions carry where they were started (`Session.From`: bigpicture, desktop, or "" for `--play` and games noticed outside Seaglass). Big picture's launch sequence plays only for its own launches (others show it only to ask something or to say a launch failed); after a game the window comes back in the mode it was started from (`?mode=desktop` too, so *start in big picture* applies only to the first window), a `--play` game leaves the interface closed, and the controller coming back after a game no longer counts as connecting one (*open big picture when a controller connects*).

1.7.1 is an audit release: session, achievement, metadata, store, controller and interface fixes ([releases/v1.7.1.md](releases/v1.7.1.md)).

## 1.6.0 and 1.6.1 (2026-09-27): achievements

Steam-style achievements for every game Seaglass can read: a card in the details (count, progress, the latest unlocks), the full list (unlocked first, rarity, dates, hidden ones masked), a big picture screen, and "N achievements unlocked" after playing. Details in [achievements.md](achievements.md).

- **Sources**: Steam (its local stats cache, else the Web API with the user's key), Epic (GraphQL; progress with the Epic sign-in), GOG (Galaxy's database; names and icons with a new GOG sign-in), and 15 Steam API emulator formats for external copies, named by `steam_settings`, Steam's cached schema or the Web API. Rarity from Steam's global stats (no key), Epic and GOG.
- **Scan**: `EmuDir` (where the emulator sits in the game folder) goes from the scan to `library.Game`; new markers for TENOKE (`SteamDataSer_stats.ini`), gbe_fork (`configs.user.ini`) and Razor1911 (`.1911`).
- **Idle budget kept**: read when details open and once after a session, cached by the files' times; nothing online while a game runs.
- **Not read yet**: VOICES38, CPY, PLAZA, FLT, Steamworks Fix, Hoodlum and DARKSiDERS (no public description of their files), and EA, Ubisoft, Battle.net and Xbox games.

1.6.1 drops the empty achievements card on games with nothing to look up, and reads achievements again after an update that changes them.

## 1.5.0 (2026-09-27): Seaglass

WaterLauncher is renamed **Seaglass**, the repository `ApolloF/Seaglass`, the license AGPL-3.0 (1.4 and earlier stay MIT). Maintainer's list:

- The PS button in big picture took the window out of full screen (`Shell.OpenMain` called Wails' `Restore`, which also leaves full screen). It now only un-minimises, and big picture goes back to full screen if it left it.
- Orbit puts the ten games played in the last 60 days in the middle, then favorites, then the rest (`orbitOrder`).
- Libraries can be hidden (`settings.hiddenSources`: steam, epic, gog, ea, ubisoft, battlenet, xbox, external, standalone, folder; `external` was `unofficial` before v1.9) in desktop Settings → Library and big picture Settings.
- *Install Syncer* / *Update Syncer* downloads Syncer's latest `Syncer-amd64-installer.exe` from its releases, checks it against the SHA-256 GitHub computed on upload (`digest`), and runs it silently (per user). *About Syncer* links to the project.
- The add-on host and DLSS Updater's add-on moved to the `feature/dlss-addon` branch, with [ideas](https://github.com/ApolloF/Seaglass/blob/feature/dlss-addon/docs/dlss-addon.md) for integrating it better.

**Carrying WaterLauncher over.** WaterLauncher's release key was lost, so 1.4 can't update to Seaglass (it only trusts that key, and only downloads from `ApolloF/WaterLauncher`). Seaglass has a new release key (2026-09-27) and signs "Seaglass release <tag>". Installing Seaglass over WaterLauncher still carries it over:

1. The installer finds WaterLauncher's uninstall entry, installs into a Seaglass folder next to WaterLauncher's, and removes the old program, shortcuts and uninstall entry (keeps a desktop shortcut if there was one).
2. Seaglass moves `%APPDATA%\WaterLauncher` and `%LOCALAPPDATA%\WaterLauncher` into its own folders on first start (merging, never deleting, not while WaterLauncher runs), and turns the old *start with Windows* entry into its own.

The new release key was backed up offline the same day (`go run ./tools/release backup <file>`, see [RELEASING.md](RELEASING.md)).

## 1.4.0 (2026-09-26): proving it in the real app

Everything checked so far ran in the browser mock. This round drives the real `Seaglass.exe`, then builds on it. One branch and pull request per phase.

| # | Phase | Status |
|---|---|---|
| 1 | Dev harness: `--remote-debugging`, `--virtual-pad`, `--dev-data`, the dev pipe, `tools/fakegame`, `tools/harness` (Playwright over CDP) | done |
| 2 | Every big picture screen and layout plus desktop mode, controller only, then keyboard only, at 1280×720, 1920×1080, 2560×1440, 3840×2160, 1920×1200 and 3440×1440; start and close the fake game on every route (direct, launcher handover, slow start, crash) | done |
| 3 | Frame times and memory with 500 and 2,000 games at 4K; fix the worst spots | done |
| 4 | `TestMatchAudit` grown to ~700 real titles and folder names; matching ≥ 97 % without false matches | done |
| 5 | Art picking in big picture, a controller button test screen, a first-start welcome flow, collections | done |

**Maintainer's additions (2026-09-26):** hide the mouse pointer in big picture while the controller is used (done in phase 2); Orbit flickers and stutters while swiping (phase 3); backdrops picked better and sharper (done, PR #14); Alan Wake II (a DODI repack of a game only Epic sells) had no art and "no controller support known" (done, PR #14: PCGamingWiki and the Epic store's page content).

**Phase 1.** The harness is in [tools/harness](../tools/harness/README.md). Dev flags work only in dev builds. With `--dev-data` the library is frozen and the real games in it start the fake game (store links cleared, controller mode *Native*), so a test never starts a real game or touches Steam's shortcuts. `%APPDATA%\Seaglass` is backed up before each run and restored after. First run: the app started and was reachable over CDP in 2.2 s; the virtual DualSense showed up as `DualSense Wireless Controller (virtual)` and moved the cover grid's selection.

**Phase 2.** `tools/harness/tour.mjs` drives desktop mode and 10 big picture screens in each of the three layouts (31 screens), with the virtual controller and then with the keyboard, at six screen sizes with the display scale Windows picks for each (1440p at 125 %, 4K at 150 %), and checks every screenshot for text that overlaps text, is cut off, or overflows its box. `launches.mjs` plays the four fake games from big picture with the controller and from desktop mode. Found and fixed:

- Keyboard: in Search, PageUp / PageDown / Tab were typed into the field, so the keyboard couldn't leave Search except with Esc. They now switch sections and open Quick access from there too.
- The mouse pointer showed over big picture while using the controller or keyboard; it now hides then, and after 3 s without the mouse moving.
- A game that crashed (exit code 0xC0000005) ended with "Welcome back". Game processes are now held open for their exit code, and a crash says "closed unexpectedly (error 0xC0000005)" in big picture and desktop mode.
- A slow-starting game showed "Playing 0:00 · The game is running" for as long as it loaded without a window. Sessions now know when the game's window came to the front, and show "Loading" until then (up to a minute).
- "1 min in total" after 3 s: now "Less than a minute in total".
- The game page's text was hard to read over bright backdrops (a stronger shade behind it), and the last Settings row sat under the list's bottom fade (scroll padding).
- Harness: the fake game hung when Go moved its message loop to another thread (now locked), and WebView2's browser process ends when every window closes for a game, so the harness reconnects.

Checked and fine: every screen reachable with the controller and the keyboard; big picture looks the same at every size (CSS zoom), 16:10 and 21:9 get extra room instead of bars; all four launch routes from both modes: the interface stays until the game's window is in front, closes, the controller goes passive, the launcher handover is followed, the interface is back within about a second of the game closing, and playtime counts (9–15 s).

**Phase 3.** `tools/harness/perf.mjs` holds a controller direction for 5 s in six scenarios (desktop grid, Deck row, Deck library, Search, Console row, Orbit) with 500 and 2,000 made-up games on a 4K screen at 150 %, and reads a Chrome trace: frames presented or dropped, frames drawn with holes (tiles not painted yet), main-thread time, long tasks, JavaScript heap, DOM size, WebView2's and the core's private memory. (A `requestAnimationFrame` counter was tried first; it made the page restyle every animated element each frame, so it measured itself.) `tracesum.py` sums a trace by thread and event.

Before and after, 2,000 games (500 in brackets):

| Scenario | Frames presented | Dropped | Main thread (5 s) | WebView2 |
|---|---|---|---|---|
| Orbit, before | 62 (91) | 459 (582) | 1.9 s | 1,275 MB (1,289) |
| Orbit, after | 754 (911) | 151 (55) | 2.8 s | 647 MB (814) |
| Console row, before | 882 (834) | 211 (231) | 3.0 s (2.7) | 987 MB (746) |
| Console row, after | 809 (650) | 117 (252) | 0.73 s (0.75) | 388 MB (375) |
| Deck row, before | 785 (825) | 79 (67) | 1.5 s | 442 MB |
| Deck row, after | 1,048 (1,068) | 21 (19) | 1.3 s | 452 MB |

The DOM stays the same size with 500 or 2,000 games (virtualised grids and rows), the Go core stays at 64–80 MB, and the desktop grid and Search were fine as they were. Fixed:

- **Orbit flickered and stuttered while gliding** (the maintainer's report): every bubble was a 560-pixel element scaled down, so at 4K about a hundred ~1,100-pixel layers, each with a drop shadow whose blur changed with every move (a full repaint of every layer). The GPU couldn't keep them, and frames came out with bubbles and the backdrop missing. Bubbles are now 240 pixels (the selected one is scaled up and drawn again sharp when a game opens), shadows are fixed CSS, the glow is a gradient moved by a transform instead of a 220-pixel blur moved by left/top, the name capsule has no backdrop blur, and bubbles fade in with CSS instead of Svelte transitions (which tick from JavaScript every frame). The clock's colour transition, which can't run on the compositor, is gone too.
- **Console**: the tiles animated their width and height (layout and repaint every frame); with a direction held they now change size at once, and the full-screen backdrop is only decoded where the selection stops.
- **Every layout**: the accent colour is a custom property on the root, and changing one restyles every element on screen; it now follows the selection once it rests (160 ms). Deck's blurred glow behind the rows does the same.

Measured with an emulated 4K screen on a 2880×1800 laptop, so absolute frame counts include the emulation; the before and after runs used the same setup.

**Phase 4.** `TestMatchAudit` now reads 958 cases from `internal/app/testdata/match_audit.tsv` (made by `gen_match_audit.py`): 333 real games as their store names them, as scene releases, repacks, installer entries, underscored, run-together or lower-case folders, 200 hand-written real-world folder and installer names (dropped subtitles, abbreviations, store install folders), and 50 folders that aren't games. Each case says which game it is, so a wrong match is caught, not just a miss. It runs what Seaglass does (game database, Steam store search, PCGamingWiki) and keeps the sources' answers in a cache, so a rerun takes seconds.

| | Found right | Wrong game | Missed | Non-games matched |
|---|---|---|---|---|
| Before | 846 of 910 (93.0 %) | 22 | 42 | 5 of 50 |
| After | 900 of 908 (99.1 %) | 0 | 8 | 0 of 50 |

(Two games that aren't on PC were dropped from the list, and games that were renamed, like Hitman 3 → HITMAN World of Assassination, accept either name.) Fixed:

- "DiRT Rally 2.0" lost its "2.0" as if it were a version; two plain numbers now stay (versions have three, or a "v").
- Scene groups DOGE, P2P, TiNYiSO, DARKSiDERS, CHRONOS and others are recognised.
- Editions: "The Final Cut", "Windows Edition", "Reloaded", "20 Year Celebration", "40th Anniversary Edition"; "Anniversary" alone no longer counts (it made "Tomb Raider: Anniversary" look like "Tomb Raider").
- Folders named after an expansion ("Cyberpunk 2077 Phantom Liberty", "Monster Hunter Rise Sunbreak") matched the DLC; a store hit that is a DLC now becomes its game.
- The store search accepts the store's name without its edition when only one game fits ("Tomb Raider Game of the Year"), which also stopped the wiki's 1996 Tomb Raider being chosen.
- Common short names and first words are written out (`scan.Aliases`: SkyrimSE, L4D2, KOTOR, HoMM3, AC Valhalla, ACOdyssey, RE Village, MHWilds, MK1, Civ 6, "Mafia 2 DE", …).
- The game database is also searched without little words ("Indiana Jones Great Circle"), by subtitle ("Infinite Wealth", "Bannerlord"), with "Edition" added ("Mass Effect Legendary"), and for names run together with their edition ("Fallout3GameoftheYearEdition"); the looser ones wait for a check, and each gives up when two games fit.
- Folders that aren't games (`scan.NotAGame`: Tools, Redist, Mods, Steam, Vortex, Discord, emulators, …) are never matched or looked up; "Vortex" and "Steam" had matched games.

**Phase 5.** Each was driven in the real app with the virtual controller:

- **Art in big picture**: the game page's *Art* opens a picker for the cover, the background (backdrop), the banner (hero) and the logo, switched with L1 / R1. Pictures come from Steam (its library art and every screenshot, originals first), the Epic store's page for games only Epic sells, PCGamingWiki's cover and, with a key, SteamGridDB; each is downloaded and stored like fetched art (the page's security policy only shows local art), and a choice goes into `artOverrides`, so refreshes keep it. Without a SteamGridDB key a line says where to add one. (`LibraryService.ArtChoices` / `SetArt`; only stored art names are accepted.)
- **Controller test**: *Settings → Test controller*. The controller layer streams every button and axis while the screen is open (`PadService.TestInput`, event `pad:raw`); buttons light up while held and stay marked once they've worked, triggers show how far they're pressed, sticks move and show their values (drift shows as movement at rest). Holding ✕ plays each rumble effect, holding ○ leaves (every button is being tested), Esc leaves at once.
- **Welcome**: a new install opens with four steps (what Seaglass finds, with the games found so far; where it looks, with *Add a folder*; how you play: layout, big picture when a controller connects, start in big picture; ready). Mouse, keyboard and controller all work, in either mode; the rest of the window is inert behind it. `settings.welcomed`; settings saved before it existed count as welcomed, so updating doesn't show it.
- **Collections**: games can be in collections of your own (`Game.collections`, kept through scans and merged with owned-game records). Desktop mode lists them in the sidebar, and a collection's heading renames or deletes it; the game's details add and remove them (with suggestions). Big picture shows them as Library tabs (L2 / R2, the tabs around the current one when there are many) and has a *Collections* picker on the game page with a few common ones to start with. New names are typed in desktop mode.
- Also: toasts sit above big picture's prompts, and the game page's five buttons fit on one line.

## 1.3.0 (2026-09-26): controller, look and feel

### Tester feedback

On `fix/tester-feedback`, from a tester's screenshots of v1.2.

1. **Orbit drew over everything**: its bubbles, clock and hints (z-index up to 300) showed through the game page, Quick access and the launch sequence. The layouts now isolate their stacking.
2. **Orbit tiling**: the ring walk visited a ring's sides in the wrong order, so games shared cells (9 of 29 hidden under others and out of reach) and the rest spread unevenly. `bigpicture/orbit.ts` fixes the spiral and adds a lens that sizes each bubble from how close its neighbours end up (never overlapping, gaps in proportion), and moves that find the nearest game ahead, keep to the column going up and down, and get past holes. The glide no longer overshoots and speeds up while a direction is held.
3. **Console details**: the stick moved between games. It now moves between Play, favourite, hide and the three cards; a card opens in place (About in full, the controller mode to choose, where this copy is); L1 / R1 change the game.
4. **Blurry backgrounds**: heroes are stored 1920×620, so full-screen they were stretched 1.74×, and they're the same picture as the game's tile. A new **backdrop** (first Steam screenshots, else the middle of the 3840-wide hero; at least 1280 wide at 16:9) fills big picture backgrounds. `meta.Version` 2 fetches metadata once more for it.
5. **Folder names that don't quite match**: "Assassin Creed Black Flag Resynced" missed "Assassin's Creed: Black Flag Resynced". `scan.LooseKey` ignores apostrophes, possessive and plural s, Roman numerals, a leading "The" and camel case; the game database and the Steam store search both use it (an ambiguous key matches nothing). A clear store hit now also names the game and skips the check, and scans keep that until they know better.
6. "Not played yet · Played 2 months ago" (Steam knew the date but counted under a minute) now reads "Played 2 months ago".

The mock takes `?layout=console|orbit` and `?games=N` for trying layouts with a bigger library.

### Controller, look and feel

On `fix/controller-and-polish`, from the maintainer's own list after the tester feedback above.

1. **Input**:
   - The left stick points one way at a time (the axis pushed furthest), and it no longer lets go of a held D-pad direction when it wobbles at rest, which stopped the D-pad repeating.
   - A controller whose mapping has no D-pad buttons gets its D-pad from the hat it reports (only when it never sends D-pad buttons, so nothing moves twice).
   - The DualSense touchpad click opens Search, like Create: the Search prompt showed a rectangle, which on a DualSense is the touchpad.
   - Prompts are drawn for what's in use: PlayStation shapes (Options and Create as the small buttons they are), Xbox letters, or keyboard keys while the keyboard is used or no controller is connected. Every prompt can be clicked.
   - One press arriving twice (Steam's desktop configuration turns the controller into a keyboard while Seaglass reads the same controller) is dropped.
   - The controller in use goes in the log.
2. **Sections**: L1 / R1 (Q / E, PageUp / PageDown) step through Home, Library and Search from anywhere, shown as tabs at the top of each. Library's filters moved to L2 / R2, Orbit's zoom too; a Console game's details keep L1 / R1 for the previous and next game.
3. **Keyboard**: Esc on Home opens Quick access (which has desktop mode, and says F11), M opens it, F searches. In Search, typed letters go into the search and Enter goes to the results.
4. **Haptics**: firmer, fixed-length effects the controller doesn't lose (a tick to move, a bump at an edge, confirm, a double pulse for errors, a swell to launch), played by the controller layer's own clock. Every move and every edge now gives one.
5. **Desktop mode with a controller** (`lib/desknav.ts`): the stick and D-pad move focus to the nearest control that way, ✕ presses it, ○ backs out, L1 / R1 step through the sidebar, Options opens Settings, Create jumps to the search box. The cover grid moves its own selection and lets focus leave at its edges.
6. **Resolution**: big picture is zoomed with CSS `zoom` instead of a transform, so it's laid out and drawn at the screen's own resolution (sharp at 4K, readable at 720p). 16:10 screens get extra height and 21:9 extra width instead of bars.
7. **Orbit**: bubbles are the game's key art with its logo (a new `tile`, 384² made from the hero and logo; put together in the interface until it's fetched), drawn at their largest size and scaled down so they stay sharp; the selected game's backdrop fills the screen behind them. Opening a game grows its bubble into sharp full-size art while the rest fly out, instead of fading to ghosts behind the details. A class name clash that squashed the open view is gone.
8. **Backdrops**: Steam screenshots as uploaded (often 4K) rather than the 1920 copies, stored at 2560 wide. A game without a backdrop gets its hero or cover blurred into a soft background instead of a small picture stretched over the screen. `meta.Version` 3 fetches metadata once more.
9. **Metadata for well-known games** (`TestMatchAudit` runs 75 real names through the real game database and store search: 68 matched before, 73 now):
   - Dotted folder names lose their version before the dots become spaces ("Game.Name.v1.10-GROUP" → "Game Name"), and "version 1.0.3179" goes too.
   - Brand prefixes ("Marvel's", "Tom Clancy's") and well-known abbreviations ("GTA V", "RDR2") are understood; editions are stripped for store titles too.
   - The Steam store search tries the name without its edition and with the abbreviation written out.
   - Steam's CDN art files are tried when its store API lists nothing (delisted games).
   - Epic's catalog (the launcher's own token, no sign-in) gives Epic-only games their art and details.
10. **New on this PC** (was Found on this PC, whose △ hint pointed nowhere): games matched only by folder name come first with a Check badge and open their page, which asks "Is this …?": that's right, pick the right game from the Steam store, or not a game (hidden).
11. **Syncer**: *Settings → Saves* (and big picture Settings) shows whether Syncer is installed, running and connected, its games, last backup, paused syncing and saves with two versions, with the one thing to do about it. New: how long to wait for a sync before playing, and whether to start Syncer when it isn't running.
12. **Playing**: the interface stays up until the game's own window is in front (up to 25 s), instead of showing the desktop while the game loads, and comes back as soon as the game is gone rather than after its session ends; a launcher handing over to its game just steps it aside again. Big picture's window opens full screen from its first frame.
13. The Settings icon is a gear.

The mock also takes `?art=steam` (real art from Steam's CDN, from the browser) and `?syncer=missing|old|off`, and `window.mockPad("down")` presses a controller button.

## 1.2.0 (2026-09-26)

On `feature/v1.2`.

1. **Safer releases**: `tools/release cut` (RELEASING.md) after a masked merge failure nearly tagged v1.1.0 on a `main` without its code; CI's installer smoke test (silent install, `--tray`, `--quit`, silent uninstall, no crash output).
2. **Diagnostics and crash capture**: `debug.SetCrashOutput` into `crash.log` (kept as `crash-previous.log` by the next start, which says so), *Copy diagnostics* / *Report a problem* in *Settings → About*, `--diagnostics` for when the interface won't open, interface errors into the log. The report shortens the user folder and holds no keys or account names.
3. **Saves after games started elsewhere**: a noticed game's session gets Syncer's after-exit backup (started, not waited on). Syncer syncs continuously and backs up every few hours by itself, so this only adds a restore point right after the session; the before-launch sync can't apply to a game that's already running.

## 1.1.0 (2026-09-26): faster scans, less memory, games started elsewhere, signed releases

Recommendations 1 to 4 made after 1.0 (below), on `feature/v1.1`. Order: 3, 2, 4 (code only), then 1 (needs decisions and accounts from the user). Each part lands with tests and a measurement, then one v1.1.0 release.

**Status (2026-09-26):** A, B, C and D1 done; D2 prepared, waiting for SignPath's approval. Outcomes:

- A: repeat enrich of 100 generated games (1,000 files each) 700–830 ms → 8 ms; repeat scans on this PC 63 ms → 18 ms.
- B: private bytes in the tray 79 MB → 62 MB. The < 50 MB goal isn't reachable with Wails: a bare Wails v3 app is 46 MB here (importing it loads shell32 and friends at init; a plain Go program is 12 MB). What's left of ours is ~16 MB. The heap profile showed the game database index as nearly all of the Go heap.
- C: done as planned; checked by hand with a folder game started from Explorer.
- D1: key made 2026-09-26 on the maintainer's PC (public key in `internal/update/keys.go`); CI makes drafts; `tools/release` signs and publishes. **The offline backup has to be made by the maintainer** (it asks for a password).
- D2: CI steps for SignPath (two signing rounds, uninstaller built separately with `-DINNER` / `-DSIGNED_UNINSTALLER`, checked locally by installing and uninstalling with a separately built uninstaller). Setup steps in [SIGNING.md](SIGNING.md).

### A. Scan cost per game (recommendation 3)

Problem: `DetectEmulation` walks each game folder (up to 30,000 entries, 7 levels) and `PickExe` walks it again (20,000 entries, 4 levels) on every scan: 5–30 ms per game here, seconds with hundreds of games.

1. **One walk**: `scan.Inspect(dir, title)` does both in a single `WalkDir`, returning the emulation result, the picked exe and a *fingerprint*.
2. **Fingerprint**: the modification times of the folders that matter (the root, every folder to depth 2, and every folder holding a marker, a `steam_api*.dll` or an exe candidate), plus size and time of the marker files and DLLs. Adding, removing or renaming a file changes its folder's time; an in-place replacement (a patched `steam_api64.dll`) changes the file's own.
3. **Cache** per folder in memory: a later scan only stats the fingerprint (tens of calls) and reuses the result when nothing moved. A full walk still happens at least once a day per game and at every start.
4. **Measure** with a generated library (`scan` benchmark: 100 games of ~2,000 files each in a temp folder) and on this PC. Target: repeat enrich of 500 games under 300 ms total.

### B. Memory while playing (recommendation 2)

Problem: the core holds ~75 MB private with the interface closed; goal < 50 MB.

1. **Attribute first**: `WL_HEAPPROFILE=<file>` writes a Go heap profile 60 s after a game starts (or on `--quit`); compare Go heap (`runtime.MemStats`) with the process's private bytes to split Go from native (SDL, Wails, WebView2 loader).
2. **Game database index** (~15 MB live, the biggest known item): only scans use it, and scans wait while a game runs. Hold it through a `weak.Pointer` and reload it from the cached file (~1 MB gzip) when the next scan needs it after the GC dropped it; drop the strong reference when a game starts.
3. **GOG Galaxy database**: read pages with `ReadAt` from the open file instead of loading up to 512 MB whole.
4. **Native side**: check SDL in passive mode (HIDAPI off) and Wails' idle allocations once the profile shows them; set `debug.SetMemoryLimit` only if the profile shows GC headroom as the cause.
5. **Measure** before and after on this PC: private bytes 60 s into a game with the interface closed.

### C. Games started outside Seaglass (recommendation 4)

Problem: playtime and game mode only work for games started from Seaglass. A game started from Steam or a desktop shortcut isn't noticed, and the controller layer stays in its active mode (SDL's HIDAPI drivers) while that game has the DualSense.

1. **No polling**: a WinEvent hook on `EVENT_SYSTEM_FOREGROUND` (out of context, no injection, no administrator rights) on a small thread with its own message loop. Each time a window comes to the front, its process id arrives. (WMI's `Win32_ProcessStartTrace` would need administrator rights, and the non-admin WMI query polls.)
2. **Match**: the process's image path against the installed games' folders (a sorted index built after each scan); Seaglass's own processes and non-game folders are skipped; each process id is checked once.
3. **Session**: an "external" session in `launch.Manager` without hooks: the existing tracker follows the process tree and the folder, counts playtime, and ends the same way. The controller goes passive (or off, per setting), the tray says what's playing, the PS button opens the overlay. The interface isn't closed for a game you started elsewhere.
4. **Setting**: *Notice games started outside Seaglass* (on by default) under *While playing*, in both Settings screens.
5. **Tests**: the matcher and session start with fake processes; by hand: start a Steam game from Steam and a folder game from Explorer.

### D. Signed releases (recommendation 1)

Two independent parts.

1. **Release signature, key outside GitHub** (so a compromised GitHub account can't ship an update):
   - `tools/release`: `keygen` makes an ed25519 key pair; the private key is stored encrypted with DPAPI on the maintainer's PC and exported once, password-protected, for an offline backup.
   - CI publishes tags as **draft** releases. `go run ./tools/release publish vX.Y.Z` downloads the draft's assets, checks their `.sha256`, writes `SHA256SUMS` and `SHA256SUMS.sig`, uploads both and publishes the release. Drafts are invisible to the updater.
   - The updater embeds the public key(s) and, from v1.1 on, installs only updates whose hash is listed in a `SHA256SUMS` with a valid signature. v1.0 keeps using the `.sha256` files, so v1.0 → v1.1 still works.
   - Rotation: a list of accepted keys; a new key is added by a release signed with the old one.
2. **Authenticode** (SmartScreen, and the publisher check the updater already has): SignPath Foundation (free for open source) signs from GitHub Actions after they approve the project. Needs the user to apply. Then: sign `Seaglass.exe`, build the uninstaller separately so it can be signed too (NSIS can't call a remote signer mid-build), sign the installer.

Decisions for the user: where the release key lives and how it's backed up; whether to apply to SignPath (or pay for Azure Artifact Signing).

## 1.0.0 (2026-09-26)

**Built for v1.0**

- **Installer** (`build/windows/nsis/project.nsi`): per-user (`%LOCALAPPDATA%\Programs\Seaglass`, no UAC), so updates never need elevation. It closes a running copy with `--quit`, remembers the install folder for updates, deletes only its own files on uninstall (the folder may have been picked by hand), removes the start-with-Windows entry, and asks before deleting the library and settings. `/relaunch` (and `/tray`) start Seaglass again after a silent update. About 7.5 MB with the WebView2 bootstrapper.
- **Updates** (`internal/update`, `internal/app/updates.go`): the newest non-preview release from `api.github.com/…/releases/latest`. Downloads come only from below `github.com/ApolloF/Seaglass/releases/download/` (redirects limited to GitHub's download hosts), are checked against the release's `.sha256`, and, once builds are signed, against the running exe's Authenticode publisher. Installed copies run the installer silently; others swap their exe (a running exe can be renamed). Checked once, 15 s after start (from v1.8; before: 90 s after start and every 12 h), put off while a game runs; after that only by hand. A downloaded update installs at the next start (not with `--play`), right after that check when Seaglass shows only its tray icon and no game runs (it comes back in the tray), or at once with *Restart and update*. `pending.json` counts attempts, so an update that didn't take isn't retried on its own.
- **Start with Windows**: `HKCU\…\Run\Seaglass = "<exe>" --tray`. Settings shows when Task Manager's switch turned it off, and the entry is repaired when Seaglass moved.
- **Single instance**: a second start hands its arguments over before the library is opened. The mutex name follows Wails beta.26; recheck it when upgrading Wails.
- **Signing**: `build/windows/sign.ps1` and the CI secrets `SIGN_PFX_BASE64`/`SIGN_PFX_PASSWORD`; unsigned until a certificate exists.
- **CI**: builds the installer, signs when configured, publishes `v1+` tags without a suffix as full releases (`make_latest`), and runs the race detector when gcc is available.
- **Checked by hand on this PC**: silent install to a folder with spaces, `--quit` with and without a running copy, an installer update from 1.0.0 to 1.0.1 started at sign-in (`--tray`) that relaunched in the tray and tidied up, an exe-swap update of a copy that wasn't installed (including the no-retry guard), and a silent uninstall that removed the files, shortcuts, Run entry and uninstall key while keeping the data.

**Audit (2026-09-26)**, fixed in v1.0:

| Area | Finding | Fix |
|---|---|---|
| Data safety | The save timer and quitting could write `library.json.tmp` at the same time and corrupt the library (it was then set aside and a fresh one started) | Saves serialized; the file loaded at start is kept as `library.json.bak` and used when the main file is damaged |
| Data safety | Quitting mid-game lost up to a minute of playtime (`Launch.Close` didn't wait) | Close waits up to 3 s for the playtime hand-over |
| Performance | The owned-games merge compared every owned game with every found game, under the write lock: 126 ms per scan with 3,500 games | Index by store id: 0.46 ms |
| Performance | Every scan walked each shortcut's folder (up to 4,000 entries) and each game folder for its size: 550–800 ms repeat scans on this PC | Cached by folder modification time, and sizes for 6 h: 63 ms |
| Performance | Each metadata result, favorite or playtime tick reloaded the whole library in the interface | `games:updated` carries just the changed games |
| Idle cost | SDL was polled every 8 ms forever, with or without a controller (~400 ms CPU per 30 s idle) | 250 ms without a controller, 33 ms in games, 16/8 ms in use, not at all when off: too little to measure |
| Idle cost | Scans, metadata and owned-games syncs ran during games | They wait until the game ends; memory is returned to Windows when the interface closes and after big batches |
| Leaks | Art replaced by refreshes, and art of removed games, was never deleted | Unused art pruned once per start (a day's grace) |
| Robustness | An add-on that stopped reading stdin blocked writes while holding the lock its reader needed (deadlock), and could hang shutdown | Separate write lock; the goodbye is sent in the background |
| Robustness | The image decode limit of 50 megapixels allowed ~200 MB spikes | 24 megapixels |
| Startup | A second instance (every `--play` shortcut) opened the library, settings and add-ons before handing over | Checked before anything is loaded |

Checked and fine: no known vulnerabilities in Go modules (govulncheck) or npm packages (npm audit); the CSP injected into release builds; URL and host allowlists in `meta`, `owned` and `update`; DPAPI secrets never logged; the Syncer pipe owner check; add-on hash pinning; size-limited, fuzzed parsers; no shell anywhere.

**Recommendations after 1.0** (1 to 4 became 1.1; 9 became 1.2; the open ones are in [ROADMAP.md](ROADMAP.md)):

1. **Get a signing certificate** (SignPath Foundation is free for open source), then signature-required updates switch on by themselves. Also consider signing each release's checksums with an ed25519 key kept outside GitHub: today a compromised GitHub account could replace both an asset and its `.sha256`.
2. **Memory while playing**: the Go core is ~75 MB private with the interface closed (goal < 50 MB). The game database index is ~15 MB live; the rest needs a heap profile (`pprof`) to attribute (SDL, Wails, runtime). An interned or on-disk index would help, and GOG's database could be read by pages instead of whole.
3. **Scan cost per game**: emulator detection and exe picking still read each game folder on every scan (~5–30 ms per game), so with hundreds of games a full scan takes seconds. Cache them by folder and marker-file modification times, like `looksLikeGame` now.
4. **Passive play tracking** of games started outside Seaglass: a process-start event subscription (WMI `Win32_ProcessStartTrace`) avoids polling while idle.
5. **Tests**: Vitest for the focus engine and big picture navigation; a Playwright run of the mock interface at the two target sizes; an installer smoke test in CI (silent install to a temp folder, `--quit`, silent uninstall, as done by hand for v1.0).
6. **Wails**: v3 is still beta; keep the pin, and when upgrading recheck the single-instance mutex name, event payloads and bindings.
7. **Uninstall tidy-up**: offer to remove the non-Steam shortcuts Seaglass added to Steam (they're in a "Seaglass" collection).
8. **Localisation**: the interface is English only, with strings inline in components. Extract them before adding languages.
9. **Diagnostics**: panics in the core only reach the log; a *Copy diagnostics* button (log tail, versions, settings without secrets) would make bug reports easier.

## 0.1 to 0.7 (2026-09-25 and 2026-09-26): the original plan

The plan Seaglass was built from, as written then (with notes added while each phase was built). Some of it has moved on since: the library is a JSON file rather than SQLite, and the add-on host now lives on the `feature/dlss-addon` branch.

### Phases

Each phase ends with a working build, a GitHub prerelease and a check-in.

| # | Version | Scope |
|---|---|---|
| 1 | v0.1 | Scaffold (Wails v3, Svelte 5, CI). Steam playtime import (pulled forward from v0.4). Detection code copied into `internal/` for now, split into `gamekit` in phase 5. SQLite schema, scanner (stores, external copies, folders), Desktop D with real data (grid, filters, details), Settings skeleton, mock backend |
| 2 | v0.2 | Metadata and art (Steam, GOG, SteamGridDB; PCGamingWiki dropped), image pipeline, accent colours, *Found on this PC* review |
| 3 | v0.3 | Big picture: focus engine, SDL3 controller layer, **Deck** first, then Console, then Orbit; glyphs, haptics, lightbar, on-screen keyboard |
| 4 | v0.4 | Launch and tracking, hooks pipeline, game mode (with tray), PS-button overlay, Steam Input routing, `--play` (playtime import moved to v0.1) |
| 5 | v0.5 | `gamekit` repo and Syncer PR (pipe API, `--api`), launcher client, save status, conflicts, backup after exit |
| 6 | v0.6 | Add-on protocol, host and permissions UI; DLSS Updater `--addon` PR |
| 7 | v0.7 | Owned-but-not-installed games from Steam, GOG and Epic (opt-in) |

### Architecture

```
                    ┌───────────────────────── Seaglass.exe (Go core, always running) ─────────────────────────┐
 metadata hosts ◄───┤ library (JSON)  scan  identify  meta+images  launch/track  pad (SDL3)  syncer client  addon host   │
 (allowlist)        └──────┬───────────────────────┬──────────────────────┬───────────────────────┬───────────────────────┘
                           │ Wails bindings/events │                      │ \\.\pipe\syncer       │ stdio JSON-RPC
                  ┌────────▼────────┐   ┌──────────▼─────────┐   ┌────────▼────────┐     ┌────────▼────────┐
                  │ Desktop window  │   │ Big picture window │   │ Syncer.exe      │     │ add-on (e.g.    │
                  │ (WebView2)      │   │ + overlay window   │   │ (separate app)  │     │ DLSS Updater)   │
                  └─────────────────┘   └────────────────────┘   └─────────────────┘     └─────────────────┘
                   windows are closed while a game runs; the Go core keeps tracking and listening for the PS button
```

Repo layout:

```
main.go                      app setup, windows, tray, single instance
internal/
  library/                   schema, migrations, queries, models
  scan/                      orchestrator, watchers
    sources/                 steam, epic, gog, xbox, ea, ubisoft, battlenet, uninstall, shortcuts, folders
    external/                Steam API emulator signatures -> store IDs
    exe/                     picking the main exe (ported from DLSS Updater's GameInspector)
  identify/                  ID match, Ludusavi, PCGamingWiki, fuzzy titles, confidence
  meta/                      steamstore, gog, pcgw, steamgriddb, images (fetch, validate, resize, cache, accent colour)
  launch/                    sessions: hooks pipeline, process tracking, playtime
  steaminput/                non-Steam shortcuts in shortcuts.vdf for the Steam Input route
  pad/                       SDL3 binding, DualSense features, intents, background PS-button listener
  syncer/                    pipe client (Syncer's launcher API)
  addons/                    manifest, host, permissions, JSON-RPC
  platform/                  DPAPI, known folders, foreground and full-screen checks
  update/                    GitHub release check, verified download, installer or exe swap
frontend/src/
  lib/                       api (plus a mock backend), focus engine, input intents, glyphs, sounds, stores
  shared/                    GameArt, Logo, LaunchSequence, QuickAccess, Toggle, OSK
  desktop/                   layout D
  bigpicture/deck|console|orbit/
docs/                        PLAN.md, addon-protocol.md, syncer-api.md
```

Data: `%APPDATA%\Seaglass` (`settings.json`, `library.json`, log), `%LOCALAPPDATA%\Seaglass\cache` (game database, images, WebView2 data).

### Library and detection

| Source | Signal | Identity |
|---|---|---|
| Steam | `libraryfolders.vdf`, `appmanifest_*.acf` | AppID |
| Epic | `ProgramData\Epic\...\Manifests\*.item` | catalog namespace and item |
| GOG | registry `GOG.com\Games`, `goggame-*.info` | GOG ID |
| EA, Ubisoft, Battle.net, Xbox | same as Syncer's `installed.go` | store IDs |
| Emulated Steam | `steam_appid.txt`, `steam_settings\` (Goldberg/GSE), `steam_emu.ini` (CODEX), RUNE/EMPRESS/TENOKE/SmartSteamEmu configs, `OnlineFix.ini`, an unsigned `steam_api(64).dll` | AppID from the files |
| GOG rips | `goggame-*.info` outside a GOG install | GOG ID |
| Repacks and installers | Uninstall registry entries (Inno Setup and others): `InstallLocation`, `DisplayIcon` | by title |
| Shortcuts | Start menu and desktop `.lnk` targets | by title |
| Watched folders | every subfolder is a candidate, live updates through `ReadDirectoryChangesW` | by title |

- **Identify:** exact ID first, then the Ludusavi manifest (install folder name to title and AppID, already used by Syncer), then PCGamingWiki, then normalised fuzzy title matching. Each match gets a confidence score. Low-confidence matches wait in *Found on this PC* for a check (setting on by default).
- **Main exe:** prefer the launcher exe in the game's root folder and known engine patterns; skip uninstallers, redistributables and crash handlers. Signals include version info and icon.
- **Duplicates:** the same folder or the same store ID merges. A store copy and an external copy can both exist and are labelled.
- **Owned but not installed (opt-in, v0.7):**
  - Steam: `IPlayerService/GetOwnedGames` with the user's own Web API key (DPAPI) and the SteamID of the account in use (from `gamekit/steam`).
  - GOG: Galaxy's `galaxy-2.0.db`, read with a small read-only SQLite reader (`internal/sqlite`, WAL included) instead of a ~7 MB SQLite library.
  - Epic: the public launcher client's OAuth. The user signs in on epicgames.com and pastes the one-time code; only the refresh token is kept (DPAPI).
  - Owned-only games are library records keyed `owned:<store>:<id>`. A scan that finds the game installed merges them and keeps the user's choices.
  - *Install* opens the store (`steam://install/…`, `goggalaxy://openGameView/…`, `com.epicgames.launcher://apps/…?action=install`).
- **Speed:** the first scan runs in parallel, and later scans are incremental, driven by watchers.

### Metadata and art

- **Steam (no key; checked 2026-09-25):** `store.steampowered.com/api/appdetails` gives the description, genres, developer, release date and categories (55 DualShock, 57/58 DualSense, 28 full controller support). `IStoreBrowseService/GetItems` with `include_assets` gives the library cover, hero, logo and header. This covers every emulated-Steam copy too.
- **GOG:** `api.gog.com/products/{id}`. **SteamGridDB:** optional key, for games without a Steam ID and to pick alternative covers. Games the manifest doesn't know are also looked up on the Steam store by exact title.
- **PCGamingWiki** was dropped during v0.2 (its API refuses Cargo queries) and came back in v1.4 through MediaWiki's own API, which returns a page's wikitext: controller support (PlayStation controllers and which models, DualSense features, full or partial support), the Steam and GOG ids, developers, genres, the cover, and where else the game is sold. Steam's store categories (DualShock 55, DualSense 57/58) often leave PlayStation support out, so the wiki fills or upgrades it; the game's files (Sony's `libScePad.dll`, or SDL) still count too.
- **Games only Epic sells**, found without an Epic library entry (a repack, a folder): the wiki names the Epic store page, whose public page content gives the key art (2560×1440), the portrait cover, screenshots, the description and the developer.
- **Backdrops** are picked from up to six candidates (Steam's library hero, the first screenshots as uploaded, the Epic store's key art and screenshots): sharp (up to 3840 wide), calm and not too bright on the left where the text goes, something to look at on the right, and key art slightly preferred over gameplay with its interface.
- **Fallback:** a generated cover from the title plus the icon pulled from the exe.
- **Image pipeline:** HTTPS to allowlisted hosts only, with size and time limits. Every image is decoded and re-encoded (which strips anything hidden in the file) at the sizes the UI needs, stored by content hash, and served through the asset handler with long cache lifetimes. An accent colour is extracted per game for glows and the lightbar.
- User overrides (title, cover, hero) are never overwritten by a refresh.

### Launching, tracking, controller

- **Launch:** Steam games use `steam://rungameid/…` (setting: direct exe). Everything else starts with `CreateProcess`: explicit path, working folder and argument array, no shell. Elevation happens only when the game's manifest requires it.
- **Tracking:** the process tree plus matching on the install folder, which covers launcher-to-game handoffs. It polls every 2 s only while a game runs. Playtime is saved every minute. Steam playtime is imported from `localconfig.vdf`. Optional passive tracking of games started outside Seaglass isn't built yet: it needs polling while idle, which works against the idle budget.
- **Command line:** `Seaglass.exe --play <id>` starts a game without opening the interface (for shortcuts).
- **Hooks pipeline:** before launch (Syncer sync, add-ons) and after exit (Syncer backup, add-ons). Each step has a timeout, progress and a skip option, and the result is shown in the launch sequence.
- **DualSense (SDL3):**
  - Features: hotplug, glyph detection (PS or Xbox), haptic ticks, a lightbar in the game's accent colour, and the PS button to summon the launcher.
  - While a game runs, Seaglass releases the controller and keeps only a passive PS-button listener. It sends no output reports and never switches the controller's report mode.
- **Per-game controller mode:** *Auto* / *Native* / *Steam Input*.
  - Auto uses Steam categories 55/57/58, Steam's controller support field, and whether `libScePad.dll` or SDL is in the game folder. A game goes through Steam Input only when a PlayStation controller is in use and the game supports Xbox controllers but not PlayStation ones.
  - The Steam Input route uses non-Steam shortcuts that Seaglass manages in `shortcuts.vdf`, in a "Seaglass" collection. Changes are applied while Steam is closed, or after asking to restart it. These games launch through `steam://rungameid/<shortcut>`.
- **Game mode:** all interface windows close; the smoke test measured about 420 MB for WebView2 alone. Seaglass stays in the tray (pulled forward from v1.0, because game mode needs it). Closing the window yourself quits, except while a game runs. Pressing PS opens a borderless topmost overlay window. Nothing is ever injected into games, so anti-cheat stays happy.
  - Measured in v0.4: the Go core uses about 35 MB while a game runs with the interface closed.
  - Passive listening restarts SDL without its HIDAPI drivers, so nothing is written to the controller. Enhanced reports stay off in every mode: once on, a Bluetooth PlayStation controller keeps them until turned off, and DirectInput games get no input.

### Interface

- The **Big picture** layouts share the focus engine (spatial navigation in zones), input intents (keyboard, or gamepad events from Go; the browser gamepad API isn't used), glyphs, sounds, haptics, an on-screen keyboard for search, and a controller-friendly Settings screen.
- **Desktop D:** filters, cover grid, details panel, Settings window. It auto-switches to big picture when a controller connects (setting).
- **Speed:** virtualised grids and rows, animations only on transform and opacity, images pre-sized, `content-visibility`, and reduced motion respected.
- **Themes:** CSS token sets per layout. User theme packs (tokens only, no JavaScript) come later.
- **Mock backend:** the frontend runs in a normal browser with fake data, for fast iteration and screenshots.

### Syncer integration

Done in v0.5. Syncer's side is [ApolloF/syncer#7](https://github.com/ApolloF/syncer/pull/7); the protocol is Syncer's [docs/api.md](https://github.com/ApolloF/syncer/blob/main/docs/api.md), and Seaglass's use of it is in [syncer-api.md](syncer-api.md).

1. Syncer imports `gamekit` for VDF, Steam libraries, tamper checks and the manifest parser. `internal/steam` keeps its API as thin wrappers, and the manifest parser gives identical results (13,719 games with Windows saves).
2. `\\.\pipe\syncer`: JSON-RPC, one message per line, served by the window. A protected DACL grants only the current user, and remote clients are refused. `Syncer.exe --api` serves it without a window and exits a minute after the last connection. The client checks the server process's owner.
3. Methods: `status`, `games`, `gameStatus` (by title, Steam app, install folder, or a title the launcher registered), `syncNow` (with timeout), `backupNow` (optionally waiting), `conflicts`, `resolveConflict`, `open`, `registerGames`, `subscribe` (`changed` notifications).
4. The protocol is versioned (`status.protocol`, now 1). Seaglass handles Syncer being missing (*Get Syncer*) or older than 0.11 (*Update Syncer*; it never starts an old `Syncer.exe --api`, which would open its window).

Follow-ups in Syncer, after its `feature/steam-autocloud-copies` work lands: use `gamekit/steam.Accounts`, and let registered launcher games feed discovery (for now they only help `gameStatus`).

### Add-ons

- **Manifest** `addon.json`: id, name, version, publisher, exe, protocol version, hooks, contributions (badges, actions, a settings schema) and permissions (for example `modifyGameFiles`, `network`).
- **Hooks:** `library.gameAdded`, `game.beforeLaunch` (progress, skip), `game.afterExit`, `game.status`, `game.actions`, `settings`.
- **Trust:** you enable each add-on explicitly and see its permissions. Seaglass pins its SHA-256 and asks again when it changes, and shows the Authenticode publisher when signed. Add-ons run with normal user rights, with timeouts and crash isolation.
- **DLSS Updater:** a new `--addon` mode ([ApolloF/dlssupdater#1](https://github.com/ApolloF/dlssupdater/pull/1)). *Settings → General → Connect to Seaglass* writes its `addon.json`. The .NET 8 SDK (8.0.425) is installed per user in `%LOCALAPPDATA%\Microsoft\dotnet`.
  - Before launch: re-apply DLSS if a game patch reverted it.
  - Status: DLSS and OptiScaler versions.
  - Actions: install OptiScaler, restore DLSS, open in DLSS Updater.
- The spec is [addon-protocol.md](addon-protocol.md).
- **Where add-ons live:** `%LOCALAPPDATA%\Seaglass\addons\<id>\addon.json`. Approvals (enabled, pinned SHA-256) are in `%APPDATA%\Seaglass\addons.json`.
- **Before-launch order:** Syncer's *Sync saves* first, then add-ons, then the Steam Input shortcut.
- **Authenticode:** only the signed or unsigned state is shown for now, not the publisher's name. `library.gameAdded` goes only to add-ons that are running.

### Testing

- **Go:** fixture folders for every detection signature, golden tests for identification, fuzz tests for parsers, and tests for the launch and tracking state machine.
- **Frontend:** `svelte-check` and Vitest (focus engine, intents). The mock backend is checked visually at 1600×1000 and 1920×1080.
- **Performance:** a fake-library generator with 2,000 games, measuring fps and memory.
- **Controllers:** a checklist per phase covering DualSense over USB and Bluetooth, and an Xbox controller.
- **CI:** GitHub Actions on `windows-latest` runs the tests and a build. Tagged versions build the installer.

### Workflow and releases

- `main` always builds. Each phase gets a branch (`feature/v0.1-foundation`, and so on) and a PR, merged after tests and the build pass. Small fixes can go straight to `main`.
- At the end of each phase, the tag `vX.Y.0` makes GitHub Actions build `Seaglass.exe` (plus the installer once it exists) with SHA-256 checksums, and publishes a **prerelease** with notes on what changed and what to test.
- The Syncer and DLSS Updater changes (phases 5 and 6) happen in their own repos: `gamekit` as a new public repo, and PRs (or direct pushes to `main`) for Syncer and DLSS Updater.
- License: AGPL-3.0 (MIT until 1.4).

### Environment (set up 2026-09-25)

- Go 1.27.0, Wails CLI v3.0.0-beta.26, Node 24.19 and npm 11.17, NSIS, Git 2.55, GitHub CLI 2.101 (signed in as ApolloF, used as the Git credential helper), WebView2 153.
- Repo: `https://github.com/ApolloF/Seaglass` (public), branch `main`.
- CI (`.github/workflows/build.yml`) builds and tests every push and PR on `windows-latest`. A `v*` tag publishes a prerelease with the exe, its SHA-256, and `docs/releases/<tag>.md` as notes.
- Smoke test: a Wails v3 Svelte app built in 34 s into a 10.5 MB exe. At runtime the Go process used about 67 MB and WebView2 about 423 MB.
- Installed later: SDL3 3.4.16 (phase 3, from the libsdl-org release, hash checked) and the .NET 8 SDK (phase 6).
