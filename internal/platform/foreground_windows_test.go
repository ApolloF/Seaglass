package platform

import "testing"

// The hook starts and stops cleanly, and can be started again.
func TestWatchForegroundLifecycle(t *testing.T) {
	for i := 0; i < 3; i++ {
		stop, err := WatchForeground(func(uint32) {})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := WatchForeground(func(uint32) {}); err == nil {
			t.Error("a second watcher started alongside the first")
		}
		stop()
		stop() // twice is fine
	}
}
