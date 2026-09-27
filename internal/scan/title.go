package scan

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Normalize keeps only letters and digits (any script), lower-cased, so
// "Game™: Edition" and "game edition" compare equal.
func Normalize(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(strings.ReplaceAll(s, "&", "and")) {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Roman numerals written as numbers, for LooseKey. A lone "i" is left
// alone: it's more often a word than a 1.
var romans = map[string]string{
	"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10",
	"xi": "11", "xii": "12", "xiii": "13", "xiv": "14", "xv": "15", "xvi": "16",
}

// Brand names stores put before some titles and folders often leave out:
// "Marvel's Spider-Man: Miles Morales", "Tom Clancy's The Division".
var reBrand = regexp.MustCompile(`(?i)^\s*(marvel|tom clancy|sid meier|clive barker|american mcgee)['’]?s\s+`)

// LooseKey is a more forgiving Normalize for names that don't match
// exactly, as folder names often don't: apostrophes, possessive and
// plural s, Roman numerals, a leading "The" and a leading brand ("Marvel's")
// don't count, so "Assassin Creed Black Flag Resynced" and "Assassin's
// Creed: Black Flag Resynced" get the same key, as do "Baldurs Gate III"
// and "Baldur's Gate 3".
func LooseKey(s string) string {
	if b := reBrand.FindStringIndex(s); b != nil && len(s)-b[1] >= 6 {
		s = s[b[1]:]
	}
	// One pass, as the game database's 50,000 titles all go through here.
	out := make([]byte, 0, len(s))
	var buf [64]byte
	word := buf[:0]
	words, leadThe := 0, false
	flush := func() {
		if len(word) == 0 {
			return
		}
		words++
		if n, ok := romans[string(word)]; ok {
			out = append(out, n...)
		} else if words == 1 && string(word) == "the" {
			leadThe = true
		} else if len(word) >= 4 && word[len(word)-1] == 's' {
			out = append(out, word[:len(word)-1]...)
		} else {
			out = append(out, word...)
		}
		word = word[:0]
	}
	var prev rune
	for _, c := range s {
		switch {
		case c == '\'' || c == '’' || c == '`':
			continue // "Assassin's" stays one word
		case c == '&':
			flush()
			word = append(word, "and"...)
			flush()
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			// Camel case and letter-digit runs are words too ("AssassinsCreed4").
			if unicode.IsLower(prev) && unicode.IsUpper(c) || unicode.IsLetter(prev) && unicode.IsDigit(c) || unicode.IsDigit(prev) && unicode.IsLetter(c) {
				flush()
			}
			word = utf8.AppendRune(word, unicode.ToLower(c))
		default:
			flush()
		}
		prev = c
	}
	flush()
	if leadThe && words == 1 {
		return "the"
	}
	return string(out)
}

var (
	// Bracketed release tags: "[FitGirl Repack]", "(v1.2.3)", "{GOG}".
	reBrackets = regexp.MustCompile(`\s*[\[\(\{][^\]\)\}]*[\]\)\}]`)
	// Trailing scene group or repacker tags: "-RUNE", "_CODEX", " - FitGirl Repack".
	reGroup = regexp.MustCompile(`(?i)[\s._-]+(rune|codex|empress|tenoke|plaza|skidrow|reloaded|cpy|flt|hoodlum|razor1911|dodi|fitgirl|elamigos|kaos|xatab|gog|repack|multi\d*|goldberg|onlinefix|online-fix|doge|p2p|tinyiso|darksiders|chronos|ali213|3dm|prophet|fckdrm|voksi|simplex|delusional|anomaly|rld)(\s*repacks?)?$`)
	// Version and build suffixes: "v1.0.3", "Build 12345", "Update 5",
	// "version 1.0.3179", "1.0.3179". Two plain numbers ("2.0") stay: they
	// are part of titles ("DiRT Rally 2.0") more often than versions.
	reVersion = regexp.MustCompile(`(?i)[\s._-]+(v\s?\d[\w.]*|version[\s._-]?\d[\w.]*|build[\s._-]?\d+|update[\s._-]?\d+|\d+\.\d+\.\d+(\.\d+)*)$`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

// CleanTitle turns an installer or folder name into a game title:
// "Baldurs.Gate.3-RUNE" → "Baldurs Gate 3", "Hades [FitGirl Repack]" → "Hades",
// "Elden.Ring.v1.10-FitGirl" → "Elden Ring".
func CleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = reBrackets.ReplaceAllString(s, "")
	// Scene-style names use dots or underscores instead of spaces. The
	// tags and version come off first, while "v1.10" still has its dot.
	scene := !strings.Contains(s, " ") && (strings.Count(s, ".") >= 2 || strings.Count(s, "_") >= 1)
	strip := func() {
		for i := 0; i < 3; i++ {
			before := s
			s = reGroup.ReplaceAllString(s, "")
			s = reVersion.ReplaceAllString(s, "")
			s = strings.TrimRight(s, " .-_")
			if s == before {
				break
			}
		}
	}
	strip()
	if scene {
		s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
		strip()
	}
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Edition and cut suffixes: "Deluxe Edition", "GOTY", "- Complete Edition".
var reEdition = regexp.MustCompile(`(?i)\s*[-:–]?\s*(digital\s+)?(deluxe|ultimate|complete|definitive|goty|game of the year|gold|premium|special|collector'?s|standard|enhanced|(\d+(st|nd|rd|th)\s+)?anniversary\s+edition|remastered|director'?s cut|(the\s+)?final cut|windows\s+edition|reloaded|\d+\s+year celebration)(\s+(edition|version|cut))?\s*$`)

// StripEdition drops an edition from a title: "The Witcher 3: Wild Hunt -
// Complete Edition" → "The Witcher 3: Wild Hunt".
func StripEdition(s string) string { return strings.TrimSpace(reEdition.ReplaceAllString(s, "")) }

var reRemake = regexp.MustCompile(`(?i)\s*[-:–(\[]?\s*remake[)\]]?\s*$`)

// WithoutRemake drops "Remake" from the end of a title: stores sell most
// remakes under the plain name ("Resident Evil 4 Remake" is "Resident Evil
// 4" on Steam, the original "Resident Evil 4 (2005)"). "" when there's
// none. It's a last try: some remakes do keep the word.
func WithoutRemake(s string) string {
	t := strings.TrimSpace(reRemake.ReplaceAllString(s, ""))
	if t == strings.TrimSpace(s) || len(Normalize(t)) < 3 {
		return ""
	}
	return t
}

// Abbreviations folders are often named by, followed by a space, a
// number or nothing ("GTA V", "RDR2"; not "Codename"), or Roman numerals
// straight after GTA ("GTAIV").
var reAbbrev = regexp.MustCompile(`(?i)^(?:(gta|rdr|nfs|cod)(\s+.*|\d.*|)|(gta)([ivx]+))$`)
var abbrevs = map[string]string{"gta": "Grand Theft Auto", "rdr": "Red Dead Redemption", "nfs": "Need for Speed", "cod": "Call of Duty"}

// ExpandAbbrev writes out a title that starts with a well-known
// abbreviation: "GTA V" → "Grand Theft Auto V", "RDR2" → "Red Dead
// Redemption 2". Other titles come back as they are.
func ExpandAbbrev(s string) string {
	m := reAbbrev.FindStringSubmatch(strings.TrimSpace(s))
	switch {
	case m == nil:
		return s
	case m[3] != "":
		return abbrevs[strings.ToLower(m[3])] + " " + m[4]
	}
	return strings.TrimSpace(abbrevs[strings.ToLower(m[1])] + " " + strings.TrimSpace(m[2]))
}

// SortTitle drops leading articles so "The Sims 4" sorts under S.
func SortTitle(s string) string {
	l := strings.ToLower(strings.TrimSpace(s))
	for _, a := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(l, a) && len(l) > len(a) {
			return l[len(a):]
		}
	}
	return l
}
