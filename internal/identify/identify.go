package identify

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ApolloF/WaterLauncher/internal/scan"
)

// Index answers lookups by id, title and install folder name.
type Index struct {
	bySteam map[int]*Entry
	byGog   map[string]*Entry
	byName  map[string]*Entry   // normalized title or alias
	byLoose map[string]*Entry   // scan.LooseKey of titles and aliases; nil when two games share one
	byDir   map[string][]*Entry // normalized install folder name
	// Looser still, for folder names only, nil when ambiguous: without
	// "and", "the", "of" and "a" ("Indiana Jones Great Circle"), and by
	// the subtitle alone ("Infinite Wealth" for "Like a Dragon: Infinite
	// Wealth").
	byLooser map[string]*Entry
	bySub    map[string]*Entry
}

func build(es []Entry) *Index {
	ix := &Index{
		bySteam:  make(map[int]*Entry, len(es)/2),
		byGog:    make(map[string]*Entry, len(es)/8),
		byName:   make(map[string]*Entry, len(es)),
		byLoose:  make(map[string]*Entry, len(es)),
		byDir:    make(map[string][]*Entry, len(es)/2),
		byLooser: make(map[string]*Entry, len(es)),
		bySub:    make(map[string]*Entry, len(es)/4),
	}
	unique := func(m map[string]*Entry, k string, e *Entry) {
		if prev, ok := m[k]; !ok {
			m[k] = e
		} else if prev != nil && prev != e && (prev.SteamID == 0 || prev.SteamID != e.SteamID) {
			m[k] = nil
		}
	}
	for i := range es {
		e := &es[i]
		if e.SteamID > 0 {
			if _, ok := ix.bySteam[e.SteamID]; !ok {
				ix.bySteam[e.SteamID] = e
			}
		}
		if e.GogID != "" {
			ix.byGog[e.GogID] = e
		}
		for _, n := range append([]string{e.Name}, e.Aliases...) {
			if k := scan.Normalize(n); len(k) >= 2 {
				if _, ok := ix.byName[k]; !ok {
					ix.byName[k] = e
				}
			}
			if k := scan.LooseKey(n); len(k) >= 4 {
				unique(ix.byLoose, k, e) // ambiguous: better no match than a wrong one
			}
			if k := looserKey(n); len(k) >= 6 {
				unique(ix.byLooser, k, e)
			}
			if k := subtitleKey(n); k != "" {
				unique(ix.bySub, k, e)
			}
		}
		for _, d := range e.InstallDirs {
			if k := scan.Normalize(d); len(k) >= 3 {
				ix.byDir[k] = append(ix.byDir[k], e)
			}
		}
	}
	return ix
}

// Len is the number of titles the index knows.
func (ix *Index) Len() int {
	if ix == nil {
		return 0
	}
	return len(ix.byName)
}

// SteamName returns the manifest's title for a Steam app id.
func (ix *Index) SteamName(id int) string {
	if ix == nil {
		return ""
	}
	if e := ix.bySteam[id]; e != nil {
		return e.Name
	}
	return ""
}

// Match is who a scanned game turned out to be.
type Match struct {
	Title      string
	SteamAppID int
	GogID      string
	Confidence int    // 0–100
	How        string // how the identity was established
}

// Edition words stripped when an exact title doesn't match.

