# Linux / Steam Deck feasibility (spike)

Status: exploration on `spike/linux`, 2026-10-04. Not for merging. Seaglass stays a Windows launcher (PLAN.md section 1); this page answers "could it run on Linux and SteamOS, and is that worth doing?"

## TL;DR

- **It compiles for Linux now.** The Windows-only code sits behind `_windows.go` files, and `_linux.go` stubs return `platform.ErrNotSupported` or empty results. The Windows build, `go vet` and `go test ./...` on Windows are unchanged and pass.
  - With `CGO_ENABLED=0`, `GOOS=linux go build -tags server ./...` passes and so does `go vet -tags server ./...`. That builds a 20 MB static ELF using Wails' headless `server` mode.
  - The real GUI build needs cgo, GTK4 and WebKitGTK 6.0, so it can't be cross-compiled from Windows. The `linux-spike` workflow (ubuntu-24.04) runs `go build ./...`, `go vet ./...` and builds `bin/seaglass` with cgo: all green.
  - Tests on Linux: 8 of 18 packages pass; the rest assume Windows (section 2).
- **A Linux build that does nothing useful yet:** it finds no games, can't launch any, and has no controller.
- **MVP effort: about M–L, roughly 4–6 focused weeks.** That covers Steam and Heroic libraries, launching through `steam://` and `heroic://`, playtime from watching `/proc`, SDL3 through purego, and an AppImage.
- **Recommendation: no-go as a Game Mode launcher on the Deck. A small, conditional go for Linux desktop and Desktop Mode, only if Syncer gets a Linux port too.** Details are in the Recommendation section.

## 1. What is Windows-only today (inventory)

Paths are as they are on `spike/linux`. The Windows code moved there verbatim; on `main` the same functions sit in the file without the `_windows` suffix.

### Registry

