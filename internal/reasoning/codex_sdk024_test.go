package reasoning

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

// Exercise the published SDK through Sidecar's real transport selection and
// bound-client cache, using only the existing fake App Server subprocess.
func TestCodexQueuedDeadlinePreservesActiveInvocation(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
	}{{"single", 0}, {"pool", 1}} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
			t.Setenv("AKU_FAKE_CODEX_HANG_TURN", "2")
			marker := filepath.Join(t.TempDir(), "active-turn")
			t.Setenv("AKU_FAKE_CODEX_HANG_MARKER", marker)
			provider, err := NewCodexAppServer(config.Config{
				Root: filepathRoot(t),
				Reasoning: config.ReasoningConfig{
					Executable: os.Args[0], TimeoutMS: 10000, CodexSessionPoolSize: test.size,
					Planning:   config.ModelConfig{Model: "gpt-5.6-luna", Effort: "high"},
					Evaluation: config.ModelConfig{Model: "gpt-5.6-luna", Effort: "high"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = provider.Close() })
			run, observation := fakeAppServerInput()
			if _, _, err := provider.Plan(context.Background(), run, observation, nil); err != nil {
				t.Fatalf("warm invocation: %v", err)
			}

			activeCtx, cancelActive := context.WithCancel(context.Background())
			defer cancelActive()
			activeDone := make(chan error, 1)
			go func() {
				_, _, err := provider.Plan(activeCtx, run, observation, nil)
				activeDone <- err
			}()
			ready := false
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
				if _, err := os.Stat(marker); err == nil {
					ready = true
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !ready {
				t.Fatal("fake active invocation did not enter its held turn")
			}

			queuedCtx, cancelQueued := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancelQueued()
			queuedDone := make(chan error, 1)
			go func() {
				_, _, err := provider.Plan(queuedCtx, run, observation, nil)
				queuedDone <- err
			}()
			select {
			case err := <-queuedDone:
				var failure *inference.Error
				if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &failure) ||
					failure.Code != inference.FailureCodeTimeout || failure.Stage != inference.FailureStageQueue {
					t.Fatalf("queued invocation lost its deadline/queue category: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("queued invocation ignored its deadline")
			}
			select {
			case err := <-activeDone:
				t.Fatalf("queued deadline interrupted the active owner: %v", err)
			default:
			}

			cancelActive()
			select {
			case err := <-activeDone:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("active cancellation lost context identity: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("active invocation did not stop after cancellation")
			}
			if plan, _, err := provider.Plan(context.Background(), run, observation, nil); err != nil || plan.Decision != "finish" {
				t.Fatalf("fresh invocation after cancellation: decision=%s err=%v", plan.Decision, err)
			}
		})
	}
}
