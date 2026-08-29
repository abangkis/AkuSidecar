package store

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestVisionEvaluationQueueIsBoundedFIFOAndDeduplicated(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceInstagram}
	session, err := createVisibleUpdateSession(state, ctx, "vision queue", settings)
	if err != nil || len(session.Runs) != 1 {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	run := session.Runs[0]
	inputs := []domain.VisionEvaluationInput{
		{EvidenceKey: "instagram:p:old", Candidate: visionBlock("instagram:p:old", "old.jpg")},
		{EvidenceKey: "instagram:p:new", Candidate: visionBlock("instagram:p:new", "new.jpg")},
	}
	jobs, err := state.EnqueueVisionEvaluations(ctx, run, inputs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].Status != "pending" || jobs[1].Status != "deferred" {
		t.Fatalf("bounded jobs=%+v", jobs)
	}
	jobs, err = state.EnqueueVisionEvaluations(ctx, run, []domain.VisionEvaluationInput{inputs[0]}, 1)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("deduplicated jobs=%+v err=%v", jobs, err)
	}
	claimed, err := state.ClaimNextVisionEvaluation(ctx, domain.SourceInstagram, "vision-test")
	if err != nil || claimed == nil || claimed.EvidenceKey != "instagram:p:old" || claimed.AttemptCount != 1 {
		t.Fatalf("FIFO claim=%+v err=%v", claimed, err)
	}
	if err := state.CompleteVisionEvaluation(ctx, claimed.ID, map[string]any{"description": "ready"}); err != nil {
		t.Fatal(err)
	}
	jobs, err = state.ListVisionEvaluations(ctx, run.ID, run.Source)
	if err != nil {
		t.Fatal(err)
	}
	statusByEvidence := map[string]string{}
	for _, queued := range jobs {
		statusByEvidence[queued.EvidenceKey] = queued.Status
	}
	if statusByEvidence["instagram:p:old"] != "ready" || statusByEvidence["instagram:p:new"] != "pending" {
		t.Fatalf("deferred promotion=%+v err=%v", jobs, err)
	}
}

func TestVisionEvaluationRetryBackoffTerminalFailureAndManualRetry(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceInstagram}
	session, err := createVisibleUpdateSession(state, ctx, "vision retry", settings)
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	jobs, err := state.EnqueueVisionEvaluations(ctx, run, []domain.VisionEvaluationInput{{EvidenceKey: "instagram:p:retry", Candidate: visionBlock("instagram:p:retry", "retry.jpg")}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	job, err := state.ClaimNextVisionEvaluation(ctx, domain.SourceInstagram, "vision-test")
	if err != nil || job == nil {
		t.Fatalf("first claim=%+v err=%v", job, err)
	}
	if err := state.FailVisionEvaluation(ctx, job.ID, "provider unavailable"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := state.db.QueryRowContext(ctx, `SELECT status FROM vision_evaluation_jobs WHERE id=?`, jobs[0].ID).Scan(&status); err != nil || status != "retry_wait" {
		t.Fatalf("first failure status=%q err=%v", status, err)
	}
	// Make the backoff due without waiting in the test.
	if _, err := state.db.ExecContext(ctx, `UPDATE vision_evaluation_jobs SET next_attempt_at=? WHERE id=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), job.ID); err != nil {
		t.Fatal(err)
	}
	job, err = state.ClaimNextVisionEvaluation(ctx, domain.SourceInstagram, "vision-test")
	if err != nil || job == nil || job.AttemptCount != 2 {
		t.Fatalf("retry claim=%+v err=%v", job, err)
	}
	if err := state.FailVisionEvaluation(ctx, job.ID, "still unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT status FROM vision_evaluation_jobs WHERE id=?`, job.ID).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("terminal status=%q err=%v", status, err)
	}
	if err := state.RetryVisionEvaluation(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT status,attempt_count FROM vision_evaluation_jobs WHERE id=?`, job.ID).Scan(&status, new(int)); err != nil || status != "pending" {
		t.Fatalf("manual retry status=%q err=%v", status, err)
	}
}

func visionBlock(permalink, media string) domain.Block {
	return domain.Block{
		EvidenceKey: permalink, PlatformID: domain.NormalizeNativeIdentity(domain.SourceInstagram, permalink),
		Permalink: permalinkToURL(permalink), Media: []map[string]any{{"kind": "image", "url": "https://instagram.example.fbcdn.net/" + media}},
	}
}

func permalinkToURL(identity string) string {
	for _, prefix := range []string{"instagram:p:", "instagram:reel:", "instagram:tv:"} {
		if len(identity) > len(prefix) && identity[:len(prefix)] == prefix {
			kind := identity[len("instagram:") : len(prefix)-1]
			return "https://www.instagram.com/" + kind + "/" + identity[len(prefix):] + "/"
		}
	}
	return "https://www.instagram.com/p/fixture/"
}
