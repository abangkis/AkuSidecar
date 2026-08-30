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

func TestContentContextHTTPRouteIsGETOnlyAndBounded(t *testing.T) {
	server, _ := openLibraryHTTPFixture(t)
	for _, test := range []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "missing Timeline", method: http.MethodGet, path: "/api/timeline/missing/content-context", want: http.StatusNotFound},
		{name: "zero limit", method: http.MethodGet, path: "/api/timeline/missing/content-context?limit=0", want: http.StatusBadRequest},
		{name: "large limit", method: http.MethodGet, path: "/api/timeline/missing/content-context?limit=6", want: http.StatusBadRequest},
		{name: "non numeric limit", method: http.MethodGet, path: "/api/timeline/missing/content-context?limit=nope", want: http.StatusBadRequest},
		{name: "mutation method", method: http.MethodPost, path: "/api/timeline/missing/content-context", want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			server.api().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestContentContextHTTPReturnsBoundedPublicMatchReasonAndProjection(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateUpdateSession(ctx, "content context HTTP fixture", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil || len(session.Runs) != 1 {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	run := session.Runs[0]
	evidenceKey := "x:http-content-context"
	reasoned := domain.ReasonedItem{
		EvidenceKey: evidenceKey, Source: domain.SourceX,
		WhatChanged: "Quantum context update", WhyItMatters: "Local research context",
		SourceURL: "https://x.com/reader/status/http-content-context", Author: "Timeline Author",
	}
	assessment := domain.CandidateAssessment{
		EvidenceKey: evidenceKey, TopicTags: []string{"quantum", "research"},
	}
	if err := state.CompleteRun(ctx, run,
		domain.ReasoningResult{Items: []domain.ReasonedItem{reasoned}, CandidateAssessments: []domain.CandidateAssessment{assessment}},
		[]store.ScoredAssessment{{Assessment: assessment, Selected: true}},
		[]domain.TimelineItem{{ID: "timeline-http-content-context", SessionID: session.ID, RunID: run.ID, Source: domain.SourceX, EvidenceKey: evidenceKey, Item: reasoned}},
		map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := state.FinalizeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	memoryInput := libraryHTTPInput("2502")
	memoryInput.Identity.CanonicalEvidenceKey = "x:http-content-context-memory"
	memoryInput.Identity.CanonicalPermalink = "https://x.com/reader/status/2502"
	memoryInput.Title = "Quantum research context"
	memoryInput.Summary = "Local context for the update"
	memoryInput.Tags = []string{"quantum", "research"}
	memory, err := state.CreateMemoryRecallStub(ctx, memoryInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, memory.ID, domain.MemoryFullCopyInput{Content: "private retained context payload"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/timeline/timeline-http-content-context/content-context?limit=5", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Matches []struct {
			Item        map[string]json.RawMessage `json:"item"`
			MatchReason string                     `json:"matchReason"`
		} `json:"matches"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Matches) != 1 {
		t.Fatalf("matches=%+v", payload.Matches)
	}
	var returnedID, returnedTitle, reason string
	if err := json.Unmarshal(payload.Matches[0].Item["id"], &returnedID); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload.Matches[0].Item["title"], &returnedTitle); err != nil {
		t.Fatal(err)
	}
	reason = payload.Matches[0].MatchReason
	if returnedID != memory.ID || returnedTitle != memoryInput.Title || reason == "" {
		t.Fatalf("match id=%q title=%q reason=%q", returnedID, returnedTitle, reason)
	}
	if reason != "Matches title, summary, tags, retained text" {
		t.Fatalf("unexpected deterministic reason=%q", reason)
	}
	for _, forbidden := range []string{"fullContent", "provenance", "actions", "identityDigest", "contentFingerprint", "lifecycleState", "fullContentVersionId", "contentBytes", "reason"} {
		if _, exists := payload.Matches[0].Item[forbidden]; exists {
			t.Fatalf("context item exposed private field %q: %s", forbidden, response.Body.String())
		}
	}
}
