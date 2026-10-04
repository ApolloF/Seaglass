package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/syncer"
	"github.com/ApolloF/Seaglass/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// crashFile receives what Go prints when Seaglass crashes (an
// unhandled panic or a fatal error). It's emptied at every start, so a
// file with something in it means the last run crashed.
func crashFile(dir string) string { return filepath.Join(dir, "crash.log") }

// previousCrashFile keeps the last crash's output.
func previousCrashFile(dir string) string { return filepath.Join(dir, "crash-previous.log") }

var crashedLastTime bool

// CaptureCrashes routes crash output to crash.log, first keeping what the
// previous run left there. It reports whether that run crashed.
func CaptureCrashes() bool {
	dir := platform.AppDir()
	crashedLastTime = captureCrashes(dir)
	if crashedLastTime {
		logx.Printf("the last run crashed; its output is in %s", previousCrashFile(dir))
	}
	return crashedLastTime
}

func captureCrashes(dir string) bool {
	crashed := false
	p := crashFile(dir)
	if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
		_ = os.Remove(previousCrashFile(dir))
		crashed = os.Rename(p, previousCrashFile(dir)) == nil
	}
	f, err := os.Create(p)
	if err != nil {
		return crashed
	}
	defer f.Close() // SetCrashOutput keeps its own handle
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	return crashed
}

// uiErrors caps how many interface errors one run writes to the log.
var uiErrors struct {
	sync.Mutex
	n int
}

// ReportUIError logs an error the interface ran into (an exception or a
// rejected promise), so it shows up in the log and in diagnostics.
func (s *SettingsService) ReportUIError(message string) {
	uiErrors.Lock()
	defer uiErrors.Unlock()
	if uiErrors.n >= 20 {
		return
	}
	uiErrors.n++
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 500 {
		message = message[:500] + "…"
	}
	logx.Printf("interface error: %s", message)
}

// CopyDiagnostics puts a report for a bug report on the clipboard: versions,
// settings, what the library holds, controller, Syncer, updates,
// the last crash and the end of the log. It holds no keys or tokens, and
// the user folder is shortened to %USERPROFILE%.
func (s *SettingsService) CopyDiagnostics() error {
	text := s.c.diagnostics()
	if a := application.Get(); a == nil || !a.Clipboard.SetText(text) {
		return fmt.Errorf("couldn't copy to the clipboard")
	}
	logx.Printf("diagnostics copied (%d characters)", len(text))
	return nil
}

// WriteDiagnostics writes the diagnostics report to the desktop and
// returns its path (Seaglass.exe --diagnostics).
func WriteDiagnostics(c *Core) (string, error) {
	dir := platform.Desktop
	if dir == "" {
		dir = platform.AppDir()
	}
	p := filepath.Join(dir, "Seaglass diagnostics.txt")
	return p, os.WriteFile(p, []byte(c.diagnostics()), 0o644)
}

// ReportProblem opens a new GitHub issue for Seaglass.
func (s *SettingsService) ReportProblem() error {
	return platform.OpenWebPage("https://github.com/ApolloF/Seaglass/issues/new")
}

func (c *Core) diagnostics() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("Seaglass diagnostics, %s", time.Now().Format(time.RFC3339))
	line("")
	line("## Versions")
	exe, _ := os.Executable()
	kind := "installed"
	switch update.KindFor(exe) {
	case update.KindExe:
		kind = "not installed"
	case "":
		kind = "not installed, folder not writable"
	}
	if !update.Valid(c.Version) {
		kind = "development build"
	}
	line("Seaglass %s (%s), %s", c.Version, kind, runtime.Version())
	line("%s", osVersion())
	line("WebView2 %s", webView2Version())
	if i, ok := syncer.Find(); ok {
		line("Syncer %s", orUnknown(i.Version))
	} else {
		line("Syncer not installed")
	}
	st := platform.GetStartup()
	line("Starts with Windows: %v (turned off in Task Manager: %v)", st.On, st.DisabledByUser)
	if crashedLastTime {
		line("The previous run crashed.")
	}

	line("")
	line("## Settings")
	cfg := c.Settings.Get()
	if j, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		line("%s", j)
	}
	line("SteamGridDB key: %v", platform.LoadSecret(sgdbSecret) != "")
	acc := c.owned.accounts()
	line("Store accounts: Steam %v, GOG %v, Epic %v", acc.Steam.Connected, acc.GOG.Connected, acc.Epic.Connected)

	line("")
	line("## Library")
	games := c.Lib.Games()
	bySource := map[string]int{}
	installed, hidden, review, withArt := 0, 0, 0, 0
	for _, g := range games {
		bySource[g.Source]++
		if g.Installed {
			installed++
		}
		if g.Hidden {
			hidden++
		}
		if g.NeedsReview {
			review++
		}
		if g.Meta != nil && g.Meta.Cover != "" {
			withArt++
		}
	}
	line("%d games: %d installed, %d hidden, %d to check, %d with a cover", len(games), installed, hidden, review, withArt)
	sources := make([]string, 0, len(bySource))
	for src, n := range bySource {
		sources = append(sources, fmt.Sprintf("%s %d", src, n))
	}
	sort.Strings(sources)
	line("By source: %s", strings.Join(sources, ", "))
	scan := c.State()
	line("Last scan: %d games in %d ms; game database %d titles", scan.Games, scan.TookMs, c.Manifest.Len())
	if scan.Error != "" {
		line("Scan error: %s", scan.Error)
	}

	line("")
	line("## Controller and session")
	switch ps := c.padState(); {
	case ps.Connected:
		line("Controller: %s (%v), wireless %v, battery %d%%", ps.Name, ps.Kind, ps.Wireless, ps.Battery)
	case ps.Error != "":
		line("Controller: %s", ps.Error)
	default:
		line("Controller: none connected")
	}
	if m := c.padManager(); m != nil {
		in, running := m.InMode()
		line("Controller layer: %s, SDL %s", in, map[bool]string{true: "running", false: "not running"}[running])
	}
	ses := c.Launch.Current()
	if ses.ID > 0 {
		line("Last session: %q, %s via %s, %d s played %s %s", ses.Title, ses.Phase, orUnknown(ses.Route), ses.Seconds, ses.Error, ses.Note)
	}

	line("")
	line("## Updates")
	u := c.updates.state()
	line("%s; latest %s; failed %v %s", u.Status, orUnknown(u.Latest), u.Failed, u.Error)

	if b, err := os.ReadFile(previousCrashFile(platform.AppDir())); err == nil && len(b) > 0 {
		line("")
		line("## Last crash")
		line("%s", tail(string(b), 60))
	}

	line("")
	line("## Log (last 200 lines)")
	line("%s", strings.Join(logx.Tail(200), "\n"))
	return redact(b.String())
}

// redact shortens the user's folder, which carries their Windows user name.
func redact(s string) string {
	if platform.Profile == "" {
		return s
	}
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(platform.Profile))
	s = re.ReplaceAllString(s, "%USERPROFILE%")
	// JSON doubles the backslashes.
	re = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(strings.ReplaceAll(platform.Profile, `\`, `\\`)))
	return re.ReplaceAllString(s, `%USERPROFILE%`)
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
