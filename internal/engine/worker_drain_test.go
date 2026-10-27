package engine

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestEngineIdleIncludesDeferredWorkerCleanup(t *testing.T) {
	state, err := store.Open(filepath.Join(t.TempDir(), "drain.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false))
	if err != nil {
		t.Fatal(err)
	}
	e := New(state, reasoning.Deterministic{}, config.Config{}, log.New(io.Discard, "", 0))
	// Hold the real terminal-cleanup lock. An unknown run returns immediately
	// from process(), retires its active entry, then waits here in its defer.
	e.captureMu.Lock()
	held := true
	t.Cleanup(func() {
		if held {
			e.captureMu.Unlock()
		}
		e.Shutdown()
		if !e.WaitForIdle(time.Second) {
			t.Error("worker did not drain")
		}
		if err := state.Close(); err != nil {
			t.Error(err)
		}
	})
	e.launchProcess("missing-synthetic-run", false)
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.mu.RLock()
		retired := len(e.active) == 0 && e.processWorkers == 1
		e.mu.RUnlock()
		if retired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not reach deferred cleanup")
		}
		time.Sleep(time.Millisecond)
	}
	if e.WaitForIdle(20 * time.Millisecond) {
		t.Fatal("idle was reported while deferred database/capture cleanup was blocked")
	}
	e.captureMu.Unlock()
	held = false
	if !e.WaitForIdle(time.Second) {
		t.Fatal("released worker cleanup never became idle")
	}
}
