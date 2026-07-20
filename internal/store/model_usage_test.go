package store

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func tokenPointer(value int64) *int64 { return &value }

func TestModelUsageProjectsEveryReasoningCategoryWithoutDoubleCountingBreakouts(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateSession(ctx, "usage projection", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) < 2 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	if err := state.SaveTelemetry(ctx, domain.ReasoningTelemetry{
		ID: "usage-plan", RunID: runs[0].ID, Phase: "acquisition_planning", Provider: "codex-app-server",
		Model: "gpt-test", Effort: "high", DurationMS: 1000, Status: "completed",
		InputTokens: tokenPointer(100), CachedInputTokens: tokenPointer(60), OutputTokens: tokenPointer(20), ReasoningOutputTokens: tokenPointer(5), CreatedAt: domain.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveTelemetry(ctx, domain.ReasoningTelemetry{
		ID: "usage-evaluate", RunID: runs[1].ID, Phase: "candidate_evaluation", Provider: "codex-app-server",
		Model: "gpt-test", Effort: "xhigh", DurationMS: 2000, Status: "failed", CreatedAt: domain.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{
		SessionID: session.ID, Status: "completed", Provider: "codex-app-server", Model: "gpt-test", Effort: "high",
		CandidateCount: 1, ShortlistCount: 1, UniqueItems: 1, DurationMS: 3000,
		Usage: domain.ModelUsage{Input: tokenPointer(200), CachedInput: tokenPointer(150), Output: tokenPointer(40), ReasoningOutput: tokenPointer(10)}, CreatedAt: domain.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	job, err := state.CreateAIDetectionJob(ctx, domain.AIDetectionJob{SessionID: session.ID, Provider: "codex-app-server", Model: "gpt-test", Effort: "high", CandidateCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.StartAIDetectionJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.FinishAIDetectionJob(ctx, job.ID, "completed", 4000, domain.ModelUsage{
		Input: tokenPointer(300), CachedInput: tokenPointer(200), Output: tokenPointer(60), ReasoningOutput: tokenPointer(15),
	}, nil); err != nil {
		t.Fatal(err)
	}

	report, err := state.SessionModelUsage(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Categories) != 4 || report.DurationMS != 10000 || report.UsageCoverage != "partial" {
		t.Fatalf("report=%+v", report)
	}
	if report.Usage.Input == nil || *report.Usage.Input != 600 || report.Usage.CachedInput == nil || *report.Usage.CachedInput != 410 || report.Usage.Output == nil || *report.Usage.Output != 120 {
		t.Fatalf("usage=%+v", report.Usage)
	}
	if report.Categories[0].InvocationCount != 1 || report.Categories[1].Status != "failed" || report.Categories[1].UsageCoverage != "unavailable" {
		t.Fatalf("categories=%+v", report.Categories)
	}

	aggregate, err := state.AggregateModelUsage(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.SessionCount != 1 || aggregate.Usage.Input == nil || *aggregate.Usage.Input != 600 || aggregate.DurationMS != report.DurationMS {
		t.Fatalf("aggregate=%+v", aggregate)
	}
}

func TestModelUsageExplainsCategoriesThatDidNotInvokeAModel(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "local path", settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, domain.Now(), session.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{
		SessionID: session.ID, Status: "bypassed", Provider: "local-index", Model: "none", Effort: "none", CreatedAt: domain.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	report, err := state.SessionModelUsage(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range report.Categories {
		if category.Status != "not_invoked" || category.InvocationCount != 0 || category.Note == "" {
			t.Fatalf("category=%+v", category)
		}
	}
	if report.UsageCoverage != "not_applicable" {
		t.Fatalf("coverage=%s", report.UsageCoverage)
	}
}
