package app

import (
	"context"
	"errors"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventMetaState reports metadata fetching progress to the interface.
const EventMetaState = "meta:state"

// MetaState is what the metadata worker is doing.
type MetaState struct {
	Running bool `json:"running"`
	Done    int  `json:"done"`
	Total   int  `json:"total"`
}

func init() { application.RegisterEvent[MetaState](EventMetaState) }

const (
	sgdbSecret   = "steamgriddb"
	metaMaxAge   = 30 * 24 * time.Hour
	metaRetryGap = 24 * time.Hour
)

// metaWorker fetches metadata and art in the background, one game at a
// time, most recently played first.
type metaWorker struct {
	c      *Core
	client *meta.Client

	mu      sync.Mutex
	queue   []int64
	queued  map[int64]bool
	failed  map[int64]time.Time
	state   MetaState
	wake    chan struct{}
	changed *time.Timer
	dirty   map[int64]bool // games with new metadata the interface hasn't heard about
}

func newMetaWorker(c *Core) *metaWorker {
	w := &metaWorker{c: c, queued: map[int64]bool{}, failed: map[int64]time.Time{}, wake: make(chan struct{}, 1)}
	w.client = meta.NewClient(platform.CacheDir("art"), func() string { return platform.LoadSecret(sgdbSecret) })
	return w
}

// queueMissing queues installed games without (fresh) metadata, and
// owned games that aren't installed when those are shown.
func (w *metaWorker) queueMissing() {
	hasKey := platform.LoadSecret(sgdbSecret) != ""
	showOwned := w.c.Settings.Get().ShowOwned
	games := w.c.Lib.Games()
	sort.SliceStable(games, func(i, j int) bool {
		if games[i].Installed != games[j].Installed {
			return games[i].Installed // installed games first
		}
		a := max(games[i].LastPlayed, games[i].StoreLastPlayed)
		b := max(games[j].LastPlayed, games[j].StoreLastPlayed)
		return a > b
	})
	now := time.Now()
	w.mu.Lock()
	for _, g := range games {
		if !(g.Installed || showOwned && g.Owned) || g.Hidden || w.queued[g.ID] {
			continue
		}
		if t, ok := w.failed[g.ID]; ok && now.Sub(t) < metaRetryGap {
			continue
		}
		fresh := g.Meta != nil && g.Meta.Version >= meta.Version && now.Sub(time.Unix(g.Meta.FetchedAt, 0)) < metaMaxAge
		if fresh && (g.Meta.Cover != "" || !hasKey) {
			continue
		}
		w.queue = append(w.queue, g.ID)
		w.queued[g.ID] = true
		w.state.Total++
	}
	w.mu.Unlock()
	w.poke()
}

// queueNow puts one game at the front of the queue, even if it has metadata.
func (w *metaWorker) queueNow(id int64) {
	w.mu.Lock()
	delete(w.failed, id)
	if !w.queued[id] {
		w.queue = append([]int64{id}, w.queue...)
		w.queued[id] = true
		w.state.Total++
	}
	w.mu.Unlock()
	w.poke()
}

func (w *metaWorker) poke() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *metaWorker) run(ctx context.Context) {
	for {
		id, ok := w.next()
		if !ok {
			if w.State().Running {
				// A batch is done: decoding art grows the heap; give it back.
				debug.FreeOSMemory()
			}
			w.setState(func(s *MetaState) { *s = MetaState{} })
			select {
			case <-ctx.Done():
				return
			case <-w.wake:
				continue
			}
		}
		if !w.c.waitIdle(ctx) {
			return
		}
		w.setState(func(s *MetaState) { s.Running = true })
		err := w.fetch(ctx, id)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, meta.ErrRateLimited) {
			logx.Printf("metadata: rate limited, pausing a minute")
			w.mu.Lock()
			if !w.queued[id] { // queued again while it was fetched
				w.queue = append([]int64{id}, w.queue...)
				w.queued[id] = true
			}
			w.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}
		w.setState(func(s *MetaState) { s.Done++ })
	}
}

func (w *metaWorker) next() (int64, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) == 0 {
		return 0, false
	}
	id := w.queue[0]
	w.queue = w.queue[1:]
	delete(w.queued, id)
	return id, true
}

