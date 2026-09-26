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
