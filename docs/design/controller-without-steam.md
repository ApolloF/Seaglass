# Controller support without Steam

Status: proposal (phase 1). Nothing here is built yet.

## Problem

When a game supports Xbox controllers but not PlayStation ones, Seaglass starts it through Steam so that Steam Input makes a DualSense or DualShock 4 look like an Xbox pad. Florian reports that this works badly. The goal is to make PlayStation (and other) controllers work in XInput-only games without Steam.

## How the Steam route works today

1. **Choosing the route.** `route()` in `internal/app/launch.go:126-146` decides first, and `frontend/src/lib/route.ts` repeats the same logic for the interface.
   - A store link always wins.
   - Next comes the per-game `PadMode` (`internal/library/library.go:72`): `""` is Auto, the others are `native` and `steam`.
   - In Auto, a game goes through Steam Input only when all of these hold: a PlayStation pad is connected, the game's metadata says Xbox controller support is `full` or `partial`, and the game has no DualSense support.
2. **Adding the shortcut.** `steamInputStep` (`launch.go:298-350`) looks for a non-Steam shortcut in `shortcuts.vdf` (`internal/steaminput`).
   - If the shortcut is missing and Steam is running, Seaglass asks: restart Steam, start without it, or cancel.
   - Steam is shut down, every Steam-mode game is added in one batch, and the step sets `steam://rungameid/<id>`.
3. **Starting the game.** `platform.OpenURI(steam://…)` returns no process id. Tracking therefore falls back to the folder rule only (`internal/launch/track.go:123`, `:100-105`). Seaglass's own pad layer goes Passive (`padForGame`, `launch.go:235-245`).

## Why it works badly

These failure modes come from reading the code and docs. None was reproduced on a PC.

- **Steam has to restart to see a new shortcut.** It only reads `shortcuts.vdf` when it starts (ROADMAP risk table). The first launch of each game asks to restart Steam, and a Steam that hangs while closing fails the launch (1-minute cap, `steaminput.go:225-250`).
- **Steam has to be installed, signed in and running.** `ErrNoSteam` only checks for a `userdata` folder (`steaminput.go:53-66`). Steam's own updates and its login screen sit between the user and the game.
- **The result depends on a Steam setting Seaglass can't see.** Steam's *Enable Steam Input for PlayStation controllers* decides whether anything gets translated (`docs/releases/v0.4.0.md:32`). Seaglass never reads it. Steam's per-game layouts can also change the mapping.
- **Steam and Seaglass both want the PS button.** Seaglass uses Home for its overlay. Steam opens its own overlay or Big Picture on the same press. The docs tell people to turn Steam's shortcut off, but the "detect and warn" item in the ROADMAP was never built.
- **Playtime tracking gets weaker.** With no process id, a game whose exe is outside the known folders is never seen. A Steam start that takes more than 5 minutes (updates, shader caches) ends the session unseen (`launch.go:179-181`).
- **Shortcuts pile up.** The shortcut id comes from the exe and the name (`steaminput.go:41-43`). Renaming a game or moving its exe adds a new shortcut and leaves the old one in Steam. Nothing tidies them up (open ROADMAP item).
- **A broken `shortcuts.vdf` breaks every Steam-mode launch** (`steaminput.go:100-108`). Steam could also start between Seaglass's "is Steam running" check and its write (`steaminput.go:115`).

## Options

