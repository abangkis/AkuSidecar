package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestDeepReviewAtFeedbackTimestampClosesPendingRequest(t *testing.T) {
	const timestamp = "2026-07-30T18:01:46.6054094Z"
	if !happenedAtOrAfter(timestamp, timestamp) {
		t.Fatal("a deep review completed at the persisted feedback timestamp must close the pending request")
	}
	if happenedAtOrAfter("2026-07-30T18:01:45Z", timestamp) {
		t.Fatal("an assessment before the feedback request must not close it")
	}
}

func insertAIDetectionTimelineItem(t *testing.T, state *Store) (domain.Session, domain.TimelineItem) {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "AI Detector acceptance", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	item := domain.TimelineItem{
		ID: "timeline-ai-test", SessionID: session.ID, RunID: runs[0].ID, Source: runs[0].Source,
		EvidenceKey: "x:ai-detector-test", Item: domain.ReasonedItem{EvidenceKey: "x:ai-detector-test", Author: "Test author", WhatChanged: "Test post"},
	}
	itemRaw, _ := json.Marshal(item.Item)
	assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: item.EvidenceKey})
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.SessionID, item.RunID, item.Source, item.EvidenceKey, 0, string(itemRaw), string(assessmentRaw), "{}", domain.Now()); err != nil {
		t.Fatal(err)
	}
	return session, item
}

