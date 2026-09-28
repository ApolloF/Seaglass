# Achievements

Seaglass shows each game's achievements: icons, locked or unlocked, when, progress and how rare they are. Official installs are read from the store's own files or servers; unofficial copies from the unlock files their Steam emulator writes. The code is `internal/achievements` (reading and merging), `internal/owned` (store servers) and `internal/app/achievements.go` (the service, the cache and the note after playing).

## How it works

A game's achievements are two things put together:

- the **schema**: every achievement's ID, name, description, icons and whether it's hidden;
- the **unlocks**: which are unlocked, when, and progress.

Unlock IDs match schema IDs ignoring case (CODEX writes them in a different case than Steam). Unlocks the schema doesn't know are listed by their ID; so is every unlock when there's no schema, with a hint on how to get names.

**Rarity** (share of all players who have an achievement) comes from Steam's `GetGlobalAchievementPercentagesForApp` (no key needed) for anything keyed by Steam's names: Steam games and Steam emulators. The app is `SteamAppID`, else `MetaAppID`. Epic and GOG send their own rarity.

**When it's read.** When a game's details or big picture sheet open, and 3 s after a play session ends (external sessions too). Nothing is polled. A result is reused while the files it came from are unchanged (their size and time, the language, which accounts are signed in; see `achStamp`); a result that needed a store's servers is also reused for up to 6 hours. While a game runs nothing is asked online (`Launch.Active`), and after Steam answers "429 too many requests" the next Steam call waits 1.5 s.

**After playing.** What the game had unlocked is kept when its session starts; once it has exited, the achievements are read again and the new ones are sent as `achievements:session`, which shows "N achievements unlocked" with their icons.

**Icons** are downloaded (from allowlisted hosts) or copied from the game folder, checked, re-encoded and stored content-addressed in `%LOCALAPPDATA%\Seaglass\achievements\icons`, and served at `/ach/<sha>.png`: the release CSP only allows images from Seaglass itself. They live apart from game art, whose pruning doesn't know them.

**Cache.** `%LOCALAPPDATA%\Seaglass\achievements\games\<gameID>.json` (last result per game) and `…\schema\<source>-<id>-<lang>.json` (schemas from the stores for 30 days, rarity for 7). With `--dev-data` they live next to that library instead (`achievements-cache`), so a test harness never mixes its game ids with the real library's. Turning achievements off or on, or showing hidden ones, clears the game results. Results carry `achievements.Version`: raising it makes every game read again after an update that changes what the same files mean.

**Language.** Steam's interface language (`HKCU\Software\Valve\Steam\Language`), English when a name has no translation. Epic gets the matching locale.

**Settings.** *Show achievements* (on) and *Show hidden achievements* (off: a hidden achievement you haven't unlocked shows as "Hidden achievement" until you do; the lists can reveal them one screen at a time).

## Which source

| Game | Schema | Unlocks |
|---|---|---|
| Unofficial copy, or found in a folder or installer (not GOG) | `steam_settings\achievements.json` next to the emulator, else Steam's cached schema, else Steam's Web API with your key | the emulator's unlock file (below) |
| Steam | Steam's cached schema, else the Web API (key) | Steam's cached stats, else `GetPlayerAchievements` (key, public game details) |
| Epic | Epic's store (anonymous) | Epic, when signed in to Epic in Settings |
| GOG, or a DRM-free GOG copy | GOG's servers, when signed in to GOG in Settings | GOG Galaxy's database, else GOG's servers |
| EA, Ubisoft, Battle.net, Xbox | not read yet | |

A game with nothing to look up (no emulator and no unlock file, no Steam app, Epic or GOG id) shows no achievements card rather than an empty one.

## Steam's local files

`<Steam>\appcache\stats\UserGameStatsSchema_<appid>.bin` (binary KeyValues): `<appid>\stats\<statid>` with `type` 4 or 5 holds achievements, one per `bits\<n>` with `name`, `bit` and `display\{name,desc}\<language>`, `display\hidden`, `display\icon` and `display\icon_gray` (file names under `cdn.akamai.steamstatic.com/steamcommunity/public/images/apps/<appid>/`).

`UserGameStats_<account>_<appid>.bin`: `cache\<statid>\data` is a bit field (bit *n* set = unlocked), the time in `AchievementTimes\<n>` (under the stat, or under `cache`). Accounts come from `gamekit/steam.Accounts`, the one in use first.

These files use binary KeyValues types 0x00–0x08 only as far as is known (Steam Achievement Manager reads them with the same set), so gamekit needed no change. Checked against built files only: no Steam install was at hand. The live check is `WL_REAL_ACH=1`.

## Emulator unlock files

`{roaming}` is `%APPDATA%`, `{local}` `%LOCALAPPDATA%`, `{public}` `%PUBLIC%`, `{programdata}` `%ProgramData%`, `{documents}` the user's Documents; `{emu}` is the folder the emulator sits in (`Game.EmuDir`, relative to the game folder), `{appid}` the Steam app. When several places have a file (an old crack's leftovers), the newest names the source and anything unlocked in any of them counts. Every file is capped at 4 MB and treated as untrusted; each format has a fuzz test.

