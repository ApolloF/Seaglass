"""Writes match_audit.tsv for TestMatchAudit: real games named the ways
stores, installers, repacks and folders name them, each with the title it
should be found as, plus folders that aren't games (want "-").

    python gen_match_audit.py

Columns: kind (store:<store>, folder, installer), what the scanner sees
(installer: name|folder), and the accepted titles ("|" between several).
The variants are picked deterministically, so the file only changes when
this script does.
"""
import hashlib
import io
import re

# Steam's own names (or the store's, for games Steam doesn't sell),
# with the store a copy most often comes from.
GAMES = """
Grand Theft Auto V|epic
Red Dead Redemption 2|epic
Cyberpunk 2077|gog
The Witcher 3: Wild Hunt|gog
The Witcher 2: Assassins of Kings Enhanced Edition|gog
The Witcher: Enhanced Edition|gog
Elden Ring|steam
Sekiro: Shadows Die Twice|steam
Dark Souls III|steam
Dark Souls: Remastered|steam
Dark Souls II: Scholar of the First Sin|steam
Armored Core VI: Fires of Rubicon|steam
Baldur's Gate 3|gog
Baldur's Gate: Enhanced Edition|gog
Divinity: Original Sin 2 - Definitive Edition|gog
Hogwarts Legacy|epic
Alan Wake 2|epic
Alan Wake Remastered|epic
Control|epic
Death Stranding Director's Cut|epic
Marvel's Spider-Man Remastered|steam
Marvel's Spider-Man: Miles Morales|steam
Marvel's Spider-Man 2|steam
Marvel's Guardians of the Galaxy|epic
Horizon Zero Dawn|epic
Horizon Forbidden West Complete Edition|steam
God of War=God of War (2018)|steam
God of War Ragnarök|steam
Ghost of Tsushima DIRECTOR'S CUT|steam
The Last of Us Part I|steam
Uncharted: Legacy of Thieves Collection|steam
Days Gone|steam
Returnal|steam
Ratchet & Clank: Rift Apart|steam
Stellar Blade|steam
Helldivers 2|steam
Borderlands 3|epic
Borderlands 2|steam
Tiny Tina's Wonderlands|epic
Rocket League|epic
Fall Guys|epic
Fortnite|epic
Hades|epic
Hades II|steam
Celeste|epic
Hollow Knight|gog
Stardew Valley|gog
Terraria|gog
Minecraft Dungeons|xbox
Dead Cells|gog
Cuphead|gog
Ori and the Will of the Wisps|xbox
Ori and the Blind Forest: Definitive Edition|steam
Undertale|steam
Disco Elysium - The Final Cut=Disco Elysium|gog
Outer Wilds|epic
Subnautica|epic
Subnautica: Below Zero|epic
No Man's Sky|gog
Valheim|steam
Palworld|steam
Lethal Company|steam
Phasmophobia|steam
Among Us|epic
Satisfactory|epic
Factorio|gog
RimWorld|gog
Frostpunk|gog
Frostpunk 2|gog
Cities: Skylines|epic
Cities: Skylines II|xbox
Planet Coaster|steam
Planet Zoo|steam
Two Point Hospital|steam
Two Point Campus|steam
Stellaris|steam
Crusader Kings III|xbox
Europa Universalis IV|steam
Hearts of Iron IV|steam
Victoria 3|steam
Total War: WARHAMMER III|epic
Total War: THREE KINGDOMS|steam
Total War: ROME REMASTERED|steam
Sid Meier's Civilization VI|epic
Sid Meier's Civilization V|steam
Sid Meier's Civilization VII|steam
XCOM 2|epic
Age of Empires IV|xbox
Age of Empires II: Definitive Edition|xbox
Age of Mythology: Retold|xbox
Company of Heroes 3|steam
Anno 1800|ubisoft
Anno 117: Pax Romana|ubisoft
Assassin's Creed Valhalla|ubisoft
Assassin's Creed Odyssey|ubisoft
Assassin's Creed Origins|ubisoft
Assassin's Creed Mirage|ubisoft
Assassin's Creed Shadows|ubisoft
Assassin's Creed IV Black Flag|ubisoft
Assassin's Creed Unity|ubisoft
Assassin's Creed Syndicate|ubisoft
Assassin's Creed II|ubisoft
Far Cry 6|ubisoft
Far Cry 5|ubisoft
Far Cry 4|ubisoft
Far Cry 3|ubisoft
Far Cry Primal|ubisoft
Far Cry New Dawn|ubisoft
Watch Dogs: Legion|ubisoft
Watch_Dogs 2|ubisoft
Tom Clancy's Rainbow Six Siege|ubisoft
Tom Clancy's The Division 2|ubisoft
Tom Clancy's Ghost Recon Breakpoint|ubisoft
Tom Clancy's Ghost Recon Wildlands|ubisoft
Tom Clancy's Splinter Cell Blacklist|ubisoft
Prince of Persia: The Lost Crown|ubisoft
Immortals Fenyx Rising|ubisoft
Star Wars Outlaws|ubisoft
Avatar: Frontiers of Pandora|ubisoft
Rayman Legends|ubisoft
For Honor|ubisoft
The Crew Motorfest|ubisoft
Trackmania=Trackmania (2020)|ubisoft
Forza Horizon 5|xbox
Forza Horizon 4|xbox
Forza Motorsport|xbox
Microsoft Flight Simulator (2020) 40th Anniversary Edition=Microsoft Flight Simulator|xbox
Microsoft Flight Simulator 2024|xbox
Halo: The Master Chief Collection|xbox
Halo Infinite|xbox
Gears 5|xbox
Gears Tactics|xbox
Starfield|xbox
Sea of Thieves|xbox
Grounded|xbox
Hi-Fi RUSH|xbox
Indiana Jones and the Great Circle|xbox
Avowed|xbox
South of Midnight|xbox
Pentiment|xbox
The Outer Worlds|epic
Fallout 4|gog
Fallout 76|xbox
Fallout: New Vegas|gog
Fallout 3: Game of the Year Edition|gog
The Elder Scrolls V: Skyrim Special Edition|gog
The Elder Scrolls IV: Oblivion Remastered|xbox
The Elder Scrolls Online|steam
DOOM Eternal|xbox
DOOM (2016)=DOOM|steam
DOOM: The Dark Ages|xbox
Wolfenstein II: The New Colossus|steam
Wolfenstein: The New Order|gog
Prey=Prey (2017)|epic
Dishonored 2|steam
Dishonored: Death of the Outsider|steam
Deathloop|steam
Ghostwire: Tokyo|epic
The Evil Within 2|steam
Quake|steam
EA SPORTS FC 24|ea
EA SPORTS FC 25|ea
Battlefield 2042|ea
Battlefield V|ea
Battlefield 1|ea
Battlefield 4|ea
The Sims 4|ea
Apex Legends|ea
Mass Effect Legendary Edition|ea
Dragon Age: The Veilguard|ea
Dragon Age: Inquisition|ea
Dead Space=Dead Space (2023)|ea
It Takes Two|ea
Split Fiction|ea
Star Wars Jedi: Fallen Order|ea
Star Wars Jedi: Survivor|ea
STAR WARS Battlefront II|ea
Need for Speed Unbound|ea
Need for Speed Heat|ea
Need for Speed Hot Pursuit Remastered|ea
Need for Speed Most Wanted|ea
F1 24|ea
Titanfall 2|ea
Crysis Remastered|epic
Diablo IV|battlenet
Diablo II: Resurrected|battlenet
Diablo III|battlenet
Overwatch 2|battlenet
Call of Duty|battlenet
Call of Duty: Black Ops 6|battlenet
Call of Duty: Modern Warfare III|battlenet
Call of Duty: Black Ops III|steam
Call of Duty: WWII|steam
StarCraft II|battlenet
World of Warcraft|battlenet
Hearthstone|battlenet
Resident Evil 4=Resident Evil 4 (2023)|steam
Resident Evil Village|steam
Resident Evil 2=Resident Evil 2 (2019)|steam
Resident Evil 3=Resident Evil 3 (2020)|steam
Resident Evil 7 Biohazard|steam
Monster Hunter: World|steam
Monster Hunter Rise|steam
Monster Hunter Wilds|steam
Street Fighter 6|steam
Devil May Cry 5|steam
Dragon's Dogma 2|steam
Mega Man 11|steam
TEKKEN 8|steam
TEKKEN 7|steam
Mortal Kombat 11|steam
Mortal Kombat 1|steam
Injustice 2|steam
Hogwarts Legacy|steam
Batman: Arkham Knight|epic
Batman: Arkham City - Game of the Year Edition|steam
Batman: Arkham Asylum Game of the Year Edition|steam
Middle-earth: Shadow of War|steam
Mad Max|steam
Dying Light 2 Stay Human|epic
Dying Light|epic
Dead Island 2|epic
Metro Exodus|epic
Metro 2033 Redux|epic
Metro: Last Light Redux|epic
S.T.A.L.K.E.R. 2: Heart of Chornobyl|gog
S.T.A.L.K.E.R.: Shadow of Chernobyl|gog
Kingdom Come: Deliverance|epic
Kingdom Come: Deliverance II|steam
Mafia: Definitive Edition|epic
Mafia II: Definitive Edition|epic
Mafia III: Definitive Edition|epic
BioShock Infinite|epic
BioShock Remastered|epic
Sleeping Dogs: Definitive Edition|gog
Just Cause 3|epic
Just Cause 4 Reloaded=Just Cause 4|epic
Tomb Raider=Tomb Raider (2013)|epic
Rise of the Tomb Raider|epic
Shadow of the Tomb Raider|epic
Deus Ex: Mankind Divided|steam
Life is Strange|epic
Life is Strange: True Colors|epic
Final Fantasy VII Remake Intergrade|epic
FINAL FANTASY XVI|steam
FINAL FANTASY XV WINDOWS EDITION=Final Fantasy XV|steam
FINAL FANTASY X/X-2 HD Remaster|steam
FINAL FANTASY VII REBIRTH|steam
NieR:Automata|steam
NieR Replicant ver.1.22474487139...=NieR Replicant|steam
Persona 5 Royal|xbox
Persona 4 Golden|steam
Persona 3 Reload|xbox
Yakuza: Like a Dragon|xbox
Like a Dragon: Infinite Wealth|steam
Yakuza 0|steam
Like a Dragon Gaiden: The Man Who Erased His Name|steam
Judgment|steam
Octopath Traveler II|steam
DRAGON QUEST XI S: Echoes of an Elusive Age - Definitive Edition=Dragon Quest XI S - Definitive Edition|steam
Tales of Arise|steam
Ni no Kuni II: Revenant Kingdom|steam
Sonic Frontiers|steam
Sonic Generations|steam
Sonic Mania|epic
Team Sonic Racing|steam
Hitman 3=HITMAN World of Assassination|epic
HITMAN World of Assassination|steam
Hitman: Absolution|steam
Sniper Elite 5|steam
Sniper Elite 4|steam
Zombie Army 4: Dead War|epic
Doom 3|steam
Half-Life 2|steam
Half-Life: Alyx|steam
Portal 2|steam
Portal|steam
Left 4 Dead 2|steam
Counter-Strike 2|steam
Dota 2|steam
Team Fortress 2|steam
Black Myth: Wukong|steam
Lies of P|steam
Stray|epic
Cult of the Lamb|steam
Kynseed|steam
Slay the Spire|epic
Balatro|steam
Vampire Survivors|epic
Inscryption|epic
Hotline Miami|gog
Katana ZERO|gog
Dead by Daylight|epic
Rust|steam
DayZ|steam
ARK: Survival Evolved|epic
ARK: Survival Ascended|steam
The Forest|steam
Sons Of The Forest|steam
Raft|steam
Green Hell|steam
7 Days to Die|steam
Don't Starve Together|steam
Deep Rock Galactic|steam
Risk of Rain 2|epic
Warframe|epic
Destiny 2|epic
Path of Exile|steam
Path of Exile 2|steam
Lost Ark|steam
Genshin Impact|epic
Honkai: Star Rail|epic
Wuthering Waves|epic
Zenless Zone Zero|epic
The Finals|steam
Marvel Rivals|steam
Valorant|epic
League of Legends|epic
PUBG: BATTLEGROUNDS|steam
Escape from Tarkov|steam
Hunt: Showdown 1896|steam
Ready or Not|steam
Squad|steam
Arma 3|steam
Euro Truck Simulator 2|steam
American Truck Simulator|steam
Farming Simulator 22|epic
Farming Simulator 25|steam
Assetto Corsa Competizione|epic
Assetto Corsa|steam
DiRT Rally 2.0|epic
EA SPORTS WRC|ea
Snowrunner|epic
BeamNG.drive|steam
Kerbal Space Program|steam
Microsoft Solitaire Collection|xbox
Firewatch|gog
What Remains of Edith Finch|epic
Gris|gog
Journey|epic
Limbo|gog
Inside|epic
Little Nightmares II|epic
It Takes Two|steam
A Plague Tale: Innocence|epic
A Plague Tale: Requiem|epic
Hellblade: Senua's Sacrifice|epic
Senua's Saga: Hellblade II=Hellblade II: Senua's Saga|xbox
Detroit: Become Human|epic
Heavy Rain|epic
Beyond: Two Souls|epic
Death Stranding|epic
The Callisto Protocol|epic
Dead Space 2|ea
Outlast|epic
Amnesia: The Bunker|gog
SOMA|gog
Alien: Isolation|epic
Silent Hill 2=Silent Hill 2 (2024)|steam
Until Dawn|steam
Lords of the Fallen|epic
Nioh 2 - The Complete Edition|epic
Wo Long: Fallen Dynasty|steam
Remnant II|epic
Remnant: From the Ashes|epic
Star Wars: Knights of the Old Republic|steam
Mass Effect 2|ea
Pillars of Eternity|gog
Pathfinder: Wrath of the Righteous - Enhanced Edition|gog
Wasteland 3|gog
Tyranny|gog
Pentiment|steam
Cyberpunk 2077: Phantom Liberty=Cyberpunk 2077|gog
Planescape: Torment: Enhanced Edition|gog
Heroes of Might and Magic III: Complete|gog
Diablo|gog
Warcraft III: Reforged|battlenet
Stronghold Crusader HD=Stronghold Crusader|steam
Command & Conquer Remastered Collection|ea
Homeworld Remastered Collection|epic
Sins of a Solar Empire II|epic
Manor Lords|gog
Against the Storm|epic
Dwarf Fortress|steam
Oxygen Not Included|epic
Project Zomboid|steam
Kenshi|gog
Mount & Blade II: Bannerlord|epic
Chivalry 2|epic
War Thunder|steam
World of Tanks|steam
Sea of Stars|steam
Chained Echoes|steam
Pizza Tower|steam
Enter the Gungeon|epic
The Binding of Isaac: Rebirth|steam
Spelunky 2|steam
Terraria|steam
Starbound|gog
Core Keeper|steam
Dave the Diver|steam
Unpacking|steam
Tunic|epic
Death's Door|steam
Hollow Knight: Silksong|steam
Blasphemous 2|epic
Ori and the Will of the Wisps|steam
Nine Sols|steam
Prince of Persia: The Sands of Time|ubisoft
Grand Theft Auto IV: The Complete Edition=Grand Theft Auto IV|steam
Grand Theft Auto: San Andreas|steam
Max Payne 3|steam
L.A. Noire|steam
Bully: Scholarship Edition|steam
Red Dead Redemption|steam
Fable Anniversary|steam
Jusant|epic
Sifu|epic
Tchia|epic
Kena: Bridge of Spirits|epic
Crash Bandicoot N. Sane Trilogy|steam
Spyro Reignited Trilogy|steam
Psychonauts 2|xbox
It Takes Two|epic
Unravel Two|ea
Little Nightmares|steam
Hogwarts Legacy|epic
LEGO Star Wars: The Skywalker Saga|epic
LEGO Batman: The Videogame|steam
LEGO The Lord of the Rings|steam
Lego Harry Potter Collection|steam
Goat Simulator 3|epic
PowerWash Simulator|steam
House Flipper 2|steam
Gas Station Simulator|epic
Car Mechanic Simulator 2021|epic
Supermarket Simulator|steam
Schedule I|steam
R.E.P.O.|steam
Peak|steam
Content Warning|steam
Buckshot Roulette|steam
Clair Obscur: Expedition 33|epic
Split Fiction|steam
Kingdom Hearts HD 1.5 + 2.5 ReMIX|epic
Metal Gear Solid V: The Phantom Pain|steam
Metal Gear Solid Δ: Snake Eater|steam
Death Stranding 2: On the Beach|steam
Mafia: The Old Country|steam
Borderlands 4|steam
Battlefield 6|ea
Dune: Awakening|steam
"""