Seaglass already *reads* every common controller through the embedded SDL3 (DS4, DualSense including Bluetooth, Switch Pro, Xbox, generic HID with SDL's mapping database). That part is solved. What's missing is a way to show the game an **Xbox controller**, and to **hide the physical pad** so that games which also read it directly don't get every press twice.

### A. ViGEmBus (virtual Xbox 360 pad) plus HidHide (hide the real pad)

This is what DS4Windows has used for years.

- **ViGEmBus** is a kernel bus driver that plugs in virtual Xbox 360 or DS4 pads, which games see as real XInput devices.
  - Licence: BSD-3-Clause. The client protocol comes from ViGEmClient (MIT): the app opens the bus's device interface and sends `DeviceIoControl` requests (plug a target, submit a report, wait for rumble notifications). Go can do that through `golang.org/x/sys/windows` without cgo. No Go client exists; a pure Rust one does (`vigem-client` crate), which shows the protocol can be reimplemented without the C SDK.
  - Status: archived on 2023-11-02 after a trademark dispute; the last release is v1.22.0 (installer `ViGEmBus_1.22.0_x64_x86_arm64.exe`).
  - Signing: the driver is Microsoft-signed for Windows 10/11 x86, x64 and ARM64, and existing installs keep working. No breakage on Windows 11 24H2/25H2 was found in the sources, but none were checked against it either. **Risk:** the driver is frozen, so a future Windows change would not be fixed.
- **HidHide** is a filter driver that hides chosen HID devices from every app except allowed ones.
  - Licence: MIT. The last release found is v1.5.230 (May 2024), and the maintainer has no capacity for major work.
  - It is configured through `\\.\HidHide` with documented IOCTLs (allow list, block list, active flag). These take MULTI_SZ lists, so Go can drive them without cgo.
  - It also has a **session block list** for feeder apps. Entries live only in kernel memory and disappear when the owning process exits, so a crash of Seaglass can't leave a controller hidden. It needs checking whether the owner keeps seeing the device it hid, or whether Seaglass must also be on the allow list.
- **Controllers:** every controller SDL reads, presented to the game as an Xbox 360 pad.
- **Install:** two signed installers from the projects' GitHub releases. Each needs one UAC prompt, and HidHide may ask for a restart. Not bundled; see *Licences*.
- **Anti-cheat:** some games refuse to start while DS4Windows runs (for example the Battlefield 6 beta). No bans tied to ViGEmBus alone were found. Expect a few games to block it; the per-game mode then switches that game back to Native or Steam.

### B. VIIPER over usbip-win2 (virtual USB devices)

- **What it is:** VIIPER (Alia5, the author of SISR/GlosSI) is written in Go. It is a USB/IP *server* that emulates real USB devices: Xbox 360, DS4, DualSense (including Edge) and Switch 2 Pro.
- **Windows driver:** VIIPER needs **usbip-win2** (BSD-2, WHLK/attestation signed, actively developed) attached to localhost.
- **Licences:** the server is GPL-3.0 and its clients are MIT. GPL-3.0 code may be combined with Seaglass's AGPL-3.0 (AGPL §13), so it could even be linked in as a Go module.
- **Pros:** maintained, can present a DualSense as a DualSense, and the driver is current.
- **Cons:**
  - The usbip-win2 install restarts the USB 3 hubs, and its README advises making a restore point first.
  - VIIPER describes itself as early (v0.7.0, June 2026).
  - There is no way to hide the physical pad, so HidHide is still needed.
  - Neither project has been tried with Seaglass.

### C. Driverless

- **Windows.Gaming.Input.Custom and GameInput** can't *create* a gamepad. GameInput 3.5 improves DualSense *reading*, not virtual devices.
- **InputInjector gamepad injection** needs a restricted capability in a packaged app, so it doesn't fit an unpackaged Go exe.
- **Proxy `xinput1_3.dll` / x360ce-style DLLs** in the game folder need no driver. But they mean writing into game folders, break with GameInput or system32 loading, and look like cheating to anti-cheat. Seaglass's rule against modifying games rules this out.
- **Windows** still doesn't expose a DualSense through XInput.

### D. Keep Steam, but fix its rough edges

This would cover detecting Steam's PS-controller and Guide settings, cleaning up old shortcuts, and better tracking. It doesn't remove the dependency the request is about, but these remain useful fixes for anyone who keeps choosing Steam.

## Recommendation

**Build A: a "controller bridge" inside Seaglass on ViGEmBus + HidHide, with the drivers installed on request from their official releases.** Keep the Steam route as the per-game and global fallback. Put the virtual-pad side behind a small interface so a VIIPER/usbip-win2 backend (B) can replace ViGEmBus later if ViGEmBus stops working on a future Windows.

Why A over B now: it is the setup that DS4Windows users have run for years on Windows 10/11, with a one-click signed install and no USB hub restart. Its weak point is maintenance, not function, and the backend interface leaves room to switch.

### Design

**Components**

- **`internal/vpad`** (new, Windows only; pure parts testable everywhere):
  - `report.go` (pure): maps an SDL gamepad snapshot to an XUSB report.
    - Buttons go into the XInput bitmask.
    - Triggers scale from 0..32767 to 0..255.
    - Stick Y is inverted (SDL down = +, XInput up = +), with `-32768` clamped when negated.
    - The touchpad click and Create both become Back. Guide is **not** forwarded: the PS button stays Seaglass's overlay button.
  - `vigem.go`:
    - finds the bus interface with `CM_Get_Device_Interface_List` and opens it;
    - plugs an X360 target, then waits until the device is ready;
    - submits reports and unplugs;
    - runs a goroutine that waits for rumble notifications (overlapped `DeviceIoControl`) and passes rumble back to the physical pad.
    - The struct layouts and IOCTL codes are ported from ViGEmClient (MIT, credited in the notices), with size tests.
  - `hidhide.go`:
    - opens `\\.\HidHide`, adds the physical pad to the session block list and clears it on exit;
    - checks HidHide is active;
    - a pure helper turns SDL's device path (`\\?\HID#VID_054C&PID_0CE6#…`) into the instance id HidHide wants (`HID\VID_054C&PID_0CE6\…`), with MULTI_SZ encoding.
  - `Bridge` (the interface `Plug / Send / Rumble / Close`) and `Status()` (`{viGEm: installed+version, hidHide: installed+active}`).
- **`internal/pad`**:
  - A new `Bridge` mode, like Active but with HIDAPI on, so the DualSense works fully over Bluetooth and rumble reaches it.
  - Each poll (about 4 ms in this mode, so latency stays low) reads the gamepad's axes and buttons through SDL and hands a snapshot to a sink.
  - Home still produces the overlay action.
  - If the pad disconnects, the virtual pad stays plugged in at rest; when a pad comes back it carries on. The game keeps player 1.
- **`internal/app/launch.go`**:
  - A new route `RouteBridge = "bridge"`.
  - A plan step `controller` ("Xbox controller"), run before the game starts because some games only look for XInput pads at start-up:
    1. hide the pad;
    2. plug the virtual pad;
    3. put the pad layer in Bridge mode.
  - `onSession` undoes it when the game ends: unplug the virtual pad, clear the session block list, return to Active.
  - Since Seaglass starts the game directly, it gets a real process id, and the tracking problems of the Steam route go away.

**Route choice (Go and `route.ts` kept in step)**

- **Per-game `PadMode`:** `""` Auto, `native`, `xbox` (new: Seaglass's virtual Xbox controller), `steam`.
- **Auto:** where today's logic picks Steam Input, it picks the global *translator* instead:
  - Seaglass's bridge when ViGEmBus is installed;
  - otherwise Steam if the user chose it as the fallback;
  - otherwise direct, with a one-line notice: "Set up Seaglass's Xbox controller in Settings to play this game with your DualSense."

**Settings** (`internal/settings`, per PC because drivers are per PC):

- `PadTranslator string`: `"seaglass"` (default) or `"steam"`. It decides which tool turns a PlayStation controller into an Xbox one in Auto.
- `HidePadWhilePlaying bool`, default on, used only when HidHide is installed.
- Validation goes in `normalize`. The fields are mirrored in `types.ts` and `api.mock.ts`.

**Interface** (Settings → Controller, and BPSettings):

- A status row: "Xbox controller for games: ready" / "needs two small drivers".
- A **Set up** button.
- The translator choice.
- The hide toggle.
- Per-game modes gain *Xbox controller* in `Details.svelte`, `GameSheet.svelte` and `Console.svelte`.
- The launch text reads "Your controller works as an Xbox controller in this game."

**First-run setup**

- **Set up** downloads the two official installers from the projects' GitHub releases.
  - It checks them against SHA-256 values pinned in the code, the same way `SDL3.dll` is pinned.
  - It runs them; each shows its own UAC prompt.
  - It then re-checks `Status()`. If HidHide asks for a restart, Seaglass says so.
- Nothing is installed silently or without the button.
- A service method `ControllerService.SetUpDrivers()` backs the button, with a mock that walks through the states.

**When a driver is missing**

- **Without ViGEmBus:** Auto falls back as described (Steam or direct), and `xbox` mode shows the setup notice instead of failing the launch.
- **Without HidHide**, or if hiding fails: the game still starts with the virtual pad, and the launch panel warns "Some games may see your controller twice." Most XInput-only games ignore the DualSense anyway, which is why they need the bridge.
- **If Seaglass crashes mid-game**, the drivers clean up. HidHide drops session entries when the process dies. ViGEmBus removes targets when the owning handle closes (this needs confirming on a real PC).

**Licences:** nothing third-party is bundled.

- The installers are downloaded from their official releases at the user's request.
- The Go IOCTL client is our own code, ported from MIT/BSD sources and credited in `tools/notices`.
- BSD-3, MIT and BSD-2 (and GPL-3.0, for B later) are all compatible with AGPL-3.0.

### Testing

- **Unit tests (CI, claude-dev):**
  - report mapping: every button, stick inversion edges `-32768/32767`, trigger scaling, Guide not forwarded;
  - device path → instance id and MULTI_SZ encoding;
  - `unsafe.Sizeof` of the IOCTL structs against ViGEmClient's sizes;
  - route choice for each mode, translator and driver status, in Go and in `route.test.ts`;
  - settings normalize;
  - the launch plan includes and undoes the `controller` step (fake Bridge);
  - mock mode for the setup flow and the new labels, in both themes.
- **Live test, gated** (`WL_REAL_VIGEM=1 go test -run RealViGEm ./internal/vpad`): plugs an X360 target, sends a report, and checks it with `XInputGetState` from `xinput1_4.dll`. It needs ViGEmBus installed, so it isn't run on claude-dev unless installing the driver there is approved. It needs no physical controller.
- **Only on a real PC with a real controller (Florian):**
  - Set up from the button: two UAC prompts, plus a restart if asked.
  - DualSense over USB, then Bluetooth, in an XInput-only game: input, rumble, no double input in a game that also reads HID.
  - The PS button opens the overlay during the game and doesn't reach it.
  - Unplug and replug the pad mid-game.
  - Kill Seaglass mid-game: the pad must be visible to other apps again and the virtual pad gone.
  - A DS4 and a Switch Pro if available.
  - One anti-cheat game, to see whether it refuses to start.

### Tier

**Standard.**

- The goal is clear and acceptance can be written down.
- It touches about four areas: a new `vpad` package, pad modes, launch plan/route, settings plus UI.
- A wrong design costs moderately: the IOCTL interop and the pad threading are fiddly, but contained behind one interface.
- The `vigem.go` / `hidhide.go` / pad-thread pieces go to the Opus lead-implementer, and the settings/UI/route plumbing to the Sonnet implementer.