| Group | Where | Format |
|---|---|---|
| Goldberg | `{roaming}\Goldberg SteamEmu Saves\{appid}\achievements.json`; `local_save.txt` next to the emulator names a folder beside it (`SAVE` when empty) | JSON `{"ID": {"earned", "earned_time", "progress", "max_progress"}}` |
| gbe_fork | `{roaming}\GSE Saves\{appid}\achievements.json`; `[user::saves] local_save_path` in `steam_settings\configs.user.ini` | as Goldberg |
| EMPRESS | `{roaming}\EMPRESS\remote\{appid}\achievements.json`, `{public}\Documents\EMPRESS\{appid}\remote\{appid}\achievements.json` | as Goldberg |
| CODEX | `{public}\Documents\Steam\CODEX\{appid}\achievements.ini`, `{roaming}\Steam\CODEX\{appid}\achievements.ini` | INI `[ID]` `Achieved`, `UnlockTime`, `CurProgress`, `MaxProgress` (progress at its maximum counts as unlocked); `[SteamAchievements]` lists the unlocked IDs in unlock order (`00000=ID`, …, `Count`), and a listed ID without a section counts as unlocked |
| RUNE | `{public}\Documents\Steam\RUNE\{appid}\achievements.ini` | as CODEX |
| OnlineFix | `{public}\Documents\OnlineFix\{appid}\Stats\Achievements.ini`, `…\{appid}\Achievements.ini` | INI `achieved=true`, `timestamp` |
| CreamAPI | `{roaming}\CreamAPI\{appid}\stats\CreamAPI.Achievements.cfg` | INI `achieved`, `unlocktime` (seven digits: ×1000) |
| Reloaded, dodi | `{programdata}\RLD!\{appid}\achievements.ini`, `{programdata}\Steam\{Player,RLD!,dodi}\{appid}\stats\achievements.ini` | INI, `State`, `Time`, `CurProgress`, `MaxProgress` as little-endian hex; `[Steam]` is skipped |
| SKIDROW | `{documents}\SKIDROW\{appid}\SteamEmu\UserStats\achiev.ini`, `{local}\SKIDROW\…` | `ID=1@…@time` |
| SmartSteamEmu | `{roaming}\SmartSteamEmu\{appid}\stats.bin`, `{emu}\SmartSteamEmu\{appid}\stats.bin` | little-endian count, then 24-byte records: CRC32 of the ID (byte-reversed), unlock time at 8, state in the last 4. Matched by hashing the schema's IDs, so it needs a schema |
| TENOKE | `{emu}\SteamData\user_stats.ini` | `[ACHIEVEMENTS]` lines `ID = {unlocked = true, time = …}` |
| 3DM | `{emu}\3DMGAME\*\stats\achievements.ini` | as CODEX |
| ALI213 | `{emu}\Profile\*\Stats\Achievements.Bin` | INI `HaveAchieved`, `HaveAchievedTime` |
| Razor1911 | `{roaming}\.1911\{appid}\achievement` | lines `ID achieved time` |
| Uplay emulator, VOICES38 (Ubisoft games) | `{roaming}\Goldberg UplayEmu Saves\<ubisoft id>\achievements.json` (or `R1 UplayEmu Saves`, `SaveType`/`SavePath`, `AchSaveType=1` + `AchSavePath` from `upc_r2.ini` / `uplay_r2.ini` / `uplay_r1.ini` next to the loader) | as Goldberg, keys `<AchKeyPrefix><number>`. The folder is the game's when the ini's `GameId` names it, its keys start with `AchKeyPrefix` or match the schema, or it's the only one there and only one game in the library runs on a Uplay emulator; otherwise nothing is guessed. Names from `achievements_schema.json` next to the loader (`{key: {displayName, description}}`), else the Steam version's schema, matched by the number the keys end in |
| Nemirtinga's Epic emulator | `{roaming}\NemirtingasEpicEmu\*\<id>\achievements.json`; the id from `nemirtingasepicemu.json` (`AppId`, `ProductId`, `SandboxId` or `Namespace`) | a list of `{AchievementId, Progress / Unlocked}`. Its times aren't wall-clock times: shown as "unlocked, time unknown". Names come from Epic when the id is the game's sandbox |

