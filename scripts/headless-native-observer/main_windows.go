//go:build windows

// Passive QA companion. It reads native window metadata only; it never changes
// windows or reads titles, content, input or screenshots. Not a product runtime.
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/nativetrace"
)

func main() {
	var mu sync.Mutex
	encoder := json.NewEncoder(os.Stdout)
	write := func(value any) { mu.Lock(); defer mu.Unlock(); _ = encoder.Encode(value) }
	ended := make(chan struct{})
	var once sync.Once
	var observer nativetrace.Manager
	// Start before Chrome launches. Raw PIDs permit later attribution to the
	// exact init receipt PID without guessing ownership of other Chrome windows.
	observer.Start("headless-parity", 0, func(sample nativetrace.Sample) {
		write(map[string]any{"type": "sample", "sample": sample})
		if sample.Trigger == "trace_end" {
			once.Do(func() { close(ended) })
		}
	})
	write(map[string]any{"type": "ready", "maxDurationMs": nativetrace.MaxDuration.Milliseconds()})
	scanner := bufio.NewScanner(os.Stdin)
	_ = scanner.Scan() // One stop line or EOF ends the passive observer.
	observer.Stop()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
	}
}
