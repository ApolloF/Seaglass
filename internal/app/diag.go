package app

import (
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
)

// heapDiag writes a Go heap profile and logs the runtime's memory numbers
// when WL_HEAPPROFILE names a file: a minute after start and a minute
// after a game starts (when the interface is closed). For attributing the
// core's memory; off unless asked for.
func heapDiag(when string) {
	path := os.Getenv("WL_HEAPPROFILE")
	if path == "" {
		return
	}
	time.AfterFunc(time.Minute, func() {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		logx.Printf("memory (%s): heap in use %d KB, heap held %d KB, returned %d KB, stacks %d KB, runtime total %d KB, goroutines %d",
			when, m.HeapAlloc>>10, (m.HeapSys-m.HeapReleased)>>10, m.HeapReleased>>10, m.StackSys>>10, m.Sys>>10, runtime.NumGoroutine())
		f, err := os.Create(path + "." + when + ".pprof")
		if err != nil {
			return
		}
		defer f.Close()
		_ = pprof.WriteHeapProfile(f)
	})
}
