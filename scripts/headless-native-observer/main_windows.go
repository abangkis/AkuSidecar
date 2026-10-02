//go:build windows

// Passive QA companion. It reads native window metadata only; it never changes
// windows or reads titles, content, input or screenshots. Not a product runtime.
package main

import (
	"bufio"
	"context"
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
	write(map[string]any{"type": "ready", "maxDurationMs": nativetrace.MaxDuration.Milliseconds(), "rootBindingSupported": true})
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 256), 1024)
	var cancelRoot context.CancelFunc
	var rootDone chan struct{}
	for scanner.Scan() {
		if scanner.Text() == "stop" {
			break
		}
		var binding struct {
			Type string `json:"type"`
			PID  uint32 `json:"pid"`
		}
		if json.Unmarshal(scanner.Bytes(), &binding) != nil || binding.Type != "bind_root" || binding.PID == 0 || cancelRoot != nil {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), nativetrace.MaxDuration)
		cancelRoot = cancel
		rootDone = make(chan struct{})
		ready := make(chan struct{})
		go observeRoot(ctx, binding.PID, write, ready, rootDone)
		<-ready
		write(map[string]any{"type": "root_ready", "pid": binding.PID})
	}
	if cancelRoot != nil {
		cancelRoot()
		select {
		case <-rootDone:
		case <-time.After(2 * time.Second):
		}
	}
	observer.Stop()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
	}
}
