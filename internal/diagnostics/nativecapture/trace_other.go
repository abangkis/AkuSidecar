//go:build !windows

package nativecapture

import (
	"context"
	"time"
)

func observe(_ context.Context, _ uint32, ready chan struct{}, emit func(Sample)) {
	close(ready)
	emit(Sample{At: time.Now().UTC().Format(time.RFC3339Nano), Trigger: "trace_start", Status: "unsupported_platform", Windows: []Window{}})
}