// Identify establishes the game behind a candidate. ix may be nil (no
// manifest yet): the candidate's own ids and title are used as they are.
func (ix *Index) Identify(c scan.Candidate) Match {
	m := Match{Title: c.Title, SteamAppID: c.SteamAppID, GogID: c.GogID}
	store := c.Source.Store()

	switch {
	case store && c.SteamAppID > 0:
		m.Confidence, m.How = 100, "Steam library"
		return m
	case c.SteamAppID > 0:
		m.Confidence = 95
		m.How = "Steam AppID " + strconv.Itoa(c.SteamAppID) + " read from " + c.AppIDFrom
		if n := ix.SteamName(c.SteamAppID); n != "" {
			m.Title = n
		}
		return m
	case c.GogID != "" && ix != nil && ix.byGog[c.GogID] != nil:
		e := ix.byGog[c.GogID]
		m.Confidence, m.How = 95, "GOG id "+c.GogID
		if !store || m.Title == "" {
			m.Title = e.Name
		}
		m.SteamAppID = e.SteamID
		return m
	}

	if !store && scan.NotAGame(c.Title) && (c.Dir == "" || scan.NotAGame(filepath.Base(c.Dir))) {
		m.How, m.Confidence = "Not a game's name", 0
		return m
	}
	if store {
		// The store's title is authoritative; the manifest only adds a Steam id for art.
		m.Confidence, m.How = 100, c.How
		e := ix.byTitle(c.Title)
		for _, t := range []string{c.Title, scan.StripEdition(c.Title)} {
			if e == nil {
				e = ix.byLooseTitle(t)
			}
		}
		if e == nil {
			e = ix.byTitle(scan.StripEdition(c.Title))
		}
		if e != nil && e.SteamID > 0 {
			m.SteamAppID = e.SteamID
		}
		return m
	}
	if e := ix.byTitle(c.Title); e != nil {
		return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 85, How: "Matched by title"}
	}
	if stripped := scan.StripEdition(c.Title); stripped != c.Title {
		if e := ix.byTitle(stripped); e != nil {
			return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 75, How: "Matched by title (without edition)"}
		}
	}
	if ix != nil {
		if es := ix.byDir[scan.Normalize(filepath.Base(c.Dir))]; len(es) == 1 {
			e := es[0]
			return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 70, How: "Matched by install folder name"}
		}
	}
	// Spelled a little differently, as folder names often are ("Assassin
	// Creed" for "Assassin's Creed"), shortened ("AC Valhalla", "SkyrimSE",
	// "Mafia 2 DE"), without "Edition" ("Mass Effect Legendary"), or run
	// together with their edition ("Fallout3GameoftheYearEdition").
	names := titleForms(c.Title)
	if c.Dir != "" {
		names = append(names, titleForms(scan.CleanTitle(filepath.Base(c.Dir)))...)
	}
	for _, t := range names {
		if e := ix.byLooseTitle(t); e != nil {
			return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 72, How: "Matched by a similar title"}
		}
	}
	if ix != nil {
		for _, t := range names {
			if e := ix.byName[compactWithoutEdition(t)]; e != nil {
				return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 72, How: "Matched by a similar title"}
			}
		}
		// Looser matches are less sure: they wait for a check.
		for _, t := range names {
			if k := looserKey(t); len(k) >= 6 && ix.byLooser[k] != nil {
				e := ix.byLooser[k]
				return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 66, How: "Matched by a similar title"}
			}
		}
		for _, t := range names {
			if k := scan.LooseKey(t); len(k) >= 8 && ix.bySub[k] != nil {
				e := ix.bySub[k]
				return Match{Title: e.Name, SteamAppID: e.SteamID, GogID: e.GogID, Confidence: 62, How: "Matched by the title's subtitle"}
			}
		}
	}
	// Unknown to the manifest: keep the name, trusting installer records more than folder names.
	m.How = "Not matched to a known game"
	if c.TitleTrusted {
		m.Confidence = 60
	} else {
		m.Confidence = 40
	}
	return m
}

func (ix *Index) byTitle(title string) *Entry {
	if ix == nil {
		return nil
	}
	k := scan.Normalize(title)
	if len(k) < 2 {
		return nil
	}
	// "Baldurs Gate 3" and "Baldur's Gate 3" normalize the same.
	return ix.byName[k]
}

// byLooseTitle finds a game whose title has the same scan.LooseKey, when
// only one game does.
func (ix *Index) byLooseTitle(title string) *Entry {
	if ix == nil {
		return nil
	}
	k := scan.LooseKey(title)
	if len(k) < 4 {
		return nil
	}
	return ix.byLoose[k]
}

// titleForms are the ways to read a title when looking for a similar
// one: as it is, without its edition, written out, and with "Edition".
func titleForms(t string) []string {
	out := []string{t, scan.StripEdition(t), scan.ExpandAbbrev(t), t + " Edition"}
	return append(out, scan.Aliases(t)...)
}

// Editions as they appear in names run together ("Fallout3GameoftheYearEdition").
var compactEditions = func() []string {
	var out []string
	for _, e := range []string{"game of the year edition", "goty edition", "goty", "definitive edition", "complete edition",
		"enhanced edition", "deluxe edition", "ultimate edition", "gold edition", "remastered", "windows edition", "anniversary edition"} {
		out = append(out, scan.Normalize(e))
	}
	return out
}()

// compactWithoutEdition is a name's Normalize key without an edition at
// its end, for names written without spaces; "" when there's none.
func compactWithoutEdition(t string) string {
	if strings.ContainsAny(t, " ") {
		return "" // with spaces, StripEdition has had its turn
	}
	k := scan.Normalize(t)
	for _, e := range compactEditions {
		if rest, ok := strings.CutSuffix(k, e); ok && len(rest) >= 4 {
			return rest
		}
	}
	return ""
}

var stopWords = map[string]bool{"and": true, "the": true, "of": true, "a": true, "an": true}

// looserKey is a LooseKey without little words ("Indiana Jones Great
// Circle" and "Indiana Jones and the Great Circle" share one).
func looserKey(t string) string {
	var kept []string
	for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return r == ' ' || r == ':' || r == '-' || r == '–' || r == '_' || r == '.' }) {
		if !stopWords[w] {
			kept = append(kept, w)
		}
	}
	return scan.LooseKey(strings.Join(kept, " "))
}

// subtitleKey is the LooseKey of what follows a title's colon, when that
// is a name of its own (two words or more, not an edition): "Infinite
// Wealth" for "Like a Dragon: Infinite Wealth". "" otherwise.
func subtitleKey(t string) string {
	_, sub, ok := strings.Cut(t, ": ")
	if !ok || len(strings.Fields(sub)) < 2 || scan.StripEdition(sub) != sub {
		return ""
	}
	if k := scan.LooseKey(sub); len(k) >= 8 {
		return k
	}
	return ""
}
