package vpad

// Installer is a driver's official installer, pinned to one release.
type Installer struct {
	Name   string // the driver, for messages
	Repo   string // https://github.com/<owner>/<repo>, where it's published
	URL    string // the release asset
	SHA256 string // of that file, checked before it runs
}

// Installers are downloaded when the person asks to set the drivers up,
// never bundled. The hashes were taken from the files GitHub serves for
// these releases; a file that doesn't match is not run.
var Installers = []Installer{
	{
		Name:   "ViGEmBus",
		Repo:   "https://github.com/nefarius/ViGEmBus",
		URL:    "https://github.com/nefarius/ViGEmBus/releases/download/v1.22.0/ViGEmBus_1.22.0_x64_x86_arm64.exe",
		SHA256: "89220a7865076b342892f98865f3499fb7c4cfd673159e89d352c360fd014c6a",
	},
	{
		Name:   "HidHide",
		Repo:   "https://github.com/nefarius/HidHide",
		URL:    "https://github.com/nefarius/HidHide/releases/download/v1.5.230.0/HidHide_1.5.230_x64.exe",
		SHA256: "f4bbbcb82e6258641b887c74bc81c4c5f66e4aa811808dfc304347687b7605f6",
	},
}

// Needed lists the installers a PC with status s still needs.
func Needed(s Status) []Installer {
	var out []Installer
	for _, in := range Installers {
		switch {
		case in.Name == "ViGEmBus" && (s.ViGEm || s.ViGEmVersion != ""):
		case in.Name == "HidHide" && (s.HidHide || s.HidHideVersion != ""):
		default:
			out = append(out, in)
		}
	}
	return out
}
