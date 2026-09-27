# DLSS Updater in Seaglass

The add-on host (`internal/addons`, `internal/app/addons.go`, Settings → Add-ons, the add-on cards on a game's details) lives on this branch, not on `main`. In 1.4 it was hard to notice that it did anything:

- Nothing happens until three manual steps are done: *Connect to WaterLauncher* in DLSS Updater, turning it on in *Settings → Add-ons*, and approving the exe's hash, which is asked again after every DLSS Updater update.
- Once on, it only shows as cards on a game's details in desktop mode and as badges in big picture. Its actions (update DLSS, install OptiScaler, restore) are desktop-only.
- The one automatic part, putting DLSS back before a game starts when a patch replaced it, runs silently. When it works there's nothing to see.

## Ways to bring it back

Roughly from least to most work:

1. **Found automatically.** Seaglass looks for `DLSSUpdater.exe` (its install folder, or an `addon.json` next to it) and offers it once: "DLSS Updater is installed. Let Seaglass keep DLSS current before games start?" A single approval replaces the connect, enable and hash steps. Pin the publisher (signature) or the GitHub release instead of the exe hash, so updates don't ask again.
2. **Visible when it matters.** Show a *DLSS* line in the launch sequence ("DLSS 310.2 put back" or "DLSS up to date") and in the overlay, so the automatic check can be seen. Add the actions to big picture's game page (Options → DLSS) with the same declarative UI the desktop cards use.
3. **A library view.** A *DLSS* filter in desktop mode listing games with DLSS, their version, and which ones a patch reverted, with *Update all*.
4. **Built in, no add-on.** Seaglass already knows every game's folder and exe. Reading `nvngx_dlss*.dll` versions is a few lines of Go (file version resource), so the badges and "reverted by a patch" could come without DLSS Updater. DLSS Updater would then only be needed for installing OptiScaler, ReShade and NVIDIA profiles, opened with the game preselected (`DLSSUpdater.exe --game <exe>`).
5. **The other way round.** DLSS Updater reads Seaglass's library (read-only `library.json`) so it knows games it can't find itself: repacks, plain folders, emulated Steam copies.

Option 1 plus 2 keeps the add-on protocol as it is and fixes most of what made it seem to do nothing. Option 4 is the lightest for users who only want DLSS kept current.
