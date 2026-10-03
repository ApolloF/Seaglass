// Package syncer talks to Syncer (github.com/ApolloF/syncer), which keeps
// game saves in sync between PCs and backs them up. Syncer serves a local
// JSON-RPC 2.0 API on the named pipe \\.\pipe\syncer, one JSON message per
// line; only the current Windows user can connect. When Syncer's window
// isn't open, "Syncer.exe --api" serves it until it's idle.
package syncer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/ApolloF/Seaglass/internal/platform"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\ApolloFSyncer`

// Pipe is where Syncer listens.
const Pipe = `\\.\pipe\syncer`

// Protocol is the API version this client speaks.
const Protocol = 1

// ErrNotInstalled means Syncer isn't installed on this PC.
var ErrNotInstalled = errors.New("Syncer isn't installed")

// Status is Syncer's overall state.
type Status struct {
	Protocol   int       `json:"protocol"`
	Version    string    `json:"version"`
	Window     bool      `json:"window"`
	Syncthing  bool      `json:"syncthing"`
	Paused     bool      `json:"paused"`
	PausedTill time.Time `json:"pausedUntil"`
	BackingUp  bool      `json:"backingUp"`
	LastBackup time.Time `json:"lastBackup"`
	Games      int       `json:"games"`
	Conflicts  int       `json:"conflicts"`
}

// Folder is one save folder Syncer looks after.
type Folder struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Path      string    `json:"path"`
	Sync      bool      `json:"sync"`
	Backup    bool      `json:"backup"`
	State     string    `json:"state"`
	NeedBytes int64     `json:"needBytes"`
	Errors    int       `json:"errors"`
	Conflicts int       `json:"conflicts"`
	Exists    bool      `json:"exists"`
	Modified  time.Time `json:"modified"`
	BackedUp  time.Time `json:"backedUp"`
	NewerOn   string    `json:"newerOn"`
	NewerAt   time.Time `json:"newerAt"`
	// Account: the game has separate saves per account, and this folder
	// holds the saves of the account playing on this PC.
	Account string `json:"account,omitempty"`
}

// Game identifies a game to Syncer.
type Game struct {
	Title      string `json:"title"`
	Dir        string `json:"dir,omitempty"`
	SteamAppID int    `json:"steamAppId,omitempty"`
	GogID      string `json:"gogId,omitempty"`
}

// GameStatus is what Syncer knows about one game's saves.
type GameStatus struct {
	Known   bool     `json:"known"`
	Folders []Folder `json:"folders"`
}

// SyncResult is the outcome of SyncNow for one folder.
type SyncResult struct {
	ID        string `json:"id"`
	Done      bool   `json:"done"`
	State     string `json:"state"`
	NeedBytes int64  `json:"needBytes"`
}

// BackupResult is the outcome of BackupNow.
type BackupResult struct {
	Started  bool      `json:"started"`
	Finished bool      `json:"finished"`
	OK       bool      `json:"ok"`
	At       time.Time `json:"at"`
	Copied   int       `json:"copied"`
	Errors   []string  `json:"errors"`
}

// RPCError is an error Syncer answered with.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return "Syncer: " + e.Message }

// UnknownMethod reports whether Syncer answered that it doesn't have the
// method: it's older than the feature.
func UnknownMethod(err error) bool {
	var e *RPCError
	return errors.As(err, &e) && e.Code == -32601
}

// Account is one person with their own saves in Syncer.
type Account struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color,omitempty"`
	Active bool   `json:"active"`
}

// Accounts are Syncer's accounts: with them on, each person keeps their
// own saves of the games they split.
type Accounts struct {
	Enabled  bool      `json:"enabled"`
	Active   string    `json:"active,omitempty"` // the account playing on this PC
	Accounts []Account `json:"accounts"`
	Split    []struct {
		Game     string   `json:"game"`
		Label    string   `json:"label"`
		Accounts []string `json:"accounts"`
	} `json:"split"`
}

// LauncherData is a launcher's own data folder that Syncer syncs and
// backs up like a game's saves.
type LauncherData struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Sync   bool   `json:"sync"`
	Backup bool   `json:"backup"`
	Added  bool   `json:"added"` // added by this call
	// Dismissed: someone stopped syncing it in Syncer, and it stays that way.
	Dismissed bool `json:"dismissed"`
}

