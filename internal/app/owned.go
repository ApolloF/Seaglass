package app

import (
	"context"
	"encoding/json"
	"errors"
	"runtime/debug"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/owned"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventAccounts tells the interface the store accounts changed.
const EventAccounts = "accounts:changed"

func init() { application.RegisterEvent[Accounts](EventAccounts) }

const (
	steamKeySecret = "steam-webapi"
	epicSecret     = "epic-account"
	ownedEvery     = 12 * time.Hour
)

// StoreAccount is one store account as Settings shows it.
type StoreAccount struct {
	Connected bool   `json:"connected"`
	Available bool   `json:"available"` // the store is on this PC (GOG: Galaxy's library)
	Name      string `json:"name,omitempty"`
	Games     int    `json:"games"`
	Synced    int64  `json:"synced,omitempty"` // unix seconds
	Syncing   bool   `json:"syncing"`
	Error     string `json:"error,omitempty"`
}

// Accounts are the store accounts owned games come from.
type Accounts struct {
	Steam StoreAccount `json:"steam"`
	GOG   StoreAccount `json:"gog"`
	Epic  StoreAccount `json:"epic"`
	// GOGSignIn is the GOG account signed in for achievements (GOG
	// Galaxy's library, above, needs no sign-in).
	GOGSignIn StoreAccount `json:"gogSignIn"`
}

type epicAccount struct {
	Refresh string `json:"refresh"`
	Account string `json:"account"`
	Name    string `json:"name"`
}

// ownedState syncs owned games from the connected accounts.
type ownedState struct {
	c      *Core
	client *owned.Client

	mu     sync.Mutex
	status map[string]StoreAccount // steam, gog, epic: last sync outcome
	busy   bool

	epicMu  sync.Mutex // one Epic token refresh at a time
	epicTok owned.EpicToken
	gogMu   sync.Mutex // one GOG token refresh at a time
	gogTok  owned.GOGToken
	gogExp  time.Time
}

func newOwnedState(c *Core) *ownedState {
	return &ownedState{c: c, client: owned.NewClient(), status: map[string]StoreAccount{}}
}

func loadEpic() (epicAccount, bool) {
	var a epicAccount
	s := platform.LoadSecret(epicSecret)
	if s == "" || json.Unmarshal([]byte(s), &a) != nil || a.Refresh == "" {
		return a, false
	}
	return a, true
}

func saveEpic(a epicAccount) error {
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return platform.SaveSecret(epicSecret, string(b))
}

// accounts reports the accounts and their last sync.
func (o *ownedState) accounts() Accounts {
	o.mu.Lock()
	defer o.mu.Unlock()
	a := Accounts{Steam: o.status["steam"], GOG: o.status["gog"], Epic: o.status["epic"]}
	a.Steam.Connected = platform.LoadSecret(steamKeySecret) != ""
	a.Steam.Available = true
	a.GOG.Available = owned.GalaxyDB() != ""
	a.GOG.Connected = o.c.Settings.Get().OwnedGOG && a.GOG.Available
	ep, ok := loadEpic()
	a.GOGSignIn.Available = true
	if _, ok := loadGOG(); ok {
		a.GOGSignIn.Connected = true
	}
	a.Epic.Connected, a.Epic.Available = ok, true
	if ok {
		a.Epic.Name = ep.Name
	}
	for _, s := range []*StoreAccount{&a.Steam, &a.GOG, &a.Epic} {
		s.Syncing = o.busy && s.Connected
	}
	return a
}

func (o *ownedState) setStatus(store string, fn func(*StoreAccount)) {
	o.mu.Lock()
	s := o.status[store]
	fn(&s)
	o.status[store] = s
	o.mu.Unlock()
}

// sync fetches every connected account's games and merges them.
func (o *ownedState) sync(ctx context.Context) {
	o.mu.Lock()
	if o.busy {
		o.mu.Unlock()
		return
	}
	o.busy = true
	o.mu.Unlock()
	o.c.emit(EventAccounts, o.accounts())
	defer func() {
		o.mu.Lock()
		o.busy = false
		o.mu.Unlock()
		o.c.emit(EventAccounts, o.accounts())
		o.c.emit(EventLibraryChanged, "owned")
		o.c.meta.queueMissing()
		// GOG Galaxy's database is read whole; don't keep the heap it grew.
		debug.FreeOSMemory()
	}()
	if key := platform.LoadSecret(steamKeySecret); key != "" {
		o.run(ctx, "steam", func(ctx context.Context) ([]library.Owned, error) {
			id, err := owned.SteamID()
			if err != nil {
				return nil, err
			}
			return o.client.Steam(ctx, key, id)
		})
	}
	if o.c.Settings.Get().OwnedGOG {
		o.run(ctx, "gog", func(context.Context) ([]library.Owned, error) { return owned.GOG(owned.GalaxyDB()) })
	}
	if _, ok := loadEpic(); ok {
		o.run(ctx, "epic", func(ctx context.Context) ([]library.Owned, error) {
			access, _, err := o.epicAccess(ctx)
			if err != nil {
				return nil, err
			}
			return o.client.Epic(ctx, access)
		})
	}
}

func (o *ownedState) run(ctx context.Context, store string, fetch func(context.Context) ([]library.Owned, error)) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	games, err := fetch(cctx)
	if err != nil {
		logx.Printf("owned %s: %v", store, err)
		o.setStatus(store, func(s *StoreAccount) { s.Error = err.Error() })
		return
	}
	added, removed := o.c.Lib.ApplyOwned(store, games, time.Now())
	logx.Printf("owned %s: %d games (%d new, %d gone)", store, len(games), added, removed)
	o.setStatus(store, func(s *StoreAccount) {
		s.Error, s.Games, s.Synced = "", len(games), time.Now().Unix()
	})
}