# Real folder and installer names as people and repackers leave them:
# dropped subtitles, abbreviations, install folders, scene tags. Each
# with the game it is.
QUIRKS = """
folder|D:\\Games\\Elden.Ring.v1.10-FitGirl|Elden Ring
folder|D:\\Games\\Red Dead Redemption 2 [FitGirl Repack]|Red Dead Redemption 2
folder|D:\\Games\\Cyberpunk.2077.Phantom.Liberty-RUNE|Cyberpunk 2077
folder|D:\\Games\\Hogwarts.Legacy.Deluxe.Edition-EMPRESS|Hogwarts Legacy
folder|D:\\Games\\The Witcher 3 Wild Hunt GOTY|The Witcher 3: Wild Hunt
folder|D:\\Games\\GTA V|Grand Theft Auto V
folder|D:\\Games\\GTA5|Grand Theft Auto V
folder|D:\\Games\\GTAIV|Grand Theft Auto IV: The Complete Edition|Grand Theft Auto IV
folder|D:\\Games\\GTA San Andreas|Grand Theft Auto: San Andreas
folder|D:\\Games\\RDR2|Red Dead Redemption 2
folder|D:\\Games\\Sekiro Shadows Die Twice|Sekiro: Shadows Die Twice
folder|D:\\Games\\Sekiro|Sekiro: Shadows Die Twice
folder|D:\\Games\\DARK SOULS III|Dark Souls III
folder|D:\\Games\\Spider-Man Miles Morales|Marvel's Spider-Man: Miles Morales
folder|D:\\Games\\God of War|God of War|God of War (2018)
folder|D:\\Games\\Ghost of Tsushima DIRECTOR'S CUT|Ghost of Tsushima DIRECTOR'S CUT
folder|D:\\Games\\HITMAN 3|Hitman 3|HITMAN World of Assassination
folder|D:\\Games\\Resident Evil 4|Resident Evil 4|Resident Evil 4 (2023)
folder|D:\\Games\\Batman Arkham Knight|Batman: Arkham Knight
folder|D:\\Games\\Star Wars Jedi Fallen Order|Star Wars Jedi: Fallen Order
folder|D:\\Games\\Mortal Kombat 11 Ultimate|Mortal Kombat 11
folder|D:\\Games\\TEKKEN 8|TEKKEN 8
folder|D:\\Games\\Street Fighter 6|Street Fighter 6
folder|D:\\Games\\Monster Hunter World|Monster Hunter: World
folder|D:\\Games\\Palworld|Palworld
folder|D:\\Games\\Lethal Company|Lethal Company
folder|D:\\Games\\Black Myth Wukong|Black Myth: Wukong
folder|D:\\Games\\STALKER 2 Heart of Chornobyl|S.T.A.L.K.E.R. 2: Heart of Chornobyl
folder|D:\\Games\\Kingdom Come Deliverance II|Kingdom Come: Deliverance II
folder|D:\\Games\\DOOM Eternal|DOOM Eternal
folder|D:\\Games\\Metro Exodus Enhanced Edition|Metro Exodus
folder|D:\\Games\\Horizon Forbidden West Complete Edition|Horizon Forbidden West Complete Edition
folder|D:\\Games\\Final Fantasy VII Remake Intergrade|Final Fantasy VII Remake Intergrade
folder|D:\\Games\\Persona 5 Royal|Persona 5 Royal
folder|D:\\Games\\Yakuza Like a Dragon|Yakuza: Like a Dragon
folder|D:\\Games\\Sonic Frontiers|Sonic Frontiers
folder|D:\\Games\\Need for Speed Heat|Need for Speed Heat
folder|D:\\Games\\Assassins Creed Mirage|Assassin's Creed Mirage
folder|D:\\Games\\FarCry5|Far Cry 5
folder|D:\\Games\\Baldurs.Gate.3.v4.1.1-GOG|Baldur's Gate 3
folder|D:\\Games\\Stardew_Valley|Stardew Valley
folder|D:\\Games\\HollowKnight|Hollow Knight
folder|D:\\Games\\Hades II|Hades II
folder|D:\\Games\\Portal 2|Portal 2
folder|D:\\Games\\Assassin Creed Black Flag Resynced|Assassin's Creed: Black Flag Resynced
folder|D:\\Games\\AC Valhalla|Assassin's Creed Valhalla
folder|D:\\Games\\ACOdyssey|Assassin's Creed Odyssey
folder|D:\\Games\\Witcher 3|The Witcher 3: Wild Hunt
folder|D:\\Games\\Skyrim Special Edition|The Elder Scrolls V: Skyrim Special Edition
folder|D:\\Games\\SkyrimSE|The Elder Scrolls V: Skyrim Special Edition
folder|D:\\Games\\Fallout4|Fallout 4
folder|D:\\Games\\Fallout New Vegas|Fallout: New Vegas
folder|D:\\Games\\FalloutNV|Fallout: New Vegas
folder|D:\\Games\\Oblivion Remastered|The Elder Scrolls IV: Oblivion Remastered
folder|D:\\Games\\Mass Effect Legendary|Mass Effect Legendary Edition
folder|D:\\Games\\Dying Light 2|Dying Light 2 Stay Human
folder|D:\\Games\\Dying.Light.2.Stay.Human.v1.17-P2P|Dying Light 2 Stay Human
folder|D:\\Games\\CoD Black Ops 6|Call of Duty: Black Ops 6
folder|D:\\Games\\COD WWII|Call of Duty: WWII
folder|D:\\Games\\NFS Unbound|Need for Speed Unbound
folder|D:\\Games\\NFS Most Wanted|Need for Speed Most Wanted
folder|D:\\Games\\Nier Automata|NieR:Automata
folder|D:\\Games\\Divinity Original Sin 2|Divinity: Original Sin 2 - Definitive Edition
folder|D:\\Games\\Disco Elysium|Disco Elysium - The Final Cut
folder|D:\\Games\\Civilization VI|Sid Meier's Civilization VI
folder|D:\\Games\\Civ 6|Sid Meier's Civilization VI
folder|D:\\Games\\Total War Warhammer 3|Total War: WARHAMMER III
folder|D:\\Games\\Warhammer 3|Total War: WARHAMMER III
folder|D:\\Games\\Crusader Kings 3|Crusader Kings III
folder|D:\\Games\\HOI4|Hearts of Iron IV
folder|D:\\Games\\Europa Universalis IV|Europa Universalis IV
folder|D:\\Games\\Cities Skylines 2|Cities: Skylines II
folder|D:\\Games\\Anno1800|Anno 1800
folder|D:\\Games\\Rainbow Six Siege|Tom Clancy's Rainbow Six Siege
folder|D:\\Games\\The Division 2|Tom Clancy's The Division 2
folder|D:\\Games\\Ghost Recon Wildlands|Tom Clancy's Ghost Recon Wildlands
folder|D:\\Games\\Watch Dogs Legion|Watch Dogs: Legion
folder|D:\\Games\\WatchDogs2|Watch_Dogs 2
folder|D:\\Games\\Forza Horizon 5|Forza Horizon 5
folder|D:\\Games\\MSFS2020|Microsoft Flight Simulator (2020) 40th Anniversary Edition
folder|D:\\Games\\Halo MCC|Halo: The Master Chief Collection
folder|D:\\Games\\Gears5|Gears 5
folder|D:\\Games\\Indiana Jones Great Circle|Indiana Jones and the Great Circle
folder|D:\\Games\\Wolfenstein 2|Wolfenstein II: The New Colossus
folder|D:\\Games\\Wolfenstein.II.The.New.Colossus-CODEX|Wolfenstein II: The New Colossus
folder|D:\\Games\\Dishonored2|Dishonored 2
folder|D:\\Games\\RE Village|Resident Evil Village
folder|D:\\Games\\RE4 Remake|Resident Evil 4|Resident Evil 4 (2023)
folder|D:\\Games\\Resident.Evil.2.Remake-CODEX|Resident Evil 2|Resident Evil 2 (2019)
folder|D:\\Games\\RE7|Resident Evil 7 Biohazard
folder|D:\\Games\\Monster Hunter Rise Sunbreak|Monster Hunter Rise
folder|D:\\Games\\MHWilds|Monster Hunter Wilds
folder|D:\\Games\\DMC5|Devil May Cry 5
folder|D:\\Games\\Devil May Cry 5 Special Edition|Devil May Cry 5
folder|D:\\Games\\Dragons Dogma 2|Dragon's Dogma 2
folder|D:\\Games\\MK1|Mortal Kombat 1
folder|D:\\Games\\Arkham City GOTY|Batman: Arkham City - Game of the Year Edition
folder|D:\\Games\\Shadow of War|Middle-earth: Shadow of War
folder|D:\\Games\\Metro 2033 Redux|Metro 2033 Redux
folder|D:\\Games\\STALKER Shadow of Chernobyl|S.T.A.L.K.E.R.: Shadow of Chernobyl
folder|D:\\Games\\KCD|Kingdom Come: Deliverance
folder|D:\\Games\\Mafia Definitive Edition|Mafia: Definitive Edition
folder|D:\\Games\\Mafia 2 DE|Mafia II: Definitive Edition
folder|D:\\Games\\Bioshock Infinite|BioShock Infinite
folder|D:\\Games\\Sleeping Dogs|Sleeping Dogs: Definitive Edition
folder|D:\\Games\\Shadow of the Tomb Raider Definitive Edition|Shadow of the Tomb Raider
folder|D:\\Games\\Rise of the Tomb Raider 20 Year Celebration|Rise of the Tomb Raider
folder|D:\\Games\\FF7 Remake|Final Fantasy VII Remake Intergrade
folder|D:\\Games\\FFXVI|FINAL FANTASY XVI
folder|D:\\Games\\Final Fantasy XV|FINAL FANTASY XV WINDOWS EDITION
folder|D:\\Games\\Persona 3 Reload|Persona 3 Reload
folder|D:\\Games\\Yakuza 0|Yakuza 0
folder|D:\\Games\\Infinite Wealth|Like a Dragon: Infinite Wealth
folder|D:\\Games\\Octopath Traveler 2|Octopath Traveler II
folder|D:\\Games\\Dragon Quest XI S|DRAGON QUEST XI S: Echoes of an Elusive Age - Definitive Edition|Dragon Quest XI S - Definitive Edition
folder|D:\\Games\\Half-Life 2|Half-Life 2
folder|D:\\Games\\Half Life Alyx|Half-Life: Alyx
folder|D:\\Games\\L4D2|Left 4 Dead 2
folder|D:\\Games\\Lies of P|Lies of P
folder|D:\\Games\\Cult.of.the.Lamb.v1.4.5-GOG|Cult of the Lamb
folder|D:\\Games\\Slay the Spire|Slay the Spire
folder|D:\\Games\\Vampire_Survivors|Vampire Survivors
folder|D:\\Games\\Sons of the Forest|Sons Of The Forest
folder|D:\\Games\\7 Days To Die|7 Days to Die
folder|D:\\Games\\DST|Don't Starve Together
folder|D:\\Games\\Deep Rock Galactic|Deep Rock Galactic
folder|D:\\Games\\Risk of Rain 2|Risk of Rain 2
folder|D:\\Games\\PoE2|Path of Exile 2
folder|D:\\Games\\ETS2|Euro Truck Simulator 2
folder|D:\\Games\\Euro Truck Simulator 2|Euro Truck Simulator 2
folder|D:\\Games\\Farming Simulator 22|Farming Simulator 22
folder|D:\\Games\\ACC|Assetto Corsa Competizione
folder|D:\\Games\\Assetto Corsa Competizione|Assetto Corsa Competizione
folder|D:\\Games\\DiRT Rally 2.0|DiRT Rally 2.0
folder|D:\\Games\\SnowRunner|Snowrunner
folder|D:\\Games\\KSP|Kerbal Space Program
folder|D:\\Games\\A Plague Tale Requiem|A Plague Tale: Requiem
folder|D:\\Games\\Hellblade|Hellblade: Senua's Sacrifice
folder|D:\\Games\\Detroit Become Human|Detroit: Become Human
folder|D:\\Games\\Death Stranding Directors Cut|Death Stranding Director's Cut
folder|D:\\Games\\The Callisto Protocol|The Callisto Protocol
folder|D:\\Games\\Alien Isolation|Alien: Isolation
folder|D:\\Games\\Silent Hill 2 Remake|Silent Hill 2|Silent Hill 2 (2024)
folder|D:\\Games\\Nioh 2|Nioh 2 - The Complete Edition
folder|D:\\Games\\Remnant 2|Remnant II
folder|D:\\Games\\KOTOR|Star Wars: Knights of the Old Republic
folder|D:\\Games\\Pathfinder WotR|Pathfinder: Wrath of the Righteous - Enhanced Edition
folder|D:\\Games\\Wasteland 3|Wasteland 3
folder|D:\\Games\\HoMM3|Heroes of Might and Magic III: Complete
folder|D:\\Games\\Heroes 3 Complete|Heroes of Might and Magic III: Complete
folder|D:\\Games\\C&C Remastered|Command & Conquer Remastered Collection
folder|D:\\Games\\Manor Lords|Manor Lords
folder|D:\\Games\\Bannerlord|Mount & Blade II: Bannerlord
folder|D:\\Games\\Mount and Blade 2 Bannerlord|Mount & Blade II: Bannerlord
folder|D:\\Games\\Binding of Isaac Rebirth|The Binding of Isaac: Rebirth
folder|D:\\Games\\Silksong|Hollow Knight: Silksong
folder|D:\\Games\\MGSV TPP|Metal Gear Solid V: The Phantom Pain
folder|D:\\Games\\Metal Gear Solid V The Phantom Pain|Metal Gear Solid V: The Phantom Pain
folder|D:\\Games\\LA Noire|L.A. Noire
folder|D:\\Games\\Crash Bandicoot N Sane Trilogy|Crash Bandicoot N. Sane Trilogy
folder|D:\\Games\\LEGO Skywalker Saga|LEGO Star Wars: The Skywalker Saga
folder|D:\\Games\\Clair Obscur Expedition 33|Clair Obscur: Expedition 33
folder|D:\\Games\\Expedition 33|Clair Obscur: Expedition 33
folder|D:\\Games\\REPO|R.E.P.O.
folder|C:\\Program Files (x86)\\Steam\\steamapps\\common\\Counter-Strike Global Offensive|Counter-Strike 2
folder|C:\\Program Files (x86)\\Steam\\steamapps\\common\\dota 2 beta|Dota 2
folder|C:\\Program Files\\Epic Games\\GTAV|Grand Theft Auto V
folder|C:\\Program Files\\Epic Games\\RedDeadRedemption2|Red Dead Redemption 2
folder|C:\\Program Files\\Epic Games\\AlanWake2|Alan Wake 2
folder|C:\\Program Files\\Epic Games\\HogwartsLegacy|Hogwarts Legacy
folder|C:\\Program Files\\Epic Games\\rocketleague|Rocket League
folder|C:\\XboxGames\\Starfield|Starfield
folder|C:\\XboxGames\\Forza Horizon 5|Forza Horizon 5
folder|C:\\Program Files (x86)\\Ubisoft\\Ubisoft Game Launcher\\games\\Assassin's Creed Valhalla|Assassin's Creed Valhalla
folder|C:\\Program Files\\EA Games\\Battlefield 2042|Battlefield 2042
folder|C:\\Program Files (x86)\\Overwatch|Overwatch 2
folder|C:\\Program Files (x86)\\Diablo IV|Diablo IV
folder|C:\\GOG Games\\Cyberpunk 2077|Cyberpunk 2077
folder|C:\\GOG Games\\The Witcher 3 Wild Hunt GOTY|The Witcher 3: Wild Hunt
installer|Elden Ring Shadow of the Erdtree Deluxe Edition|C:\\Games\\ELDEN RING|Elden Ring
installer|Grand Theft Auto V version 1.0.3179|C:\\Games\\Grand Theft Auto V|Grand Theft Auto V
installer|Cyberpunk 2077 - Ultimate Edition|C:\\Games\\Cyberpunk 2077|Cyberpunk 2077
installer|Resident Evil Village Gold Edition|C:\\Games\\RE Village|Resident Evil Village
installer|Dying Light 2 Stay Human|C:\\Games\\Dying Light 2|Dying Light 2 Stay Human
installer|Alan Wake II|C:\\Program Files (x86)\\DODI-Repacks\\Alan Wake II|Alan Wake 2
installer|Baldur's Gate 3|C:\\Program Files (x86)\\DODI-Repacks\\Baldurs Gate 3|Baldur's Gate 3
installer|Hogwarts Legacy: Digital Deluxe Edition|C:\\Games\\Hogwarts Legacy|Hogwarts Legacy
installer|Marvel's Spider-Man 2 - Digital Deluxe Edition|C:\\Games\\Marvel's Spider-Man 2|Marvel's Spider-Man 2
installer|Black Myth: Wukong - Deluxe Edition, v1.0.8.14860 + 4 DLCs|C:\\Games\\Black Myth Wukong|Black Myth: Wukong
installer|God of War Ragnarok - Digital Deluxe Edition|C:\\Games\\God of War Ragnarok|God of War Ragnarök
installer|Starfield: Premium Edition|C:\\Games\\Starfield|Starfield
installer|Star Wars Outlaws: Gold Edition|C:\\Games\\Star Wars Outlaws|Star Wars Outlaws
installer|Final Fantasy XVI: Complete Edition|C:\\Games\\FINAL FANTASY XVI|FINAL FANTASY XVI
installer|Stellar Blade Complete Edition|C:\\Games\\Stellar Blade|Stellar Blade
installer|Kingdom Come Deliverance II Gold Edition|C:\\Games\\KCD2|Kingdom Come: Deliverance II
installer|Indiana Jones and the Great Circle: Premium Edition|C:\\Games\\TheGreatCircle|Indiana Jones and the Great Circle
installer|S.T.A.L.K.E.R. 2: Heart of Chornobyl - Ultimate Edition|C:\\Games\\STALKER 2|S.T.A.L.K.E.R. 2: Heart of Chornobyl
installer|Monster Hunter Wilds: Premium Deluxe Edition|C:\\Games\\MonsterHunterWilds|Monster Hunter Wilds
installer|Assassin's Creed Shadows - Deluxe Edition|C:\\Games\\AC Shadows|Assassin's Creed Shadows
installer|DOOM: The Dark Ages - Premium Edition|C:\\Games\\DOOMTheDarkAges|DOOM: The Dark Ages
installer|Silent Hill 2 Deluxe Edition|C:\\Games\\SILENT HILL 2|Silent Hill 2|Silent Hill 2 (2024)
installer|Split Fiction|C:\\Games\\Split Fiction|Split Fiction
installer|Clair Obscur: Expedition 33 - Deluxe Edition|C:\\Games\\Expedition 33|Clair Obscur: Expedition 33
installer|The Last of Us Part I - Digital Deluxe Edition|C:\\Games\\The Last of Us Part I|The Last of Us Part I
installer|Horizon Forbidden West: Complete Edition|C:\\Games\\Horizon Forbidden West|Horizon Forbidden West Complete Edition
installer|Ghost of Tsushima: Director's Cut|C:\\Games\\Ghost of Tsushima|Ghost of Tsushima DIRECTOR'S CUT
installer|Lies of P: Deluxe Edition|C:\\Games\\Lies of P|Lies of P
installer|Palworld v0.4.11|C:\\Games\\Palworld|Palworld
installer|Hades II v1.0|C:\\Games\\Hades II|Hades II
folder|D:\\Games\\DirectX|-
folder|D:\\Games\\_CommonRedist|-
folder|D:\\Games\\Redist|-
folder|D:\\Games\\Tools|-
folder|D:\\Games\\Mods|-
folder|D:\\Games\\Saves|-
folder|D:\\Games\\Backup|-
folder|D:\\Games\\New folder|-
folder|D:\\Games\\Downloads|-
folder|D:\\Games\\Emulators|-
folder|D:\\Games\\Trainers|-
folder|D:\\Games\\Screenshots|-
folder|D:\\Games\\Wallpapers|-
folder|D:\\Games\\Cheat Engine 7.5|-
folder|D:\\Games\\MSI Afterburner|-
folder|D:\\Games\\RivaTuner Statistics Server|-
folder|D:\\Games\\Discord|-
folder|D:\\Games\\OBS Studio|-
folder|D:\\Games\\Vortex|-
folder|D:\\Games\\Mod Organizer 2|-
folder|D:\\Games\\ReShade|-
folder|D:\\Games\\DLSS Swapper|-
folder|D:\\Games\\Special K|-
folder|D:\\Games\\Syncer|-
folder|D:\\Games\\WinRAR|-
folder|D:\\Games\\7-Zip|-
folder|D:\\Games\\Steam|-
folder|D:\\Games\\SteamLibrary|-
folder|D:\\Games\\Epic Games|-
folder|D:\\Games\\GOG Galaxy|-
folder|D:\\Games\\Ubisoft Connect|-
folder|D:\\Games\\EA App|-
folder|D:\\Games\\Battle.net|-
folder|D:\\Games\\Xbox|-
folder|D:\\Games\\Playnite|-
folder|D:\\Games\\LaunchBox|-
folder|D:\\Games\\RetroArch|-
folder|D:\\Games\\Yuzu|-
folder|D:\\Games\\PCSX2|-
folder|D:\\Games\\Dolphin|-
folder|D:\\Games\\qBittorrent|-
folder|D:\\Games\\FitGirl Repacks|-
folder|D:\\Games\\DODI Repacks|-
folder|D:\\Games\\old|-
folder|D:\\Games\\temp|-
folder|D:\\Games\\Uninstall|-
folder|D:\\Games\\Game|-
folder|D:\\Games\\Games|-
folder|D:\\Games\\My Games|-
folder|D:\\Games\\Shaders|-
"""

