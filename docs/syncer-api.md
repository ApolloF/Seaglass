# Syncer integration

Seaglass talks to [Syncer](https://github.com/ApolloF/syncer) through Syncer's launcher API. The API itself is specified in Syncer's repository: [docs/api.md](https://github.com/ApolloF/syncer/blob/main/docs/api.md). This page covers how Seaglass uses it.

## Connection

- **Client:** `internal/syncer`. It dials `\\.\pipe\syncer`, then checks that the process serving the pipe belongs to the current Windows user. One JSON-RPC 2.0 message per line.
- **Starting the helper:** when nothing serves the pipe and Syncer 0.11 or newer is installed, Seaglass starts `Syncer.exe --api`. The helper exits a minute after the last connection. Older Syncer versions would open their window instead, so they are only reported as needing an update.
  - The version is the `DisplayVersion` of `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\ApolloFSyncer`.
- **Registering games:** after a scan that changed the library, and before each launch, Seaglass sends its installed games (`registerGames`: title, install folder, Steam app, GOG id). This way Syncer can match saves to repacks and unofficial copies. After a scan it only does this when Syncer is already running.

## Launching

| Hook | Step | What happens |
|---|---|---|
| Before launch | **Sync saves** (setting *Sync saves before playing*) | `gameStatus` for the game. If Syncer doesn't know it, the step is done. If a save has two versions, you're asked: *Open Syncer* (cancels the launch), *Play anyway* or *Cancel*. For a newer save waiting on another PC, the choices are *Wait for it* (up to 2.5 min), *Play anyway* or *Cancel*. Then `syncNow` on the synced folders (1 min). If it isn't finished, the step shows a warning and the game starts anyway. |
| After exit | **Back up saves** (setting *Back up saves after playing*) | `backupNow` with `wait` (2.5 min). A backup Syncer is already running counts as done. |

Both steps can be skipped from the launch screen. Neither step blocks a game from starting unless you choose *Cancel* or *Open Syncer*.

## Interface

The game details (desktop) and the game page (big picture) show a saves line from `gameStatus`:

- *Synced · backed up 2 hours ago*
- *Backed up today · not synced*
- *2 versions of a save*
- *Newer save on TV-PC*
- *Not in Syncer*

When Syncer is missing or too old, the details offer *Get Syncer* or *Update Syncer* (the releases page). Otherwise they offer *Open Syncer*.
