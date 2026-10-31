//go:build auth_session_dev

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Deliberately ignores cancellation to model a transport completing too late.
// The real owner and Store must enforce authority independently of the adapter.
type lateNativeTransport struct {
	*sourceNativeOffline
	onSeed, onCapture func()
}

func (f *lateNativeTransport) Begin(context.Context, sourceNativeChallenge, time.Time) (sourceNativeCollector, error) {
	return f, nil
}
func (f *lateNativeTransport) Seed(ctx context.Context, c sourceNativeChallenge) (sourceNativePair, error) {
	if f.onSeed != nil {
		f.onSeed()
	}
	return f.sourceNativeOffline.Seed(ctx, c)
}
func (f *lateNativeTransport) Capture(ctx context.Context, payload map[string]any) (domain.Observation, error) {
	if f.onCapture != nil {
		f.onCapture()
	}
	return f.sourceNativeOffline.Capture(ctx, payload)
}

func TestSourceNativeLateStartupCannotIssuePermit(t *testing.T) {
	o, base, run, command := sourceNativeFixture(t)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &lateNativeTransport{sourceNativeOffline: base, onSeed: cancel}
	o.transport = f
	batch, err := o.BeginBatch(parent, parent, 1, run)
	if err == nil || batch == nil {
		t.Fatal("late seed admitted or cleanup obligation lost")
	}
	if err := batch.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := batch.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if base.closed != 1 || base.captures != 0 {
		t.Fatal("partial startup cleanup was not exactly once")
	}
	if _, err := batch.Capture(context.Background(), command); err == nil {
		t.Fatal("closed startup accepted capture")
	}
	observations, err := o.engine.store.Observations(context.Background(), run.ID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("late startup persisted observations: %d, %v", len(observations), err)
	}
}

func TestSourceNativeLateCaptureCannotCommit(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline", "cancel_cleanup_failure"} {
		t.Run(mode, func(t *testing.T) {
			o, base, run, command := sourceNativeFixture(t)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &lateNativeTransport{sourceNativeOffline: base}
			o.transport = f
			batch, err := o.BeginBatch(parent, parent, 1, run)
			if err != nil {
				t.Fatal(err)
			}
			b := batch.(*sourceNativeBatch)
			f.onCapture = func() {
				if mode == "deadline" {
					b.started = b.started.Add(-time.Minute)
				} else {
					cancel()
				}
			}
			_, captureErr := b.Capture(parent, command)
			if captureErr == nil || base.captures != 1 {
				t.Fatal("late observation was not rejected after capture")
			}
			base.cleanup = mode == "cancel_cleanup_failure"
			original := b.failure
			closeErr := b.Close(context.Background())
			if (closeErr != nil) != base.cleanup {
				t.Fatalf("cleanup outcome: %v", closeErr)
			}
			if original == nil || b.failure != original {
				t.Fatal("capture cause lost during cleanup")
			}
			if _, err := b.Capture(context.Background(), command); err == nil {
				t.Fatal("late claim replay accepted")
			}
			observations, err := o.engine.store.Observations(context.Background(), run.ID)
			if err != nil || len(observations) != 0 {
				t.Fatalf("late capture persisted observations: %d, %v", len(observations), err)
			}
			if base.closed != 1 {
				t.Fatal("collector cleanup not exactly once")
			}
		})
	}
}
