package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func testEngine(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 2}}
	runtime := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(ExpectedHeartbeat())
	return runtime, state
}

func TestShutdownCancelsButRetainsActiveWorkUntilWorkerExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &Engine{active: map[string]context.CancelFunc{"work": cancel}}
	runtime.Shutdown()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("shutdown did not cancel active work")
	}
	if runtime.WaitForIdle(50 * time.Millisecond) {
		t.Fatal("work must remain registered until its worker exits")
	}
	runtime.mu.Lock()
	delete(runtime.active, "work")
	runtime.mu.Unlock()
	if !runtime.WaitForIdle(50 * time.Millisecond) {
		t.Fatal("runtime did not become idle after worker exit")
	}
}

type shutdownBlockingProvider struct {
	reasoning.Deterministic
	started chan struct{}
	once    sync.Once
}

func TestProgressiveWaitStartsNextCaptureWhileReasoningContinues(t *testing.T) {
	ctx := context.Background()
	runtime, _ := testEngine(t)
	provider := &shutdownBlockingProvider{started: make(chan struct{})}
	runtime.provider = provider
	session, err := runtime.StartSession(ctx, "progressive sources")
	if err != nil {
		t.Fatal(err)
	}
	first := session.Runs[0]
	command, err := runtime.ClaimCommand(ctx, first.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	observation := domain.Observation{Source: first.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:progressive", Text: "First source"}}}}, Coverage: map[string]any{"status": "complete"}}
	if _, err := runtime.AcceptObservation(ctx, command.ID, first.ID, observation); err != nil {
		t.Fatal(err)
	}
	progressive := waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return value.Runs[0].Status == "reasoning" && value.Runs[1].Status == "waiting_for_bridge"
	})
	if progressive.Coverage["sourceWaitMode"] != "progressive_wait" {
		t.Fatalf("session did not preserve scheduling mode: %+v", progressive.Coverage)
	}
	if _, err := runtime.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	barrier, err := runtime.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if barrier.Status != "running" || barrier.Coverage["pipelineStage"] != nil {
		t.Fatalf("global pipeline crossed nonterminal source barrier: %+v", barrier)
	}
	runtime.Shutdown()
	if !runtime.WaitForIdle(time.Second) {
		t.Fatal("progressive reasoning did not stop")
	}
}

func TestFullWaitKeepsNextSourceQueuedUntilReasoningCompletes(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.SourceWaitMode = "full_wait"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	provider := &shutdownBlockingProvider{started: make(chan struct{})}
	runtime.provider = provider
	session, err := runtime.StartSession(ctx, "full wait sources")
	if err != nil {
		t.Fatal(err)
	}
	first := session.Runs[0]
	command, err := runtime.ClaimCommand(ctx, first.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	observation := domain.Observation{Source: first.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:full-wait", Text: "First source"}}}}, Coverage: map[string]any{"status": "complete"}}
	if _, err := runtime.AcceptObservation(ctx, command.ID, first.ID, observation); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("reasoning did not start")
	}
	current, err := runtime.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Runs[1].Status != "queued" || current.Coverage["sourceWaitMode"] != "full_wait" {
		t.Fatalf("full wait advanced early: %+v", current)
	}
	runtime.Shutdown()
	if !runtime.WaitForIdle(time.Second) {
		t.Fatal("full-wait reasoning did not stop")
	}
}

func (provider *shutdownBlockingProvider) Analyze(ctx context.Context, run domain.Run, _ domain.Observation, _ []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	provider.once.Do(func() { close(provider.started) })
	<-ctx.Done()
	telemetry := domain.ReasoningTelemetry{
		ID: domain.NewID("reasoning"), RunID: run.ID, Phase: "candidate_evaluation",
		Provider: "shutdown-fixture", Model: "fixture", Effort: "none",
		Status: "failed", CreatedAt: domain.Now(),
	}
	return domain.ReasoningResult{}, telemetry, ctx.Err()
}

func TestShutdownPreservesAcceptedCaptureAndRestartResumesReasoning(t *testing.T) {
	ctx := context.Background()
	provider := &shutdownBlockingProvider{started: make(chan struct{})}
	runtime, state := singleSourceEngine(t, provider)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	active := waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return value.Runs[0].Status == "waiting_for_bridge"
	})
	run := active.Runs[0]
	command, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim command=%+v err=%v", command, err)
	}
	observation := domain.Observation{
		Source: domain.SourceX, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:durable-restart", Text: "Durable captured evidence"}}}},
		Coverage:  map[string]any{"status": "partial", "observedBlockCount": 1},
	}
	if _, err := runtime.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("reasoning did not start")
	}
	runtime.Shutdown()
	if !runtime.WaitForIdle(time.Second) {
		t.Fatal("interrupted reasoning did not stop")
	}
	paused, err := state.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status != "reasoning" || paused.Coverage["acquisitionRounds"] != float64(1) {
		t.Fatalf("paused run lost durable capture: %+v", paused)
	}

	restarted := New(state, reasoning.Deterministic{}, config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
	resumed, err := restarted.ResumePendingReasoning(ctx)
	if err != nil || resumed != 1 {
		t.Fatalf("resume count=%d err=%v", resumed, err)
	}
	completed := waitSession(t, restarted, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(completed.Items) != 1 || completed.Items[0].EvidenceKey != "x:durable-restart" {
		t.Fatalf("resumed items=%+v", completed.Items)
	}
}

type followUpProvider struct{ reasoning.Deterministic }

func (provider followUpProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	plan, telemetry, err := provider.Deterministic.Plan(ctx, run, observation, knowledge)
	plan.Decision = "request_follow_up"
	plan.Reason = "test optional frontier"
	return plan, telemetry, err
}

type failOnceAnalysisProvider struct {
	reasoning.Deterministic
	mu     sync.Mutex
	failed bool
}

func (provider *failOnceAnalysisProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	provider.mu.Lock()
	if !provider.failed {
		provider.failed = true
		provider.mu.Unlock()
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: "candidate_evaluation", Provider: "fail-once", Model: "fixture", Effort: "none", Status: "failed", CreatedAt: domain.Now()}, errors.New("temporary reasoning failure")
	}
	provider.mu.Unlock()
	return provider.Deterministic.Analyze(ctx, run, observation, knowledge)
}