func TestAIDetectionAcceptanceMatrixAndUserAuthority(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, item := insertAIDetectionTimelineItem(t, state)

	fast := domain.AIAssessment{
		ID: "fast-assessment", TimelineID: item.ID, SessionID: session.ID, Stage: "fast", Status: "strong_signals",
		ConfidenceBand: "medium", EvidenceCodes: []string{"author_declared_ai"}, Provider: "local-deterministic",
		AssessedObject: "social_post", SignalScope: "social_post",
		DetectorVersion: "fast-text-v1", Rationale: "Explicit author declaration.", CreatedAt: "2026-07-17T01:00:00Z",
	}
	if err := state.SaveAIAssessments(ctx, []domain.AIAssessment{fast}); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	value := items[0].AIDetection
	if value == nil || value.BadgeLabel != "Author-declared AI · Preliminary" || !value.RouteToSignals || value.HideEligible {
		t.Fatalf("fast presentation=%+v", value)
	}

	job, err := state.CreateAIDetectionJob(ctx, domain.AIDetectionJob{SessionID: session.ID, Provider: "test", Model: "configured-model", Effort: "xhigh", CandidateCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.StartAIDetectionJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	if !items[0].AIDetection.PendingDeep || items[0].AIDetection.DeepStatus != "running" {
		t.Fatalf("pending deep presentation=%+v", items[0].AIDetection)
	}

	deep := domain.AIAssessment{
		ID: "deep-assessment", TimelineID: item.ID, SessionID: session.ID, Stage: "deep", Status: "no_signal_detected",
		ConfidenceBand: "low", Provider: "test", DetectorVersion: "deep-v1", Rationale: "The declaration was quoted context.",
		AssessedObject: "social_post", SignalScope: "quoted_post",
		SupersedesID: fast.ID, CreatedAt: "2026-07-17T01:01:00Z",
	}
	if err := state.SaveAIAssessments(ctx, []domain.AIAssessment{deep}); err != nil {
		t.Fatal(err)
	}
	input, cached, output, reasoning := int64(120), int64(80), int64(30), int64(10)
	if err := state.FinishAIDetectionJob(ctx, job.ID, "completed", 25, domain.ModelUsage{Input: &input, CachedInput: &cached, Output: &output, ReasoningOutput: &reasoning, ProviderModel: "qwen3.8:27b", NativeReasoning: "max"}, nil); err != nil {
		t.Fatal(err)
	}
	loadedJob, err := state.AIDetectionJob(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedJob == nil || loadedJob.Status != "completed" || loadedJob.Model != "qwen3.8:27b" || loadedJob.Effort != "max" || loadedJob.DurationMS != 25 || loadedJob.InputTokens == nil || *loadedJob.InputTokens != input || loadedJob.CachedInputTokens == nil || *loadedJob.CachedInputTokens != cached {
		t.Fatalf("AI detection job=%+v", loadedJob)
	}
	inbox, _, err := state.ListInboxSessions(ctx, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].AIDetection == nil || inbox[0].AIDetection.ID != job.ID {
		t.Fatalf("Inbox AI detection=%+v", inbox)
	}
	if inbox[0].AIDetectorYield == nil || inbox[0].AIDetectorYield.FastReviewed != 1 ||
		inbox[0].AIDetectorYield.DeepEligible != 1 || inbox[0].AIDetectorYield.DeepReviewed != 1 {
		t.Fatalf("Inbox AI Detector yield=%+v", inbox[0].AIDetectorYield)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	value = items[0].AIDetection
	if value.BadgeLabel != "AI assessment corrected" || !value.Corrected || value.RouteToSignals || value.HideEligible || value.PendingDeep {
		t.Fatalf("deep correction presentation=%+v", value)
	}

	correction, err := state.AddAIFeedback(ctx, item.ID, domain.AIFeedbackInput{
		Verdict: "ai", TargetType: "post", SignalScope: "social_post",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	value = items[0].AIDetection
	if value.BadgeLabel != "Marked as AI by you" || !value.UserOverride || !value.RouteToSignals || !value.HideEligible || value.CorrectionID != correction.ID {
		t.Fatalf("user authority presentation=%+v", value)
	}
	if _, err := state.UndoAIFeedback(ctx, correction.ID); err != nil {
		t.Fatal(err)
	}
	var feedbackRows int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_feedback_events WHERE target_key=?`, correction.TargetKey).Scan(&feedbackRows); err != nil {
		t.Fatal(err)
	}
	if feedbackRows != 2 {
		t.Fatalf("AI feedback and its clear event must both remain append-only; rows=%d", feedbackRows)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	if items[0].AIDetection.BadgeLabel != "AI assessment corrected" || items[0].AIDetection.UserOverride {
		t.Fatalf("undo did not restore resolved assessment=%+v", items[0].AIDetection)
	}
}

func TestExplicitAccountPolicyGeneralizesOnlyPresentationForMatchingIdentity(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, first := insertAIDetectionTimelineItem(t, state)
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(id, evidenceKey, author string, rank int) {
		itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: evidenceKey, Author: author, WhatChanged: "Candidate " + id})
		assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{
			EvidenceKey: evidenceKey, TopicTags: []string{"unclassified"}, TopicFacets: []string{"other"}, Materiality: .5,
		})
		if _, err := state.db.ExecContext(ctx, `
			INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`,
			id, session.ID, runs[0].ID, runs[0].Source, evidenceKey, rank, string(itemRaw), string(assessmentRaw), "{}", domain.Now()); err != nil {
			t.Fatal(err)
		}
	}
	insert("timeline-same-account", "x:same-account", "Test author", 1)
	insert("timeline-other-account", "x:other-account", "Different author", 2)

	feedback, err := state.AddAIFeedback(ctx, first.ID, domain.AIFeedbackInput{
		Verdict: "ai", TargetType: "account", SignalScope: "author_account", Reason: "account_identifies_as_agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Rank != 0 || items[1].Rank != 1 || items[2].Rank != 2 {
		t.Fatalf("AI policy must not rerank or select Timeline items: %+v", items)
	}
	if items[0].AIDetection == nil || items[1].AIDetection == nil ||
		!items[0].AIDetection.RouteToSignals || !items[1].AIDetection.RouteToSignals ||
		!items[1].AIDetection.PersonalPolicy.AccountRule {
		t.Fatalf("matching captured account identity did not receive explicit policy: %+v", items)
	}
	if items[2].AIDetection != nil {
		t.Fatalf("account policy leaked to a different identity: %+v", items[2].AIDetection)
	}
	history, err := state.AIFeedbackHistory(ctx, items[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != feedback.ID {
		t.Fatalf("account feedback must be inspectable from another matching item: %+v", history)
	}
}

func TestUnsureFeedbackIsNonVerdictAndScopeAwareNotAIKeepsPostSignal(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, item := insertAIDetectionTimelineItem(t, state)
	if err := state.SaveAIAssessments(ctx, []domain.AIAssessment{{
		ID: "post-signal", TimelineID: item.ID, SessionID: session.ID, Stage: "fast", Status: "strong_signals",
		ConfidenceBand: "medium", EvidenceCodes: []string{"author_declared_ai"}, AssessedObject: "social_post",
		SignalScope: "social_post", Provider: "local", DetectorVersion: "fast-v1", CreatedAt: domain.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddAIFeedback(ctx, item.ID, domain.AIFeedbackInput{
		Verdict: "not_ai", TargetType: "media", SignalScope: "attached_media", Reason: "known_human_authored",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].AIDetection.RouteToSignals {
		t.Fatalf("media-scoped correction must not erase a post-text signal: %+v", items[0].AIDetection)
	}
	if _, err := state.AddAIFeedback(ctx, item.ID, domain.AIFeedbackInput{
		Verdict: "unsure", TargetType: "post", SignalScope: "social_post", Reason: "insufficient_evidence",
	}); err != nil {
		t.Fatal(err)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	if items[0].AIDetection.PersonalPolicy == nil || !items[0].AIDetection.PersonalPolicy.ReviewRequested ||
		items[0].AIDetection.UserOverride {
		t.Fatalf("unsure must request review without becoming an AI/not-AI verdict: %+v", items[0].AIDetection)
	}
}

func TestDirectPlatformOriginEvidenceRemainsHideEligible(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, item := insertAIDetectionTimelineItem(t, state)
	values := []domain.AIAssessment{
		{ID: "platform-fast", TimelineID: item.ID, SessionID: session.ID, Stage: "fast", Status: "strong_signals", ConfidenceBand: "high", EvidenceCodes: []string{"platform_ai_label"}, AssessedObject: "social_post", SignalScope: "social_post", Provider: "local", DetectorVersion: "fast-v1", CreatedAt: "2026-07-17T01:00:00Z"},
		{ID: "platform-deep", TimelineID: item.ID, SessionID: session.ID, Stage: "deep", Status: "insufficient_evidence", ConfidenceBand: "low", AssessedObject: "social_post", SignalScope: "none", Provider: "deep", DetectorVersion: "deep-v1", SupersedesID: "platform-fast", CreatedAt: "2026-07-17T01:01:00Z"},
	}
	if err := state.SaveAIAssessments(ctx, values); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	value := items[0].AIDetection
	if value == nil || value.BadgeLabel != "Platform AI label" || !value.RouteToSignals || !value.HideEligible {
		t.Fatalf("direct evidence presentation=%+v", value)
	}
}

func TestOutdatedDeepStrongSignalLosesPresentationAuthority(t *testing.T) {
	value := resolveAIDetection([]domain.AIAssessment{
		{ID: "fast", Stage: "fast", Status: "no_signal_detected", ConfidenceBand: "low", AssessedObject: "social_post", SignalScope: "none", DetectorVersion: "fast-text-v1", CreatedAt: "2026-07-17T01:00:00Z"},
		{ID: "deep", Stage: "deep", Status: "strong_signals", ConfidenceBand: "high", EvidenceCodes: []string{"author_declared_ai"}, AssessedObject: "social_post", SignalScope: "social_post", DetectorVersion: "codex-deep-v3", CreatedAt: "2026-07-17T01:01:00Z"},
	}, "completed", nil)
	if value.Status != "no_signal_detected" || value.BadgeLabel != "AI assessment corrected" || !value.Corrected || value.RouteToSignals || value.HideEligible {
		t.Fatalf("resolved detection=%+v", value)
	}
}

func TestCurrentDeepStrongSignalKeepsPresentationAuthority(t *testing.T) {
	value := resolveAIDetection([]domain.AIAssessment{
		{ID: "fast", Stage: "fast", Status: "strong_signals", ConfidenceBand: "medium", EvidenceCodes: []string{"author_declared_ai"}, AssessedObject: "social_post", SignalScope: "social_post", DetectorVersion: "fast-text-v1", CreatedAt: "2026-07-17T01:00:00Z"},
		{ID: "deep", Stage: "deep", Status: "strong_signals", ConfidenceBand: "high", EvidenceCodes: []string{"author_declared_ai"}, AssessedObject: "social_post", SignalScope: "social_post", DetectorVersion: domain.CurrentAIDeepDetectorVersion, CreatedAt: "2026-07-17T01:01:00Z"},
	}, "completed", nil)
	if value.BadgeLabel != "AI signals confirmed" || !value.RouteToSignals || !value.HideEligible {
		t.Fatalf("resolved detection=%+v", value)
	}
}

func TestDirectMediaProvenanceRoutesToDrawer(t *testing.T) {
	value := resolveAIDetection(nil, "", []domain.MediaProvenanceAssessment{{
		Status: "completed", ManifestState: "valid", TrustState: "trusted", AIOrigin: "generated",
		MediaIndex: 0, EvidenceCodes: []string{"c2pa_trained_algorithmic_media"},
		VerifierVersion: "c2pa-image-v1", Rationale: "C2PA declares trained-algorithmic media.",
	}})
	if value == nil || !value.RouteToSignals || !value.DirectMediaProvenance {
		t.Fatalf("expected direct media provenance to route to signals: %+v", value)
	}
	if value.AssessedObject != "attached_media" || value.BadgeLabel != "Verified AI media" {
		t.Fatalf("expected object-scoped trusted media label: %+v", value)
	}
}

func TestUntrustedOrAbsentMediaProvenanceStaysInline(t *testing.T) {
	for _, test := range []struct {
		name          string
		manifestState string
		aiOrigin      string
	}{
		{name: "invalid manifest", manifestState: "invalid", aiOrigin: "unknown"},
		{name: "no manifest", manifestState: "no_manifest", aiOrigin: "none"},
		{name: "verified non-ai manifest", manifestState: "valid", aiOrigin: "none"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := resolveAIDetection(nil, "", []domain.MediaProvenanceAssessment{{
				Status: "completed", ManifestState: test.manifestState, TrustState: "not_evaluated", AIOrigin: test.aiOrigin,
				MediaIndex: 0, VerifierVersion: "c2pa-image-v1",
			}})
			if value == nil {
				t.Fatal("expected a detection envelope for the completed media inspection")
			}
			if value.RouteToSignals || value.HideEligible || value.DirectMediaProvenance || len(value.MediaSignals) != 0 {
				t.Fatalf("untrusted/absent provenance must remain inline: %+v", value)
			}
		})
	}
}

func TestTypedPlatformMediaLabelRoutesWithoutClaimingPostAuthorship(t *testing.T) {
	value := resolveAIDetection(nil, "", nil, map[string]any{
		"originSignals": []any{map[string]any{
			"kind": "platform_ai_label", "scope": "attached_media",
			"authority": "platform", "label": "AI info", "source": "facebook",
		}},
	})
	if value == nil || !value.RouteToSignals || !value.DirectOriginEvidence {
		t.Fatalf("expected platform media label to route to signals: %+v", value)
	}
	if value.AssessedObject != "attached_media" || value.SignalScope != "attached_media" || value.BadgeLabel != "Platform AI media label" {
		t.Fatalf("expected attached-media scope to remain explicit: %+v", value)
	}
}

func TestContentCredentialsAloneRemainNeutral(t *testing.T) {
	value := resolveAIDetection(nil, "", nil, map[string]any{
		"originSignals": []any{map[string]any{
			"kind": "content_credentials", "scope": "attached_media",
			"authority": "platform", "label": "Content Credentials", "source": "linkedin",
		}},
	})
	if value == nil || value.RouteToSignals || value.Status == "strong_signals" {
		t.Fatalf("content credentials do not by themselves establish AI origin: %+v", value)
	}
}

func TestUserCorrectionOverridesMediaRouting(t *testing.T) {
	value := resolveAIDetectionWithPolicy(nil, "", []domain.MediaProvenanceAssessment{{
		Status: "completed", ManifestState: "valid", TrustState: "trusted", AIOrigin: "generated",
		MediaIndex: 0, EvidenceCodes: []string{"c2pa_trained_algorithmic_media"},
		VerifierVersion: "c2pa-image-v1",
	}}, nil, &domain.PersonalAIPolicy{
		Applied: true, Verdict: "not_ai", TargetType: "media", SignalScope: "attached_media",
		FeedbackEventID: "feedback-1",
	}, 1)
	if value == nil || value.RouteToSignals || !value.UserOverride {
		t.Fatalf("expected personal not-AI correction to restore inline routing: %+v", value)
	}
	if len(value.MediaSignals) != 1 {
		t.Fatalf("expected provenance history to remain inspectable: %+v", value)
	}
}
