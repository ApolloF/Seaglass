// Package achievements reads a game's achievements: what it has (the
// schema: names, descriptions, icons) and which the player unlocked. Store
// installs are read from the store's own files or API; unofficial copies
// from the unlock files their Steam emulator writes. Every file is treated
// as untrusted: reads are capped and parsers never panic.
package achievements

import (
	"sort"
	"strings"
)

// Def is one achievement in a game's schema.
type Def struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Desc     string `json:"desc,omitempty"`
	Icon     string `json:"icon,omitempty"`     // https URL or a local file; the interface gets /ach/ URLs
	IconGray string `json:"iconGray,omitempty"` // the locked icon
	Hidden   bool   `json:"hidden,omitempty"`
}

// Unlock is what an unlock file says about one achievement.
type Unlock struct {
	Achieved bool
	At       int64 // unix seconds; 0 when unknown
	Progress float64
	Max      float64
}

// Achievement is one achievement as the interface shows it.
type Achievement struct {
	Def
	Unlocked   bool     `json:"unlocked"`
	UnlockedAt int64    `json:"unlockedAt,omitempty"` // unix seconds; 0 when unlocked at an unknown time
	Progress   float64  `json:"progress,omitempty"`
	Max        float64  `json:"max,omitempty"`
	Percent    *float64 `json:"percent,omitempty"` // share of all players who have it (Steam's global stats)
}

// List is a game's achievements.
type List struct {
	GameID    int64         `json:"gameId"`
	Source    string        `json:"source"` // "steam", "epic", "gog", "Goldberg", "CODEX", …; "" when none was found
	Total     int           `json:"total"`
	Unlocked  int           `json:"unlocked"`
	Items     []Achievement `json:"items"`
	UpdatedAt int64         `json:"updatedAt"`
	Hint      string        `json:"hint,omitempty"` // what's missing, and how to get it
	// Partial: time ran out before every icon was stored; read it again later.
	Partial bool `json:"-"`
}

// Merge combines a schema with unlocks. Unlock IDs match schema IDs
// ignoring case (CODEX writes them in a different case than Steam). Unlocks
// the schema doesn't know are listed by their ID; so is every unlock when
// there's no schema.
func Merge(defs []Def, unlocks map[string]Unlock) []Achievement {
	byID := make(map[string]Unlock, len(unlocks))
	for id, u := range unlocks {
		k := strings.ToLower(id)
		if old, ok := byID[k]; ok && old.Achieved && !u.Achieved {
			continue
		}
		byID[k] = u
	}
	out := make([]Achievement, 0, max(len(defs), len(unlocks)))
	seen := map[string]bool{}
	for _, d := range defs {
		k := strings.ToLower(d.ID)
		if d.ID == "" || seen[k] {
			continue
		}
		seen[k] = true
		a := Achievement{Def: d}
		if a.Name == "" {
			a.Name = d.ID
		}
		if u, ok := byID[k]; ok {
			a.apply(u)
		}
		out = append(out, a)
	}
	var extra []string
	for id := range unlocks {
		if k := strings.ToLower(id); !seen[k] && id != "" {
			seen[k] = true
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		a := Achievement{Def: Def{ID: id, Name: id}}
		a.apply(byID[strings.ToLower(id)])
		out = append(out, a)
	}
	return out
}

func (a *Achievement) apply(u Unlock) {
	a.Unlocked = u.Achieved
	if u.Achieved {
		a.UnlockedAt = u.At
	}
	if u.Max > 0 {
		a.Progress, a.Max = u.Progress, u.Max
	}
}

// Count fills in Total and Unlocked from Items.
func (l *List) Count() {
	l.Total, l.Unlocked = len(l.Items), 0
	for _, a := range l.Items {
		if a.Unlocked {
			l.Unlocked++
		}
	}
}

// SetRarity puts global unlock percentages (by achievement ID, any case)
// on the list's items.
func (l *List) SetRarity(pct map[string]float64) {
	if len(pct) == 0 {
		return
	}
	lower := make(map[string]float64, len(pct))
	for k, v := range pct {
		lower[strings.ToLower(k)] = v
	}
	for i := range l.Items {
		if v, ok := lower[strings.ToLower(l.Items[i].ID)]; ok {
			v := v
			l.Items[i].Percent = &v
		}
	}
}
