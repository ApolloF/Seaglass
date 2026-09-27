package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/addons"
	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/gamekit/steam"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventAddonProgress carries an add-on action's progress to the interface.
const EventAddonProgress = "addon:progress"

// AddonProgress is one progress line of an add-on action.
type AddonProgress struct {
	Addon string `json:"addon"`
	Text  string `json:"text"`
}

func init() { application.RegisterEvent[AddonProgress](EventAddonProgress) }

// addonState is the add-on host and the user's approvals.
type addonState struct {
	root  string // %LOCALAPPDATA%\Seaglass\addons
	host  *addons.Host
	trust *addons.Trust

	mu       sync.Mutex
	signed   map[string]bool // exe path|hash → carries a valid signature
	manifest map[string]*addons.Manifest
}

func newAddonState(version string) *addonState {
	return &addonState{
		root:  platform.CacheDir("addons"),
		host:  addons.NewHost(version, logx.Printf),
		trust: addons.OpenTrust(filepath.Join(platform.AppDir(), "addons.json")),
	}
}

// discover reads the add-ons again.
func (s *addonState) discover() ([]*addons.Manifest, map[string]error) {
	found, broken := addons.Discover(s.root)
	s.mu.Lock()
	s.manifest = map[string]*addons.Manifest{}
	for _, m := range found {
		s.manifest[m.ID] = m
	}
	s.mu.Unlock()
	return found, broken
}

// enabled returns the add-ons that are on, still match their approval and
// asked for hook.
func (s *addonState) enabled(hook string) []*addons.Manifest {
	found, _ := s.discover()
	var out []*addons.Manifest
	for _, m := range found {
		if a := s.trust.Get(m.ID); a.Enabled && m.Has(hook) {
			out = append(out, m)
		}
	}
	return out
}

func (s *addonState) call(ctx context.Context, m *addons.Manifest, method string, params, out any, progress func(string)) error {
	a := s.trust.Get(m.ID)
	if !a.Enabled {
		return errors.New(m.Name + " is off")
	}
	return s.host.Call(ctx, m, a.SHA256, method, params, out, progress)
}

// addonGame is a game as add-ons see it.
func addonGame(g library.Game) map[string]any {
	out := map[string]any{"id": g.ID, "title": g.DisplayTitle(), "dir": g.Dir, "source": g.Source}
	if g.Exe != "" {
		out["exe"] = g.Exe
	}
	if app := max(g.SteamAppID, g.MetaAppID); app > 0 {
		out["steamAppId"] = app
	}
	return out
}

// ---- the service ----

// AddonsService manages add-ons and asks them about games.
type AddonsService struct{ c *Core }

// NewAddonsService binds add-ons to core.
func NewAddonsService(c *Core) *AddonsService { return &AddonsService{c} }

// ServiceShutdown stops every add-on.
func (s *AddonsService) ServiceShutdown() error {
	s.c.addons.host.StopAll()
	return nil
}

// Permission is one declared permission, explained.
type Permission struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AddonView is an add-on as Settings shows it.
type AddonView struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Publisher   string       `json:"publisher"`
	Description string       `json:"description"`
	Homepage    string       `json:"homepage"`
	Exe         string       `json:"exe"`
	Hooks       []string     `json:"hooks"`
	Permissions []Permission `json:"permissions"`
	Signed      bool         `json:"signed"`
	SHA256      string       `json:"sha256"`
	Enabled     bool         `json:"enabled"`
	Running     bool         `json:"running"`
	// State: "off", "on", "changed" (the program changed since approval),
	// "missing" (the program is gone) or "broken" (the manifest is wrong).
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// List returns every add-on found, working or not.
func (s *AddonsService) List() []AddonView {
	st := s.c.addons
	found, broken := st.discover()
	out := []AddonView{}
	for _, m := range found {
		out = append(out, s.view(m))
	}
	for dir, err := range broken {
		out = append(out, AddonView{ID: dir, Name: dir, State: "broken", Error: err.Error(), Hooks: []string{}, Permissions: []Permission{}})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].State != "broken" && out[j].State == "broken" })
	return out
}

