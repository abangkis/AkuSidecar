package store

import (
	"context"
	"testing"
	"time"

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
	session, err := createVisibleUpdateSession(state, ctx, "usage projection", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) < 2 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	if err := state.SaveTelemetry(ctx, domain.ReasoningTelemetry{
		ID: "usage-plan", RunID: runs[0].ID, Phase: "acquisition_planning", Provider: "codex-app-server",
		Model: "gpt-test", Effort: "high", ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental", DurationMS: 1000, Status: "completed",
		CallerLatencyMS: 1100, QueueWaitMS: 10, ProviderExecutionMS: 900, ResponseTotalMS: 1000,
		InputTokens: tokenPointer(100), CachedInputTokens: tokenPointer(60), OutputTokens: tokenPointer(20), ReasoningOutputTokens: tokenPointer(5), CreatedAt: domain.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveTelemetry(ctx, domain.ReasoningTelemetry{
		ID: "usage-evaluate", RunID: runs[1].ID, Phase: "candidate_evaluation", Provider: "codex-app-server",
		Model: "gpt-test", Effort: "xhigh", ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental", DurationMS: 2000, Status: "failed", CreatedAt: domain.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{
		SessionID: session.ID, Status: "completed", Provider: "codex-app-server", Model: "gpt-test", Effort: "high",
		CandidateCount: 1, ShortlistCount: 1, UniqueItems: 1, DurationMS: 3000,
		CallerLatencyMS: 3000, QueueWaitMS: 20, ProviderExecutionMS: 2500, ResponseTotalMS: 2600,
		Usage: domain.ModelUsage{Input: tokenPointer(200), CachedInput: tokenPointer(150), Output: tokenPointer(40), ReasoningOutput: tokenPointer(10), ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental"}, CreatedAt: domain.Now(),
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
		ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental",
		CallerLatencyMS: 4000, QueueWaitMS: 30, ProviderExecutionMS: 3500, ResponseTotalMS: 3600,
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
	if report.Usage.CallerLatencyMS != 10100 || report.Usage.QueueWaitMS != 60 || report.Usage.ProviderExecutionMS != 6900 || report.Usage.ResponseTotalMS != 7200 {
		t.Fatalf("timing usage=%+v", report.Usage)
	}
	if report.Usage.ModelDescriptorVersion != "descriptor-1" || report.Usage.ModelMaturity != "experimental" {
		t.Fatalf("receipt metadata usage=%+v", report.Usage)
	}
	if report.Categories[0].InvocationCount != 1 || report.Categories[1].Status != "failed" || report.Categories[1].UsageCoverage != "unavailable" {
		t.Fatalf("categories=%+v", report.Categories)
	}

	aggregate, err := state.AggregateModelUsage(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.SessionCount != 1 || len(aggregate.Categories) != 6 || aggregate.Usage.Input == nil || *aggregate.Usage.Input != 600 || aggregate.DurationMS != report.DurationMS {
		t.Fatalf("aggregate=%+v", aggregate)
	}
}

func TestAggregateModelUsageIncludesAsynchronousLivingTopicReceipts(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	if err := state.RecordLivingTopicModelInvocation(ctx, "living_topic_routing", "timeline-1", "completed", "gemini", "gemini-test", "low", 250*time.Millisecond, domain.ModelUsage{
		Input: tokenPointer(80), CachedInput: tokenPointer(20), Output: tokenPointer(10), ReasoningOutput: tokenPointer(0),
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.RecordLivingTopicModelInvocation(ctx, "living_topic_understanding", "topic-1", "completed", "gemini", "gemini-test", "low", 500*time.Millisecond, domain.ModelUsage{
		Input: tokenPointer(120), CachedInput: tokenPointer(40), Output: tokenPointer(30), ReasoningOutput: tokenPointer(0),
	}); err != nil {
		t.Fatal(err)
	}
	report, err := state.AggregateModelUsage(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Categories) != 6 || report.Usage.Input == nil || *report.Usage.Input != 200 || report.Usage.Output == nil || *report.Usage.Output != 40 || report.DurationMS != 750 {
		t.Fatalf("report=%+v", report)
	}
	for _, categoryID := range []string{"living_topic_routing", "living_topic_understanding"} {
		found := false
		for _, category := range report.Categories {
			if category.ID == categoryID {
				found = category.InvocationCount == 1 && category.UsageCoverage == "complete"
			}
		}
		if !found {
			t.Fatalf("missing accounted category %s: %+v", categoryID, report.Categories)
		}
	}
}

func TestModelUsageExplainsCategoriesThatDidNotInvokeAModel(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "local path", settings)
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
