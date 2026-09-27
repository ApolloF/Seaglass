package app

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/owned"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventAchievementsSession tells the interface which achievements a play
// session unlocked, once the game has exited.
const EventAchievementsSession = "achievements:session"

// SessionAchievements are the achievements one session unlocked.
type SessionAchievements struct {
	GameID   int64                      `json:"gameId"`
	Title    string                     `json:"title"`
	Unlocked []achievements.Achievement `json:"unlocked"`
}

func init() {
	application.RegisterEvent[SessionAchievements](EventAchievementsSession)
}

// netResultAge is how long a result that needed a store's servers is
// reused while its files haven't changed; local-only results are reused
// until their files change.
const netResultAge = 6 * time.Hour

// achState reads and caches achievements. Reads run one at a time: they're
// cheap locally, and one game's network calls shouldn't race another's.
type achState struct {
	c     *Core
	cache *achievements.Cache
	icons *achievements.Icons
	net   achievements.Net
	// deps and active are replaced by tests.
	deps   func() achievements.Deps
	active func() bool // a game runs: stay offline

	run sync.Mutex // one resolve at a time
	mu  sync.Mutex
	mem map[int64]achievements.Entry
}

func newAchState(c *Core, client *owned.Client, mc *meta.Client) *achState {
	a := &achState{
		c:     c,
		cache: &achievements.Cache{Dir: platform.CacheDir("achievements")},
		net:   client,
		mem:   map[int64]achievements.Entry{},
	}
	a.icons = &achievements.Icons{
		Dir: platform.CacheDir("achievements", "icons"),
		Fetch: func(ctx context.Context, src, dir string) (string, error) {
			return mc.FetchImage(ctx, src, dir, meta.Icon)
		},
		Store: func(dir string, b []byte) (string, error) { return meta.StoreImage(dir, b, meta.Icon) },
	}
	a.deps = a.liveDeps
	a.active = func() bool { return c.Launch != nil && c.Launch.Active() }
	return a
}

// liveDeps is this PC's Steam, the user's key and account, and the stores.
func (a *achState) liveDeps() achievements.Deps {
	d := achievements.Deps{
		Env: achievements.DefaultEnv(), Lang: achievements.SteamLanguage(),
		SteamRoot: scan.SteamDir(), SteamKey: platform.LoadSecret(steamKeySecret),
		Net: a.net, Cache: a.cache, Icons: a.icons,
	}
	d.SteamAccounts = scan.SteamAccounts(d.SteamRoot)
	d.EpicLocale = owned.EpicLocale(d.Lang)
	if _, ok := loadEpic(); ok && a.c.owned != nil {
		d.Epic = a.c.owned.epicAccess
	}
	if d.SteamKey != "" {
		if id, err := owned.SteamID(); err == nil {
			d.SteamID = id
		}
	}
	return d
}

// stamp sums up what a game's result depends on.
func achStamp(g library.Game, d achievements.Deps) string {
	return achievements.Stamp(achievements.Files(g, d),
		"v1", d.Lang, strconv.FormatBool(d.SteamKey != ""), strconv.FormatBool(d.Epic != nil), g.Source, g.Emulator, g.EmuDir, g.Dir,
		strconv.Itoa(g.SteamAppID), strconv.Itoa(g.MetaAppID), g.EpicApp, g.GogID)
}

// last is the game's last result, from memory or disk.
func (a *achState) last(id int64) (achievements.Entry, bool) {
	a.mu.Lock()
	e, ok := a.mem[id]
	a.mu.Unlock()
	if ok {
		return e, true
	}
	if e, ok = a.cache.Load(id); ok {
		a.mu.Lock()
		a.mem[id] = e
		a.mu.Unlock()
	}
	return e, ok
}

// get returns a game's achievements: the cached result while what it was
// made from hasn't changed, else read again. fresh always reads again.
func (a *achState) get(ctx context.Context, id int64, fresh bool) (achievements.List, error) {
	g, ok := a.c.Lib.Get(id)
	if !ok {
		return achievements.List{}, library.ErrNotFound
	}
	if !a.c.Settings.Get().Achievements {
		return achievements.List{GameID: id, Items: []achievements.Achievement{}}, nil
	}
	a.run.Lock()
	defer a.run.Unlock()
	d := a.deps()
	stamp := achStamp(g, d)
	if e, ok := a.last(id); ok && !fresh && e.Stamp == stamp && (!e.Net || time.Since(time.Unix(e.List.UpdatedAt, 0)) < netResultAge) {
		return e.List, nil
	}
	d.Offline = a.active()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	l, usedNet := achievements.Resolve(ctx, g, d)
	e := achievements.Entry{Stamp: stamp, Net: usedNet, List: *l}
	if d.Offline {
		e.Stamp = "" // read again once the game is closed and the stores can be asked
	}
	a.mu.Lock()
	a.mem[id] = e
	a.mu.Unlock()
	if err := a.cache.Save(id, e); err != nil {
		logx.Printf("achievements: saving %d: %v", id, err)
	}
	return e.List, nil
}

// clear forgets every result (the settings they depend on changed).
func (a *achState) clear() {
	a.mu.Lock()
	clear(a.mem)
	a.mu.Unlock()
	a.cache.Clear()
}

// afterSession reads a game's achievements again once it has exited and
// tells the interface which ones the session unlocked. Emulators write
// their files as the game closes, so it waits a moment first.
func (a *achState) afterSession(gameID int64, title string, wait time.Duration) {
	if !a.c.Settings.Get().Achievements {
		return
	}
	before, had := a.last(gameID)
	select {
	case <-a.c.ctx.Done():
		return
	case <-time.After(wait):
	}
	after, err := a.get(a.c.ctx, gameID, true)
	if err != nil || !had {
		return // without a list from before, every unlock would look new
	}
	if fresh := newlyUnlocked(before.List, after); len(fresh) > 0 {
		logx.Printf("achievements: %q unlocked %d", title, len(fresh))
		a.c.emit(EventAchievementsSession, SessionAchievements{GameID: gameID, Title: title, Unlocked: fresh})
	}
}

// newlyUnlocked lists what after has unlocked that before didn't, newest first.
func newlyUnlocked(before, after achievements.List) []achievements.Achievement {
	was := map[string]bool{}
	for _, it := range before.Items {
		if it.Unlocked {
			was[it.ID] = true
		}
	}
	var out []achievements.Achievement
	for _, it := range after.Items {
		if it.Unlocked && !was[it.ID] {
			out = append(out, it)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].UnlockedAt > out[j-1].UnlockedAt; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ---- the service ----

// AchievementsService is a game's achievements, for the interface.
type AchievementsService struct{ c *Core }

// NewAchievementsService binds achievements to core.
func NewAchievementsService(c *Core) *AchievementsService { return &AchievementsService{c} }

// Get returns a game's achievements. Results are reused while the files
// they came from haven't changed; fresh reads them again.
func (s *AchievementsService) Get(id int64, fresh bool) (achievements.List, error) {
	return s.c.ach.get(s.c.ctx, id, fresh)
}