func (s *AddonsService) view(m *addons.Manifest) AddonView {
	st := s.c.addons
	v := AddonView{ID: m.ID, Name: m.Name, Version: m.Version, Publisher: m.Publisher, Description: m.Description,
		Homepage: m.Homepage, Exe: m.ExePath(), Hooks: m.Hooks, Permissions: []Permission{}, Running: st.host.Running(m.ID)}
	for _, p := range m.Permissions {
		v.Permissions = append(v.Permissions, Permission{p, addons.Permissions[p]})
	}
	a := st.trust.Get(m.ID)
	sum, err := addons.HashFile(v.Exe)
	switch {
	case err != nil:
		v.State, v.Error = "missing", "The add-on's program wasn't found"
	case a.Enabled && a.SHA256 != sum:
		v.State = "changed"
	case a.Enabled:
		v.State, v.Enabled = "on", true
	default:
		v.State = "off"
	}
	v.SHA256 = sum
	if sum != "" {
		key := v.Exe + "|" + sum
		st.mu.Lock()
		if st.signed == nil {
			st.signed = map[string]bool{}
		}
		ok, known := st.signed[key]
		st.mu.Unlock()
		if !known {
			ok = steam.Signed(v.Exe)
			st.mu.Lock()
			st.signed[key] = ok
			st.mu.Unlock()
		}
		v.Signed = ok
	}
	return v
}

