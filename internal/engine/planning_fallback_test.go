package engine

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

type incompletePlannerProvider struct {
	reasoning.Deterministic
	failure           error
	evaluationFailure bool
}

func (p *incompletePlannerProvider) Plan(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	_, telemetry, _ := p.Deterministic.Plan(ctx, run, obs, knowledge)
	telemetry.Status = "failed"
	// A partial decision must never queue the follow-up.
	return reasoning.AcquisitionPlan{Decision: "request_follow_up"}, telemetry, p.failure
}

func (p *incompletePlannerProvider) Analyze(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	result, telemetry, err := p.Deterministic.Analyze(ctx, run, obs, knowledge)
	if p.evaluationFailure {
		telemetry.Status = "failed"
		err = errors.New("evaluation fixture failed")
	}
	return result, telemetry, err
}

func TestIncompletePlanningPreservesAcceptedCaptureForEvaluation(t *testing.T) {
	incomplete := &inference.Error{Code: inference.FailureCodeIncomplete, Category: inference.FailureCategoryResponseMissing, Stage: inference.FailureStageProvider, Retry: inference.RetryCaller, ProviderResponseStatus: "incomplete", PartialOutputSeen: true}
	for _, test := range []struct {
		name              string
		failure           error
		evaluationFailure bool
		fallback          bool
		completed         bool
	}{
		{"incomplete", incomplete, false, true, true},
		{"evaluation failure remains failed", incomplete, true, true, false},
		{"authentication remains failed", &inference.Error{Code: inference.FailureCode("authentication_failed"), Stage: inference.FailureStageProvider}, false, false, false},
		{"cancellation remains failed", errors.Join(incomplete, context.Canceled), false, false, false},
		{"untyped failure remains failed", errors.New("fixture failure"), false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
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
			p := &incompletePlannerProvider{failure: test.failure, evaluationFailure: test.evaluationFailure}
			runtime := New(state, p, config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 2}}, log.New(io.Discard, "", 0))
			runtime.RecordHeartbeat(ExpectedHeartbeat())
			session, err := runtime.StartVisibleUpdate(ctx, "Planning fallback fixture")
			if err != nil {
				t.Fatal(err)
			}
			active := waitSession(t, runtime, session.ID, func(v domain.Session) bool { return v.Runs[0].Status == "waiting_for_bridge" })
			run := active.Runs[0]
			command, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
			if err != nil || command == nil {
				t.Fatalf("command=%+v err=%v", command, err)
			}
			obs := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{ScrollY: 1350, Blocks: []domain.Block{{EvidenceKey: "x:fallback-evidence", PlatformID: "x:status:10001", Author: "Fixture", Text: "Accepted bounded candidate"}}}}, Coverage: map[string]any{"performedScrolls": 2, "scrollStopReason": "budget_exhausted", "frontier": map[string]any{"scrollY": float64(1350), "anchorKeys": []any{"x:status:10001"}, "hasMoreCandidateSignal": true, "newCandidateCount": 1}}}
			if _, err = runtime.AcceptObservation(ctx, command.ID, run.ID, obs); err != nil {
				t.Fatal(err)
			}
			terminal := waitSession(t, runtime, session.ID, func(v domain.Session) bool {
				return v.Status == "completed" || v.Status == "failed" || v.Status == "partial"
			})
			if (terminal.Status == "completed") != test.completed {
				t.Fatalf("session=%+v", terminal)
			}
			stored, err := runtime.Run(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			receipt, _ := stored.Coverage["acquisitionPlanning"].(map[string]any)
			if (receipt["fallbackPolicy"] == "evaluate_captured_skip_follow_up") != test.fallback {
				t.Fatalf("receipt=%+v", receipt)
			}
			if integerCoverageValue(stored.Coverage["acquisitionRounds"]) != 1 {
				t.Fatal("partial planner decision queued an additional capture")
			}
			latest, err := runtime.LatestTimelineCheck(ctx)
			if err != nil || latest == nil {
				t.Fatalf("latest=%+v err=%v", latest, err)
			}
			if latest.Diagnostics.CapturedCandidates != 1 {
				t.Fatalf("accepted capture lost: %+v", latest)
			}
			if test.completed && (latest.AddedItems != 1 || latest.Diagnostics.EvaluatedCandidates != 1 || latest.Diagnostics.PlanningFallbackRuns != 1 || latest.Outcome != "added") {
				t.Fatalf("fallback result=%+v diagnostics=%+v", latest, latest.Diagnostics)
			}
			if !test.fallback && latest.Outcome != "planning_failed" {
				t.Fatalf("failed planning was hidden: %+v", latest)
			}
			if test.evaluationFailure && latest.Outcome != "reasoning_failed" {
				t.Fatalf("evaluation failure hidden: %+v", latest)
			}
		})
	}
}