func TestFailedFollowUpFallsBackToAcceptedObservation(t *testing.T) {
	ctx := context.Background()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	runtime := New(state, followUpProvider{}, config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 2}}, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(ExpectedHeartbeat())
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	active := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Runs[0].Status == "waiting_for_bridge" })
	run := active.Runs[0]
	command, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("initial command=%+v err=%v", command, err)
	}
	evidenceKey := "x:000000000000000000000099"
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: evidenceKey, Text: "Valid initial evidence"}}}}, Coverage: map[string]any{"performedScrolls": 4}}
	if _, err := runtime.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Runs[0].Stage == "follow_up_capture" })
	time.Sleep(20 * time.Millisecond)
	followUp, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || followUp == nil {
		t.Fatalf("follow-up command=%+v err=%v", followUp, err)
	}
	if _, err := runtime.FailCommand(ctx, followUp.ID, run.ID, domain.Failure{Code: "frontier_mismatch", Stage: "capture", Message: "frontier changed"}); err != nil {
		t.Fatal(err)
	}
	completed := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Runs[0].Status == "completed" })
	if len(completed.Items) != 1 || completed.Items[0].EvidenceKey != evidenceKey {
		t.Fatalf("fallback items=%+v", completed.Items)
	}
	inbox, _, err := runtime.Inbox(ctx, 1, 0)
	if err != nil || len(inbox) != 1 || inbox[0].Runs[0].FollowUpFallback == nil || inbox[0].Runs[0].FollowUpFallback.Code != "frontier_mismatch" {
		t.Fatalf("fallback inbox=%+v err=%v", inbox, err)
	}
}

func TestUnifiedSessionCompletesAllDefaultSources(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:000000000000000000000001")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 3 && value.Runs[1].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceLinkedIn, "linkedin:000000000000000000000002")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return value.Runs[2].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceFacebook, "facebook:post:000000000000000000000003")
	completed := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(completed.Items) != 3 {
		t.Fatalf("items=%d session=%+v", len(completed.Items), completed)
	}
	timeline, err := runtime.Timeline(ctx, 10, 0)
	if err != nil || len(timeline) != 3 {
		t.Fatalf("timeline=%d err=%v", len(timeline), err)
	}
}

