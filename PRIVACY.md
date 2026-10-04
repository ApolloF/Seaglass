# Privacy

Last updated: 4 October 2026

Seaglass is a free Windows game launcher. It runs on your PC and has no server of its own. ApolloF receives no data from the app: no account, no telemetry, no analytics, no automatic crash reports.

The same text is published at https://apps.apollof.nl/seaglass/privacy/.

## Who is responsible

Seaglass is made and published by ApolloF, in the Netherlands. Contact: me@apollof.nl. Because the app never sends your data to ApolloF, ApolloF doesn't hold or process it; it stays on your PC and with the services listed below. If you email ApolloF, ApolloF is the controller of that email (see Your rights).

## What Seaglass reads on your PC

To find your games, Seaglass reads the libraries of the launchers installed on your PC (such as Steam, Epic, GOG Galaxy, EA, Ubisoft, Battle.net and Xbox), the folders you add, and achievement files games and launchers keep locally. It records playtime for games you start. All of this stays on your PC.

Seaglass also reads which Steam account is signed in on this PC, from Steam's local files, and the Windows machine ID (only as a hash, to name this PC's files in the profile described under With Syncer).

## Accounts you can connect

Connecting a store account is optional and only adds games you own but haven't installed, or achievements.

- **Steam:** you paste your own Steam Web API key. Seaglass sends it, with the Steam ID of the account signed in to Steam on this PC, to Valve's Steam Web API to list your owned games and achievements.
- **Epic Games:** you sign in on epicgames.com yourself and paste the one-time code; Seaglass never sees your password. Seaglass keeps the resulting sign-in token, your Epic account ID and display name, and uses them to read your Epic library and your achievements.
- **GOG:** owned games come from GOG Galaxy's local database. Signing in to GOG (on gog.com, then pasting the address or one-time code) is only for achievements; Seaglass keeps the sign-in token and your GOG user ID.
- **SteamGridDB:** you can paste your own SteamGridDB API key for extra cover art.

Keys and sign-in tokens are stored in `%APPDATA%\Seaglass\secrets`, encrypted with Windows' data protection so only your Windows account on this PC can read them. They are sent only to the service they belong to, never to ApolloF. Clearing a key or choosing *Sign out* in Seaglass deletes it from your PC and removes the games it added from your library. The sign-in and key pages (Steam, Epic, GOG) open in your default browser, which talks to those sites itself.

## Network requests

