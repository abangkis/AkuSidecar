package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func insertAIDetectionTimelineItem(t *testing.T, state *Store) (domain.Session, domain.TimelineItem) {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateSession(ctx, "AI Detector acceptance", settings)
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

	job, err := state.CreateAIDetectionJob(ctx, domain.AIDetectionJob{SessionID: session.ID, Provider: "test", CandidateCount: 1})
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
		SupersedesID: fast.ID, CreatedAt: "2026-07-17T01:01:00Z",
	}
	if err := state.SaveAIAssessments(ctx, []domain.AIAssessment{deep}); err != nil {
		t.Fatal(err)
	}
	if err := state.FinishAIDetectionJob(ctx, job.ID, "completed", 25, domain.ModelUsage{}, nil); err != nil {
		t.Fatal(err)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	value = items[0].AIDetection
	if value.BadgeLabel != "AI assessment corrected" || !value.Corrected || value.RouteToSignals || value.HideEligible || value.PendingDeep {
		t.Fatalf("deep correction presentation=%+v", value)
	}

	correction, err := state.AddAICorrection(ctx, item.ID, "ai")
	if err != nil {
		t.Fatal(err)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	value = items[0].AIDetection
	if value.BadgeLabel != "Marked as AI by you" || !value.UserOverride || !value.RouteToSignals || !value.HideEligible || value.CorrectionID != correction.ID {
		t.Fatalf("user authority presentation=%+v", value)
	}
	if _, err := state.UndoAICorrection(ctx, correction.ID); err != nil {
		t.Fatal(err)
	}
	items, _ = state.ListSessionItems(ctx, session.ID)
	if items[0].AIDetection.BadgeLabel != "AI assessment corrected" || items[0].AIDetection.UserOverride {
		t.Fatalf("undo did not restore resolved assessment=%+v", items[0].AIDetection)
	}
}

func TestDirectPlatformOriginEvidenceRemainsHideEligible(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, item := insertAIDetectionTimelineItem(t, state)
	values := []domain.AIAssessment{
		{ID: "platform-fast", TimelineID: item.ID, SessionID: session.ID, Stage: "fast", Status: "strong_signals", ConfidenceBand: "high", EvidenceCodes: []string{"platform_ai_label"}, Provider: "local", DetectorVersion: "fast-v1", CreatedAt: "2026-07-17T01:00:00Z"},
		{ID: "platform-deep", TimelineID: item.ID, SessionID: session.ID, Stage: "deep", Status: "insufficient_evidence", ConfidenceBand: "low", Provider: "deep", DetectorVersion: "deep-v1", SupersedesID: "platform-fast", CreatedAt: "2026-07-17T01:01:00Z"},
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