func TestRepeatedExactEvidenceProducesZeroAdditions(t *testing.T) {
	ctx := context.Background()
	runtime, state := singleSourceEngine(t, reasoning.Deterministic{})
	evidence := "x:000000000000000000000401"

	first, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, first.ID, domain.SourceX, evidence)
	first = waitSession(t, runtime, first.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(first.Items) != 1 {
		t.Fatalf("first items=%d", len(first.Items))
	}

	second, err := runtime.StartSession(ctx, "What changed now?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, second.ID, domain.SourceX, evidence)
	second = waitSession(t, runtime, second.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(second.Items) != 0 {
		t.Fatalf("repeated evidence added again: %+v", second.Items)
	}
}

type continuityProvider struct {
	reasoning.Deterministic
	mu    sync.Mutex
	calls int
}

func TestBindReasoningSourceURLsUsesOnlyCapturedCanonicalEvidence(t *testing.T) {
	observation := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{
		{EvidenceKey: "x:000000000000000000000901", Permalink: "https://x.com/author/status/901"},
		{EvidenceKey: "x:000000000000000000000902", Permalink: "https://attacker.example/redirect"},
	}}}}
	result := domain.ReasoningResult{Items: []domain.ReasonedItem{
		{EvidenceKey: "x:000000000000000000000901", Source: domain.SourceLinkedIn, SourceURL: "https://attacker.example/model-controlled", SourceURLKind: "external_reference"},
		{EvidenceKey: "x:000000000000000000000902", Source: domain.SourceX, SourceURL: "https://attacker.example/model-controlled", SourceURLKind: "native_post"},
	}}

	bindReasoningSourceURLs(observation, &result)
	if result.Items[0].Source != domain.SourceX || result.Items[0].SourceURL != "https://x.com/author/status/901" || result.Items[0].SourceURLKind != "native_post" {
		t.Fatalf("canonical item=%+v", result.Items[0])
	}
	if result.Items[1].Source != domain.SourceX || result.Items[1].SourceURL != "" || result.Items[1].SourceURLKind != "" {
		t.Fatalf("invalid captured URL must not survive: %+v", result.Items[1])
	}
}

func TestValidateObservationRejectsNonCanonicalPermalink(t *testing.T) {
	observation := domain.Observation{
		Source:    domain.SourceX,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000903", Text: "Untrusted permalink fixture", Permalink: "https://attacker.example/post"}}}},
		Coverage:  map[string]any{"browserAdapter": "test"},
	}
	if err := validateObservation(observation); err == nil || !strings.Contains(err.Error(), "permalink") {
		t.Fatalf("validateObservation error=%v", err)
	}
}

func TestValidateObservationAcceptsNativeMediaOnlyEvidence(t *testing.T) {
	observation := domain.Observation{
		Source: domain.SourceFacebook,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "facebook:000000000000000000000904",
			PlatformID:  "facebook:post:904",
			Author:      "Example",
			Media:       []map[string]any{{"kind": "image", "url": "https://scontent.example.fbcdn.net/example.jpg"}},
		}}}},
		Coverage: map[string]any{"browserAdapter": "test"},
	}
	if err := validateObservation(observation); err != nil {
		t.Fatalf("validateObservation error=%v", err)
	}
}

func TestValidateObservationRejectsIdentityOnlyBlock(t *testing.T) {
	observation := domain.Observation{
		Source: domain.SourceFacebook,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "facebook:000000000000000000000905",
			PlatformID:  "facebook:post:905",
			Author:      "Example",
		}}}},
		Coverage: map[string]any{"browserAdapter": "test"},
	}
	if err := validateObservation(observation); err == nil || !strings.Contains(err.Error(), "no admissible") {
		t.Fatalf("validateObservation error=%v", err)
	}
}

func (provider *continuityProvider) Analyze(_ context.Context, run domain.Run, observation domain.Observation, _ []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	provider.mu.Lock()
	provider.calls++
	call := provider.calls
	provider.mu.Unlock()
	block := observation.Snapshots[0].Blocks[0]
	delta := []string{"new_event", "context", "material_update"}[call-1]
	materiality := .6
	if delta == "material_update" {
		materiality = .1
	}
	result := domain.ReasoningResult{
		Summary: "continuity fixture",
		Items: []domain.ReasonedItem{{
			ID: domain.NewID("item"), WhatChanged: "Changed", WhyItMatters: "Fixture",
			Source: run.Source, SourceURL: "https://example.test/post", SourceURLKind: "native_post",
			EvidenceKey: block.EvidenceKey, EventKey: "shared-event", KnowledgeDelta: delta,
			Author: "author", Confidence: .9, EvidenceState: "primary",
		}},
		CandidateAssessments: []domain.CandidateAssessment{{
			EvidenceKey: block.EvidenceKey, TopicFacets: []string{"other"}, ContentType: "news",
			Novelty: .5, Urgency: .2, Actionability: .2, Materiality: materiality,
			EvidenceStrength: .8, Rationale: "continuity fixture",
		}},
		Limitations: []string{},
	}
	telemetry := domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: "candidate_evaluation", Provider: "continuity", Model: "fixture", Effort: "none", Status: "completed", CreatedAt: domain.Now()}
	return result, telemetry, nil
}

