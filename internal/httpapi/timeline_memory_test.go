package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type timelineMemoryHTTPFixture struct {
	TimelineID string
	Text       string
}

func createTimelineMemoryHTTPFixture(t *testing.T, state *store.Store, status, text, suffix string) timelineMemoryHTTPFixture {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := state.CreateUpdateSession(ctx, "timeline memory HTTP fixture", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Runs) != 1 {
		t.Fatalf("fixture runs=%d", len(session.Runs))
	}
	run := session.Runs[0]
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := state.ClaimCommand(ctx, run.ID, "timeline-memory-http-test")
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil {
		t.Fatal("fixture command was not claimed")
	}
	publishedAt := "2026-08-29T10:00:00Z"
	evidenceKey := "x:timeline-memory:" + suffix
	block := domain.Block{
		EvidenceKey: evidenceKey, PlatformID: suffix, Author: "HTTP Fixture Author", Text: text,
		Permalink: "https://x.com/example/status/" + suffix, PublishedAt: &publishedAt,
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, domain.Observation{
		Source: run.Source, CapturedAt: publishedAt,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}},
		Coverage:  map[string]any{"status": "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	item := domain.ReasonedItem{
		ID: evidenceKey, EvidenceKey: evidenceKey, Source: domain.SourceX,
		WhatChanged: "HTTP Timeline memory fixture", WhyItMatters: "Full-copy route fixture",
		SourceURL: block.Permalink, Author: block.Author, PublishedAt: &publishedAt,
	}
	assessment := domain.CandidateAssessment{EvidenceKey: evidenceKey, TopicTags: []string{"memory"}, TopicFacets: []string{"test"}}
	timelineID := "timeline-memory-http-" + suffix
	item.ID = timelineID
	timelineItem := domain.TimelineItem{
		ID: timelineID, SessionID: session.ID, RunID: run.ID, Source: domain.SourceX,
		EvidenceKey: evidenceKey, Item: item, Coverage: map[string]any{"status": "complete"},
	}
	if err := state.CompleteRun(ctx, run, domain.ReasoningResult{Items: []domain.ReasonedItem{item}},
		[]store.ScoredAssessment{{Assessment: assessment, BaseScore: 1, FinalScore: 1, Selected: true}},
		[]domain.TimelineItem{timelineItem}, map[string]any{"status": "complete"}); err != nil {
		t.Fatal(err)
	}
	if err := state.FinalizeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		if err := state.CancelSession(ctx, session.ID); err != nil {
			t.Fatal(err)
		}
	}
	return timelineMemoryHTTPFixture{TimelineID: timelineID, Text: text}
}

