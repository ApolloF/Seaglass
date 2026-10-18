package scan

// The store clients below keep their installs in the Windows registry.
// On Linux they run under Proton, Heroic or Lutris instead; see
// docs/linux-feasibility.md for where those keep their libraries.

func gogCandidates() []Candidate     { return nil }
func eaCandidates() []Candidate      { return nil }
func ubisoftCandidates() []Candidate { return nil }

// uninstallEntries lists Windows' installed apps; Linux has no such list.
func uninstallEntries() []uninstallEntry { return nil }
