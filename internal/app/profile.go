package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/profile"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/syncer"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Events for the profile.
const (
	EventProfile  = "profile:changed"  // who's playing, and whether Syncer syncs the profile
	EventSettings = "settings:changed" // settings saved on another PC were taken
)

// Profile is who's playing on this PC and how their playtime,
// achievements and settings get to their other PCs, for the interface.
type Profile struct {
	// Accounts are Syncer's; Enabled false when it has none, is off, or
	// is too old for them.
	Enabled  bool             `json:"enabled"`
	Active   string           `json:"active,omitempty"`
	Accounts []syncer.Account `json:"accounts"`
	// Owner is whose playtime this PC adds to: the active account, the
	// last one this PC had, or "shared".
	Owner     string `json:"owner"`
	OwnerName string `json:"ownerName,omitempty"`
	// Syncer: installed, answered, and has accounts.
	Installed bool `json:"installed"`
	Reachable bool `json:"reachable"`
	Supported bool `json:"supported"`
	// Synced: Syncer syncs the profile folder (Backup: backs it up).
	Synced    bool   `json:"synced"`
	Backup    bool   `json:"backup"`
	Dismissed bool   `json:"dismissed"`
	DataError string `json:"dataError,omitempty"`
	Dir       string `json:"dir"`
	// SettingsFrom is the PC the settings in use were saved on ("" this one).
	SettingsFrom string `json:"settingsFrom,omitempty"`
	CheckedAt    int64  `json:"checkedAt,omitempty"`
}

func init() {
	application.RegisterEvent[Profile](EventProfile)
	application.RegisterEvent[settings.Settings](EventSettings)
}

// profileState keeps the profile and Syncer's accounts in step with the
// library and settings.
type profileState struct {
	c       *Core
	store   *profile.Store // nil when the folder couldn't be opened
	name    string         // this PC's name
	ownerAt string         // where the last owner is kept ("" in tests: nowhere)
	// seededAt marks that the library was carried over (not synced).
	seededAt string

	apply sync.Mutex // one apply at a time
	mu    sync.Mutex
	info  Profile
	unl   map[string]map[string]int64 // merged achievements by game key
	pcs   string                      // PC the settings came from
	data  time.Time                   // last launcherData call that worked
	kick  chan struct{}
}

func newProfileState(c *Core) *profileState {
	p := &profileState{c: c, kick: make(chan struct{}, 1), unl: map[string]map[string]int64{}}
	dir := filepath.Join(platform.AppDir(), "Profile")
	id, name := thisPC()
	p.name, p.ownerAt = name, ownerFile()
	p.seededAt = filepath.Join(platform.AppDir(), "profile-seeded.txt")
	st, err := profile.Open(dir, id, name)
	if err != nil {
		logx.Printf("profile: %v", err)
		if st == nil {
			return p
		}
	}
	p.store = st
	if owner := loadOwner(p.ownerAt); owner != "" {
		if _, err := st.SetOwner(owner); err != nil {
			logx.Printf("profile: %v", err)
		}
	}
	p.info = Profile{Owner: st.Owner(), Dir: dir, Accounts: []syncer.Account{}}
	return p
}