The schema next to Goldberg-style emulators is `steam_settings\achievements.json`: a list of `{name, displayName, description, hidden, icon, icon_gray | icongray}`; `displayName` and `description` are strings or objects by language. Icons are files relative to `steam_settings\` or `steam_settings\achievement_images\`, and only files inside the game folder are used.

VOICES38's own loader seems to write no achievements at all (the open Goldberg R2 loader it's based on doesn't); only builds with `Achievements = 1` do. When a Uplay game's ini doesn't have it, the card offers **Turn on**: it sets `Achievements = 1` in `[Settings]` of the ini (or makes one named after the loader DLL), keeping the old ini as `<ini>.seaglass.bak`. Once it's on, the card waits for the game to be played; if a play after the ini changed still saved nothing, the loader doesn't save achievements and the card goes away (it comes back when a file shows up). LumaPlay (registry, `HKCU\SOFTWARE\LumaPlay\<user>\<id>\Achievements`) isn't read yet.

**Not read yet** (no public description of their files; samples welcome): CPY, PLAZA, FLT, Steamworks Fix. Hoodlum and DARKSiDERS use the CODEX INI format, but where they keep it isn't known. Their games say "Seaglass can't read X's achievement files yet". The TENOKE, SmartSteamEmu, ALI213, Razor1911 and Nemirtinga formats are written from descriptions and tested against built files only: real samples would confirm them.

## Store servers

All through `internal/owned`: allowlisted hosts, HTTPS, response size limits, and errors that never carry the key.

- **Steam**: `ISteamUserStats/GetSchemaForGame/v2` (key, `l=` language) and `GetPlayerAchievements/v1` (key, the account in use) with the Steam Web API key from Settings → Accounts; `GetGlobalAchievementPercentagesForApp/v2` without one. The key also gives unofficial copies names and icons.
- **Epic**: GraphQL at `launcher.store.epicgames.com/graphql`. `productAchievementsRecordBySandbox(sandboxId, locale)` needs no sign-in (the sandbox is the namespace in `Game.EpicApp`); `playerAchievementGameRecordsBySandbox(epicAccountId, sandboxId)` uses the Epic sign-in. One access token is shared with the owned-games sync, refreshed one at a time, the newest refresh token kept.
- **GOG**: unlocks from GOG Galaxy's database (`UserAchievements`: `gameReleaseKey = gog_<id>`, `apikey`, `unlockTime`, `isUnlocked`; read-only, WAL included). Names, icons and rarity from `gameplay.gog.com/clients/<clientId>/users/<userId>/achievements` with a GOG sign-in (Settings → Accounts, paste-the-address like Epic's; the `gog-account` secret keeps the refresh token). The client id comes from `goggame-<id>.info`; the product id is tried when that finds nothing. Galaxy's own sign-in is never used: refreshing its token could sign Galaxy out.

## Testing

- `go test ./internal/achievements ./internal/owned ./internal/app`: every emulator format (fixtures made in a temporary folder), Steam's bins (made with `vdf.MarshalBinary`), the Goldberg schema, merging, the resolver per store with a fake `Net`, the Steam, Epic and GOG clients against `httptest` servers, GOG's `UserAchievements` in the test Galaxy database, and the session baseline.
- `go test -fuzz` on each parser (`FuzzGoldbergJSON`, `FuzzINIUnlocks`, `FuzzRLD`, `FuzzSkidrow`, `FuzzTenoke`, `FuzzRazor`, `FuzzSSE`, `FuzzNemirtinga`, `FuzzGoldbergSchema`, `FuzzSteamSchema`, `FuzzSteamStats`).
- `WL_REAL_ACH=1 go test ./internal/achievements -run Real -v` reads this PC's real Steam, Epic emulator, GOG Galaxy and emulator files.
- `tools/harness/achievements.mjs`: a fake Goldberg copy of Portal in the real app: the card, the served icon, hidden masking, Steam's rarity, the note after playing (with unlocks written while the game runs), and a layout check of the full list and big picture's screen at every tour size.
- `npm run dev:mock` has one game per state: Ember Crown (full schema, a hidden one, progress), Iron Veil (IDs only), Starfall Protocol (Epic, not signed in), Hollow Tide (all unlocked), Frostline (a store Seaglass can't read). Playing a game in the mock unlocks two.