func TestKnownEventRequiresMaterialDelta(t *testing.T) {
	ctx := context.Background()
	provider := &continuityProvider{}
	runtime, state := singleSourceEngine(t, provider)
	wantItems := []int{1, 0, 1}
	for index, evidence := range []string{
		"x:000000000000000000000411",
		"x:000000000000000000000412",
		"x:000000000000000000000413",
	} {
		session, err := runtime.StartSession(ctx, "What changed?")
		if err != nil {
			t.Fatal(err)
		}
		completeActiveRun(t, runtime, state, session.ID, domain.SourceX, evidence)
		session = waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
		if len(session.Items) != wantItems[index] {
			t.Fatalf("session %d delta items=%d want=%d", index, len(session.Items), wantItems[index])
		}
	}
}

func singleSourceEngine(t *testing.T, provider reasoning.Provider) (*Engine, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.CalibrationEnabled = false
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	runtime := New(state, provider, config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(ExpectedHeartbeat())
	return runtime, state
}

func TestAIDetectionSettingDisablesFastAndDeepPaths(t *testing.T) {
	ctx := context.Background()
	runtime, state := singleSourceEngine(t, reasoning.Deterministic{})
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled = false
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := runtime.StartSession(ctx, "AI detection disabled")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:ai-disabled")
	completed := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(completed.Items) != 1 || completed.Items[0].AIDetection != nil {
		t.Fatalf("disabled AI detection still produced an assessment: %+v", completed.Items)
	}
	job, err := state.AIDetectionJob(ctx, session.ID)
	if err != nil || job != nil {
		t.Fatalf("disabled AI detection job=%+v err=%v", job, err)
	}
}

func TestUnchangedResurfaceFailsFastBeforeReasoning(t *testing.T) {
	ctx := context.Background()
	runtime, state := singleSourceEngine(t, reasoning.Deterministic{})
	first, err := runtime.StartSession(ctx, "First observation")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, first.ID, domain.SourceX, "x:resurface")
	waitSession(t, runtime, first.ID, func(value domain.Session) bool { return value.Status == "completed" })

	second, err := runtime.StartSession(ctx, "Repeated observation")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, second.ID, domain.SourceX, "x:resurface")
	completed := waitSession(t, runtime, second.ID, func(value domain.Session) bool { return value.Status == "completed" })
	run := completed.Runs[0]
	trace, err := runtime.InboxRunTrace(ctx, run.ID, "captured", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Items) != 1 || trace.Items[0].Outcome != "resurfaced_unchanged" || trace.Items[0].Evaluated {
		t.Fatalf("unexpected resurface trace: %+v", trace)
	}
	inbox, _, err := state.ListInboxSessions(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 || inbox[0].Runs[0].CapturedCandidates != 1 || inbox[0].Runs[0].EvaluatedCandidates != 0 || inbox[0].Runs[0].SkippedResurfaces != 1 {
		t.Fatalf("unexpected resurface inbox: %+v", inbox)
	}
	for _, stage := range []string{"captured", "evaluated", "selected", "added"} {
		if _, ok := inbox[1].Runs[0].StageDurationsMS[stage]; !ok {
			t.Fatalf("first run missing %s timing: %+v", stage, inbox[1].Runs[0].StageDurationsMS)
		}
	}
}

