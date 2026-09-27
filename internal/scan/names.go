package scan

import (
	"strings"
	"unicode"
)

// Names folders are often given for well-known games, written out. Keyed
// by Normalize, so "SkyrimSE", "skyrim se" and "Skyrim_SE" all count.
var nameAliases = map[string]string{
	"skyrimse": "The Elder Scrolls V: Skyrim Special Edition", "skyrimae": "The Elder Scrolls V: Skyrim Special Edition",
	"skyrim": "The Elder Scrolls V: Skyrim", "oblivion": "The Elder Scrolls IV: Oblivion", "morrowind": "The Elder Scrolls III: Morrowind",
	"falloutnv": "Fallout: New Vegas", "fnv": "Fallout: New Vegas", "fo4": "Fallout 4", "fo3": "Fallout 3",
	"l4d": "Left 4 Dead", "l4d2": "Left 4 Dead 2", "dst": "Don't Starve Together", "poe": "Path of Exile", "poe2": "Path of Exile 2",
	"acc": "Assetto Corsa Competizione", "kotor": "Star Wars: Knights of the Old Republic",
	"kotor2": "Star Wars Knights of the Old Republic II: The Sith Lords",
	"homm3":  "Heroes of Might and Magic III", "heroes3": "Heroes of Might and Magic III", "heroes3complete": "Heroes of Might and Magic III: Complete",
	"mgsv": "Metal Gear Solid V: The Phantom Pain", "mgsvtpp": "Metal Gear Solid V: The Phantom Pain", "mgs5": "Metal Gear Solid V: The Phantom Pain",
	"kcd": "Kingdom Come: Deliverance", "kcd2": "Kingdom Come: Deliverance II",
	"msfs": "Microsoft Flight Simulator", "msfs2020": "Microsoft Flight Simulator", "msfs2024": "Microsoft Flight Simulator 2024",
	"csgo": "Counter-Strike 2", "cs2": "Counter-Strike 2", "tf2": "Team Fortress 2", "bg3": "Baldur's Gate 3",
	"ds1": "Dark Souls: Remastered", "ds2": "Dark Souls II: Scholar of the First Sin", "ds3": "Dark Souls III",
	"bf1": "Battlefield 1", "bf4": "Battlefield 4", "bfv": "Battlefield V", "bf2042": "Battlefield 2042",
	"eu4": "Europa Universalis IV", "eu5": "Europa Universalis V", "hoi4": "Hearts of Iron IV", "ck2": "Crusader Kings II", "ck3": "Crusader Kings III",
	"p5r": "Persona 5 Royal", "p4g": "Persona 4 Golden", "p3r": "Persona 3 Reload",
	"ffxiv": "FINAL FANTASY XIV Online", "ffxv": "FINAL FANTASY XV WINDOWS EDITION", "ffxvi": "FINAL FANTASY XVI",
	"ff7r": "FINAL FANTASY VII REMAKE INTERGRADE", "ff7remake": "FINAL FANTASY VII REMAKE INTERGRADE", "ff7rebirth": "FINAL FANTASY VII REBIRTH",
	"dmc5": "Devil May Cry 5", "mhw": "Monster Hunter: World", "mhr": "Monster Hunter Rise", "sf6": "Street Fighter 6",
	"ow2": "Overwatch 2", "wow": "World of Warcraft", "sc2": "StarCraft II", "d2r": "Diablo II: Resurrected", "d4": "Diablo IV",
	"bannerlord": "Mount & Blade II: Bannerlord", "ets2": "Euro Truck Simulator 2", "ats": "American Truck Simulator",
	"ksp": "Kerbal Space Program", "ksp2": "Kerbal Space Program 2", "tw3": "The Witcher 3: Wild Hunt", "witcher3": "The Witcher 3: Wild Hunt",
	"re8": "Resident Evil Village", "re7": "Resident Evil 7 Biohazard",
	"aoe2": "Age of Empires II: Definitive Edition", "aoe2de": "Age of Empires II: Definitive Edition", "aoe4": "Age of Empires IV",
	"sotf": "Sons Of The Forest", "mcc": "Halo: The Master Chief Collection", "halomcc": "Halo: The Master Chief Collection",
	"drg": "Deep Rock Galactic", "ror2": "Risk of Rain 2", "tboi": "The Binding of Isaac: Rebirth", "hl2": "Half-Life 2",
	"hla": "Half-Life: Alyx", "pubg": "PUBG: BATTLEGROUNDS", "r6s": "Tom Clancy's Rainbow Six Siege", "r6siege": "Tom Clancy's Rainbow Six Siege",
	"wd2": "Watch_Dogs 2", "wdlegion": "Watch Dogs: Legion",
}

