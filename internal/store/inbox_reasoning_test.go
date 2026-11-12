package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestInboxReasoningUsesHistoricalRunAttemptsOnly(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "historical provider", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) < 2 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	// Current configuration cannot relabel a historical Gemini failure.
	settings.ReasoningProvider = "codex-app-server"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []domain.ReasoningTelemetry{
		{ID: "failed-plan", RunID: runs[0].ID, Phase: "acquisition_planning", Provider: "gemini-flash-lite", Status: "failed"},
		{ID: "fallback-eval", RunID: runs[0].ID, Phase: "candidate_evaluation", Provider: "deterministic", Status: "completed"},
		{ID: "unknown-provider", RunID: runs[1].ID, Phase: "candidate_evaluation", Provider: " ", Status: "failed"},
	} {
		receipt.CreatedAt = domain.Now()
		if err := state.SaveTelemetry(ctx, receipt); err != nil {
			t.Fatal(err)
		}
	}
	// Session-level/async ledgers are unrelated to source-run attribution.
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{SessionID: session.ID, Status: "completed", Provider: "codex-app-server", CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CreateAIDetectionJob(ctx, domain.AIDetectionJob{SessionID: session.ID, Provider: "codex-app-server", CandidateCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := state.RecordLivingTopicModelInvocation(ctx, "living_topic_routing", "unrelated", "completed", "codex-app-server", "fixture", "high", 0, domain.ModelUsage{}); err != nil {
		t.Fatal(err)
	}
	inbox, _, err := state.ListInboxSessions(ctx, 10, 0)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox=%+v err=%v", inbox, err)
	}
	byID := map[string]domain.InboxReasoning{}
	for _, run := range inbox[0].Runs {
		byID[run.ID] = run.Reasoning
	}
	want := domain.InboxReasoning{Providers: []string{"deterministic", "gemini-flash-lite"}, InvocationCount: 2, FailedInvocationCount: 1}
	if !reflect.DeepEqual(byID[runs[0].ID], want) {
		t.Fatalf("historical fallback=%+v want=%+v", byID[runs[0].ID], want)
	}
	want = domain.InboxReasoning{Providers: []string{}, InvocationCount: 1, FailedInvocationCount: 1, UnknownProviderCount: 1}
	if !reflect.DeepEqual(byID[runs[1].ID], want) {
		t.Fatalf("missing provider=%+v want=%+v", byID[runs[1].ID], want)
	}
	for _, run := range runs[2:] {
		if got := byID[run.ID]; got.InvocationCount != 0 || len(got.Providers) != 0 {
			t.Fatalf("uninvoked run acquired another provider: %+v", got)
		}
	}
}
