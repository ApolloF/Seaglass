package app

import (
	"context"
	"path/filepath"
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
	// baseline is what each running session's game had unlocked when it started.
	baseline map[int64]achievements.List
}

func newAchState(c *Core, client *owned.Client, mc *meta.Client) *achState {
	a := &achState{
		c:     c,
		cache: &achievements.Cache{Dir: achCacheDir()},
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

// achCacheDir holds results per game id, so it goes with the library: a
// test harness's data (--dev-data) keeps its own, apart from the real one.
func achCacheDir() string {
	if platform.AppDirOverridden() {
		return filepath.Join(platform.AppDir(), "achievements-cache")
	}
	return platform.CacheDir("achievements")
}

// liveDeps is this PC's Steam, the user's key and account, and the stores.
func (a *achState) liveDeps() achievements.Deps {
	d := achievements.Deps{
		Env: achievements.DefaultEnv(), Lang: achievements.SteamLanguage(),
		SteamRoot: scan.SteamDir(), SteamKey: platform.LoadSecret(steamKeySecret),
		Net: a.net, Cache: a.cache, Icons: a.icons,
	}
	d.SteamAccounts = scan.SteamAccounts(d.SteamRoot)
	for _, g := range a.c.Lib.Games() {
		if g.Installed && achievements.UplayEmulator(g.Emulator) {
			d.UplayGames++
		}
	}
	d.EpicLocale = owned.EpicLocale(d.Lang)
	if db := owned.GalaxyDB(); db != "" {
		d.GalaxyDB = db
		d.GOGUnlocks = func(id string) (map[string]achievements.Unlock, error) { return owned.GOGUnlocks(db, id) }
	}
	if _, ok := loadGOG(); ok && a.c.owned != nil {
		d.GOG = a.c.owned.gogAccess
	}
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
		strconv.Itoa(achievements.Version), d.Lang, strconv.FormatBool(d.SteamKey != ""), strconv.FormatBool(d.Epic != nil), strconv.FormatBool(d.GOG != nil), g.Source, g.Emulator, g.EmuDir, g.Dir,
		strconv.Itoa(g.SteamAppID), strconv.Itoa(g.MetaAppID), g.EpicApp, g.GogID, strconv.Itoa(d.UplayGames))
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
		if cur, had := a.mem[id]; had {
			e = cur // a read that finished meanwhile is newer than the file
		} else {
			a.mem[id] = e
		}
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
	d := a.deps()
	stamp := achStamp(g, d)
	hit := func() (achievements.List, bool) {
		e, ok := a.last(id)
		if ok && !fresh && e.Stamp == stamp && (!e.Net || time.Since(time.Unix(e.List.UpdatedAt, 0)) < netResultAge) {
			return e.List, true
		}
		return achievements.List{}, false
	}
	// A cached result doesn't wait for another game's read.
	if l, ok := hit(); ok {
		return a.withUnlocks(g, l), nil
	}
	a.run.Lock()
	defer a.run.Unlock()
	if l, ok := hit(); ok { // read by another call meanwhile
		return a.withUnlocks(g, l), nil
	}
	d.Offline = a.active()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	l, usedNet := achievements.Resolve(ctx, g, d)
	e := achievements.Entry{Stamp: stamp, Net: usedNet, List: *l}
	if d.Offline || l.Partial {
		e.Stamp = "" // read again: once the game is closed and the stores can be asked, or to finish the icons
	}
	a.mu.Lock()
	a.mem[id] = e
	a.mu.Unlock()
	if err := a.cache.Save(id, e); err != nil {
		logx.Printf("achievements: saving %d: %v", id, err)
	}
	return a.withUnlocks(g, e.List), nil
}

// withUnlocks adds what the person playing unlocked on their other PCs.
// The cache keeps only what this PC's files and stores say.
func (a *achState) withUnlocks(g library.Game, l achievements.List) achievements.List {
	if a.c.profile == nil {
		return l
	}
	return a.c.profile.withUnlocks(g, l)
}

// clear forgets every result (the settings they depend on changed).
func (a *achState) clear() {
	a.mu.Lock()
	clear(a.mem)
	a.mu.Unlock()
	a.cache.Clear()
}

// sessionStarted keeps what the game had unlocked when a session began
// (once per session), to tell afterwards what the session added. The
// interface may read the achievements again while the game runs, so the
// last result at the end isn't "before" anymore. Without a result yet,
// one is read now, from local files only (the game is starting).
func (a *achState) sessionStarted(sessionID, gameID int64) {
	if sessionID <= 0 || gameID <= 0 || !a.c.Settings.Get().Achievements {
		return
	}
	a.mu.Lock()
	if a.baseline == nil {
		a.baseline = map[int64]achievements.List{}
	}
	if _, ok := a.baseline[sessionID]; ok {
		a.mu.Unlock()
		return
	}
	a.baseline[sessionID] = achievements.List{GameID: -1} // taken; filled in below
	a.mu.Unlock()
	go func() {
		e, ok := a.last(gameID)
		l := e.List
		if g, found := a.c.Lib.Get(gameID); ok && found {
			l = a.withUnlocks(g, l) // as get gives it after the session
		}
		if !ok {
			var err error
			if l, err = a.get(a.c.ctx, gameID, false); err != nil {
				return
			}
		}
		a.mu.Lock()
		if b, ok := a.baseline[sessionID]; ok && b.GameID == -1 {
			a.baseline[sessionID] = l
		}
		a.mu.Unlock()
	}()
}

// afterSession reads a game's achievements again once it has exited and
// tells the interface which ones the session unlocked. Emulators write
// their files as the game closes, so it waits a moment first.
func (a *achState) afterSession(sessionID, gameID int64, title string, wait time.Duration) {
	if !a.c.Settings.Get().Achievements {
		return
	}
	a.mu.Lock()
	before, had := a.baseline[sessionID]
	delete(a.baseline, sessionID)
	a.mu.Unlock()
	had = had && before.GameID == gameID
	select {
	case <-a.c.ctx.Done():
		return
	case <-time.After(wait):
	}
	after, err := a.get(a.c.ctx, gameID, true)
	if err != nil || !had {
		return // without a list from before, every unlock would look new
	}
	if fresh := newlyUnlocked(before, after); len(fresh) > 0 {
		logx.Printf("achievements: %q unlocked %d", title, len(fresh))
		if g, ok := a.c.Lib.Get(gameID); ok && a.c.profile != nil {
			a.c.profile.addAchievements(g, fresh)
		}
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

// forget drops a session's baseline (its game never ran).
func (a *achState) forget(sessionID int64) {
	a.mu.Lock()
	delete(a.baseline, sessionID)
	a.mu.Unlock()
}