// thisPC names this PC's files: its name and a hash of Windows' machine
// id, so two PCs with the same name (or a copied AppData) don't share one.
func thisPC() (id, name string) {
	name, _ = os.Hostname()
	raw := machineID()
	if raw == "" {
		raw = name
	}
	h := sha256.Sum256([]byte("seaglass-profile:" + strings.ToLower(raw)))
	base := regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(name, "")
	if len(base) > 40 {
		base = base[:40]
	}
	if base == "" {
		base = "pc"
	}
	return base + "-" + hex.EncodeToString(h[:4]), cmpOr(name, "PC")
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ownerFile keeps the last account this PC had (it isn't synced): Syncer
// may not be running, or accounts may have been turned off since.
func ownerFile() string { return filepath.Join(platform.AppDir(), "profile-owner.txt") }

func loadOwner(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if o := strings.TrimSpace(string(b)); profile.ValidOwner(o) {
		return o
	}
	return ""
}

// profileKey names a game the same way on every PC: by its store id, or
// by its title when it has none. Play is recorded under it.
func profileKey(g *library.Game) string {
	if ks := profileKeys(g); len(ks) > 0 {
		return ks[0]
	}
	return ""
}

// profileKeys are every key a game's play may be recorded under: its key
// now first, then the ones it had before it was matched to a store or
// renamed. A game's totals add them all up (each play went to one).
func profileKeys(g *library.Game) []string {
	var ks []string
	add := func(k string) {
		if k != "" && !slices.Contains(ks, k) {
			ks = append(ks, k)
		}
	}
	if g.SteamAppID > 0 {
		add("steam:" + strconv.Itoa(g.SteamAppID))
	}
	if g.MetaAppID > 0 {
		add("steam:" + strconv.Itoa(g.MetaAppID))
	}
	if g.GogID != "" {
		add("gog:" + g.GogID)
	}
	if g.EpicApp != "" {
		add("epic:" + strings.ToLower(g.EpicApp))
	}
	for _, t := range []string{g.DisplayTitle(), g.Title} {
		if k := scan.LooseKey(t); k != "" {
			add("title:" + k)
		}
	}
	return ks
}

// start carries over what this PC knew before, then keeps things in step.
func (p *profileState) start() {
	if p.store == nil {
		return
	}
	// Once per install: a renamed PC gets a new file while its old one
	// still counts, so seeding again would count its playtime twice.
	if p.store.Healthy() && !p.seeded() {
		if p.store.Fresh() {
			p.seed()
		}
		if p.seededAt != "" {
			_ = os.WriteFile(p.seededAt, []byte(p.name), 0o644)
		}
	}
	p.applyAll()
	go p.loop()
}

// seeded reports whether this install already carried the library over.
func (p *profileState) seeded() bool {
	if p.seededAt == "" {
		return false
	}
	_, err := os.Stat(p.seededAt)
	return err == nil
}

// seed writes this PC's playtime, last plays, achievements (as last read)
// and settings into its first profile file.
func (p *profileState) seed() {
	games := map[string]profile.Game{}
	for _, g := range p.c.Lib.Games() {
		k := profileKey(&g)
		if k == "" {
			continue
		}
		x := games[k]
		x.Title = g.DisplayTitle()
		x.Playtime += g.Playtime
		x.LastPlayed = max(x.LastPlayed, g.LastPlayed)
		if p.c.ach != nil {
			if e, ok := p.c.ach.last(g.ID); ok {
				for _, it := range e.List.Items {
					if it.Unlocked {
						if x.Achievements == nil {
							x.Achievements = map[string]int64{}
						}
						x.Achievements[it.ID] = it.UnlockedAt
					}
				}
			}
		}
		if x.Playtime > 0 || x.LastPlayed > 0 || len(x.Achievements) > 0 {
			games[k] = x
		}
	}
	b, _ := json.Marshal(p.c.Settings.Get().Portable())
	p.store.Seed(games, b)
	logx.Printf("profile: started with %d games' playtime and achievements", len(games))
}

func (p *profileState) loop() {
	files := time.NewTicker(20 * time.Second)
	defer files.Stop()
	syncer := time.NewTicker(2 * time.Minute)
	defer syncer.Stop()
	p.refresh(false)
	for {
		select {
		case <-p.c.ctx.Done():
			_ = p.store.Flush()
			return
		case <-files.C:
			p.applyAll()
		case <-syncer.C:
			p.refresh(false)
		case <-p.kick:
			p.refresh(false)
		}
	}
}

// changed asks for a look at Syncer soon.
func (p *profileState) changed() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

// applyAll puts the owner's totals into the library, takes newer settings
// and keeps the merged achievements for get.
func (p *profileState) applyAll() {
	if p.store == nil || !p.store.Healthy() {
		return
	}
	p.apply.Lock()
	defer p.apply.Unlock()
	m := p.store.Merged()
	ids := p.c.Lib.SetPlayed(func(g *library.Game) (int64, int64, bool) {
		ks := profileKeys(g)
		if len(ks) == 0 {
			return 0, 0, false // not in the profile: the library keeps its own
		}
		var pt, lp int64
		for _, k := range ks {
			t := m.Games[k]
			pt, lp = pt+t.Playtime, max(lp, t.LastPlayed)
		}
		return pt, lp, true
	})
	unl := make(map[string]map[string]int64, len(m.Games))
	for k, t := range m.Games {
		if len(t.Achievements) > 0 {
			unl[k] = t.Achievements
		}
	}
	p.mu.Lock()
	p.unl = unl
	p.mu.Unlock()
	if len(ids) > 20 {
		p.c.emit(EventLibraryChanged, "profile")
	} else if len(ids) > 0 {
		p.c.gamesChanged(ids...)
	}
	p.c.setMu.Lock() // no settings saved in between (they'd be lost)
	defer p.c.setMu.Unlock()
	cfg := p.c.Settings.Get()
	if !cfg.SameSettings || m.Settings == nil {
		return
	}
	mine := p.store.SettingsAt()
	if m.SettingsAt <= mine {
		p.setFrom(m, mine)
		return
	}
	next, err := cfg.WithPortable(m.Settings)
	if err != nil {
		return
	}
	saved, err := p.c.saveSettings(next)
	if err != nil {
		logx.Printf("profile: taking settings from %s: %v", m.SettingsPC, err)
		return
	}
	p.store.SetSettings(m.Settings, m.SettingsAt)
	p.setFrom(m, m.SettingsAt)
	logx.Printf("profile: took the settings saved on %s", m.SettingsPC)
	p.c.emit(EventSettings, saved)
}

func (p *profileState) setFrom(m profile.Merged, mine int64) {
	from := ""
	if m.SettingsAt == mine && m.SettingsAt > 0 && m.SettingsPC != "" {
		if m.SettingsPC != p.name {
			from = m.SettingsPC
		}
	}
	p.mu.Lock()
	changed := p.info.SettingsFrom != from
	p.info.SettingsFrom = from
	info := p.info
	p.mu.Unlock()
	if changed {
		p.c.emit(EventProfile, info)
	}
}

// ---- recording ----

func (p *profileState) played(g library.Game) {
	if p.store == nil {
		return
	}
	if k := profileKey(&g); k != "" {
		p.store.Played(k, g.DisplayTitle(), time.Now().Unix())
	}
}

func (p *profileState) addPlaytime(g library.Game, secs int64) {
	if p.store == nil {
		return
	}
	if k := profileKey(&g); k != "" {
		p.store.AddPlaytime(k, g.DisplayTitle(), secs)
	}
}

func (p *profileState) addAchievements(g library.Game, items []achievements.Achievement) {
	if p.store == nil || len(items) == 0 {
		return
	}
	k := profileKey(&g)
	if k == "" {
		return
	}
	m := map[string]int64{}
	for _, it := range items {
		if it.Unlocked {
			m[it.ID] = it.UnlockedAt
		}
	}
	p.store.AddAchievements(k, g.DisplayTitle(), m)
	p.mu.Lock()
	all := map[string]int64{}
	for id, at := range p.unl[k] {
		all[id] = at
	}
	for id, at := range m {
		if old, ok := all[id]; !ok || (at > 0 && (old == 0 || at < old)) {
			all[id] = at
		}
	}
	p.unl[k] = all
	p.mu.Unlock()
}

// settingsSaved records settings changed on this PC.
func (p *profileState) settingsSaved(old, saved settings.Settings) {
	if p.store == nil {
		return
	}
	a, _ := json.Marshal(old.Portable())
	b, _ := json.Marshal(saved.Portable())
	if string(a) == string(b) {
		return // only the PC's own settings changed
	}
	// Newer than any seen, even when another PC's clock runs ahead:
	// otherwise its older settings would come back.
	at := time.Now().Unix()
	if m := p.store.Merged(); m.SettingsAt >= at {
		at = m.SettingsAt + 1
	}
	p.store.SetSettings(b, at)
	p.mu.Lock()
	changed := p.info.SettingsFrom != ""
	p.info.SettingsFrom = ""
	info := p.info
	p.mu.Unlock()
	if changed {
		p.c.emit(EventProfile, info)
	}
}

// withUnlocks adds what the person unlocked on their other PCs (or before
// on this one) to a game's achievements.
func (p *profileState) withUnlocks(g library.Game, l achievements.List) achievements.List {
	lower := map[string]int64{}
	p.mu.Lock()
	for _, k := range profileKeys(&g) {
		for id, at := range p.unl[k] {
			if old, ok := lower[strings.ToLower(id)]; !ok || (at > 0 && (old == 0 || at < old)) {
				lower[strings.ToLower(id)] = at
			}
		}
	}
	p.mu.Unlock()
	if len(lower) == 0 || len(l.Items) == 0 {
		return l
	}
	items := make([]achievements.Achievement, len(l.Items))
	copy(items, l.Items)
	n := 0
	for i := range items {
		if at, ok := lower[strings.ToLower(items[i].ID)]; ok && !items[i].Unlocked {
			items[i].Unlocked, items[i].UnlockedAt = true, at
			if items[i].Max > 0 {
				items[i].Progress = items[i].Max
			}
		}
		if items[i].Unlocked {
			n++
		}
	}
	l.Items, l.Unlocked = items, n
	return l
}

// ---- Syncer ----

// refresh asks Syncer who's playing and has it sync the profile folder.
// With start, Syncer is started (without its window) when it isn't
// running, as a launch would; otherwise only a running Syncer is asked.
func (p *profileState) refresh(start bool) Profile {
	if p.store == nil {
		return p.snapshot()
	}
	info := Profile{Owner: p.store.Owner(), Dir: p.store.Dir(), Accounts: []syncer.Account{}, CheckedAt: time.Now().Unix()}
	defer func() {
		p.mu.Lock()
		info.SettingsFrom = p.info.SettingsFrom
		changed := !sameProfile(p.info, info)
		p.info = info
		p.mu.Unlock()
		if changed {
			p.c.emit(EventProfile, info)
		}
	}()
	if _, ok := syncer.Installed(); !ok {
		return info
	}
	info.Installed = true
	ctx, cancel := context.WithTimeout(p.c.ctx, 20*time.Second)
	defer cancel()
	cl, err := syncer.Dial(ctx, start)
	if err != nil {
		p.keepLast(&info)
		return info
	}
	defer cl.Close()
	if _, err := cl.Status(ctx); err != nil {
		p.keepLast(&info)
		return info
	}
	info.Reachable = true
	acc, err := cl.Accounts(ctx)
	switch {
	case syncer.UnknownMethod(err):
	case err != nil:
		p.keepLast(&info)
	default:
		info.Supported = true
		p.useAccounts(&info, acc)
	}
	if p.c.Settings.Get().SyncProfile {
		p.syncData(ctx, cl, &info)
	}
	return info
}

// useAccounts takes Syncer's accounts and the active one as the owner.
func (p *profileState) useAccounts(info *Profile, acc syncer.Accounts) {
	info.Enabled = acc.Enabled && len(acc.Accounts) > 0
	if acc.Accounts != nil {
		info.Accounts = acc.Accounts
	}
	info.Active = acc.Active
	if info.Enabled && acc.Active != "" && profile.ValidOwner(acc.Active) {
		p.setOwner(acc.Active)
	}
	info.Owner = p.store.Owner()
	for _, a := range acc.Accounts {
		if a.ID == info.Owner {
			info.OwnerName = a.Name
		}
	}
}

// keepLast keeps what Syncer said last time it answered.
func (p *profileState) keepLast(info *Profile) {
	p.mu.Lock()
	defer p.mu.Unlock()
	info.Enabled, info.Active, info.Accounts, info.OwnerName = p.info.Enabled, p.info.Active, p.info.Accounts, p.info.OwnerName
	info.Supported, info.Synced, info.Backup, info.Dismissed = p.info.Supported, p.info.Synced, p.info.Backup, p.info.Dismissed
}

// setOwner makes id the owner and shows their playtime and settings.
func (p *profileState) setOwner(id string) {
	if p == nil || p.store == nil {
		return
	}
	// Not while applyAll works on the previous owner's totals and settings.
	p.apply.Lock()
	changed, err := p.store.SetOwner(id)
	if err != nil {
		logx.Printf("profile: %v", err)
	}
	if !changed {
		p.apply.Unlock()
		return
	}
	if p.ownerAt != "" {
		_ = os.WriteFile(p.ownerAt, []byte(id), 0o644)
	}
	logx.Printf("profile: %s is playing on this PC", id)
	if p.store.SettingsAt() == 0 {
		// Nothing saved for them on this PC yet: this PC's settings are
		// theirs until settings they saved elsewhere arrive.
		b, _ := json.Marshal(p.c.Settings.Get().Portable())
		p.store.SetSettings(b, 0)
	}
	p.apply.Unlock()
	p.applyAll()
}

// syncData has Syncer sync and back up the profile folder, once per run
// (and again every half hour while it isn't).
func (p *profileState) syncData(ctx context.Context, cl *syncer.Client, info *Profile) {
	p.mu.Lock()
	recent := !p.data.IsZero() && time.Since(p.data) < 30*time.Minute
	last := p.info
	p.mu.Unlock()
	if recent && last.Synced {
		info.Synced, info.Backup = last.Synced, last.Backup
		return
	}
	_ = p.store.Flush() // the folder must exist
	_ = os.MkdirAll(p.store.Dir(), 0o755)
	d, err := cl.SyncLauncherData(ctx, "Seaglass", p.store.Dir())
	switch {
	case syncer.UnknownMethod(err):
		info.DataError = "Syncer needs an update to sync playtime, achievements and settings"
		return
	case err != nil:
		info.DataError = strings.TrimPrefix(err.Error(), "Syncer: ")
		return
	}
	info.Synced, info.Backup, info.Dismissed = d.Sync, d.Backup, d.Dismissed
	p.mu.Lock()
	p.data = time.Now()
	p.mu.Unlock()
	if d.Added {
		logx.Printf("profile: Syncer syncs it now (%s)", d.ID)
	}
}

func sameProfile(a, b Profile) bool {
	a.CheckedAt, b.CheckedAt = 0, 0
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func (p *profileState) snapshot() Profile {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info
}

// follow makes the account Syncer has playing on this PC the owner, so a
// game that starts adds to their playtime without asking anyone.
func (p *profileState) follow(ctx context.Context, cl *syncer.Client) {
	if p == nil || p.store == nil {
		return
	}
	acc, err := cl.Accounts(ctx)
	if err != nil || !acc.Enabled || !profile.ValidOwner(acc.Active) {
		return
	}
	p.setOwner(acc.Active)
	p.changed() // the interface shows who's playing
}

// switchTo puts an account's saves in place through Syncer and makes it
// the owner.
func (p *profileState) switchTo(ctx context.Context, cl *syncer.Client, id string) error {
	if err := cl.SwitchAccount(ctx, id); err != nil {
		return err
	}
	if p.store != nil {
		p.setOwner(id)
	}
	return nil
}

// ---- the service ----

// ProfileService is who's playing, for the interface.
type ProfileService struct{ c *Core }

// NewProfileService binds the profile to core.
func NewProfileService(c *Core) *ProfileService { return &ProfileService{c} }

// Get returns the last known state; fresh asks Syncer first (without
// starting it).
func (s *ProfileService) Get(fresh bool) Profile {
	if fresh {
		return s.c.profile.refresh(false)
	}
	return s.c.profile.snapshot()
}

// Switch makes another of Syncer's accounts the one playing on this PC:
// Syncer puts their saves in place, and Seaglass shows their playtime,
// achievements and settings.
func (s *ProfileService) Switch(id string) (Profile, error) {
	if s.c.Launch.Active() {
		return s.c.profile.snapshot(), errors.New("close the game first: its saves are about to move")
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 3*time.Minute)
	defer cancel()
	cl, err := syncer.Dial(ctx, true)
	if err != nil {
		return s.c.profile.snapshot(), errors.New("Syncer didn't answer")
	}
	defer cl.Close()
	if err := s.c.profile.switchTo(ctx, cl, id); err != nil {
		return s.c.profile.snapshot(), err
	}
	return s.c.profile.refresh(false), nil
}

// ---- playing ----

// records reports whether g's play goes into the profile (else the
// library keeps it, as before the profile).
func (p *profileState) records(g library.Game) bool {
	return p != nil && p.store != nil && p.store.Healthy() && profileKey(&g) != ""
}

// startedPlaying records that a game started, for the person playing.
func (c *Core) startedPlaying(g library.Game) {
	now := time.Now().Unix()
	if c.profile.records(g) {
		c.profile.played(g)
		c.profile.applyAll()
		return
	}
	_, _ = c.Lib.Update(g.ID, func(x *library.Game) { x.LastPlayed = now })
	c.gamesChanged(g.ID)
}

// addPlaytime records time played, for the person playing.
func (c *Core) addPlaytime(g library.Game, secs int64) {
	if c.profile.records(g) {
		c.profile.addPlaytime(g, secs)
		c.profile.applyAll()
		return
	}
	_, _ = c.Lib.Update(g.ID, func(x *library.Game) { x.Playtime += secs })
	c.gamesChanged(g.ID)
}

// maxAsked is how many accounts the launch question lists.
const maxAsked = 6

// whoPlaysStep asks who's playing when Syncer has several accounts, and
// puts that person's saves in place before the game starts (and before
// its saves are synced: a split game's folder is the account's).
func (c *Core) whoPlaysStep(g library.Game) launch.Step {
	title := g.DisplayTitle()
	return launch.Step{ID: "whoPlays", Label: "Who's playing", Timeout: 10 * time.Minute,
		Run: func(ctx context.Context, sc *launch.StepContext) error {
			cfg := c.Settings.Get()
			dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			cl, err := syncer.Dial(dctx, cfg.StartSyncer)
			cancel()
			if err != nil {
				sc.Progress("Syncer isn't running")
				return nil
			}
			defer cl.Close()
			if _, err := cl.Status(ctx); err != nil {
				return nil
			}
			acc, err := cl.Accounts(ctx)
			if err != nil || !acc.Enabled || len(acc.Accounts) < 2 {
				sc.Progress("One person on this PC")
				c.profile.changed()
				return nil
			}
			name := map[string]string{}
			var opts []launch.Option
			for _, a := range acc.Accounts {
				name[a.ID] = a.Name
				if a.ID == acc.Active {
					opts = append([]launch.Option{{ID: a.ID, Label: a.Name}}, opts...)
				} else if len(opts) < maxAsked-1 || acc.Active == "" {
					opts = append(opts, launch.Option{ID: a.ID, Label: a.Name})
				}
			}
			if len(opts) > maxAsked {
				opts = opts[:maxAsked]
			}
			opts = append(opts, launch.Option{ID: "cancel", Label: "Cancel"})
			who, err := sc.Ask(ctx, fmt.Sprintf("Who's playing %s?", title), opts)
			switch {
			case err != nil:
				return err
			case who == "cancel":
				return launch.ErrCancel
			case who == acc.Active:
				c.profile.setOwner(who)
				c.profile.changed()
				sc.Progress("Playing as " + name[who])
				return nil
			}
			sc.Progress("Putting " + name[who] + "'s saves in place…")
			if err := c.profile.switchTo(ctx, cl, who); err != nil {
				msg := strings.TrimPrefix(err.Error(), "Syncer: ")
				stay := "Play anyway"
				if n := name[acc.Active]; n != "" {
					stay = "Play as " + n
				}
				a, aerr := sc.Ask(ctx, fmt.Sprintf("Couldn't switch to %s: %s", name[who], msg), []launch.Option{
					{ID: "play", Label: stay},
					{ID: "cancel", Label: "Cancel"},
				})
				if aerr != nil {
					return aerr
				}
				if a != "play" {
					return launch.ErrCancel
				}
				sc.Progress("Playing as " + cmpOr(name[acc.Active], "before"))
				return nil
			}
			c.profile.changed()
			sc.Progress("Playing as " + name[who])
			return nil
		}}
}