func TestSelectionCorrectionPromotesAndUndoesAnEvaluatedCandidate(t *testing.T) {
	ctx := context.Background()
	runtime, _ := singleSourceEngine(t, reasoning.Deterministic{})
	session, err := runtime.StartSession(ctx, "Correction integration")
	if err != nil {
		t.Fatal(err)
	}
	active := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Runs[0].Status == "waiting_for_bridge" })
	run := active.Runs[0]
	command, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	blocks := make([]domain.Block, 0, 11)
	for index := 0; index < 11; index++ {
		blocks = append(blocks, domain.Block{
			EvidenceKey: fmt.Sprintf("x:%024d", index+1), Author: fmt.Sprintf("Author %d", index+1),
			Text:      fmt.Sprintf("Material source update number %d with sufficient durable evidence for deterministic correction testing.", index+1),
			Permalink: fmt.Sprintf("https://x.com/example/status/%d", index+1),
		})
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: blocks}}, Coverage: map[string]any{"quality": "complete"}}
	if _, err := runtime.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	completed := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(completed.Items) != 10 {
		t.Fatalf("initial items=%d", len(completed.Items))
	}
	trace, err := runtime.InboxRunTrace(ctx, run.ID, "evaluated", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	var missed domain.InboxFlowItem
	for _, item := range trace.Items {
		if item.Outcome == "not_selected" {
			missed = item
			break
		}
	}
	if missed.CandidateRef == "" {
		t.Fatalf("missing correction candidate in trace: %+v", trace)
	}
	correction, restored, err := runtime.CorrectSelection(ctx, run.ID, missed.CandidateRef)
	if err != nil {
		t.Fatal(err)
	}
	if correction.TimelineID != restored.ID {
		t.Fatalf("correction=%+v restored=%+v", correction, restored)
	}
	items, err := runtime.Timeline(ctx, 24, 0)
	if err != nil || len(items) != 11 {
		t.Fatalf("restored timeline=%d err=%v", len(items), err)
	}
	if _, err := runtime.UndoSelectionCorrection(ctx, correction.ID); err != nil {
		t.Fatal(err)
	}
	items, err = runtime.Timeline(ctx, 24, 0)
	if err != nil || len(items) != 10 {
		t.Fatalf("timeline after undo=%d err=%v", len(items), err)
	}
}

func TestReevaluateFailedRunReusesDurableCapture(t *testing.T) {
	ctx := context.Background()
	provider := &failOnceAnalysisProvider{}
	runtime, _ := singleSourceEngine(t, provider)
	session, err := runtime.StartSession(ctx, "Retry durable evidence")
	if err != nil {
		t.Fatal(err)
	}
	active := waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 1 && value.Runs[0].Status == "waiting_for_bridge"
	})
	run := active.Runs[0]
	command, err := runtime.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	observation := domain.Observation{
		Source: domain.SourceX, CapturedAt: domain.Now(), Coverage: map[string]any{"quality": "complete"},
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "x:000000000000000000000701",
			Author:      "Durable Author",
			Text:        "A material source update with enough evidence to evaluate after a temporary reasoning failure.",
			Permalink:   "https://x.com/example/status/701",
		}}}},
	}
	if _, err := runtime.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "failed" })
	trace, err := runtime.InboxRunTrace(ctx, run.ID, "all", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Counts.Captured != 1 || trace.Counts.Evaluated != 0 {
		t.Fatalf("failed trace counts=%+v", trace.Counts)
	}
	if _, err := runtime.ReevaluateFailedRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	inbox, _, err := runtime.Inbox(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || len(inbox[0].Runs) != 1 || inbox[0].Runs[0].AcquisitionRounds != 1 {
		t.Fatalf("re-evaluated run requested another capture: %+v", inbox)
	}
	trace, err = runtime.InboxRunTrace(ctx, run.ID, "all", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Counts.Captured != 1 || trace.Counts.Evaluated != 1 {
		t.Fatalf("re-evaluated trace counts=%+v", trace.Counts)
	}
	for _, item := range trace.Items {
		if item.Outcome == "captured_only" {
			t.Fatalf("durable capture remained unevaluated: %+v", item)
		}
	}
}

func TestFirstRunCalibrationFollowsTheInitialUnifiedSession(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:000000000000000000000011")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 3 && value.Runs[1].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceLinkedIn, "linkedin:000000000000000000000012")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return value.Runs[2].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceFacebook, "facebook:post:000000000000000000000013")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	calibration := waitActiveCalibration(t, runtime)
	if calibration.Status != "reviewing" || calibration.SampleCount != 3 || calibration.Samples[0].Source != domain.SourceX || calibration.Samples[1].Source != domain.SourceLinkedIn || calibration.Samples[2].Source != domain.SourceFacebook {
		t.Fatalf("calibration=%+v", calibration)
	}
	if _, err := runtime.StartSession(ctx, "must be blocked"); err == nil {
		t.Fatal("an active forced calibration must block another update")
	}
	more := "more_like_this"
	calibration, err = runtime.DecideCalibration(ctx, calibration.ID, 0, domain.CalibrationDecision{Label: &more})
	if err != nil || calibration.Status != "reviewing" || calibration.ResolvedCount != 1 {
		t.Fatalf("first decision calibration=%+v err=%v", calibration, err)
	}
	neutral := "neutral"
	calibration, err = runtime.DecideCalibration(ctx, calibration.ID, 1, domain.CalibrationDecision{Label: &neutral})
	if err != nil {
		t.Fatal(err)
	}
	calibration, err = runtime.DecideCalibration(ctx, calibration.ID, 2, domain.CalibrationDecision{Label: &neutral})
	if err != nil {
		t.Fatal(err)
	}
	if calibration.Status != "completed" || calibration.Snapshot == nil || calibration.Snapshot.Labels["moreLikeThis"] != 1 || calibration.Snapshot.Labels["neutral"] != 2 || calibration.Snapshot.ActivationState != "feeds_local_fit" {
		t.Fatalf("completed calibration=%+v", calibration)
	}
	status, err := state.CalibrationFirstRunStatus(ctx)
	if err != nil || status != "completed" {
		t.Fatalf("first-run status=%q err=%v", status, err)
	}
	if err := runtime.ResetLearning(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = state.CalibrationFirstRunStatus(ctx)
	if err != nil || status != "not_started" {
		t.Fatalf("reset first-run status=%q err=%v", status, err)
	}
	if active, err := state.ActiveCalibration(ctx); err != nil || active != nil {
		t.Fatalf("active calibration after reset=%+v err=%v", active, err)
	}
	if onboarding, err := state.Onboarding(ctx); err != nil || onboarding.Status != "completed" {
		t.Fatalf("onboarding after learning reset=%+v err=%v", onboarding, err)
	}
}

func TestCalibrationSnapshotReflectsAnyReadyPreferenceAuthority(t *testing.T) {
	session := domain.CalibrationSession{ID: "calibration-authority"}
	profile := preference.Profile{AuthorityReady: true, SuppressionReady: true}

	snapshot := buildCalibrationSnapshot(session, profile)
	if !snapshot.LiveInfluence {
		t.Fatal("suppression-only authority must be reported as live influence")
	}
}

func TestPartialFirstUpdateStillSuppliesCalibration(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:000000000000000000000021")
	current := waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 3 && value.Runs[1].Status == "waiting_for_bridge"
	})
	linkedin := current.Runs[1]
	command, err := runtime.ClaimCommand(ctx, linkedin.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	if _, err := runtime.FailCommand(ctx, command.ID, linkedin.ID, domain.Failure{Code: "capture_failed", Stage: "capture", Message: "test failure"}); err != nil {
		t.Fatal(err)
	}
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return value.Runs[2].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceFacebook, "facebook:post:000000000000000000000022")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "partial" })
	calibration := waitActiveCalibration(t, runtime)
	if calibration.SampleCount != 2 || calibration.Samples[0].Source != domain.SourceX || calibration.Samples[1].Source != domain.SourceFacebook {
		t.Fatalf("partial calibration=%+v", calibration)
	}
}