// loop syncs a little after start and then twice a day.
func (o *ownedState) loop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
	}
	for {
		if !o.c.waitIdle(ctx) {
			return
		}
		o.sync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(ownedEvery):
		}
	}
}

// ---- the service ----

// AccountsService connects store accounts for owned games.
type AccountsService struct{ c *Core }

// NewAccountsService binds accounts to core.
func NewAccountsService(c *Core) *AccountsService { return &AccountsService{c} }

// Get reports the accounts.
func (s *AccountsService) Get() Accounts { return s.c.owned.accounts() }

// Sync fetches the owned games again, in the background.
func (s *AccountsService) Sync() { go s.c.owned.sync(s.c.ctx) }

// SetSteamKey stores the user's Steam Web API key (encrypted for this
// Windows user) and fetches the account's games; "" disconnects Steam.
func (s *AccountsService) SetSteamKey(key string) (Accounts, error) {
	if key == "" {
		if err := platform.SaveSecret(steamKeySecret, ""); err != nil {
			return s.Get(), err
		}
		s.c.Lib.ForgetOwned("steam")
		s.c.owned.setStatus("steam", func(a *StoreAccount) { *a = StoreAccount{} })
		s.c.emit(EventLibraryChanged, "owned")
		return s.Get(), nil
	}
	if !owned.ValidSteamKey(key) {
		return s.Get(), errors.New("that doesn't look like a Steam Web API key (32 letters and digits)")
	}
	id, err := owned.SteamID()
	if err != nil {
		return s.Get(), err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, time.Minute)
	defer cancel()
	games, err := s.c.owned.client.Steam(ctx, key, id)
	if err != nil {
		return s.Get(), err
	}
	if err := platform.SaveSecret(steamKeySecret, key); err != nil {
		return s.Get(), err
	}
	added, _ := s.c.Lib.ApplyOwned("steam", games, time.Now())
	logx.Printf("owned steam: connected, %d games (%d new)", len(games), added)
	s.c.owned.setStatus("steam", func(a *StoreAccount) { *a = StoreAccount{Games: len(games), Synced: time.Now().Unix()} })
	s.c.emit(EventLibraryChanged, "owned")
	s.c.meta.queueMissing()
	return s.Get(), nil
}

// OpenSteamKeyPage opens Steam's page for creating a Web API key.
func (s *AccountsService) OpenSteamKeyPage() error {
	return platform.OpenWebPage("https://steamcommunity.com/dev/apikey")
}