func (w *metaWorker) fetch(ctx context.Context, id int64) error {
	g, ok := w.c.Lib.Get(id)
	if !ok {
		return nil
	}
	appID := g.SteamAppID
	if appID == 0 {
		appID = g.MetaAppID
	}
	// Unknown to the game database: the Steam store usually still knows the
	// game by name, for art and details. A clear hit also settles which game
	// it is (unless the user already said), so it doesn't wait for a check.
	if g.SteamAppID == 0 && g.GogID == "" && (appID == 0 || !g.Confirmed && g.Confidence < storeMatchConfidence) && looksLikeGameName(g.DisplayTitle()) {
		h, ok, err := w.searchStore(ctx, g.DisplayTitle())
		if errors.Is(err, meta.ErrRateLimited) {
			return err
		}
		if ok {
			appID = h.AppID
			if _, err := w.c.Lib.Update(id, func(g *library.Game) { adoptStoreMatch(g, h) }); err == nil {
				w.gameChanged(id)
			}
		}
	}
	// Still nothing: PCGamingWiki knows nearly every PC game by name,
	// including those only Epic sells, and often its Steam app.
	var wiki *meta.PCGW
	gogID := g.GogID
	if appID == 0 && gogID == "" && looksLikeGameName(g.DisplayTitle()) {
		p, err := w.client.PCGamingWiki(ctx, g.DisplayTitle())
		if errors.Is(err, meta.ErrRateLimited) {
			return err
		}
		if err == nil {
			wiki, appID, gogID = p, p.SteamAppID, p.GogID
			if _, err := w.c.Lib.Update(id, func(g *library.Game) { adoptWikiMatch(g, p) }); err == nil {
				w.gameChanged(id)
			}
		}
	}
	m, err := w.client.Fetch(ctx, meta.Request{Title: g.DisplayTitle(), SteamAppID: appID, GogID: gogID, EpicApp: g.EpicApp, Keep: g.Meta, PCGW: wiki})
	if err != nil {
		if !errors.Is(err, meta.ErrRateLimited) {
			w.mu.Lock()
			w.failed[id] = time.Now()
			w.mu.Unlock()
			logx.Printf("metadata for %q: %v", g.DisplayTitle(), err)
		}
		return err
	}
	var stale, artChanged bool
	_, err = w.c.Lib.Update(id, func(x *library.Game) {
		// The game may have changed while this ran: another game picked
		// (SetMatch, a scan) or art chosen. Keep what it is now.
		if stale = metaStale(g, *x); stale {
			return
		}
		if artChanged = !sameArt(g.Meta, x.Meta); artChanged {
			return
		}
		x.Meta = m
	})
	if err == nil && artChanged {
		w.queueNow(id) // again, keeping the art chosen meanwhile
	}
	if err == nil && !stale && !artChanged {
		w.gameChanged(id)
	}
	return err
}

// metaStale says whether a fetch for game g no longer fits the game as it
// is now, because it's become a different game. Whatever changed it queues
// a new fetch.
func metaStale(g, now library.Game) bool {
	return now.SteamAppID != g.SteamAppID || now.GogID != g.GogID || g.Meta != nil && now.Meta == nil
}

// sameArt says whether the art the user may have chosen is unchanged.
func sameArt(a, b *library.Meta) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Cover == b.Cover && a.Hero == b.Hero && a.Backdrop == b.Backdrop && a.Logo == b.Logo && slices.Equal(a.ArtOverrides, b.ArtOverrides)
}

// searchStore looks a title up on the Steam store: as it is, then without
// its edition ("The Witcher 3: Wild Hunt - Complete Edition" finds
// nothing, "The Witcher 3: Wild Hunt" does) and with a well-known
// abbreviation written out ("GTA V"), and last without "Remake".
func (w *metaWorker) searchStore(ctx context.Context, title string) (meta.StoreHit, bool, error) {
	tried := map[string]bool{}
	for _, t := range append(append([]string{title, scan.StripEdition(title), scan.ExpandAbbrev(title)}, scan.Aliases(title)...), scan.WithoutRemake(title)) {
		if t == "" || tried[scan.Normalize(t)] {
			continue
		}
		tried[scan.Normalize(t)] = true
		hits, err := w.client.SearchSteam(ctx, t)
		if err != nil {
			return meta.StoreHit{}, false, err
		}
		if h, ok := pickStoreHit(t, hits); ok {
			if base, ok := w.client.BaseGame(ctx, h.AppID); ok {
				h = base
			}
			return h, true, nil
		}
	}
	return meta.StoreHit{}, false, nil
}