func TestCalibrationSamplerPreservesSourceOrderAndCapsEachSource(t *testing.T) {
	var candidates []domain.CalibrationCandidate
	for index := 0; index < 6; index++ {
		candidates = append(candidates,
			domain.CalibrationCandidate{RunID: "run-x", EvidenceKey: fmt.Sprintf("x:%d", index), Source: domain.SourceX, FeedPosition: index},
			domain.CalibrationCandidate{RunID: "run-li", EvidenceKey: fmt.Sprintf("linkedin:%d", index), Source: domain.SourceLinkedIn, FeedPosition: index},
		)
	}
	runs := []domain.Run{{Source: domain.SourceX, Ordinal: 0}, {Source: domain.SourceLinkedIn, Ordinal: 1}}
	samples := sampleCalibrationCandidates(candidates, runs, 10)
	if len(samples) != 10 {
		t.Fatalf("samples=%d", len(samples))
	}
	for index, sample := range samples {
		expected := domain.SourceX
		if index%2 == 1 {
			expected = domain.SourceLinkedIn
		}
		if sample.Source != expected || sample.Candidate.FeedPosition != index/2 {
			t.Fatalf("sample[%d]=%+v", index, sample)
		}
	}
}

func TestBridgeV2RequiresExactCapabilities(t *testing.T) {
	runtime, _ := testEngine(t)
	if status := runtime.BridgeStatus(); !status.Compatible || status.State != "healthy" {
		t.Fatalf("expected exact heartbeat to pass: %+v", status)
	}
	value := ExpectedHeartbeat()
	value.Actions = value.Actions[:len(value.Actions)-1]
	status := runtime.RecordHeartbeat(value)
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("missing reload_self action must fail closed: %+v", status)
	}
	value = ExpectedHeartbeat()
	delete(value.MediaEvidenceAdapterVersions, "x")
	status = runtime.RecordHeartbeat(value)
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("missing X media evidence adapter version must fail closed: %+v", status)
	}
	value = ExpectedHeartbeat()
	value.MediaEvidenceAdapterVersions["x"] = "x-response-evidence-v0"
	status = runtime.RecordHeartbeat(value)
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("stale X media evidence adapter version must fail closed: %+v", status)
	}
}

