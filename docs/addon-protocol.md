# Seaglass add-on protocol (version 1)

An add-on is a separate program that Seaglass starts and talks to over its standard input and output. It can:

- show badges and facts on a game;
- offer actions for a game;
- run a step before a game starts or after it exits.

Its interface is declarative, so it looks the same in desktop mode and in every big picture layout. Add-ons never run inside Seaglass.

## The manifest

An add-on is described by `addon.json` in its own folder under `%LOCALAPPDATA%\Seaglass\addons\<id>\`. An add-on's installer (or the add-on itself) can write it there. You can also pick an `addon.json` in *Settings → Add-ons → Add add-on…*, and Seaglass copies it there.

```json
{
  "id": "dlssupdater",
  "name": "DLSS Updater",
  "version": "1.4.0",
  "publisher": "ApolloF",
  "description": "Keeps DLSS up to date and installs OptiScaler.",
  "homepage": "https://github.com/ApolloF/dlssupdater",
  "exe": "C:\\Users\\you\\AppData\\Local\\Programs\\DLSS Updater\\DLSSUpdater.exe",
  "args": ["--addon"],
  "protocol": 1,
  "hooks": ["game.status", "game.actions", "game.beforeLaunch"],
  "permissions": ["modifyGameFiles", "network"]
}
```

| Field | Rules |
|---|---|
| `id` | 2–64 characters: lower-case letters, digits, `.` and `-`, starting with a letter or digit. Unique. |
| `name`, `publisher` | Shown to the user; up to 64 characters. |
| `version`, `description`, `homepage` | Optional. `homepage` must be `https://`. |
| `exe` | The program. An absolute path, or relative to the manifest's folder. Must be an `.exe`. |
| `args` | Arguments, passed as they are (no shell). At most 16. |
| `protocol` | `1`. |
| `hooks` | What Seaglass may ask for (see below). Only these are ever called. |
| `permissions` | What the add-on says it does, shown before you enable it: `modifyGameFiles` (changes files in game folders), `network` (downloads from the internet), `readSaves` (reads save files). |

## Trust

- New add-ons are **off**. Enabling one shows its name, publisher, permissions, whether its program is signed (Authenticode), and the program's SHA-256.
- Seaglass pins that SHA-256. When the program changes (an update), the add-on stays off until you approve it again.
- Add-ons run with your normal user rights. They are never elevated and never injected into games.
- Every call has a timeout. An add-on that crashes or hangs is stopped and restarted on the next call, at most 3 times in 10 minutes.
- An add-on's standard error goes to Seaglass's log (first 200 lines per run).

## Messages

[JSON-RPC 2.0](https://www.jsonrpc.org/specification), one JSON object per line (UTF-8, `\n`-terminated, at most 1 MB), over the add-on's stdin (from Seaglass) and stdout (to Seaglass). Nothing else may be written to stdout.

Seaglass starts the add-on when it's first needed and sends `initialize`. After 5 minutes without calls it sends the `shutdown` notification and ends the process 3 seconds later.

### From Seaglass

| Method | Params | Result | Timeout |
|---|---|---|---|
| `initialize` | `{protocol: 1, host: {name, version}}` | `{protocol: 1, name, version}` | 15 s |
| `game.status` | `{game}` | `{badges: [Badge], lines: [{label, value}]}` | 10 s |
| `game.actions` | `{game}` | `[{id, label, description?, confirm?}]` | 10 s |
| `game.runAction` | `{game, action}` | `{message?}` | 10 min |
| `game.beforeLaunch` | `{game}` | `{message?}` | 5 min |
| `game.afterExit` | `{game, seconds}` | `{message?}` | 2 min |
| `library.gameAdded` | `{game}` | notification (no answer), sent to add-ons that are running when a scan finds a game | – |
| `shutdown` | – | notification | – |

`game.runAction` needs the `game.actions` hook.

**Game:** `{id, title, dir, exe?, steamAppId?, source}`.
- `dir`: the install folder.
- `source`: `steam`, `epic`, `gog`, `installer`, `folder`, and so on.

**Badge:** `{text, tone?, tooltip?}`.
- `tone` is `info` (the default), `ok` or `warn`.
- Keep `text` short: *DLSS 310.2*, *OptiScaler 0.9*, *DLSS reverted by a patch*.

**Actions:**
- `confirm`, when set, is a question shown before the action runs (*Restore the game's original DLSS files?*).
- An action's `message` is shown when it finishes.

**beforeLaunch / afterExit:**
- These appear as steps in the launch sequence, named after the add-on. The user can skip them.
- An error doesn't stop the game from starting: the step shows it as a warning.

### From the add-on

| Notification | Params | Meaning |
|---|---|---|
| `progress` | `{text}` | A line of progress for the call that is running (shown under the step or action). |
| `log` | `{text}` | A line for Seaglass's log. |

Errors use JSON-RPC error objects; `message` is shown to the user, so write it for people.

## Example

```
→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocol":1,"host":{"name":"Seaglass","version":"0.6.0"}}}
← {"jsonrpc":"2.0","id":1,"result":{"protocol":1,"name":"DLSS Updater","version":"1.4.0"}}
→ {"jsonrpc":"2.0","id":2,"method":"game.beforeLaunch","params":{"game":{"id":12,"title":"Cyberpunk 2077","dir":"D:\\Games\\Cyberpunk 2077","source":"gog"}}}
← {"jsonrpc":"2.0","method":"progress","params":{"text":"A game patch put back DLSS 3.7; installing DLSS 310.2"}}
← {"jsonrpc":"2.0","id":2,"result":{"message":"DLSS 310.2 is back"}}
```
