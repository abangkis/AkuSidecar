package engine

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestMergedObservationsUseOnlyLatestCaptureTelemetry(t *testing.T) {
	first := domain.Observation{Source: domain.SourceX, Coverage: map[string]any{
		"performedScrolls": float64(2), "scrollStopReason": "budget_exhausted",
		"frontier":       map[string]any{"scrollY": float64(1350), "anchorKeys": []any{"first"}},
		"captureQuality": map[string]any{"verdict": "complete"},
	}}
	last := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{ScrollY: 2025}}, Coverage: map[string]any{
		"performedScrolls": float64(1), "scrollStopReason": "no_movement",
		"frontier": map[string]any{"scrollY": float64(2025), "anchorKeys": []any{"last"}},
	}}
	merged := mergeObservations([]domain.Observation{first, last})
	mergeDurableRunCoverage(merged.Coverage, map[string]any{
		"frontier": first.Coverage["frontier"], "performedScrolls": float64(2), "captureQuality": first.Coverage["captureQuality"],
		"acquisitionPlanning": map[string]any{"decision": "request_follow_up"},
	})
	if _, exists := merged.Coverage["captureQuality"]; exists {
		t.Fatal("missing latest quality was replaced with stale quality")
	}
	if merged.Coverage["performedScrolls"] != float64(1) || merged.Coverage["scrollStopReason"] != "no_movement" {
		t.Fatalf("latest telemetry lost: %+v", merged.Coverage)
	}
	if integerCoverageValue(continuationFrom(merged)["startScrollY"]) != 2025 {
		t.Fatal("continuation used stale position")
	}
	if merged.Coverage["acquisitionPlanning"] == nil {
		t.Fatal("durable orchestration receipt lost")
	}
	merged.Coverage["sentinel"] = true
	if last.Coverage["sentinel"] != nil {
		t.Fatal("merge changed original coverage")
	}
	unknown := mergeObservations([]domain.Observation{first, {Source: domain.SourceX}})
	mergeDurableRunCoverage(unknown.Coverage, first.Coverage)
	for _, key := range []string{"frontier", "performedScrolls", "captureQuality", "scrollStopReason"} {
		if _, exists := unknown.Coverage[key]; exists {
			t.Fatalf("missing latest %s resurrected from a prior round", key)
		}
	}
	for _, readiness := range []any{false, nil, "true"} {
		merged.Coverage["frontier"].(map[string]any)["continuationReady"] = readiness
		if continuationFrom(merged) != nil {
			t.Fatalf("invalid continuation accepted: %v", readiness)
		}
	}
}

type frontierTelemetryProvider struct {
	reasoning.Deterministic
	planned chan domain.Observation
	mode    string
}

func (p *frontierTelemetryProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	plan, telemetry, err := p.Deterministic.Plan(ctx, run, observation, knowledge)
	p.planned <- observation
	frontier, _ := observation.Coverage["frontier"].(map[string]any)
	if booleanCoverageValue(frontier["hasMoreCandidateSignal"]) && integerCoverageValue(frontier["newCandidateCount"]) > 0 && observation.Coverage["captureMode"] == p.mode {
		plan.Decision = "request_follow_up"
		plan.Reason = "Known unfinished capture frontier with retained continuation"
	}
	return plan, telemetry, err
}

func TestCaptureTelemetryReachesPlannerAndBoundedFollowUp(t *testing.T) {
	for _, mode := range []string{"headless_worker", "browser"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
			settings.ActiveSources = []domain.Source{domain.SourceX}
			settings.CalibrationEnabled = false
			settings.AIDetectionEnabled = false
			state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { state.Close() })
			if _, err = state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
				t.Fatal(err)
			}
			provider := &frontierTelemetryProvider{planned: make(chan domain.Observation, 2), mode: mode}
			runtime := New(state, provider, config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 2}}, log.New(io.Discard, "", 0))
			runtime.RecordHeartbeat(ExpectedHeartbeat())
			session, err := runtime.StartVisibleUpdate(ctx, "Telemetry follow-up regression")
			if err != nil {
				t.Fatal(err)
			}
			active := waitSession(t, runtime, session.ID, func(v domain.Session) bool { return v.Runs[0].Status == "waiting_for_bridge" })
			run := active.Runs[0]
			command, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
			if err != nil || command == nil {
				t.Fatalf("initial command=%+v err=%v", command, err)
			}
			initial := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{ScrollY: 1350, Blocks: []domain.Block{{EvidenceKey: "x:telemetry-first", PlatformID: "x:status:10001", Author: "First", Text: "First bounded source candidate"}}}}, Coverage: map[string]any{
				"captureMode": mode, "performedScrolls": float64(2), "scrollStopReason": "budget_exhausted",
				"captureQuality": map[string]any{"verdict": "usable_degraded"},
				"frontier":       map[string]any{"scrollY": float64(1350), "newCandidateCount": float64(1), "hasMoreCandidateSignal": true, "anchorKeys": []any{"x:status:10001"}, "continuationReady": true},
			}}
			if _, err = runtime.AcceptObservation(ctx, command.ID, run.ID, initial); err != nil {
				t.Fatal(err)
			}
			waitSession(t, runtime, session.ID, func(v domain.Session) bool { return v.Runs[0].Stage == "follow_up_capture" || v.Status == "completed" })
			planned := <-provider.planned
			if integerCoverageValue(planned.Coverage["performedScrolls"]) != 2 {
				t.Fatal("planner lost actual scroll count")
			}
			followUp, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
			if err != nil || followUp == nil {
				t.Fatalf("follow-up command=%+v err=%v", followUp, err)
			}
			continuation := followUp.Payload["continuation"].(map[string]any)
			if continuation["startScrollY"] != float64(1350) {
				t.Fatalf("continuation=%+v", continuation)
			}
			if followUp.Payload["sourceFreshnessPolicy"] != "preserve_frontier" || followUp.Payload["pendingContentPolicy"] != "detect_only" || followUp.Payload["sameTabMutationAllowed"] != false {
				t.Fatalf("round2 freshness=%+v", followUp.Payload)
			}
			second := initial
			second.Coverage = map[string]any{"captureMode": mode, "performedScrolls": float64(0), "scrollStopReason": "no_movement", "frontier": map[string]any{"scrollY": float64(2025), "newCandidateCount": float64(0), "hasMoreCandidateSignal": false, "anchorKeys": []any{"x:status:10002"}}}
			second.Snapshots = []domain.Snapshot{{ScrollY: 2025, Blocks: []domain.Block{{EvidenceKey: "x:telemetry-second", PlatformID: "x:status:10002", Author: "Second", Text: "A second adjacent source candidate"}}}}
			if _, err = runtime.AcceptObservation(ctx, followUp.ID, run.ID, second); err != nil {
				t.Fatal(err)
			}
			completed := waitSession(t, runtime, session.ID, func(v domain.Session) bool { return v.Status == "completed" })
			if len(completed.Items) != 2 {
				t.Fatalf("merged candidates=%+v", completed.Items)
			}
			stored, err := runtime.Run(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if integerCoverageValue(stored.Coverage["acquisitionRounds"]) != 2 || stored.Coverage["scrollStopReason"] != "no_movement" {
				t.Fatalf("last round lost: %+v", stored.Coverage)
			}
			if len(provider.planned) != 0 {
				t.Fatal("planner exceeded bounded follow-up")
			}
		})
	}
}