func TestObservationGetsStableGoEvidenceIdentity(t *testing.T) {
	value := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: "1890000000000000000", Text: "Material source update"}}}}}
	normalizeObservation(&value)
	first := value.Snapshots[0].Blocks[0].EvidenceKey
	if !regexp.MustCompile(`^x:[a-f0-9]{24}$`).MatchString(first) {
		t.Fatalf("evidenceKey=%q", first)
	}
	normalizeObservation(&value)
	if value.Snapshots[0].Blocks[0].EvidenceKey != first {
		t.Fatal("normalization must be idempotent")
	}
}

func TestLinkedInPermalinkRecoveryReconcilesDuplicateCaptureBeforeReasoning(t *testing.T) {
	text := "A sufficiently long LinkedIn post body is repeated across bounded snapshots and later exposes an exact native post permalink for the same author and content."
	fallback := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:fallback", Author: "Example Company", Text: text,
			Attachments:  []domain.Attachment{{Kind: "link_preview", URL: "https://example.com/job", Title: "Head of IT"}},
			Presentation: map[string]any{"permalinkSource": "unavailable"}, FeedPosition: 3,
		}}}},
		Coverage: map[string]any{"round": 1},
	}
	native := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:native", Author: "Example Company", Text: text,
			Permalink:    "https://www.linkedin.com/feed/update/urn:li:share:7412345678901234567/",
			PlatformID:   "linkedin:share:7412345678901234567",
			Presentation: map[string]any{"permalinkSource": "embed_urn"}, FeedPosition: 5,
		}}}},
		Coverage: map[string]any{"round": 2},
	}

	for _, observations := range [][]domain.Observation{{fallback, native}, {native, fallback}} {
		merged := mergeObservations(observations)
		blocks := []domain.Block{merged.Snapshots[0].Blocks[0], merged.Snapshots[1].Blocks[0]}
		for index, block := range blocks {
			if block.EvidenceKey != "linkedin:native" || block.PlatformID != "linkedin:share:7412345678901234567" {
				t.Fatalf("block[%d] identity=%+v", index, block)
			}
			if block.Permalink != native.Snapshots[0].Blocks[0].Permalink || block.Presentation["permalinkSource"] != "embed_urn" {
				t.Fatalf("block[%d] permalink recovery=%+v", index, block)
			}
			if len(block.Attachments) != 1 || block.Attachments[0].URL != "https://example.com/job" {
				t.Fatalf("block[%d] attachments=%+v", index, block.Attachments)
			}
		}
		if blocks[0].FeedPosition != observations[0].Snapshots[0].Blocks[0].FeedPosition || blocks[1].FeedPosition != observations[1].Snapshots[0].Blocks[0].FeedPosition {
			t.Fatalf("feed positions changed: %+v", blocks)
		}
	}
}

func TestCapturedContentSignatureDoesNotCollapseShortGenericEntries(t *testing.T) {
	first := domain.Block{Author: "Example", Text: "Short repeated status", EvidenceKey: "linkedin:first"}
	second := domain.Block{Author: "Example", Text: "Short repeated status", EvidenceKey: "linkedin:second", PlatformID: "linkedin:share:2"}
	merged := reconcileCapturedSnapshots(domain.SourceLinkedIn, []domain.Snapshot{{Blocks: []domain.Block{first, second}}})
	if merged[0].Blocks[0].EvidenceKey == merged[0].Blocks[1].EvidenceKey {
		t.Fatal("short generic entries must retain distinct identities")
	}
}