// Client is one connection to Syncer. Safe for concurrent use.
type Client struct {
	conn net.Conn

	mu      sync.Mutex // guards writes and pending
	next    int64
	pending map[int64]chan response
	notify  func(method string)
	closed  chan struct{}
	err     error
}

type response struct {
	Result json.RawMessage
	Error  *RPCError
}

// Dial connects to Syncer. With start, it starts "Syncer.exe --api" when
// Syncer isn't serving the API yet.
func Dial(ctx context.Context, start bool) (*Client, error) {
	c, err := dialPipe(ctx)
	if err == nil || !start {
		return c, err
	}
	inst, ok := Find()
	if !ok {
		return nil, ErrNotInstalled
	}
	if !inst.API() {
		return nil, ErrOutdated // an older Syncer would open its window instead
	}
	cmd := exec.Command(inst.Exe, "--api")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()
	deadline := time.Now().Add(8 * time.Second)
	for {
		c, err = dialPipe(ctx)
		if err == nil || time.Now().After(deadline) || ctx.Err() != nil {
			return c, err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func dialPipe(ctx context.Context) (*Client, error) {
	dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(dctx, Pipe)
	if err != nil {
		return nil, err
	}
	if err := checkServer(conn); err != nil {
		conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, pending: map[int64]chan response{}, closed: make(chan struct{})}
	go c.read()
	return c, nil
}

// checkServer makes sure the pipe is served by a process of this Windows
// user, so another account can't pose as Syncer.
func checkServer(conn net.Conn) error {
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("can't check who serves the Syncer pipe")
	}
	var pid uint32
	if err := getNamedPipeServerProcessID(windows.Handle(f.Fd()), &pid); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var tok windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	theirs, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	mine, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !windows.EqualSid(theirs.User.Sid, mine.User.Sid) {
		return errors.New("the Syncer pipe belongs to another user")
	}
	return nil
}

var procGetNamedPipeServerProcessId = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeServerProcessId")

func getNamedPipeServerProcessID(h windows.Handle, pid *uint32) error {
	r, _, err := procGetNamedPipeServerProcessId.Call(uintptr(h), uintptr(unsafe.Pointer(pid)))
	if r == 0 {
		return err
	}
	return nil
}

// OnNotify sets a function for notifications Syncer sends ("changed").
func (c *Client) OnNotify(fn func(method string)) {
	c.mu.Lock()
	c.notify = fn
	c.mu.Unlock()
}

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.closed }

// Close ends the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) read() {
	sc := bufio.NewScanner(c.conn)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var m struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		c.mu.Lock()
		if m.ID == nil {
			fn := c.notify
			c.mu.Unlock()
			if fn != nil && m.Method != "" {
				fn(m.Method)
			}
			continue
		}
		ch := c.pending[*m.ID]
		delete(c.pending, *m.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- response{m.Result, m.Error}
		}
	}
	c.mu.Lock()
	c.err = errors.New("connection to Syncer closed")
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
	close(c.closed)
}

// Call calls a method and decodes its result into out (which may be nil).
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return c.err
	}
	c.next++
	id := c.next
	ch := make(chan response, 1)
	c.pending[id] = ch
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err == nil {
		_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = c.conn.Write(append(b, '\n')); err != nil {
			// Part of the line may be on the pipe, which would garble
			// every call after it: start over on a new connection.
			_ = c.conn.Close()
		}
	}
	if err != nil {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case r, ok := <-ch:
		if !ok {
			return errors.New("connection to Syncer closed")
		}
		if r.Error != nil {
			return r.Error
		}
		if out != nil && len(r.Result) > 0 {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	}
}

// Status returns Syncer's state.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var s Status
	err := c.Call(ctx, "status", nil, &s)
	if err == nil && s.Protocol != Protocol {
		err = fmt.Errorf("Syncer speaks API version %d; Seaglass needs version %d", s.Protocol, Protocol)
	}
	return s, err
}

// GameStatus returns the save folders Syncer has for a game.
func (c *Client) GameStatus(ctx context.Context, g Game) (GameStatus, error) {
	var s GameStatus
	err := c.Call(ctx, "gameStatus", g, &s)
	return s, err
}

// SyncNow has Syncer bring the folders up to date with the other PCs,
// waiting at most timeout.
func (c *Client) SyncNow(ctx context.Context, ids []string, timeout time.Duration) ([]SyncResult, error) {
	var r []SyncResult
	err := c.Call(ctx, "syncNow", map[string]any{"ids": ids, "timeoutMs": timeout.Milliseconds()}, &r)
	return r, err
}