- **Game details and art:** for your installed games (and, if you show them, owned games you haven't installed), Seaglass looks up the game by name or store ID at the Steam store and Steam Web API, Epic's catalog and store pages, GOG, PCGamingWiki and (with your key) SteamGridDB, and loads images from those services' servers and content delivery networks. The Epic catalog is read with Epic's public launcher app token, without an account. Images are cached on your PC.
- **Achievements:** for games with achievements, Seaglass fetches names, icons and global unlock rates from Steam, Epic and GOG. With your Steam key, Epic sign-in or GOG sign-in it also reads your own unlocks, sending the key or token with your Steam ID, Epic account ID or GOG user ID.
- **Game recognition:** it downloads the [Ludusavi manifest](https://github.com/mtkennerly/ludusavi-manifest) from GitHub (raw.githubusercontent.com) when it is missing or more than a week old.
- **Updates:** with automatic updates on (the default), it checks GitHub for a new Seaglass release at start and downloads it from GitHub. With them off it only checks when you click *Check* in Settings. Installing or updating Syncer from Seaglass checks and downloads from Syncer's GitHub releases.
- These services see your IP address, as with any web request, and what is being looked up: game names, store IDs and, for the accounts above, your keys, tokens and IDs. Nothing else about you is sent.

All requests use HTTPS and go only to allowlisted hosts. They identify the app as `Seaglass` and carry no install ID, account or machine name of their own.

| Service | Hosts | When |
|---|---|---|
| GitHub | `api.github.com`, `github.com`, GitHub's download hosts | Update checks and downloads; installing Syncer |
| GitHub | `raw.githubusercontent.com` | The Ludusavi game database, about once a week |
| Steam store | `store.steampowered.com`, `*.steamstatic.com`, `steamcdn-a.akamaihd.net` | Details and art for games in your library |
| Steam Web API | `api.steampowered.com` | Achievement data; with your key, owned games and your unlocks |
| PCGamingWiki | `www.pcgamingwiki.com`, `images.pcgamingwiki.com` | Details for games in your library |
| GOG | `api.gog.com`, `*.gog-statics.com`, `auth.gog.com`, `gameplay.gog.com` | GOG games' details and art; with sign-in, achievements |
| Epic | `*.epicgames.com`, `cdn2.unrealengine.com` | Epic games' details and art; with sign-in, library and achievements |
| SteamGridDB | `*.steamgriddb.com` | Only with your key |

## With Syncer

If you use [Syncer](https://apps.apollof.nl/syncer/), Seaglass talks to it on your PC only. It tells Syncer the title, install folder and Steam or GOG ID of your installed games so Syncer can find their saves. Seaglass keeps your playtime, achievements and settings in `%APPDATA%\Seaglass\Profile`, in files named after this PC's name. With profile sync turned on (the default, effective only when Syncer is installed), Syncer copies that folder between your own PCs and includes it in its backups. Keys and sign-in tokens are not part of it. See the [Syncer privacy policy](https://apps.apollof.nl/syncer/privacy/).

## What stays on your PC

Settings, your library, the profile, the log and crash logs stay in `%APPDATA%\Seaglass`. The log records events such as the titles of games started, play times and errors. Cached art, achievement data, the game database, update downloads and the data of the WebView2 component that draws Seaglass's window stay in `%LOCALAPPDATA%\Seaglass`. WebView2 is a Microsoft component; its own data handling follows Microsoft's and Windows' privacy settings, not Seaglass. Seaglass's interface loads no remote code (it has a strict Content Security Policy).

Outside those folders Seaglass writes only when you turn the feature on: a value under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` for *Start with Windows*, and Seaglass shortcuts in Steam's `shortcuts.vdf` for the Steam Input route for controllers. It reads, but doesn't change, the store launchers' own files and registry entries.

*Copy diagnostics* (or `Seaglass.exe --diagnostics`, which writes a file to your desktop) creates a report only when you ask for it. It holds Seaglass, Windows and WebView2 versions, your settings (including game folders), library counts, the last crash and the end of the log. It holds no keys or tokens, and your user folder is shortened to `%USERPROFILE%`. It goes only where you paste or send it.

## Third parties Seaglass contacts

- **Valve** (Steam store, Steam Web API and Steam's image servers, which run through content delivery networks): [Valve Privacy Policy](https://store.steampowered.com/privacy_agreement/)
- **Epic Games** (catalog, store pages, library, achievements, image servers): [Epic Games Privacy Policy](https://www.epicgames.com/site/privacypolicy)
- **GOG** (game details, sign-in, achievements, image servers): privacy policy linked at https://www.gog.com/
- **PCGamingWiki**: privacy policy linked at https://www.pcgamingwiki.com/
- **SteamGridDB** (only with your key): [SteamGridDB privacy policy](https://www.steamgriddb.com/privacy)
- **GitHub** (Seaglass and Syncer updates, game manifest): [GitHub Privacy Statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement)

## No selling, no sharing

Your data is never sold, rented, shared with advertisers or data brokers, or used for advertising. There are no ads, analytics or trackers in Seaglass.

## How long data is kept

Settings, your library, the profile and your keys and sign-in tokens stay on your PC until you delete them. The log rotates at 1 MiB and keeps one earlier file. `crash.log` is emptied at each start, and the previous run's crash output is kept in `crash-previous.log`. Cached art no game uses any more is removed; cached game and achievement data is refreshed periodically. ApolloF keeps nothing, because nothing is received.

## Deleting your data

- Sign out of Epic and GOG, and clear the Steam and SteamGridDB keys, in Seaglass. To also revoke access on the store side, use your account settings there.
- Uninstall Seaglass from *Settings → Apps* in Windows and answer *Yes* when it asks whether to delete your library, settings, saved keys and art. Or delete `%APPDATA%\Seaglass` and `%LOCALAPPDATA%\Seaglass` yourself.

## Your rights

Under the GDPR you can ask for access to, correction or deletion of personal data about you, restrict or object to its use, and get a copy of it. Since Seaglass sends nothing to ApolloF, the only personal data ApolloF may hold is an email you send; it is used only to answer you and deleted when it is no longer needed. Write to me@apollof.nl. You can also complain to the Dutch data protection authority, the [Autoriteit Persoonsgegevens](https://autoriteitpersoonsgegevens.nl/), or the authority where you live.

## Children

Seaglass isn't directed at children under 16 and doesn't knowingly collect data from anyone. The stores' own age rules apply to the accounts you connect.

## Changes

If this policy changes, the new version is published on https://apps.apollof.nl/seaglass/privacy/ with a new date, and in this file.

## Contact

Questions about privacy: me@apollof.nl. Bugs and feature requests: https://github.com/ApolloF/Seaglass/issues