func TestTimelineKeepHTTPUsesOnlyPersistedEvidenceAndIsIdempotent(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	fixture := createTimelineMemoryHTTPFixture(t, state, "completed", "authoritative Timeline body", "2401")

	request := httptest.NewRequest(http.MethodPost, "/api/timeline/"+fixture.TimelineID+"/keep-full-copy", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Keep status=%d body=%s", response.Code, response.Body.String())
	}
	var kept struct {
		Kept          bool              `json:"kept"`
		AlreadyKept   bool              `json:"alreadyKept"`
		RetentionTier domain.MemoryTier `json:"retentionTier"`
	}
	var keptPayload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &keptPayload); err != nil {
		t.Fatal(err)
	}
	if _, exists := keptPayload["id"]; exists {
		t.Fatalf("Keep response exposed internal memory id: %s", response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &kept); err != nil {
		t.Fatal(err)
	}
	if !kept.Kept || kept.AlreadyKept || kept.RetentionTier != domain.MemoryTierFullCopy {
		t.Fatalf("Keep response=%+v", kept)
	}
	items, err := state.ListMemoryItems(context.Background(), false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("stored memory listing=%+v err=%v", items, err)
	}
	item := items[0]
	if item.FullContent == nil || *item.FullContent != fixture.Text {
		t.Fatalf("stored authoritative content=%+v err=%v", item, err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/timeline/"+fixture.TimelineID+"/keep-full-copy", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("idempotent Keep status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &keptPayload); err != nil {
		t.Fatal(err)
	}
	if _, exists := keptPayload["id"]; exists {
		t.Fatalf("idempotent Keep response exposed internal memory id: %s", response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &kept); err != nil {
		t.Fatal(err)
	}
	if !kept.Kept || !kept.AlreadyKept {
		t.Fatalf("idempotent Keep response=%+v", kept)
	}
	items, err = state.ListMemoryItems(context.Background(), false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("idempotent memory listing=%+v err=%v", items, err)
	}
	item = items[0]
	if err != nil || item.RetentionTier != domain.MemoryTierFullCopy || item.FullContent == nil || *item.FullContent != fixture.Text {
		t.Fatalf("idempotent Keep changed the stored copy=%+v err=%v", item, err)
	}
}

func TestTimelineKeepAndLibraryReleaseHTTPBoundaries(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	blank := createTimelineMemoryHTTPFixture(t, state, "completed", "", "2402")
	request := httptest.NewRequest(http.MethodPost, "/api/timeline/"+blank.TimelineID+"/keep-full-copy", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("blank Keep status=%d body=%s", response.Code, response.Body.String())
	}
	var failure struct {
		Code string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != "timeline_memory_text_unavailable" {
		t.Fatalf("blank Keep error=%+v", failure)
	}
	if items, err := state.ListMemoryItems(ctx, false, 10); err != nil || len(items) != 0 {
		t.Fatalf("blank Keep created memory items=%+v err=%v", items, err)
	}

	fixture := createTimelineMemoryHTTPFixture(t, state, "completed", "release through HTTP", "2403")
	request = httptest.NewRequest(http.MethodPost, "/api/timeline/"+fixture.TimelineID+"/keep-full-copy", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("fixture Keep status=%d body=%s", response.Code, response.Body.String())
	}
	var kept struct {
		Kept          bool              `json:"kept"`
		AlreadyKept   bool              `json:"alreadyKept"`
		RetentionTier domain.MemoryTier `json:"retentionTier"`
	}
	var keptPayload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &keptPayload); err != nil {
		t.Fatal(err)
	}
	if _, exists := keptPayload["id"]; exists {
		t.Fatalf("Keep response exposed internal memory id: %s", response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &kept); err != nil {
		t.Fatal(err)
	}
	if !kept.Kept || kept.RetentionTier != domain.MemoryTierFullCopy {
		t.Fatalf("fixture Keep response=%+v", kept)
	}
	items, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("fixture memory listing=%+v err=%v", items, err)
	}
	keptID := items[0].ID

	request = httptest.NewRequest(http.MethodPost, "/api/library/items/"+keptID+"/release-full-copy", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Release status=%d body=%s", response.Code, response.Body.String())
	}
	var released struct {
		Released      bool              `json:"released"`
		ID            string            `json:"id"`
		RetentionTier domain.MemoryTier `json:"retentionTier"`
	}
	if err := json.NewDecoder(response.Body).Decode(&released); err != nil {
		t.Fatal(err)
	}
	if !released.Released || released.ID != keptID || released.RetentionTier != domain.MemoryTierRecall {
		t.Fatalf("Release response=%+v", released)
	}
	item, err := state.MemoryItem(ctx, keptID)
	if err != nil || item.RetentionTier != domain.MemoryTierRecall || item.FullContent != nil {
		t.Fatalf("released item=%+v err=%v", item, err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/library/items/"+keptID+"/forget-permanently", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Forget status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/library/items/"+keptID+"/release-full-copy", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("Release forgotten item status=%d body=%s", response.Code, response.Body.String())
	}
	forgottenItem, err := state.MemoryItem(ctx, keptID)
	if err != nil || forgottenItem.LifecycleState != domain.MemoryStateTombstone {
		t.Fatalf("forgotten item state=%+v err=%v", forgottenItem, err)
	}
}
