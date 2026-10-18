package syncer

import (
	"context"

	"github.com/ApolloF/Seaglass/internal/platform"
)

// dialPipe fails on Linux: Syncer is Windows-only and serves its API on a
// Windows named pipe. A port would use a Unix socket under $XDG_RUNTIME_DIR.
func dialPipe(context.Context) (*Client, error) { return nil, platform.ErrNotSupported }

// Find reports Syncer as not installed: there is no Linux build.
func Find() (Install, bool) { return Install{}, false }