STORE_MARKS = {"ubisoft": "®", "ea": "™", "epic": "™", "xbox": "", "battlenet": "®", "gog": "", "steam": ""}
GROUPS = ["RUNE", "CODEX", "EMPRESS", "TENOKE", "FLT", "PLAZA", "SKIDROW", "GOG", "DOGE"]
REPACKS = ["[FitGirl Repack]", "[DODI Repack]", "- ElAmigos", "(ElAmigos)", "[KaOs Repack]"]


def pick(s, n):
    return int(hashlib.sha1(s.encode()).hexdigest(), 16) % n


def plain(t):
    """What a folder keeps of a title: no colons, dashes or marks."""
    t = t.replace("™", "").replace("®", "").replace(":", "").replace(" - ", " ").replace("/", " ")
    return re.sub(r"\s+", " ", t).strip()


def variants(title, store):
    base = plain(title)
    out = []
    # How its store (or launcher) names it.
    if store != "steam":
        mark = STORE_MARKS[store]
        words = title.split(" ")
        if mark and len(words) > 1 and pick(title + "m", 3) == 0:
            words[-1] += mark
        out.append((f"store:{store}", " ".join(words)))
    # A folder or installer name.
    k = pick(title, 7)
    if k == 0:
        out.append(("folder", "D:\\Games\\" + base.replace(" ", ".").replace("'", "") + f".v1.{pick(title, 20)}.{pick(title+'x', 9)}-" + GROUPS[pick(title, len(GROUPS))]))
    elif k == 1:
        out.append(("folder", "D:\\Games\\" + base + " " + REPACKS[pick(title, len(REPACKS))]))
    elif k == 2:
        out.append(("folder", "D:\\Games\\" + base.replace(" ", "_")))
    elif k == 3:
        out.append(("installer", f"{title} v{pick(title, 5)}.{pick(title+'y', 30)}|C:\\Games\\{base}"))
    elif k == 4:
        out.append(("folder", "D:\\Games\\" + re.sub(r"[^A-Za-z0-9]", "", base.title() if base.islower() else base)))
    elif k == 5:
        out.append(("folder", "D:\\Games\\" + base.replace("'", "")))
    else:
        out.append(("folder", "D:\\Games\\" + base.lower()))
    return out


rows = []
seen = set()
for line in GAMES.strip().splitlines():
    names, store = line.rsplit("|", 1)
    title = names.split("=")[0]  # the store's name; others after "=" count too
    for kind, text in variants(title, store):
        if (kind, text) in seen:
            continue
        seen.add((kind, text))
        rows.append((kind, text, names.replace("=", "|")))
for line in QUIRKS.strip().splitlines():
    parts = line.split("|")
    if parts[0] == "installer":
        kind, text, want = "installer", parts[1] + "|" + parts[2], "|".join(parts[3:])
    else:
        kind, text, want = parts[0], parts[1], "|".join(parts[2:])
    if (kind, text) in seen:
        continue
    seen.add((kind, text))
    rows.append((kind, text, want))

with io.open("match_audit.tsv", "w", encoding="utf-8", newline="\n") as f:
    f.write("# Generated by gen_match_audit.py: kind, what the scanner sees, accepted titles (- for not a game)\n")
    for r in rows:
        f.write("\t".join(r) + "\n")
print(len(rows), "cases")
