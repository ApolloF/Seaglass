package app

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/ApolloF/Seaglass/internal/emulated"
	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/logx"
)

// consoleSearchLimit is as long as a search for console games may walk
// folders; files that hang (a network drive) aren't read at all.
const consoleSearchLimit = 15 * time.Second

// consoleSearch looks for console games beside a scan, one search at a
// time. The PC games never wait for it: a search still going when they're
// done comes in later, and a search that hangs isn't started again by
// every scan.
type consoleSearch struct {
	find    func(context.Context) ([]emulated.Game, error) // emulated.Find
	running atomic.Bool
}

type consoleResult struct {
	games []emulated.Game
	err   error // emulated.ErrIncomplete: games not in games may be there
}

// start begins a search. It returns nil when one is still going.
func (s *consoleSearch) start(ctx context.Context) <-chan consoleResult {
	if !s.running.CompareAndSwap(false, true) {
		return nil
	}
	ch := make(chan consoleResult, 1)
	go func() {
		defer s.running.Store(false)
		ctx, cancel := context.WithTimeout(ctx, consoleSearchLimit)
		defer cancel()
		gs, err := s.find(ctx)
		ch <- consoleResult{gs, err}
	}()
	return ch
}

// consoleResultNow is the search's result if it's done; it doesn't wait.
func consoleResultNow(ch <-chan consoleResult) (consoleResult, bool) {
	select {
	case r := <-ch:
		return r, true
	default:
		return consoleResult{}, false
	}
}

// lookedFor is which library games a scan looked for: all of them, unless
// it looked for console games and didn't get a full answer in time. Then
// the console games it may have missed stay as they are: all of them
// before it's done, those in the folders it didn't get through after.
func lookedFor(detect, ready bool, err error) func(key string) bool {
	switch {
	case !detect || ready && err == nil:
		return nil
	case !ready:
		return func(k string) bool { return !emulated.IsKey(k) }
	}
	return func(k string) bool { return !emulated.IsKey(k) || !emulated.Missed(err, k) }
}

// lateConsoles adds a search's console games once it's done, after the
// scan it was started with already showed the PC games.
func (c *Core) lateConsoles(ch <-chan consoleResult) {
	var r consoleResult
	select {
	case r = <-ch:
	case <-c.ctx.Done():
		return
	}
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if c.ctx.Err() != nil || !c.Settings.Get().DetectEmulators {
		return
	}
	found := make([]library.Found, 0, len(r.games))
	for _, g := range r.games {
		found = append(found, consoleFound(g))
	}
	looked := func(k string) bool { return emulated.IsKey(k) && !emulated.Missed(r.err, k) }
	added, removed := c.Lib.ApplyScanOf(found, time.Now(), looked)
	logx.Printf("scan: %d console games (%d new, %d gone), after the PC games", len(found), added, removed)
	c.setState(func(s *ScanState) { s.Games, s.Added = s.Games+len(found), s.Added+added })
	c.emit(EventLibraryChanged, "scan")
	c.meta.queueMissing()
	if added > 0 || removed > 0 {
		go c.registerWithSyncer()
	}
}
