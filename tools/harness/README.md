# Harness

Drives the real `bin/WaterLauncher.exe` (a dev build: `wails3 build`) the way a person would, to prove that screens, input and launching work in the app itself rather than in the browser mock.

- **Interface**: WaterLauncher starts with `--remote-debugging=9333`, and Playwright (`playwright-core`, no browser download) attaches to its WebView2 windows over the DevTools protocol. Screen sizes are emulated with `Emulation.setDeviceMetricsOverride` at device scale 1, so a 3840×2160 screenshot is 3840×2160 pixels.
- **Controller**: `--virtual-pad[=ps|xbox]` plugs in a virtual controller made by SDL itself. Buttons are pressed through the dev pipe `\\.\pipe\waterlauncher-dev` (current user only), so everything from SDL on is the real path: mapping, repeat, events, focus.
- **Data**: `--dev-data=DIR` keeps the library, settings and log in DIR and freezes the library (no scans, metadata, store accounts or update checks). `makeDevData` copies the real games for their art but points them at the fake game, clears their store links and sets them to *Native*, so a test never starts a real game or changes Steam's shortcuts. `%APPDATA%\WaterLauncher` is backed up before a run and restored after (a backup left by a crashed run is restored first).
- **Games**: `tools/fakegame` is a full-screen stand-in with `--launcher` (hands over to a copy in `Game\`), `--slow=S`, `--crash=S` and `--run=S`, read from `fakegame.txt` next to the exe.

The dev flags only work in dev builds (version `dev`); release builds ignore them.

```bash
npm install
node smoke.mjs
```

| Script | |
|---|---|
| `smoke.mjs` | starts the app, presses two buttons, prints state and memory |
| `tour.mjs [--input=pad\|keys\|both] [--layouts=deck,console,orbit] [--sizes=1920x1080,…]` | every screen of desktop mode and big picture, driven by the controller or the keyboard, screenshotted at six sizes (`sizes.mjs`) with a layout check (`audit.mjs`); a contact sheet per screen and `report.json` |
| `launches.mjs [--from=bigpicture\|desktop] [--only=Direct,Crash]` | plays each fake game (direct, launcher handover, slow start, crash) and records the session, the windows and the controller mode over time |
| `drive.mjs [name] [--bp] [--extra=N] [--layout=orbit]` | keeps the app running behind `127.0.0.1:9444` for exploring by hand: `/pad?b=down`, `/key?k=ArrowDown`, `/snap?name=x`, `/eval` (POST), `/state`, `/mem`, `/quit` |
| `refresh.mjs` | runs the app once on the real library so its metadata is current, and keeps a copy for the test data |

Contact sheets need Python with Pillow (`sheet.py`).

Output goes to `%TEMP%\wl-harness` (or `WL_OUT`).

## Dev pipe

One command per line, one answer per line (`ok`, JSON, or `error: …`):

| Command | |
|---|---|
| `press <button> [ms]` | down, wait, up. Buttons: `south east west north back guide start lb rb up down left right touchpad leftstick rightstick misc` |
| `down <button>`, `up <button>` | hold and let go |
| `axis <lx\|ly\|rx\|ry\|lt\|rt> <value>` | -32768 to 32767 |
| `plug ps\|xbox`, `unplug` | |
| `state` | controller, launch session, which windows are open |
| `mem` | the core's private bytes and Go heap |
| `quit` | |