| What | Where |
|---|---|
| GOG installs (`HKLM\SOFTWARE\WOW6432Node\GOG.com\Games`) | `internal/scan/stores_windows.go:14` |
| EA / Origin installs (four hives) | `internal/scan/stores_windows.go:37` |
| Ubisoft installs (`Ubisoft\Launcher\Installs`) | `internal/scan/stores_windows.go:64` |
| Registry helpers (`registryEach`, `regString`, `regInt`) | `internal/scan/stores_windows.go:81-110` |
| Uninstall entries (installers, repacks, **Battle.net** games via `uninstall.go:94`) | `internal/scan/uninstall_windows.go:5` |
| Steam folder (`HKCU\Software\Valve\Steam\SteamPath`) | gamekit `steam/dir_windows.go`; `steam/dir_other.go` returns `""` off Windows (gamekit change needed, not made here) |
| Steam language | `internal/achievements/steamlang_windows.go:11` |
| Start with Windows (`…\Run`, Task Manager's `StartupApproved`) | `internal/platform/startup_windows.go:24-125` |
| Machine id (`MachineGuid`) for profile file names | `internal/app/profile_windows.go:6` |
| WebView2 version (diagnostics) | `internal/app/diagnostics_windows.go:17` |
| Syncer install (`Uninstall\ApolloFSyncer`) | `internal/syncer/client_windows.go:83` |
| `%VAR%` expansion in .lnk files | `internal/lnk/ansi_windows.go:39` |

### Launcher detection

| Store | Windows signal | Where | Linux counterpart |
|---|---|---|---|
| Steam | registry → `libraryfolders.vdf`, `appmanifest_*.acf` | `internal/scan/steam.go:33`, gamekit `steam` | `~/.local/share/Steam` (`~/.steam/steam` is a symlink to it), Flatpak `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`. The VDF and ACF formats are the same, so only `Dir()` changes. |
| Epic | `%ProgramData%\Epic\…\Manifests\*.item` | `internal/scan/stores.go:48` | Heroic: `~/.config/heroic/legendaryConfig/legendary/installed.json` (or the Flatpak path) |
| GOG | registry + `goggame-*.info` | `stores_windows.go:14`, `stores.go` (`readGogInfo`) | Heroic: `gog_store/installed.json`. `goggame-*.info` scanning still works. |
| GOG Galaxy owned games | `%ProgramData%\GOG.com\Galaxy\storage\galaxy-2.0.db` | `internal/owned/gog.go:19` | No Galaxy on Linux. Use Heroic's GOG library, or keep the GOG token route. |
| EA, Ubisoft, Battle.net | registry | see above | Only inside Wine/Proton prefixes, via Lutris/Bottles/Heroic. Not worth it for an MVP. |
| Xbox | `X:\XboxGames\*\appxmanifest.xml`, `shell:AppsFolder` | `internal/scan/stores.go:150` | None (Game Pass PC doesn't run on Linux) |
| Shortcuts | Start menu and desktop `.lnk` | `internal/scan/shortcuts.go:21`, `internal/lnk` | `.desktop` files in `~/.local/share/applications` (Heroic, Lutris and Steam all write them) |
| Auto folders | `C:\Games`, `Program Files\DODI-Repacks`, … on each fixed drive | `internal/scan/folders.go:28` | `~/Games`, Heroic's default `~/Games/Heroic`, and Proton prefixes (`steamapps/compatdata/*/pfx/drive_c`) |

### Controller (DualSense)

| What | Where |
|---|---|
| SDL3.dll embedded, written to `%LOCALAPPDATA%`, loaded with `LoadLibraryEx`, called through `windows.Proc` (no cgo) | `internal/pad/dll_windows.go:44` |
| Function table and helpers (`cstr`, `gostr`) | `internal/pad/dll_windows.go:24-112` |
| Virtual pad for the harness (`SDL_AttachVirtualJoystick`, via `s.dll.FindProc`) | `internal/pad/virtual.go:91-150` |
| Linux stub (every call fails, layer stays off) | `internal/pad/dll_linux.go` |

The rest of `pad.go` (814 lines: intents, haptics, passive mode, lightbar) is portable. It only needs a loader that hands it `Call`-able functions.

### Updater and installer

| What | Where |
|---|---|
| NSIS per-user installer, `/S /relaunch /tray`, `uninstall.exe` marks an install | `build/windows/nsis/project.nsi`, `internal/update/apply.go:17-50` |
| Exe swap for portable copies (rename the running exe) | `internal/update/apply.go:56` |
| Assets are `Seaglass-setup.exe` / `Seaglass.exe` | `internal/update/update.go:28` |
| Authenticode publisher check on updates | `internal/update/pending.go:91`, `internal/platform/signer_windows.go:24` |
| ed25519 `SHA256SUMS.sig` check | `internal/update/signature.go` (portable) |

### Processes, playtime and windows

| What | Where |
|---|---|
| Launch: `steam://` and store URIs through `ShellExecute` | `internal/platform/shell.go` (allowlist) and `shell_windows.go:63`. Called from `internal/app/launch.go:190-196`. |
| Launch an exe: `CreateProcess` with a raw command line, `CREATE_BREAKAWAY_FROM_JOB`, UAC fallback | `internal/platform/shell_windows.go:25` |
| Process list (Toolhelp32), image path and start time, end, exit code | `internal/platform/process_windows.go:18-131`, used by `internal/launch/launch.go:185` and `track.go:65` |
| Games started elsewhere: WinEvent foreground hook | `internal/platform/foreground_windows.go:69`, `internal/app/external.go:47` |
| Bring to front, mark full screen (`ITaskbarList2`), HWNDs from `NativeWindow()` | `internal/platform/window_windows.go`, `internal/app/shell.go:116-130` |
| Full-screen event is `events.Windows.WindowFullscreen` (never fires on Linux) | `internal/app/shell.go:166` |
| Overlay window options (`WindowsWindow{HiddenOnTaskbar…}`) | `internal/app/shell.go:293` |
| Single instance: Wails' Windows mutex name, opened directly | `internal/platform/instance_windows.go:15`, `main.go:62` |
| WebView2 options (`WebviewUserDataPath`, `AdditionalBrowserArgs`, CDP port for the harness) | `main.go:94-141` |
| Dev pipe `\\.\pipe\…` with a SID ACL (harness) | `internal/app/dev_windows.go:12` |
| Steam Input route: `steam.exe -shutdown`, `ProcessRunning("steam.exe")` | `internal/steaminput/steaminput.go:222-233` |

### Shell and Explorer integration

- *Show in Explorer* runs `explorer.exe`: `internal/platform/shell_windows.go:14`, called from `internal/app/services.go:277,426`.
- Text files open through ShellExecute: `internal/platform/shell.go` (`OpenFile`).
- Desktop shortcuts to games: `internal/app/core.go:356`.
- The frontend's wording names Windows and Explorer in 16 places, for example "Start with Windows" in `desktop/Settings.svelte`.

### Paths and secrets

- Known folders (`FOLDERID_*`): `internal/platform/platform_windows.go:11`. Linux uses XDG: `Roaming` = `$XDG_CONFIG_HOME`, `Local` = `$XDG_DATA_HOME`. The Windows-only ones are `""` in `platform_linux.go`.
- Drive letters: `platform_windows.go:36`. Real case: `:61`. `platform.Key`/`Within` compare case-insensitively, which is wrong on Linux; it's harmless for the spike but must change for a port.
- Some joins with an empty root become relative on Linux, for example `EpicManifestDir()` (`stores.go:48`) and `owned/gog.go:19`. A port should guard them.
- DPAPI secrets (SteamGridDB, Steam Web API, Epic and GOG tokens): `internal/platform/secret_windows.go:11`. On Linux these belong in the Secret Service (libsecret over D-Bus; `godbus/dbus` is already an indirect dependency through Wails).

### Syncer

- The client dials `\\.\pipe\syncer` with go-winio and checks that the pipe server's SID matches its own: `internal/syncer/client_windows.go:21-80`.
- It starts `Syncer.exe --api`: `internal/syncer/client.go` (`Dial`).
- Syncer itself is a Windows app. A Linux Seaglass gets nothing from it until Syncer has a Linux build: a Unix socket under `$XDG_RUNTIME_DIR`, a peer check with `SO_PEERCRED`, and Windows save paths mapped into `compatdata/<appid>/pfx/drive_c/users/steamuser/…`.

### Maintainer tools

`tools/fakegame` and `tools/release` (DPAPI-held release key, console echo) are marked `//go:build windows`. The Playwright harness drives WebView2 over CDP, which WebKitGTK doesn't offer; it would need WebKitWebDriver.

## 2. Build results

| Check | Result |
|---|---|
| `GOOS=linux GOARCH=amd64 go build ./...` on `main` (before) | fails. `golang.org/x/sys/windows` is excluded (tools/fakegame, internal/achievements → registry), and Wails' Linux files need cgo (`undefined: pointer`). |
| Same, on `spike/linux`, `CGO_ENABLED=0 -tags server` | **passes** (static ELF, 20 MB) |
| `GOOS=linux go vet -tags server ./...` (Windows host, no cgo) | **passes** |
| `go build ./...` + `go vet ./...` with cgo on ubuntu-24.04 (GTK4, WebKitGTK 6.0) | **passes** (`linux-spike` workflow) |
| `go test ./...` on Linux (informational, first run) | 8 packages pass: library, logx, owned, profile, sqlite, steaminput, syncer, update. 10 fail: achievements, app, identify, launch, lnk, meta, platform, scan, settings and pad. Most failures come from `C:\` fixtures, case-insensitive paths, or stubs. pad's tests wait for SDL and hang until the 10-minute timeout; they should skip when `loadSDL` fails. |
| Windows `go build ./...`, `go vet ./...`, `go test ./...` | **pass**, unchanged |

On Linux, `go vet` also compiles the tests. Windows-only tests moved to `*_windows_test.go` (`signer`, `process_image`, `foreground`).

## 3. Findings from research

### Steam on Linux

- **Install folder:** `~/.local/share/Steam`; `~/.steam/steam` and `~/.steam/root` are symlinks to it. Flatpak uses `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`; Snap probably uses `~/snap/steam/common/.local/share/Steam` (unverified). Probe them in that order and take the first that has `steamapps/libraryfolders.vdf`.
- **Files Seaglass already reads:**
  - `libraryfolders.vdf`, appmanifests and `userdata/<id>/config/shortcuts.vdf` keep their Windows formats.
  - Instead of the registry, `~/.steam/registry.vdf` holds `language`, `AutoLoginUser` and `RunningAppID`. `RunningAppID` would be a cheap "Steam is running a game" signal, but whether it stays up to date isn't verified.
  - Proton prefixes are `<library>/steamapps/compatdata/<appid>/pfx/`. That is where save paths and external copies live.
- **Deck SD cards** mount under `/run/media/…`. The mount path changed between images, so always read it from `libraryfolders.vdf`.
- **Launching:** `steam steam://rungameid/<id>` or `xdg-open steam://…`, which hands the URL to a running client. For Flatpak it's `flatpak run com.valvesoftware.Steam steam://…`. The shortcut id math (`id<<32 | 0x02000000`) is the same as on Windows.
- **Tracking:**
  - Steam wraps every launch, its own games and non-Steam shortcuts alike, in `…/ubuntu12_32/reaper SteamLaunch AppId=<id> -- …`.
  - A program running as the same user can scan `/proc/*/cmdline` for `reaper` with `AppId=` and follow that process's descendants. This is a better signal than Windows' process tree plus folder match.
  - Under Proton, a process's `comm` is the .exe name cut to 15 characters, and its cmdline holds a `Z:\…` path. Match on the cmdline basename.

### Heroic and Lutris

- **Heroic config folder:** `~/.config/heroic/`, or `~/.var/app/com.heroicgameslauncher.hgl/config/heroic/` for the Flatpak.
- **Heroic install lists:**
  - Epic: `legendaryConfig/legendary/installed.json`
  - GOG: `gog_store/installed.json`
  - Amazon: `nile_config/nile/installed.json`
  - Sideloaded games: `sideload_apps/library.json` (unverified)
- **Heroic launch link:** `heroic://launch?appName=<id>&runner=<legendary|gog|nile|sideload>`, or the older `heroic://launch/<runner>/<appName>`.
- **Lutris library:** `~/.local/share/lutris/pga.db` (SQLite, which `internal/sqlite` can already read).
- **Lutris launch links:** `lutris:rungameid/<id>` and `lutris:rungame/<slug>`. These URIs have been flaky in some versions.

### Controller input

- **Loading SDL3 without cgo:** purego (`ebitengine/purego`) loads `libSDL3.so.0` that way. The bindings `jupiterrider/purego-sdl3` and `Zyko0/go-sdl3` show it works, so `dll_linux.go` can mirror `dll_windows.go` with `purego.Dlopen`/`RegisterLibFunc`. purego also works with cgo on, which Wails needs anyway.
- **Bundle SDL3:** Arch has `sdl3`, but SteamOS and the Flatpak runtimes are unreliable; one Flathub app broke on an old runtime SDL.
- **Permissions:**
  - DualSense haptics, LEDs and gyro go through HIDAPI over `/dev/hidraw*`. Without access, SDL falls back to evdev (`hid-playstation`) with basic input only.
  - Access comes from the `steam-devices` udev rules (`uaccess`). They're present on SteamOS and usually installed with Steam elsewhere.
  - A Flatpak can't add udev rules and needs `--device=all`.
- **Steam Input on the Deck:** Steam owns the built-in controls. Anything launched from Steam, non-Steam shortcuts included, sees a virtual Xbox 360 pad, and Steam tells SDL to hide the physical device. Seaglass's DualSense-first features mostly don't apply in Game Mode unless the user turns Steam Input off for the Seaglass shortcut.

### Wails v3 on Linux

- **Default stack:** GTK4 with WebKitGTK 6.0 since August 2026. The `gtk3` tag keeps GTK3 with webkit2gtk-4.1 for the whole v3.0 series. beta.26 (2026-09-25) has no Linux-specific notes.
- **Known problems:**
  - Blank or white windows on NVIDIA. The workaround is `WEBKIT_DISABLE_DMABUF_RENDERER=1`.
  - WebKitGTK 6.0's bwrap sandbox aborts on stock Ubuntu 23.10+.
  - Window positioning is limited on Wayland.
  - Recent fixes cover thread-unsafe URI-scheme handling and GTK4 transparency.
  - I couldn't verify tray, multi-window or full-screen status. The Seaglass overlay window (frameless, always on top, transparent) is the riskiest part.
- **Packaging:** `wails3 package` / `wails3 task linux:create:{appimage,deb,rpm}` (nFPM). There's no first-party Flatpak task; a manifest on the GNOME runtime (which ships WebKitGTK 6.0) is DIY.
- **Memory:** WebKitGTK's WebProcess is a separate process like WebView2's. Closing the windows for game mode should still free it, but this needs measuring.

### Deck Game Mode

- **A non-Steam app works as a launcher there:** add it as a non-Steam shortcut. gamescope shows one focused app at a time. Children inherit the shortcut's reaper and AppId and show as that app. ES-DE (EmuDeck), Heroic and Lutris all run this way.
- **What gamescope doesn't allow:** no xdg-desktop-portal, one app in focus, no window enumeration. Seaglass's overlay window, tray and "bring to front" don't exist there, and the PS-button overlay can't work. Steam's Quick Access menu owns that role.
- **Launching a Steam game from inside Seaglass** with `steam://rungameid` starts a separate Steam app, and focus should move to it (unverified).
- **Install route:** the root file system is read-only, so Flatpak (Discover) or an AppImage in `~/Applications` are the only real options.
- **Alternatives that fit the Deck better:** write Steam shortcuts the way Steam ROM Manager does, or write a Decky Loader plugin.

### Competitors

| App | What it is | Gap |
|---|---|---|
| Heroic | Electron launcher for Epic, GOG, Amazon and sideloaded games | Heavy; no shared library with Steam or Lutris; controller UI is secondary |
| Lutris | GTK/Python game manager with install scripts | Desktop only, complex, no controller UI |
| Bottles | Wine prefix manager | Not a library |
| Playnite | The closest equivalent to Seaglass | Still Windows-only. The Linux port is planned after an Avalonia rewrite, "maybe during 2026"; no release seen. |
| ES-DE / EmuDeck / Pegasus | Controller-first frontends | Emulation first, weak for PC stores |
| Steam ROM Manager | Writes shortcuts and art into Steam | Batch tool, no library or playtime |
| Junk Store | Decky plugin (Epic; GOG and Amazon paid) | Deck only, paid |
| Cartridges | GNOME app importing Steam, Heroic, Lutris, Bottles and itch | Desktop only, no controller UI |

**What Seaglass would add:**
- One library from local files across Steam, Heroic, Lutris and loose copies, with no accounts.
- Playtime across stores.
- **Syncer save sync between a Windows PC and a Deck.** Nothing else does this, but it depends on a Syncer Linux port.
- External-copy identification, including inside Proton prefixes.
- A DualSense-first big picture for Linux desktops and HTPCs.

**Where it would only duplicate Steam:** being the full-screen UI in Deck Game Mode.

## 4. Effort per area

S is up to 2 days, M is up to 2 weeks, L is more (or depends on another project).

| Area | Size | Notes |
|---|---|---|
| Build tags, stubs, Linux CI | **done** (S) | this branch |
| Paths (XDG), case-sensitive `Key`/`Within`, empty-root guards | S | |
| Processes via `/proc` (list, image, start time, end, exit code via `pidfd`/`waitid` or polling) | S–M | Exit codes only for direct children; otherwise "ended" without a code |
| Single instance | S | Wails has `single_instance_linux.go`; replace the Windows mutex peek in `instance_windows.go` with a lock file or D-Bus name |
| Secrets → Secret Service (libsecret over D-Bus) | S | |
| Start at login → XDG autostart `.desktop` | S | Wails has `autostart_linux.go` |
| Open links, files and folders → `xdg-open` / `steam` / `flatpak run` | S | Keep the scheme allowlist; add `heroic://` and `lutris:` |
| Steam library on Linux (gamekit `Dir()` for native and Flatpak, `registry.vdf` language) | S | Change belongs in gamekit; Seaglass's scan works on top of it |
| Heroic libraries (Epic, GOG, Amazon, sideload) + `heroic://` launch | M | New source in `internal/scan`, owned-game merge keys stay `epic:` and `gog:` |
| Lutris library (`pga.db`) + `lutris:` launch | S–M | Reuses `internal/sqlite` |
| Playtime: reaper/AppId and descendant tracking from `/proc` | M | Replaces the WinEvent hook for "started elsewhere" too (poll `/proc` every few seconds; Linux has no non-root process-start event without netlink privileges) |
| Controller: SDL3 through purego, bundled `libSDL3.so`, hidraw permission check and hint | M | `pad.go` logic is reused unchanged |
| Steam Input route on Linux (`shortcuts.vdf`, `steam -shutdown`, process name `steam`) | S–M | Same file format; on the Deck, Steam Input is on anyway |
| Window management: close-to-tray, overlay window, full screen, bring to front without HWNDs | M–L | Wayland and gamescope limit this; the overlay is the hardest part |
| WebKitGTK rendering and performance (CSS `zoom`, Orbit's compositor tricks, 4K) | M | Needs the perf harness rebuilt without CDP |
| Packaging: AppImage first, Flatpak later | M | Flatpak brings sandbox holes: Steam folders, `/proc`, hidraw |
| Updater: AppImage self-update (reuse the ed25519 `SHA256SUMS` check, swap the file) or leave it to Flathub | M | The NSIS path and Authenticode stay Windows-only |
| Achievements (Steam stats under the Linux root, emulator files inside prefixes) | M | |
| External copies inside Proton prefixes and `~/Games` | M | |
| Syncer integration | L | Needs Syncer on Linux (Unix socket, peer check, Proton save-path mapping); separate repo |
| EA, Ubisoft, Battle.net, Xbox on Linux | skip | Only via Wine prefixes; low value |
| DLSS Updater add-on | skip | Windows and .NET |
| Frontend wording (Windows, Explorer), Linux glyph and keyboard checks | S | |
| Test harness on WebKitGTK (WebKitWebDriver instead of CDP) | M | |

## 5. Minimal MVP

Scope: Steam and Heroic libraries, launching through `steam://` and `heroic://`, playtime from watching processes, the controller through SDL. Linux desktop and the Deck's Desktop Mode, shipped as an AppImage. In Game Mode it runs as a non-Steam shortcut, best effort.

1. **gamekit:** `steam.Dir()` for Linux. Probe `~/.local/share/Steam`, `~/.steam/steam` (resolving the symlink) and the Flatpak path, and require `steamapps/libraryfolders.vdf`. Also `steam.Language()` from `registry.vdf`. Release gamekit v0.2 and bump it here.
2. **platform/linux:**
   - Make `Key`/`Within` case-sensitive on Linux (a per-OS `fold` function).
   - Guard empty roots.
   - Implement `Processes`, `ProcessImage` (`/proc/<pid>/exe`, plus start time from `/proc/<pid>/stat` field 22), `EndProcess` (`kill` after checking the start time) and `ProcessRunning`.
   - `OpenURI` through `xdg-open` with the allowlist plus `heroic://`.
3. **Steam scan:** works once step 1 lands. Leave out Proton and Steam Linux Runtime app ids (already in `steamTools`).
4. **Heroic source** in `internal/scan/heroic.go`:
   - Read Legendary's `installed.json` and `gog_store/installed.json`, native and Flatpak.
   - Set `EpicApp`/`GogID` so metadata, owned-game merging and achievements keep working.
   - Set `LaunchURI = heroic://launch?appName=…&runner=…`.
5. **Launch:** `steam://rungameid/<id>` and `heroic://…` through `OpenURI`. Leave direct-exe launches out of the MVP; Windows exes need Proton or umu-launcher.
6. **Playtime:**
   - In `launch.Manager`, keep the poll loop but feed it `/proc`.
   - For Steam games, a session is the `reaper … AppId=<id>` process and its descendants.
   - For Heroic games, a session is the process tree under Heroic whose cmdline points into the game's install folder (the existing folder match).
   - It ends when the tree is gone.
7. **Controller:**
   - Write `internal/pad/dll_linux.go` on purego: `Dlopen` a bundled `libSDL3.so.0` (pinned hash, like the DLL), with the same function table.
   - If hidraw isn't readable, log it and show "install steam-devices udev rules" in Settings → Controller.
   - On the Deck in Game Mode, expect Steam's virtual Xbox pad.
8. **Windows and tray:**
   - Run the main window plus tray. Close the window while a game runs, as on Windows.
   - Leave the PS-button overlay out of the MVP: `platform.BringToFront`/`MarkFullscreen` stay no-ops.
   - Hook `events.Common` full-screen events where Windows uses `events.Windows`.
9. **Packaging:** `wails3 task linux:create:appimage`, plus `WEBKIT_DISABLE_DMABUF_RENDERER=1` in the AppRun when NVIDIA is detected.
10. **Check on hardware:**
    - A Deck in Desktop Mode, and in Game Mode added as a non-Steam shortcut: launch a Steam game and a Heroic game, playtime counts, the interface comes back.
    - An Arch or Fedora desktop with a USB and Bluetooth DualSense.

The cost is about 4–6 focused weeks. It leaves out Syncer, the updater, achievements, Lutris, external copies, the overlay and Flatpak.

## 6. Risks

- **The Deck's main mode works against the concept.**
  - In Game Mode, Steam is the launcher, Steam Input hides the DualSense, and gamescope allows one app in focus with no overlay windows.
  - Seaglass's signature features (DualSense-first input, PS-button overlay, closing the interface during play) mostly don't apply there.
- **Wails v3 on Linux is a beta on a beta.** It brings GTK4 and WebKitGTK 6.0 churn, NVIDIA and DMABUF blank windows, the bwrap sandbox on Ubuntu, and Wayland limits on positioning. The Windows app is tuned to WebView2's performance (section 22 phase 3); WebKitGTK will behave differently, with no CDP harness to measure it.
- **Syncer is the differentiator, and it's Windows-only.** Without a Syncer Linux port, Seaglass on Linux is "Cartridges with a controller UI".
- **Packaging and permissions:** a Flatpak needs broad holes (Steam folders, `/proc`, hidraw). An AppImage needs FUSE and doesn't update on its own. SteamOS's read-only root rules out anything else.
- **Matching:** case-sensitive paths, Proton's `Z:\` cmdlines and processes cut to 15 characters break assumptions spread through `scan`, `launch` and `identify` (`platform.Key` lower-cases everywhere).
- **Maintenance cost:** every Windows-only feature now needs a Linux answer or a visible "not on Linux". Two CI matrices, two packaging paths, and testers on hardware the maintainer may not have daily.
- **Shared code:** gamekit's Steam `Dir()` has to change. Syncer also uses gamekit, which is fine but couples the releases.

## 7. Recommendation

**No-go as a Steam Deck Game Mode launcher.** Steam's Game Mode UI already is that, and gamescope plus Steam Input remove most of what makes Seaglass different.

**Conditional go for a Linux desktop / Desktop Mode MVP (section 5), only if a Syncer Linux port is also on the table.**
- The compile work is done, the Go core is mostly portable, and Steam plus Heroic libraries and launching are cheap.
- The cross-PC story is where Seaglass would stand out: Windows desktop and Deck sharing saves, playtime and achievements through Syncer accounts.
- Without Syncer, the effort buys a nicer Cartridges, so park it.

A cheaper first step that's useful on its own: the gamekit Linux `Dir()` and a Heroic reader. Both also help Syncer on Linux. Then keep this branch as a reference for which seams exist.