func TestContinuationPreservesBridgeFrontierIdentity(t *testing.T) {
	value := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{
			ScrollY: 1600,
			Blocks: []domain.Block{{
				EvidenceKey: "linkedin:0123456789abcdef01234567",
				PlatformID:  "linkedin:activity:7420000000000000000",
			}},
		}},
		Coverage: map[string]any{
			"frontier": map[string]any{
				"scrollY":    float64(1600),
				"anchorKeys": []any{"linkedin:activity:7420000000000000000"},
			},
		},
	}

	continuation := continuationFrom(value)
	anchors, ok := continuation["anchorKeys"].([]string)
	if !ok || len(anchors) != 1 || anchors[0] != "linkedin:activity:7420000000000000000" {
		t.Fatalf("continuation anchors=%#v", continuation["anchorKeys"])
	}
	if anchors[0] == value.Snapshots[0].Blocks[0].EvidenceKey {
		t.Fatal("Bridge continuation must not use the Go-derived evidence key")
	}
	if continuation["startScrollY"] != 1600 {
		t.Fatalf("startScrollY=%#v", continuation["startScrollY"])
	}
}

func TestLinkedInContinuationUsesPriorObservedOverlapCheckpoint(t *testing.T) {
	value := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{
			{ScrollY: 0, Blocks: []domain.Block{{Text: "First LinkedIn post with enough stable content to identify the captured block."}}},
			{ScrollY: 560, Blocks: []domain.Block{{PlatformID: "linkedin:activity:7411111111111111111"}}},
			{ScrollY: 1120, Blocks: []domain.Block{{Text: "Final text-only frontier that can move after LinkedIn remeasures its virtualized feed."}}},
		},
		Coverage: map[string]any{
			"frontier": map[string]any{
				"scrollY":    float64(1120),
				"anchorKeys": []any{"final text-only frontier that can move"},
			},
		},
	}

	continuation := continuationFrom(value)
	anchors, ok := continuation["anchorKeys"].([]string)
	if !ok || len(anchors) != 1 || anchors[0] != "linkedin:activity:7411111111111111111" {
		t.Fatalf("continuation anchors=%#v", continuation["anchorKeys"])
	}
	if continuation["startScrollY"] != 560 {
		t.Fatalf("startScrollY=%#v", continuation["startScrollY"])
	}
}

func completeActiveRun(t *testing.T, runtime *Engine, state *store.Store, sessionID string, source domain.Source, evidenceKey string) {
	t.Helper()
	session := waitSession(t, runtime, sessionID, func(value domain.Session) bool {
		for _, run := range value.Runs {
			if run.Source == source && run.Status == "waiting_for_bridge" {
				return true
			}
		}
		return false
	})
	var run domain.Run
	for _, value := range session.Runs {
		if value.Source == source {
			run = value
			break
		}
	}
	command, err := runtime.ClaimCommand(context.Background(), run.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim: %+v %v", command, err)
	}
	permalink := "https://x.com/example/status/" + strings.TrimPrefix(evidenceKey, "x:")
	if source == domain.SourceLinkedIn {
		permalink = "https://www.linkedin.com/feed/update/urn:li:activity:" + strings.TrimPrefix(evidenceKey, "linkedin:")
	} else if source == domain.SourceFacebook {
		permalink = "https://www.facebook.com/example/posts/" + strings.TrimPrefix(evidenceKey, "facebook:post:")
	}
	value := domain.Observation{Source: source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: evidenceKey, Text: "Material source update", Author: "author", Permalink: permalink}}}}, Coverage: map[string]any{"quality": "complete"}}
	if _, err := runtime.AcceptObservation(context.Background(), command.ID, run.ID, value); err != nil {
		t.Fatal(err)
	}
}

func waitSession(t *testing.T, runtime *Engine, id string, predicate func(domain.Session) bool) domain.Session {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		value, err := runtime.Session(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if predicate(value) {
			return value
		}
		time.Sleep(20 * time.Millisecond)
	}
	value, _ := runtime.Session(context.Background(), id)
	t.Fatalf("session did not reach expected state: %+v", value)
	return domain.Session{}
}

func waitActiveCalibration(t *testing.T, runtime *Engine) domain.CalibrationSession {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		overview, err := runtime.CalibrationOverview(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if overview.Active != nil {
			return *overview.Active
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("first-run calibration did not start automatically")
	return domain.CalibrationSession{}
}

func TestCapturePayloadCarriesPerSourceHydrationTimeout(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.SourceHydrationTimeoutMS[domain.SourceFacebook] = 29000
	payload := capturePayload(domain.Run{Source: domain.SourceFacebook}, "lease", settings, 1, nil, "")
	if payload["sourceHydrationTimeoutMs"] != 29000 {
		t.Fatalf("source hydration timeout=%v", payload["sourceHydrationTimeoutMs"])
	}
}
