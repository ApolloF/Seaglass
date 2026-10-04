# Privacy

Seaglass runs on your PC and has no server of its own. It has no telemetry, analytics, ads or crash reporting, and it doesn't send anything to the maintainer. It does contact the third-party services listed below, straight from your PC, to identify your games, fetch art and keep itself up to date.

## What stays on your PC

- `%APPDATA%\Seaglass`: your library (`library.json`), settings (`settings.json`), the log (`seaglass.log`, `crash.log`), the cached achievement data, and the profile Syncer can sync (`Profile\`).
- `%APPDATA%\Seaglass\secrets`: the SteamGridDB key, the Steam Web API key and the Epic and GOG sign-ins, encrypted with Windows DPAPI for your Windows account. They're never logged and never sent anywhere except to the service they belong to.
- `%LOCALAPPDATA%\Seaglass`: game art, achievement icons, the game database, downloaded updates and WebView2's browser data.
- Elsewhere on your PC, only when you turn the feature on: a value under `HKCU\...\CurrentVersion\Run` (*Start with Windows*) and Seaglass shortcuts in Steam's `shortcuts.vdf` (the Steam Input route for controllers).

To find your games, Seaglass reads, but doesn't change, the store launchers' own files and registry entries on your PC (Steam, Epic, GOG Galaxy, EA, Ubisoft, Battle.net, Xbox) and the game folders you point it at.

## What goes over the network

All requests use HTTPS and go only to allowlisted hosts. Each service sees your IP address and the request. Requests identify the app as `Seaglass` and carry no ID of their own: no install ID, account or machine name.

| Service | When | What it receives |
|---|---|---|
| GitHub (`api.github.com`, `github.com`, GitHub's download hosts) | On start and from *Settings → General*, unless you turn automatic updates off; when you install Syncer from *Settings → Saves* | A request for the latest Seaglass or Syncer release, then the download |
| GitHub (`raw.githubusercontent.com`) | About once a week | A request for the [Ludusavi](https://github.com/mtkennerly/ludusavi-manifest) game database, which Seaglass uses to identify games |
| Steam store and Steam CDNs (`store.steampowered.com`, `api.steampowered.com`, `*.steamstatic.com`) | For games in your library | Steam app IDs, and the titles of games it couldn't identify otherwise (as a search) |
| PCGamingWiki (`www.pcgamingwiki.com`) | For games in your library | Game titles and IDs |
| GOG (`api.gog.com`, `*.gog-statics.com`) | For GOG games | GOG product IDs |
| Epic (`*.epicgames.com`, `cdn2.unrealengine.com`) | For Epic games | Epic catalog IDs |
| SteamGridDB (`www.steamgriddb.com`) | Only if you enter a SteamGridDB key | Your key and the games you look up art for |
| Steam Web API (`api.steampowered.com`) | Only if you enter a Steam Web API key | Your key and your SteamID (read from Steam on this PC), to list owned games and achievements |
| Epic accounts (`*.epicgames.com`) | Only if you sign in to Epic | Your Epic sign-in token, to list owned games and achievements |
| GOG accounts (`auth.gog.com`, `gameplay.gog.com`) | Only if you sign in to GOG | Your GOG sign-in token, to read achievements |

Signing in to Epic or GOG happens on their own website in your browser; Seaglass only receives the code you paste back.

## Syncer

[Syncer](https://github.com/ApolloF/syncer) is a separate app. Seaglass talks to it through a local named pipe, never over the network. When Syncer is installed, Seaglass tells it which games you have, asks it to sync saves before and after you play, and, with profile sync on (*Settings → Saves*, on by default), hands it your profile folder (playtime, achievements, settings and this PC's name). Syncer then stores that in the cloud storage you connected to Syncer; see Syncer's own documentation.

## Diagnostics

*Settings → About → Copy diagnostics* and `Seaglass.exe --diagnostics` put a report on your clipboard or desktop, with your user folder replaced by `%USERPROFILE%`. Nothing is sent: *Report a problem* only opens GitHub's new-issue page in your browser, and you decide what to paste.

## WebView2

The interface runs in Microsoft Edge WebView2, a part of Windows. Seaglass's interface loads no remote code or content (it has a strict Content Security Policy), but WebView2 itself follows Microsoft's privacy statement and your Windows diagnostic-data settings.

## Deleting your data

Uninstall Seaglass from *Settings → Apps* and answer *Yes* when it asks whether to delete your library, settings, saved keys and art. Or delete `%APPDATA%\Seaglass` and `%LOCALAPPDATA%\Seaglass` yourself. To revoke access, remove the SteamGridDB or Steam key and sign out of Epic or GOG in *Settings*, or revoke the key on the service's website.

## Questions

Open an issue on [GitHub](https://github.com/ApolloF/Seaglass/issues). For security problems, see [SECURITY.md](SECURITY.md).