// SetGOG turns reading GOG Galaxy's library on or off.
func (s *AccountsService) SetGOG(on bool) (Accounts, error) {
	s.c.setMu.Lock()
	v := s.c.Settings.Get()
	v.OwnedGOG = on
	_, err := s.c.Settings.Set(v)
	s.c.setMu.Unlock()
	if err != nil {
		return s.Get(), err
	}
	if on {
		s.c.owned.run(s.c.ctx, "gog", func(context.Context) ([]library.Owned, error) { return owned.GOG(owned.GalaxyDB()) })
		s.c.meta.queueMissing()
	} else {
		s.c.Lib.ForgetOwned("gog")
		s.c.owned.setStatus("gog", func(a *StoreAccount) { *a = StoreAccount{} })
	}
	s.c.emit(EventLibraryChanged, "owned")
	return s.Get(), nil
}

// OpenEpicSignIn opens Epic's sign-in page in the browser.
func (s *AccountsService) OpenEpicSignIn() error { return platform.OpenWebPage(owned.EpicLoginURL) }

// EpicSignIn finishes the Epic sign-in with the code the page showed
// (the code alone, or the whole text) and fetches the account's games.
func (s *AccountsService) EpicSignIn(pasted string) (Accounts, error) {
	code, ok := owned.EpicCode(pasted)
	if !ok {
		return s.Get(), errors.New("paste the authorizationCode the Epic page showed after signing in")
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 2*time.Minute)
	defer cancel()
	tok, err := s.c.owned.client.EpicSignIn(ctx, code)
	if err != nil {
		return s.Get(), err
	}
	// Under the refresh lock, so a refresh of an older sign-in that's
	// still running can't save its token over this one.
	o := s.c.owned
	o.epicMu.Lock()
	err = saveEpic(epicAccount{Refresh: tok.RefreshToken, Account: tok.AccountID, Name: tok.DisplayName})
	if err == nil {
		o.epicTok = tok
	}
	o.epicMu.Unlock()
	if err != nil {
		return s.Get(), err
	}
	s.c.ach.clear() // Epic games can show progress now
	logx.Printf("owned epic: signed in")
	s.c.owned.run(ctx, "epic", func(ctx context.Context) ([]library.Owned, error) { return s.c.owned.client.Epic(ctx, tok.AccessToken) })
	s.c.emit(EventLibraryChanged, "owned")
	s.c.meta.queueMissing()
	return s.Get(), nil
}

// EpicSignOut forgets the Epic sign-in and its owned games.
func (s *AccountsService) EpicSignOut() (Accounts, error) {
	// Under the refresh lock, so a refresh that's still running can't
	// save the account back after it's forgotten.
	o := s.c.owned
	o.epicMu.Lock()
	err := platform.SaveSecret(epicSecret, "")
	if err == nil {
		o.epicTok = owned.EpicToken{}
	}
	o.epicMu.Unlock()
	if err != nil {
		return s.Get(), err
	}
	s.c.ach.clear()
	s.c.Lib.ForgetOwned("epic")
	s.c.owned.setStatus("epic", func(a *StoreAccount) { *a = StoreAccount{} })
	s.c.emit(EventLibraryChanged, "owned")
	return s.Get(), nil
}

// epicAccess returns an access token for the signed-in Epic account,
// refreshing it when it's (nearly) expired. Epic hands out a new refresh
// token with each refresh; the newest is kept. One refresh at a time, so
// the owned-games sync and achievements don't spend each other's token.
func (o *ownedState) epicAccess(ctx context.Context) (access, account string, err error) {
	o.epicMu.Lock()
	defer o.epicMu.Unlock()
	ep, ok := loadEpic()
	if !ok {
		return "", "", errors.New("not signed in to Epic")
	}
	if t := o.epicTok; t.AccessToken != "" && t.RefreshToken == ep.Refresh && time.Until(t.ExpiresAt) > 5*time.Minute {
		return t.AccessToken, ep.Account, nil
	}
	tok, err := o.client.EpicRefresh(ctx, ep.Refresh)
	if err != nil {
		return "", "", err
	}
	if tok.ExpiresAt.IsZero() {
		tok.ExpiresAt = time.Now().Add(time.Hour)
	}
	// A refresh that hands out no new refresh token leaves the old one in
	// use; saving "" would remove the sign-in.
	if tok.RefreshToken != "" {
		ep.Refresh = tok.RefreshToken
	}
	if tok.AccountID != "" {
		ep.Account = tok.AccountID
	}
	if err := saveEpic(ep); err != nil {
		logx.Printf("owned: saving Epic sign-in: %v", err)
	}
	o.epicTok = tok
	return tok.AccessToken, ep.Account, nil
}