func (s *AddonsService) find(id string) (*addons.Manifest, error) {
	found, _ := s.c.addons.discover()
	for _, m := range found {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, errors.New("add-on not found")
}

// Enable turns an add-on on, approving its program as it is now (sha256
// must match what the user was shown).
func (s *AddonsService) Enable(id, sha256 string) (AddonView, error) {
	m, err := s.find(id)
	if err != nil {
		return AddonView{}, err
	}
	sum, err := addons.HashFile(m.ExePath())
	if err != nil {
		return AddonView{}, errors.New("the add-on's program wasn't found")
	}
	if sum != sha256 {
		return AddonView{}, errors.New("the add-on's program changed while you were looking; check it again")
	}
	if err := s.c.addons.trust.Set(id, addons.Approval{Enabled: true, SHA256: sum, Approved: time.Now()}); err != nil {
		return AddonView{}, err
	}
	logx.Printf("add-on %s enabled (sha256 %s)", id, sum[:12])
	return s.view(m), nil
}

// Disable turns an add-on off.
func (s *AddonsService) Disable(id string) (AddonView, error) {
	m, err := s.find(id)
	if err != nil {
		return AddonView{}, err
	}
	a := s.c.addons.trust.Get(id)
	a.Enabled = false
	if err := s.c.addons.trust.Set(id, a); err != nil {
		return AddonView{}, err
	}
	s.c.addons.host.Stop(id)
	logx.Printf("add-on %s disabled", id)
	return s.view(m), nil
}

// Add asks for an addon.json and adds that add-on (turned off).
func (s *AddonsService) Add() (AddonView, error) {
	p, err := application.Get().Dialog.OpenFile().
		SetTitle("Choose the add-on's addon.json").
		CanChooseFiles(true).CanChooseDirectories(false).
		AddFilter("Add-on manifest", "addon.json;*.json").
		PromptForSingleSelection()
	if err != nil || p == "" {
		return AddonView{}, err
	}
	m, err := addons.Install(s.c.addons.root, p)
	if err != nil {
		return AddonView{}, err
	}
	logx.Printf("add-on %s added from %s", m.ID, p)
	return s.view(m), nil
}

// Remove forgets an add-on. Its program stays where it is.
func (s *AddonsService) Remove(id string) error {
	st := s.c.addons
	st.host.Stop(id)
	dir := filepath.Join(st.root, id)
	if filepath.Dir(dir) != filepath.Clean(st.root) {
		return errors.New("invalid add-on")
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return st.trust.Set(id, addons.Approval{})
}

// OpenFolder shows the add-ons folder.
func (s *AddonsService) OpenFolder() error { return platform.ShowInExplorer(s.c.addons.root) }

// Badge is a short fact an add-on shows on a game.
type Badge struct {
	Text    string `json:"text"`
	Tone    string `json:"tone,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// Line is a labelled value an add-on shows on a game.
type Line struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// AddonAction is something an add-on can do for a game.
type AddonAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Confirm     string `json:"confirm,omitempty"`
}

// AddonGame is what one add-on says about a game.
type AddonGame struct {
	Addon   string        `json:"addon"`
	Name    string        `json:"name"`
	Badges  []Badge       `json:"badges"`
	Lines   []Line        `json:"lines"`
	Actions []AddonAction `json:"actions"`
	Error   string        `json:"error,omitempty"`
}

// ForGame asks every enabled add-on about a game: badges, lines and actions.
func (s *AddonsService) ForGame(id int64) ([]AddonGame, error) {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return nil, library.ErrNotFound
	}
	st := s.c.addons
	found, _ := st.discover()
	out := []AddonGame{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, m := range found {
		if !st.trust.Get(m.ID).Enabled || !(m.Has(addons.HookStatus) || m.Has(addons.HookActions)) {
			continue
		}
		wg.Add(1)
		go func(m *addons.Manifest) {
			defer wg.Done()
			r := AddonGame{Addon: m.ID, Name: m.Name, Badges: []Badge{}, Lines: []Line{}, Actions: []AddonAction{}}
			params := map[string]any{"game": addonGame(g)}
			if m.Has(addons.HookStatus) {
				ctx, cancel := context.WithTimeout(s.c.ctx, 10*time.Second)
				var res struct {
					Badges []Badge `json:"badges"`
					Lines  []Line  `json:"lines"`
				}
				if err := st.call(ctx, m, addons.HookStatus, params, &res, nil); err != nil {
					r.Error = err.Error()
				} else {
					r.Badges, r.Lines = clip(res.Badges, 6), clip(res.Lines, 12)
				}
				cancel()
			}
			if m.Has(addons.HookActions) && r.Error == "" {
				ctx, cancel := context.WithTimeout(s.c.ctx, 10*time.Second)
				var acts []AddonAction
				if err := st.call(ctx, m, addons.HookActions, params, &acts, nil); err != nil {
					r.Error = err.Error()
				} else {
					r.Actions = clip(acts, 8)
				}
				cancel()
			}
			mu.Lock()
			out = append(out, r)
			mu.Unlock()
		}(m)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func clip[T any](s []T, n int) []T {
	if s == nil {
		return []T{}
	}
	return s[:min(len(s), n)]
}

// RunAction runs an add-on's action for a game and returns its message.
// Progress lines arrive as "addon:progress" events.
func (s *AddonsService) RunAction(id int64, addon, action string) (string, error) {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return "", library.ErrNotFound
	}
	m, err := s.find(addon)
	if err != nil {
		return "", err
	}
	if !m.Has(addons.HookActions) {
		return "", errors.New(m.Name + " has no actions")
	}
	if len(action) == 0 || len(action) > 64 {
		return "", errors.New("unknown action")
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 10*time.Minute)
	defer cancel()
	var res struct {
		Message string `json:"message"`
	}
	err = s.c.addons.call(ctx, m, "game.runAction", map[string]any{"game": addonGame(g), "action": action}, &res, func(t string) {
		s.c.emit(EventAddonProgress, AddonProgress{Addon: addon, Text: t})
	})
	if err != nil {
		logx.Printf("add-on %s action %s on %q: %v", addon, action, g.DisplayTitle(), err)
		return "", err
	}
	logx.Printf("add-on %s action %s on %q: %s", addon, action, g.DisplayTitle(), res.Message)
	return res.Message, nil
}

// ---- launch hooks ----

// addonSteps returns the before-launch and after-exit steps of the enabled add-ons.
func (c *Core) addonSteps(g library.Game) (before, after []launch.Step) {
	params := func(extra map[string]any) map[string]any {
		p := map[string]any{"game": addonGame(g)}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	step := func(m *addons.Manifest, hook string, timeout time.Duration, extra func() map[string]any) launch.Step {
		return launch.Step{ID: "addon:" + m.ID + ":" + hook, Label: m.Name, Timeout: timeout,
			Run: func(ctx context.Context, sc *launch.StepContext) error {
				var res struct {
					Message string `json:"message"`
				}
				var x map[string]any
				if extra != nil {
					x = extra()
				}
				if err := c.addons.call(ctx, m, hook, params(x), &res, sc.Progress); err != nil {
					return err
				}
				if res.Message != "" {
					sc.Progress(res.Message)
				}
				return nil
			}}
	}
	for _, m := range c.addons.enabled(addons.HookBeforeLaunch) {
		before = append(before, step(m, addons.HookBeforeLaunch, 5*time.Minute, nil))
	}
	for _, m := range c.addons.enabled(addons.HookAfterExit) {
		after = append(after, step(m, addons.HookAfterExit, 2*time.Minute, func() map[string]any {
			return map[string]any{"seconds": c.Launch.Current().Seconds}
		}))
	}
	return before, after
}

// tellAddonsAdded tells running add-ons about games a scan found.
func (c *Core) tellAddonsAdded(games []library.Game) {
	if len(games) == 0 {
		return
	}
	for _, m := range c.addons.enabled(addons.HookGameAdded) {
		for _, g := range games {
			c.addons.host.Notify(m.ID, addons.HookGameAdded, map[string]any{"game": addonGame(g)})
		}
	}
}