// How a game the game database doesn't know was identified by a Steam
// store search, and how sure that is: enough to skip the check.
const (
	storeMatchHow        = "Matched on the Steam store"
	storeMatchConfidence = 80
)

// pickStoreHit finds the store search result that is this game: one with
// the same name, or else the only one with a similar name (scan.LooseKey:
// "Assassin Creed …" for "Assassin's Creed …"). DLC and editions with
// longer names don't count.
func pickStoreHit(title string, hits []meta.StoreHit) (meta.StoreHit, bool) {
	want := scan.Normalize(title)
	for _, h := range hits {
		if scan.Normalize(h.Name) == want {
			return h, true
		}
	}
	loose := scan.LooseKey(title)
	if len(loose) < 4 {
		return meta.StoreHit{}, false
	}
	// A similar name, then the store's name without its edition ("Tomb
	// Raider Game of the Year" for "Tomb Raider"), each only when one
	// game has it.
	for _, key := range []func(string) string{scan.LooseKey, func(s string) string { return scan.LooseKey(scan.StripEdition(s)) }} {
		var found *meta.StoreHit
		for i := range hits {
			if key(hits[i].Name) != loose {
				continue
			}
			if found != nil && found.AppID != hits[i].AppID {
				return meta.StoreHit{}, false // two games: don't guess
			}
			found = &hits[i]
		}
		if found != nil {
			return *found, true
		}
	}
	return meta.StoreHit{}, false
}

var storeMarks = strings.NewReplacer("™", "", "®", "", "©", "")

// adoptStoreMatch records a store search hit: its app for metadata and,
// unless the user confirmed the game or the game database already knew
// it, its name and identity.
func adoptStoreMatch(g *library.Game, h meta.StoreHit) {
	g.MetaAppID = h.AppID
	if g.Confirmed || g.Confidence >= storeMatchConfidence {
		return
	}
	if name := strings.Join(strings.Fields(storeMarks.Replace(h.Name)), " "); name != "" {
		g.Title, g.SortTitle = name, scan.SortTitle(name)
	}
	if g.CustomTitle != "" {
		g.SortTitle = scan.SortTitle(g.CustomTitle)
	}
	g.MatchHow, g.Confidence, g.NeedsReview = storeMatchHow, storeMatchConfidence, false
}

// wikiMatchHow says a game was identified by its PCGamingWiki page (the
// title, or one of the wiki's redirects to it, is the folder's name).
const wikiMatchHow = "Matched on PCGamingWiki"

// adoptWikiMatch takes the wiki page's title (and Steam app, for the
// metadata) as it does a store hit's.
func adoptWikiMatch(g *library.Game, p *meta.PCGW) {
	if p.SteamAppID > 0 {
		g.MetaAppID = p.SteamAppID
	}
	if g.Confirmed || g.Confidence >= storeMatchConfidence {
		return
	}
	if name := strings.Join(strings.Fields(storeMarks.Replace(p.Title)), " "); name != "" {
		g.Title, g.SortTitle = name, scan.SortTitle(name)
	}
	if g.CustomTitle != "" {
		g.SortTitle = scan.SortTitle(g.CustomTitle)
	}
	g.MatchHow, g.Confidence, g.NeedsReview = wikiMatchHow, storeMatchConfidence, false
}

// gameChanged tells the interface about new metadata, a few games at a
// time at most a few times a second.
func (w *metaWorker) gameChanged(id int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dirty == nil {
		w.dirty = map[int64]bool{}
	}
	w.dirty[id] = true
	if w.changed != nil {
		return
	}
	w.changed = time.AfterFunc(600*time.Millisecond, func() {
		w.mu.Lock()
		ids := make([]int64, 0, len(w.dirty))
		for id := range w.dirty {
			ids = append(ids, id)
		}
		w.dirty, w.changed = nil, nil
		w.mu.Unlock()
		w.c.gamesChanged(ids...)
	})
}

func (w *metaWorker) setState(fn func(*MetaState)) {
	w.mu.Lock()
	fn(&w.state)
	if !w.state.Running && len(w.queue) == 0 {
		w.state = MetaState{}
	}
	s := w.state
	w.mu.Unlock()
	w.c.emit(EventMetaState, s)
}

func (w *metaWorker) State() MetaState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// looksLikeGameName reports whether a title is worth looking up at all
// (a store search for "Tools" or "Discord" finds something, and it isn't
// this).
func looksLikeGameName(title string) bool { return !scan.NotAGame(title) }
