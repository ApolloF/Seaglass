// Package scan finds installed games on this PC: store installs, installer
// entries (repacks), shortcuts and plain game folders, and recognises
// unofficial copies (Steam emulators, cracks) and the Steam app they are.
package scan

// Source is where a game was found.
type Source string

const (
	Steam     Source = "steam"
	Epic      Source = "epic"
	GOG       Source = "gog"
	EA        Source = "ea"
	Ubisoft   Source = "ubisoft"
	BattleNet Source = "battlenet"
	Xbox      Source = "xbox"
	Installer Source = "installer" // a Windows uninstall entry (repacks, standalone installers)
	Shortcut  Source = "shortcut"  // a desktop or Start menu shortcut
	Folder    Source = "folder"    // a subfolder of a watched folder
)

// rank orders sources by how much their records can be trusted when two
// sources find the same folder: stores know best.
var rank = map[Source]int{Steam: 9, Epic: 9, GOG: 9, EA: 8, Ubisoft: 8, BattleNet: 8, Xbox: 8, Installer: 5, Shortcut: 3, Folder: 1}

// Store reports whether s is a store's own library.
func (s Source) Store() bool { return rank[s] >= 8 }

// Candidate is one game found on this PC.
type Candidate struct {
	Title        string // as the source names it
	TitleTrusted bool   // the title comes from a store or installer record, not a folder name
	Dir          string // install folder
	Exe          string // executable that starts the game ("" when a LaunchURI does)
	Args         string
	WorkDir      string
	LaunchURI    string // store link that starts the game (steam://rungameid/…)
	Source       Source
	How          string // human-readable: how it was found

	SteamAppID int
	GogID      string
	EpicApp    string // namespace:catalogItem:appName
	UbisoftID  string

	Publisher string // from the installer entry
	Repacker  string // "DODI", "FitGirl", … when a repacker installed it
	SizeBytes int64

	// Playtime the store recorded (Steam's localconfig.vdf).
	StorePlaytime   int64 // seconds
	StoreLastPlayed int64 // unix seconds

	Emulator  string // "RUNE", "CODEX", "Goldberg", … ; "" when none was found
	EmuMarker string // the file that gave it away
	EmuDir    string // folder (relative to Dir) where the marker or steam_settings was found
	AppIDFrom string // file the Steam AppID was read from (unofficial copies)
	DRMFree   string // "GOG" when a GOG game info file was found outside a GOG install
	PadHint   string // "libScePad" or "SDL": the game handles a DualSense itself
}

// Unofficial reports whether the copy runs on an emulator or was installed by a repacker.
func (c *Candidate) Unofficial() bool { return c.Emulator != "" || c.Repacker != "" }