// First words folders shorten ("AC Valhalla", "ACOdyssey", "RE Village",
// "MHWilds"), written out.
var prefixAliases = map[string]string{
	"ac": "Assassin's Creed", "re": "Resident Evil", "mh": "Monster Hunter", "mk": "Mortal Kombat", "ff": "Final Fantasy",
	"cnc": "Command & Conquer", "c&c": "Command & Conquer", "civ": "Sid Meier's Civilization", "tw": "Total War", "dq": "Dragon Quest",
	"kh": "Kingdom Hearts", "mgs": "Metal Gear Solid", "bf": "Battlefield", "gta": "Grand Theft Auto", "rdr": "Red Dead Redemption",
	"nfs": "Need for Speed", "cod": "Call of Duty", "hl": "Half-Life", "aoe": "Age of Empires", "wd": "Watch Dogs",
}

// Edition words folders shorten at the end ("Mafia 2 DE", "Arkham City GOTY").
var suffixAliases = map[string]string{"de": "Definitive Edition", "goty": "Game of the Year Edition", "ce": "Complete Edition"}

// Aliases writes out a folder name in the ways it's commonly shortened:
// a known short name ("SkyrimSE", "L4D2"), a shortened first word
// ("AC Valhalla", "MHWilds") or a shortened edition ("Mafia 2 DE").
// Nothing when there's nothing to write out.
func Aliases(s string) []string {
	var out []string
	if full := nameAliases[Normalize(s)]; full != "" {
		out = append(out, full)
	}
	words := splitWords(s)
	if len(words) >= 2 {
		if full := prefixAliases[strings.ToLower(words[0])]; full != "" {
			out = append(out, full+" "+strings.Join(words[1:], " "))
		}
		if full := suffixAliases[strings.ToLower(words[len(words)-1])]; full != "" {
			out = append(out, strings.Join(words[:len(words)-1], " ")+" "+full)
		}
	}
	return out
}

// splitWords splits a name at spaces and punctuation, camel case
// ("ACOdyssey" → AC Odyssey) and between letters and digits ("MK1").
func splitWords(s string) []string {
	var words []string
	var cur []rune
	rs := []rune(s)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = cur[:0]
		}
	}
	for i, c := range rs {
		if c == '&' && len(cur) > 0 { // "C&C" stays one word
			cur = append(cur, c)
			continue
		}
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '\'' {
			flush()
			continue
		}
		if len(cur) > 0 {
			p := cur[len(cur)-1]
			next := rune(0)
			if i+1 < len(rs) {
				next = rs[i+1]
			}
			switch {
			case unicode.IsLower(p) && unicode.IsUpper(c),
				unicode.IsLetter(p) && unicode.IsDigit(c), unicode.IsDigit(p) && unicode.IsLetter(c),
				// "ACOdyssey": the last capital of a run starts the next word.
				unicode.IsUpper(p) && unicode.IsUpper(c) && unicode.IsLower(next):
				flush()
			}
		}
		cur = append(cur, c)
	}
	flush()
	return words
}

// notGames are folder and app names that turn up next to games but
// aren't games: generic folders, launchers, tools and emulators.
var notGames = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{
		"directx", "commonredist", "redist", "redistributables", "vcredist", "support", "tools", "tool", "mods", "saves",
		"backup", "backups", "new folder", "downloads", "download", "emulators", "emulator", "trainers", "screenshots", "wallpapers",
		"old", "temp", "tmp", "uninstall", "game", "games", "my games", "shaders", "cache", "logs", "config", "settings",
		"cheat engine", "msi afterburner", "rivatuner statistics server", "discord", "obs studio", "vortex",
		"mod organizer", "mod organizer 2", "reshade", "dlss swapper", "special k", "syncer", "winrar", "7 zip", "7zip",
		"steam", "steamlibrary", "steamapps", "common", "epic games", "epic games launcher", "gog galaxy", "gog games",
		"ubisoft connect", "ubisoft game launcher", "ea app", "ea desktop", "ea games", "origin", "battle net", "xbox",
		"xboxgames", "playnite", "launchbox", "retroarch", "yuzu", "ryujinx", "pcsx2", "rpcs3", "dolphin", "cemu",
		"qbittorrent", "fitgirl repacks", "dodi repacks", "repacks", "seaglass", "waterlauncher", "dlss updater", "nvidia", "amd",
		"geforce experience", "hwinfo", "cpu z", "gpu z", "program files", "program files x86",
	} {
		m[Normalize(n)] = true
	}
	return m
}()

// NotAGame reports whether a name is a folder or app that isn't a game
// ("Tools", "Discord", "Steam"), so it's never matched or looked up.
func NotAGame(title string) bool {
	k := Normalize(title)
	return len(k) < 2 || notGames[k]
}
