# Syncer integration

Seaglass talks to [Syncer](https://github.com/ApolloF/syncer) through Syncer's launcher API. The API itself is specified in Syncer's repository: [docs/api.md](https://github.com/ApolloF/syncer/blob/main/docs/api.md). This page covers how Seaglass uses it.

## Connection

- **Client:** `internal/syncer`. It dials `\\.\pipe\syncer`, then checks that the process serving the pipe belongs to the current Windows user. One JSON-RPC 2.0 message per line.
- **Starting the helper:** when nothing serves the pipe and Syncer 0.11 or newer is installed, Seaglass starts `Syncer.exe --api`. The helper exits a minute after the last connection. Older Syncer versions would open their window instead, so they are only reported as needing an update.
  - The version is the `DisplayVersion` of `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\ApolloFSyncer`.
- **Registering games:** after a scan that changed the library, and before each launch, Seaglass sends its installed games (`registerGames`: title, install folder, Steam app, GOG id). This way Syncer can match saves to games with unusual install folders, such as external copies. After a scan it only does this when Syncer is already running.

## Launching

| Hook | Step | What happens |
|---|---|---|
| Before launch | **Sync saves** (setting *Sync saves before playing*) | `gameStatus` for the game. If Syncer doesn't know it, the step is done. If a save has two versions, you're asked: *Open Syncer* (cancels the launch), *Play anyway* or *Cancel*. For a newer save waiting on another PC, the choices are *Wait for it* (up to 2.5 min), *Play anyway* or *Cancel*. Then `syncNow` on the synced folders (1 min). If it isn't finished, the step shows a warning and the game starts anyway. |
| After exit | **Back up saves** (setting *Back up saves after playing*) | `backupNow` with `wait` (2.5 min). A backup Syncer is already running counts as done. |

Both steps can be skipped from the launch screen. Neither step blocks a game from starting unless you choose *Cancel* or *Open Syncer*.

## Who's playing (accounts)

With *Accounts* on in Syncer, each person keeps their own saves of the games they split. Seaglass follows the account playing on this PC:

- **Following:** a launch plays as the account Syncer has on this PC, without asking: the saves step reads it from Syncer (`accounts`) before syncing, so playtime and achievements go to that person. Whoever switched last, in Seaglass or in Syncer, keeps playing.
- **Asking (optional):** for a PC several people take turns on, setting *Ask who's playing* (off by default, kept per PC) has a launch with two or more accounts first ask *Who's playing?*. The person playing now is the first choice. Picking someone else calls `switchAccount` before the saves are synced (a split game's folder id belongs to the account). If switching fails (a game runs, their saves haven't arrived), you choose *Play as <current>* or *Cancel*.
- **Switching any time:** the sidebar's *<name> is playing* (desktop), *Who's playing* in quick access (big picture), and Settings → Saves list the accounts.
- **Looking:** Seaglass asks Syncer (`accounts`) every two minutes while Syncer runs, without starting it, and after each switch. When Syncer isn't running or accounts are off, the last account this PC had stays in use (`%APPDATA%\Seaglass\profile-owner.txt`).

## Playtime, achievements and settings (the profile)

Seaglass keeps them per person in `%APPDATA%\Seaglass\Profile`, and asks Syncer to sync and back that folder up (`launcherData`; setting *Sync them with Syncer*, on by default). Syncer adds it once, labelled *Seaglass (playtime, achievements, settings)*, never splits it per account, and doesn't add it again after you stop syncing it.

- **Layout:** `Profile\<account>\<pc>.json`, or `Profile\shared\<pc>.json` while no account is in use (it moves to the first account this PC gets). `<pc>` is the PC's name and a hash of its machine id. Each PC writes only its own files, so syncing never makes conflict copies and PCs playing offline at the same time lose nothing.
- **Games** are keyed the same way on every PC: `steam:<app>`, `gog:<id>`, `epic:<app>`, else `title:<loose title>`.
- **Playtime** is the sum of the person's files; **last played** the newest. The library's `playtime` and `lastPlayed` are filled in from them (for the person playing).
- **Achievements:** what a session unlocked is recorded (the ones Seaglass announces after playing). A game's list shows what this PC's files and stores say plus what the person unlocked elsewhere; the cache keeps only this PC's part.
- **Settings:** the ones that go with a person (appearance, controller, while playing, saves, achievements, updates, which libraries show) are taken from whichever PC saved them last (setting *Same settings on every PC*). Game folders, store sign-ins, *start in big picture*, *open big picture when a controller connects*, *ask who's playing* and GOG Galaxy's library stay per PC.
- **First start:** this PC's playtime, last plays, achievements (as last read) and settings go into its first file.

## Interface

The game details (desktop) and the game page (big picture) show a saves line from `gameStatus`:

- *Synced · backed up 2 hours ago*
- *Backed up today · not synced*
- *2 versions of a save*
- *Newer save on TV-PC*
- *Not in Syncer*

When Syncer is missing or too old, the details offer *Get Syncer* or *Update Syncer* (the releases page). Otherwise they offer *Open Syncer*.
