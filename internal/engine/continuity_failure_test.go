package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func persistedFailedContinuityBlock(t *testing.T) domain.Block {
	t.Helper()
	raw, err := os.ReadFile("../store/testdata/candidate-evaluation-continuity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct{ Block domain.Block }
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Cases[0].Block
}

func runContinuityUpdate(t *testing.T, e *Engine, block domain.Block) domain.Session {
	t.Helper()
	ctx := context.Background()
	session, err := e.StartVisibleUpdate(ctx, "offline persisted continuity replay")
	if err != nil {
		t.Fatal(err)
	}
	active := waitSession(t, e, session.ID, func(s domain.Session) bool { return s.Runs[0].Status == "waiting_for_bridge" })
	run := active.Runs[0]
	command, err := e.ClaimCommand(ctx, run.ID, "continuity-fixture")
	if err != nil || command == nil {
		t.Fatalf("claim=%v err=%v", command, err)
	}
	observation := domain.Observation{Source: run.Source, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}},
		Coverage:  map[string]any{"status": "complete", "continuityEvaluationPending": []string{block.EvidenceKey}}}
	if _, err := e.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	return waitSession(t, e, session.ID, func(s domain.Session) bool { return s.Status == "completed" || s.Status == "failed" })
}

func TestEngineCandidate503ThenSuccessfulVersionSkipsOnlyAfterEvaluation(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "not-selected", true: "selected"}[selected], func(t *testing.T) {
			var calls atomic.Int32
			provider := &candidateEvaluationProvider{analyze: func(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
				if calls.Add(1) == 1 {
					return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, errors.New("persisted Candidate Evaluation HTTP 503 fixture")
				}
				result, telemetry, err := candidateEvaluationResult(ctx, run, observation, knowledge)
				if !selected {
					for i := range result.CandidateAssessments {
						result.CandidateAssessments[i].EvidenceStrength = .01
					}
				}
				return result, telemetry, err
			}}
			e, state := singleSourceEngine(t, provider)
			settings, err := state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings.AIDetectionEnabled = false
			settings.PreferenceEligibilityMode = "rank_only"
			if err := state.SaveSettings(context.Background(), settings); err != nil {
				t.Fatal(err)
			}
			block := persistedFailedContinuityBlock(t)
			first := runContinuityUpdate(t, e, block)
			if first.Status != "failed" || calls.Load() != 1 {
				t.Fatalf("first=%s calls=%d", first.Status, calls.Load())
			}
			second := runContinuityUpdate(t, e, block)
			if second.Status != "completed" || calls.Load() != 2 {
				t.Fatalf("retry=%s calls=%d", second.Status, calls.Load())
			}
			trace, err := state.InboxRunTrace(context.Background(), second.Runs[0].ID, "evaluated", 20, 0)
			if err != nil || trace.Counts.Evaluated != 1 || trace.Counts.Skipped != 0 || trace.Items[0].Selected != selected {
				t.Fatalf("retry trace=%+v err=%v", trace, err)
			}
			third := runContinuityUpdate(t, e, block)
			trace, err = state.InboxRunTrace(context.Background(), third.Runs[0].ID, "skipped", 20, 0)
			if err != nil || calls.Load() != 2 || trace.Counts.Skipped != 1 || trace.Total != 1 {
				t.Fatalf("completed version not skipped: calls=%d trace=%+v err=%v", calls.Load(), trace, err)
			}
		})
	}
}

func TestEngineRepeated503RetriesOnlyOnNextUpdate(t *testing.T) {
	var calls atomic.Int32
	provider := &candidateEvaluationProvider{analyze: func(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
		calls.Add(1)
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, errors.New("persisted HTTP 503")
	}}
	e, _ := singleSourceEngine(t, provider)
	for attempt := int32(1); attempt <= 3; attempt++ {
		s := runContinuityUpdate(t, e, persistedFailedContinuityBlock(t))
		if s.Status != "failed" || calls.Load() != attempt {
			t.Fatalf("attempt %d status=%s calls=%d", attempt, s.Status, calls.Load())
		}
	}
}

func TestEngineDeliveredVersionFailedMaterialUpdateRemainsDeliverable(t *testing.T) {
	var calls atomic.Int32
	provider := &candidateEvaluationProvider{analyze: func(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
		n := calls.Add(1)
		if n == 2 {
			return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, errors.New("persisted HTTP 503 on version two")
		}
		result, telemetry, err := candidateEvaluationResult(ctx, run, observation, knowledge)
		for i := range result.Items {
			result.Items[i].KnowledgeDelta = "material_update"
			result.Items[i].EventKey = run.ID
		}
		return result, telemetry, err
	}}
	e, state := singleSourceEngine(t, provider)
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled = false
	settings.PreferenceEligibilityMode = "rank_only"
	if err := state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	block := persistedFailedContinuityBlock(t)
	first := runContinuityUpdate(t, e, block)
	if len(first.Items) != 1 {
		t.Fatalf("version one delivery=%+v", first)
	}
	stored, err := state.GetRun(context.Background(), first.Runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(stored.Coverage["continuityEvaluationPending"])
	if string(encoded) != "[]" {
		t.Fatalf("capture forged pending receipt: %s", encoded)
	}
	block.Text += " A material new development changes this source version."
	if failed := runContinuityUpdate(t, e, block); failed.Status != "failed" {
		t.Fatal("version two failure missing")
	}
	retry := runContinuityUpdate(t, e, block)
	if retry.Status != "completed" || len(retry.Items) != 1 || calls.Load() != 3 {
		t.Fatalf("version two retry not delivered: status=%s items=%d calls=%d", retry.Status, len(retry.Items), calls.Load())
	}
	keys, err := state.ResurfaceSemanticReevaluationKeys(context.Background(), retry.ID)
	if err != nil || !keys[block.EvidenceKey] {
		t.Fatalf("material version retry lost semantic replay bypass: keys=%v err=%v", keys, err)
	}
}

func TestEngineEvaluateAllPreservesPreviouslyDeliveredExclusion(t *testing.T) {
	var calls atomic.Int32
	provider := &candidateEvaluationProvider{analyze: func(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
		calls.Add(1)
		return candidateEvaluationResult(ctx, run, observation, knowledge)
	}}
	e, state := singleSourceEngine(t, provider)
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled, settings.PreferenceEligibilityMode = false, "rank_only"
	if err := state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	block := persistedFailedContinuityBlock(t)
	if first := runContinuityUpdate(t, e, block); len(first.Items) != 1 {
		t.Fatal("first delivery missing")
	}
	settings.ResurfaceMode = "evaluate_all"
	if err := state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	second := runContinuityUpdate(t, e, block)
	trace, err := state.InboxRunTrace(context.Background(), second.Runs[0].ID, "evaluated", 20, 0)
	if err != nil || calls.Load() != 2 || trace.Total != 1 || trace.Items[0].Selected || len(second.Items) != 0 {
		t.Fatalf("evaluate-all delivery exclusion changed: calls=%d trace=%+v items=%d err=%v", calls.Load(), trace, len(second.Items), err)
	}
}