// ---- GOG sign-in (achievements) ----

const gogSecret = "gog-account"

type gogAccount struct {
	Refresh string `json:"refresh"`
	User    string `json:"user"`
}

func loadGOG() (gogAccount, bool) {
	var a gogAccount
	s := platform.LoadSecret(gogSecret)
	if s == "" || json.Unmarshal([]byte(s), &a) != nil || a.Refresh == "" || a.User == "" {
		return a, false
	}
	return a, true
}

func saveGOG(a gogAccount) error {
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return platform.SaveSecret(gogSecret, string(b))
}

// gogAccess returns an access token and user id for the signed-in GOG
// account, refreshing it when it's (nearly) expired; the newest refresh
// token is kept.
func (o *ownedState) gogAccess(ctx context.Context) (access, user string, err error) {
	o.gogMu.Lock()
	defer o.gogMu.Unlock()
	a, ok := loadGOG()
	if !ok {
		return "", "", errors.New("not signed in to GOG")
	}
	if o.gogTok.AccessToken != "" && o.gogTok.RefreshToken == a.Refresh && time.Until(o.gogExp) > 5*time.Minute {
		return o.gogTok.AccessToken, a.User, nil
	}
	tok, err := o.client.GOGRefresh(ctx, a.Refresh)
	if err != nil {
		return "", "", err
	}
	if tok.RefreshToken != "" {
		a.Refresh = tok.RefreshToken
	}
	if tok.UserID != "" {
		a.User = tok.UserID
	}
	if err := saveGOG(a); err != nil {
		logx.Printf("owned: saving GOG sign-in: %v", err)
	}
	o.gogTok, o.gogExp = tok, time.Now().Add(time.Duration(max(tok.ExpiresIn, 60))*time.Second)
	return tok.AccessToken, a.User, nil
}

// OpenGOGSignIn opens GOG's sign-in page in the browser.
func (s *AccountsService) OpenGOGSignIn() error { return platform.OpenWebPage(owned.GOGLoginURL) }

// GOGSignIn finishes the GOG sign-in with the address the browser ended on
// (or the code in it). It's used for achievements only.
func (s *AccountsService) GOGSignIn(pasted string) (Accounts, error) {
	code, ok := owned.GOGCode(pasted)
	if !ok {
		return s.Get(), errors.New("paste the address the GOG page ended on after signing in (it has code= in it)")
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, time.Minute)
	defer cancel()
	tok, err := s.c.owned.client.GOGSignIn(ctx, code)
	if err != nil {
		return s.Get(), err
	}
	// Under the refresh lock, like the Epic sign-in.
	o := s.c.owned
	o.gogMu.Lock()
	err = saveGOG(gogAccount{Refresh: tok.RefreshToken, User: tok.UserID})
	if err == nil {
		o.gogTok, o.gogExp = tok, time.Now().Add(time.Duration(max(tok.ExpiresIn, 60))*time.Second)
	}
	o.gogMu.Unlock()
	if err != nil {
		return s.Get(), err
	}
	s.c.ach.clear()
	logx.Printf("gog: signed in for achievements")
	a := s.Get()
	s.c.emit(EventAccounts, a)
	return a, nil
}

// GOGSignOut forgets the GOG sign-in.
func (s *AccountsService) GOGSignOut() (Accounts, error) {
	o := s.c.owned
	o.gogMu.Lock()
	err := platform.SaveSecret(gogSecret, "")
	if err == nil {
		o.gogTok, o.gogExp = owned.GOGToken{}, time.Time{}
	}
	o.gogMu.Unlock()
	if err != nil {
		return s.Get(), err
	}
	s.c.ach.clear()
	a := s.Get()
	s.c.emit(EventAccounts, a)
	return a, nil
}
