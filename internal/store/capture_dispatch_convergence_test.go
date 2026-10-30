package store

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestCaptureDispatchOwnershipAndExpiry(t *testing.T) {
	for _, route := range []string{"bridge", "headless", "browser_quiet_hidden"} {
		t.Run(route, func(t *testing.T) {
			s := openTestStore(t)
			ctx := context.Background()
			settings, _ := s.GetSettings(ctx)
			session, err := createVisibleUpdateSession(s, ctx, "synthetic dispatch", settings)
			if err != nil {
				t.Fatal(err)
			}
			run, err := s.AdvanceSession(ctx, session.ID)
			if err != nil || run == nil {
				t.Fatalf("advance: %v", err)
			}
			command, err := s.StartRun(ctx, run.ID, map[string]any{"captureCollector": map[string]any{"version": 1, "backend": route}})
			if err != nil {
				t.Fatal(err)
			}
			stored, err := s.GetRun(ctx, run.ID)
			if err != nil || stored.CaptureCollector != route {
				t.Fatalf("route projection: %+v %v", stored, err)
			}
			snapshot, err := s.GetSession(ctx, session.ID)
			if err != nil || snapshot.Runs[0].CaptureCollector != route {
				t.Fatalf("session projection: %v", err)
			}
			created, _ := time.Parse(time.RFC3339Nano, command.CreatedAt)
			early, err := s.ExpiredBridgeCommands(ctx, created.Add(bridgeDispatchWait-time.Nanosecond))
			if err != nil || len(early) != 0 {
				t.Fatalf("early expiry: %v %v", early, err)
			}
			deadline := created.Add(bridgeDispatchWait)
			expired, err := s.ExpiredBridgeCommands(ctx, deadline)
			if err != nil {
				t.Fatal(err)
			}
			if route != "bridge" {
				if len(expired) != 0 {
					t.Fatal("native/quiet work expired as Browser dispatch")
				}
				return
			}
			if len(expired) != 1 || expired[0].ID != command.ID {
				t.Fatalf("expiry missing: %+v", expired)
			}
			// Public Bridge failure submission must still require a claimed command.
			failure := domain.Failure{Code: "capture_dispatch_timeout", Stage: "capture", Message: "synthetic"}
			if _, err = s.FailCommand(ctx, command.ID, run.ID, failure); err == nil {
				t.Fatal("public failure accepted unclaimed command")
			}
			if _, err = s.ExpireQueuedBridgeCommand(ctx, command.ID, run.ID, failure, deadline); err != nil {
				t.Fatal(err)
			}
			if c, err := s.ClaimCommand(ctx, run.ID, "late"); err != nil || c != nil {
				t.Fatalf("late claim: %+v %v", c, err)
			}
			stored, _ = s.GetRun(ctx, run.ID)
			if stored.Status != "failed" || stored.Error.Code != "capture_dispatch_timeout" {
				t.Fatalf("terminal state: %+v", stored)
			}
		})
	}
}

func TestQueuedExpiryDoesNotFailCommandClaimedBeforeDeadline(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	settings, _ := s.GetSettings(ctx)
	session, err := createVisibleUpdateSession(s, ctx, "race", settings)
	if err != nil {
		t.Fatal(err)
	}
	run, _ := s.AdvanceSession(ctx, session.ID)
	command, err := s.StartRun(ctx, run.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.ClaimCommand(ctx, run.ID, "bridge"); err != nil || c == nil {
		t.Fatalf("claim: %v", err)
	}
	_, err = s.ExpireQueuedBridgeCommand(ctx, command.ID, run.ID, domain.Failure{Code: "capture_dispatch_timeout"}, time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("stale queue snapshot failed a claimed command")
	}
	got, _ := s.GetRun(ctx, run.ID)
	if got.BridgeCommandStatus != "claimed" {
		t.Fatal(got.BridgeCommandStatus)
	}
}

func TestNativeLifecycleProjectionBeforeCommandDoesNotGrantClaim(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	settings, _ := s.GetSettings(ctx)
	session, err := createVisibleUpdateSession(s, ctx, "native reader failure", settings)
	if err != nil {
		t.Fatal(err)
	}
	id := session.Runs[0].ID
	if err = s.SetRunCoverageField(ctx, id, "captureLifecycleOwner", "native_daily_v1"); err != nil {
		t.Fatal(err)
	}
	run, err := s.GetRun(ctx, id)
	if err != nil || run.CaptureCollector != "headless" {
		t.Fatalf("projection: %+v %v", run, err)
	}
	if command, err := s.ClaimCommand(ctx, id, "ordinary"); err != nil || command != nil {
		t.Fatalf("projection granted capture: %+v %v", command, err)
	}
}
