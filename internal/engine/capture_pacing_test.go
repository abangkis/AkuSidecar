package engine

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"
)

type pacingTestClock struct{ now time.Time }

func (c *pacingTestClock) Now() time.Time { return c.now }

func TestCapturePacingEngineClaimPayloadAndStatus(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "enabled"}[enabled], func(t *testing.T) {
			ctx := context.Background()
			clock := &pacingTestClock{now: time.Now().UTC()}
			settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
			settings.ActiveSources = []domain.Source{domain.SourceX}
			settings.CalibrationEnabled = false
			state, err := store.OpenWithClock(filepath.Join(t.TempDir(), "synthetic.db"), settings, clock)
			if err != nil {
				t.Fatal(err)
			}
			defer state.Close()
			if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 1, Behavior: config.CaptureBehaviorConfig{Enabled: enabled, ScrollFraction: .6, ScrollSettleMS: 1200}}}
			e := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
			e.RecordHeartbeat(ExpectedHeartbeat())
			session, err := e.StartVisibleUpdate(ctx, "pacing fixture")
			if err != nil {
				t.Fatal(err)
			}
			command, err := e.ClaimCommand(ctx, session.Runs[0].ID, "fixture")
			if err != nil || command == nil {
				t.Fatalf("claim %v %v", command, err)
			}
			fraction, settle := .75, float64(900)
			if enabled {
				fraction = .6
				settle = 1200
			}
			if command.Payload["scrollFraction"] != fraction || command.Payload["scrollSettleMs"] != settle {
				t.Fatalf("payload %+v", command.Payload)
			}
			if err := e.CancelSession(ctx, session.ID); err != nil {
				t.Fatal(err)
			}
			next, err := e.StartVisibleUpdate(ctx, "next fixture")
			if err != nil {
				t.Fatal(err)
			}
			defer e.CancelSession(ctx, next.ID)
			command, err = e.ClaimCommand(ctx, next.Runs[0].ID, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			status, err := e.AutoUpdateStatus(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !enabled {
				if command == nil || len(status.CaptureBehavior) != 0 {
					t.Fatal("legacy changed")
				}
				return
			}
			if command != nil || len(status.CaptureBehavior) != 1 || status.CaptureBehavior[0].Reason != "capture_spacing" {
				t.Fatalf("defer %v %+v", command, status.CaptureBehavior)
			}
			clock.now, err = time.Parse(time.RFC3339Nano, status.CaptureBehavior[0].NotBefore)
			if err != nil {
				t.Fatal(err)
			}
			command, err = e.ClaimCommand(ctx, next.Runs[0].ID, "fixture")
			if err != nil || command == nil {
				t.Fatalf("eligible %v %v", command, err)
			}
		})
	}
}
