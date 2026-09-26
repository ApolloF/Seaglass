package library

import (
	"strings"
	"unicode/utf8"
)

// Limits on collections: a name, and how many one game is in.
const (
	maxCollectionName = 40
	maxCollections    = 30
)

// CleanCollections tidies collection names: spaces trimmed and
// collapsed, overlong names cut, empty ones and repeats (in any case)
// dropped, the first spelling kept.
func CleanCollections(names []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.Join(strings.Fields(n), " ")
		for utf8.RuneCountInString(n) > maxCollectionName {
			_, size := utf8.DecodeLastRuneInString(n)
			n = strings.TrimSpace(n[:len(n)-size])
		}
		k := strings.ToLower(n)
		if n == "" || seen[k] || len(out) >= maxCollections {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	return out
}

// RenameCollection renames a collection in every game that's in it (in
// any case); an empty name removes the collection. It returns the games
// that changed.
func (s *Store) RenameCollection(old, name string) []int64 {
	old = strings.ToLower(strings.Join(strings.Fields(old), " "))
	if old == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var changed []int64
	for _, g := range s.games {
		next := make([]string, 0, len(g.Collections))
		hit := false
		for _, c := range g.Collections {
			if strings.ToLower(c) == old {
				hit = true
				if name != "" {
					next = append(next, name)
				}
			} else {
				next = append(next, c)
			}
		}
		if hit {
			g.Collections = CleanCollections(next)
			changed = append(changed, g.ID)
		}
	}
	if len(changed) > 0 {
		s.scheduleSaveLocked()
	}
	return changed
}