// BackupNow starts a backup; with wait it returns once it has finished
// (or timeout has passed).
func (c *Client) BackupNow(ctx context.Context, wait bool, timeout time.Duration) (BackupResult, error) {
	var r BackupResult
	err := c.Call(ctx, "backupNow", map[string]any{"wait": wait, "timeoutMs": timeout.Milliseconds()}, &r)
	return r, err
}

// Open shows Syncer's window.
func (c *Client) Open(ctx context.Context) error { return c.Call(ctx, "open", nil, nil) }

// RegisterGames tells Syncer which games this PC has, so it can match
// saves to games with unusual folder names (repacks, external copies).
func (c *Client) RegisterGames(ctx context.Context, games []Game) error {
	return c.Call(ctx, "registerGames", map[string]any{"games": games}, nil)
}

// Subscribe asks for "changed" notifications (see OnNotify).
func (c *Client) Subscribe(ctx context.Context) error { return c.Call(ctx, "subscribe", nil, nil) }

// Accounts returns Syncer's accounts. Syncer before accounts answers
// with an error UnknownMethod recognises.
func (c *Client) Accounts(ctx context.Context) (Accounts, error) {
	var a Accounts
	err := c.Call(ctx, "accounts", nil, &a)
	return a, err
}

// SwitchAccount puts an account's saves in place on this PC. It fails
// while a game runs, or when that account's saves haven't arrived yet.
func (c *Client) SwitchAccount(ctx context.Context, id string) error {
	return c.Call(ctx, "switchAccount", map[string]any{"id": id}, nil)
}

// SyncLauncherData asks Syncer to sync and back up a launcher's own data
// folder (it must be in the user's AppData). Asking again is harmless.
func (c *Client) SyncLauncherData(ctx context.Context, launcher, path string) (LauncherData, error) {
	var d LauncherData
	err := c.Call(ctx, "launcherData", map[string]any{"launcher": launcher, "path": path}, &d)
	return d, err
}

// MinVersion is the first Syncer release with the launcher API.
const MinVersion = "0.11.0"

// ErrOutdated means the installed Syncer is too old for the launcher API.
var ErrOutdated = errors.New("Syncer needs an update (version " + MinVersion + " or newer)")

// Install is where Syncer is installed.
type Install struct {
	Exe     string
	Version string // "" when unknown
}

// API reports whether this Syncer can serve the launcher API.
func (i Install) API() bool { return i.Version != "" && !olderThan(i.Version, MinVersion) }

// Find returns the installed Syncer.
func Find() (Install, bool) {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, uninstallKey, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		icon, _, _ := k.GetStringValue("DisplayIcon")
		ver, _, _ := k.GetStringValue("DisplayVersion")
		k.Close()
		exe := strings.Trim(strings.SplitN(icon, ",", 2)[0], "\" ")
		if strings.EqualFold(filepath.Base(exe), "Syncer.exe") && filepath.IsAbs(exe) && platform.IsFile(exe) {
			return Install{Exe: exe, Version: strings.TrimPrefix(strings.TrimSpace(ver), "v")}, true
		}
	}
	if exe := filepath.Join(platform.Local, "Programs", "Syncer", "Syncer.exe"); platform.IsFile(exe) {
		return Install{Exe: exe}, true
	}
	return Install{}, false
}

// Installed returns Syncer.exe's path when Syncer is installed.
func Installed() (string, bool) {
	i, ok := Find()
	return i.Exe, ok
}

// olderThan compares dotted version numbers ("0.9.2" < "0.11.0").
func olderThan(v, than string) bool {
	a, b := strings.Split(v, "."), strings.Split(than, ".")
	for k := 0; k < max(len(a), len(b)); k++ {
		x, y := 0, 0
		if k < len(a) {
			x, _ = strconv.Atoi(strings.TrimFunc(a[k], func(r rune) bool { return r < '0' || r > '9' }))
		}
		if k < len(b) {
			y, _ = strconv.Atoi(b[k])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// Games lists every save folder Syncer looks after.
func (c *Client) Games(ctx context.Context) ([]Folder, error) {
	var fs []Folder
	err := c.Call(ctx, "games", nil, &fs)
	return fs, err
}
