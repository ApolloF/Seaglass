package library

import (
	"strconv"
	"strings"
	"time"
)

// Owned is a game an account owns, from a store (Steam, GOG, Epic).
type Owned struct {
	Store      string // steam, gog, epic
	ID         string // the store's id: Steam app id, GOG product id, Epic app name
	Title      string
	SortTitle  string
	InstallURI string // asks the store to install it
	Playtime   int64  // seconds the store recorded
	LastPlayed int64  // unix seconds
}

// ownedPrefix starts the key of a game that is only known from an account.
const ownedPrefix = "owned:"

// OwnedKey is the library key of an owned-only game.
func OwnedKey(store, id string) string { return ownedPrefix + store + ":" + strings.ToLower(id) }

// IsOwnedOnly reports whether a game is only known from an account (it was
// never found on this PC).
func (g *Game) IsOwnedOnly() bool { return strings.HasPrefix(g.Key, ownedPrefix) }

// storeID is the game's id at a store, "" when it has none there.
func (g *Game) storeID(store string) string {
	switch store {
	case "steam":
		if g.SteamAppID > 0 {
			return strconv.Itoa(g.SteamAppID)
		}
	case "gog":
		return g.GogID
	case "epic":
		return strings.ToLower(g.EpicApp)
	}
	return ""
}

// ApplyOwned merges one store's owned games. A game already in the library
// (found on this PC, installed or not anymore) is marked owned and gets the
// install link; other games are added as owned-only, not installed.
// Owned-only games the account no longer lists are removed.
func (s *Store) ApplyOwned(store string, list []Owned, now time.Time) (added, removed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := now.Unix()
	// Games found on this PC, by their id at this store.
	local := map[string]*Game{}
	for _, g := range s.games {
		if id := g.storeID(store); id != "" && !g.IsOwnedOnly() {
			local[strings.ToLower(id)] = g
		}
	}
	keep := map[string]bool{}
	for _, o := range list {
		id := strings.ToLower(strings.TrimSpace(o.ID))
		if id == "" || strings.TrimSpace(o.Title) == "" {
			continue
		}
		if g := local[id]; g != nil {
			g.Owned, g.InstallURI = true, o.InstallURI
			g.StorePlaytime = max(g.StorePlaytime, o.Playtime)
			g.StoreLastPlayed = max(g.StoreLastPlayed, o.LastPlayed)
			continue
		}
		key := OwnedKey(store, id)
		keep[key] = true
		g := s.byKey[key]
		if g == nil {
			g = &Game{ID: s.next, Key: key, AddedAt: ts, Initial: true}
			s.next++
			s.games[g.ID] = g
			s.byKey[key] = g
			added++
			switch store {
			case "steam":
				g.SteamAppID, _ = strconv.Atoi(id)
			case "gog":
				g.GogID = id
			case "epic":
				g.EpicApp = o.ID
			}
		}
		g.Title, g.SortTitle = o.Title, o.SortTitle
		if g.SortTitle == "" {
			g.SortTitle = strings.ToLower(o.Title)
		}
		if g.CustomTitle != "" {
			g.SortTitle = strings.ToLower(g.CustomTitle)
		}
		g.Source, g.SourceLabel = store, storeLabel(store)
		g.Installed, g.Owned, g.InstallURI = false, true, o.InstallURI
		g.How, g.MatchHow, g.Confidence = "Owned on "+storeLabel(store), "Owned on "+storeLabel(store), 100
		g.StorePlaytime, g.StoreLastPlayed, g.SeenAt = o.Playtime, o.LastPlayed, ts
	}
	for k, g := range s.byKey {
		if strings.HasPrefix(k, ownedPrefix+store+":") && !keep[k] {
			s.dropLocked(g)
			removed++
		}
	}
	s.scheduleSaveLocked()
	return added, removed
}

// ForgetOwned removes a store's owned-only games and the owned mark from
// the others (the account was disconnected).
func (s *Store) ForgetOwned(store string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for k, g := range s.byKey {
		if strings.HasPrefix(k, ownedPrefix+store+":") {
			s.dropLocked(g)
			n++
		} else if g.Source == store && g.Owned {
			g.Owned, g.InstallURI = false, ""
		}
	}
	s.scheduleSaveLocked()
	return n
}

// mergeOwnedLocked folds owned-only games into games a scan found with
// the same store id: the found game keeps the user's choices (favorite,
// hidden, title, playtime) and the owned mark.
func (s *Store) mergeOwnedLocked() {
	// Games found on this PC by store and id, built once: a big owned
	// library against every found game would otherwise be quadratic.
	var local map[string]*Game
	for _, o := range s.games {
		if !o.IsOwnedOnly() {
			continue
		}
		store := o.Source
		id := o.storeID(store)
		if id == "" {
			continue
		}
		if local == nil {
			local = map[string]*Game{}
			for _, g := range s.games {
				if g.IsOwnedOnly() {
					continue
				}
				for _, st := range []string{"steam", "gog", "epic"} {
					if gid := g.storeID(st); gid != "" {
						if k := st + ":" + gid; local[k] == nil {
							local[k] = g
						}
					}
				}
			}
		}
		g := local[store+":"+id]
		if g == nil {
			continue
		}
		g.Owned, g.InstallURI = true, o.InstallURI
		g.Favorite = g.Favorite || o.Favorite
		g.Collections = CleanCollections(append(g.Collections, o.Collections...))
		g.Hidden = g.Hidden && o.Hidden
		if g.CustomTitle == "" {
			g.CustomTitle = o.CustomTitle
		}
		g.Playtime += o.Playtime
		g.LastPlayed = max(g.LastPlayed, o.LastPlayed)
		g.StorePlaytime = max(g.StorePlaytime, o.StorePlaytime)
		g.StoreLastPlayed = max(g.StoreLastPlayed, o.StoreLastPlayed)
		if g.Meta == nil {
			g.Meta = o.Meta
		}
		s.dropLocked(o)
	}
}

func (s *Store) dropLocked(g *Game) {
	delete(s.games, g.ID)
	delete(s.byKey, g.Key)
}

func storeLabel(store string) string {
	switch store {
	case "steam":
		return "Steam"
	case "gog":
		return "GOG"
	case "epic":
		return "Epic"
	}
	return store
}
