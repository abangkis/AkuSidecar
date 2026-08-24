package reasoning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestMain(m *testing.M) {
	if os.Getenv("AKU_FAKE_CODEX_APP_SERVER") == "1" {
		if len(os.Args) >= 3 && os.Args[1] == "app-server" && os.Args[2] == "--help" {
			println("Usage: codex app-server [OPTIONS]")
			println("      --listen <LISTEN>")
			return
		}
		if len(os.Args) >= 2 && os.Args[1] == "--version" {
			println("codex-cli fake-test")
			return
		}
		fakeCodexAppServer()
		return
	}
	os.Exit(m.Run())
}

func TestEvaluationRequestUsesAliasesAndExcludesPriorIdentity(t *testing.T) {
	observation := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:current-opaque-key", Text: "Changed"}}}}, Coverage: map[string]any{}}
	knowledge := []domain.ReasonedItem{{ID: "prior-id", EvidenceKey: "x:prior-evidence-key", EventKey: "x:prior-event-key", WhatChanged: "Prior change"}}
	request := buildEvaluationRequest(domain.Run{ID: "run-1", Source: domain.SourceX}, observation, knowledge)
	for _, forbidden := range []string{"x:current-opaque-key", "x:prior-evidence-key", "x:prior-event-key", "prior-id"} {
		if strings.Contains(request.prompt, forbidden) {
			t.Fatalf("prompt leaked identity %q: %s", forbidden, request.prompt)
		}
	}
	if !strings.Contains(request.prompt, "candidate_001") || len(request.evidenceKeys) != 1 || request.evidenceKeys[0] != "x:current-opaque-key" {
		t.Fatalf("candidate alias missing: %+v", request)
	}
	if !strings.Contains(request.prompt, "Do not emit or infer source URLs") {
		t.Fatalf("source URL ownership contract missing: %s", request.prompt)
	}
}

func TestEvaluationRequestKeepsBoundedMediaMetadataWithoutMediaURLs(t *testing.T) {
	observation := domain.Observation{Source: domain.SourceFacebook, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
		EvidenceKey: "facebook:media-only",
		Author:      "Example",
		Media: []map[string]any{{
			"kind": "image", "alt": "A bounded source description", "width": 640.0, "height": 480.0,
			"url": "https://scontent.example.fbcdn.net/private.jpg", "posterUrl": "https://scontent.example.fbcdn.net/poster.jpg",
		}},
	}}}}, Coverage: map[string]any{}}
	request := buildEvaluationRequest(domain.Run{ID: "run-media", Source: domain.SourceFacebook}, observation, nil)
	if !strings.Contains(request.prompt, `"kind":"image"`) || !strings.Contains(request.prompt, "A bounded source description") {
		t.Fatalf("media metadata missing: %s", request.prompt)
	}
	if strings.Contains(request.prompt, "private.jpg") || strings.Contains(request.prompt, "poster.jpg") {
		t.Fatalf("media URL leaked into reasoning prompt: %s", request.prompt)
	}
	if !strings.Contains(request.prompt, "never claim to have seen visual details") {
		t.Fatalf("visual limitation contract missing: %s", request.prompt)
	}
}

func TestPlanningPromptUsesOnlyBoundedAcquisitionTelemetry(t *testing.T) {
	observation := domain.Observation{
		Source: domain.SourceX, PageURL: "https://x.com/private", PageTitle: "private page",
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:private-key", Author: "Private Actor", Text: "SOURCE_CONTENT_SENTINEL", Permalink: "https://x.com/private/status/1"}}}},
		Coverage: map[string]any{
			"performedScrolls": float64(2), "scrollStopReason": "budget_exhausted", "mediaRecovery": map[string]any{"secret": "RECOVERY_SENTINEL"},
			"captureQuality": map[string]any{"verdict": "complete", "candidateReportCount": float64(5), "retryAttempts": float64(1), "verdictCounts": map[string]any{"invalid": float64(0)}},
			"frontier":       map[string]any{"newCandidateCount": float64(1), "hasMoreCandidateSignal": true, "anchorKeys": []any{"private-native-anchor"}},
		},
	}
	knowledge := []domain.ReasonedItem{{WhatChanged: "PRIOR_KNOWLEDGE_SENTINEL", SourceURL: "https://x.com/private/status/2"}}
	prompt := buildPlanningPrompt(domain.Run{ID: "private-run-id", SessionID: "private-session-id", Source: domain.SourceX}, observation, knowledge)
	if len(prompt) > 2000 {
		t.Fatalf("planning prompt exceeded its regression budget: %d bytes", len(prompt))
	}
	for _, forbidden := range []string{"SOURCE_CONTENT_SENTINEL", "PRIOR_KNOWLEDGE_SENTINEL", "RECOVERY_SENTINEL", "private-native-anchor", "private-run-id", "private-session-id", "https://"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("planning prompt leaked %q: %s", forbidden, prompt)
		}
	}
	for _, required := range []string{`"source":"x"`, `"candidateCount":1`, `"performedScrolls":2`, `"hasMoreCandidateSignal":true`, `"continuationReady":true`} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("planning prompt missing %q: %s", required, prompt)
		}
	}
}

func TestEvaluationRequestExcludesHostOwnedFieldsAndKeepsBoundedEvidence(t *testing.T) {
	quoted := map[string]any{
		"author": "Quoted Actor", "text": "Quoted bounded evidence", "contentKind": "quote",
		"permalink": "https://x.com/private/status/quoted", "platformId": "private-quoted-platform", "links": []any{"https://private.example/quote"},
		"media": []any{map[string]any{"kind": "image", "alt": "Quoted alt evidence", "url": "https://private.example/quoted.jpg"}},
	}
	observation := domain.Observation{
		Source: domain.SourceX, PageURL: "https://x.com/home", PageTitle: "Private X home", CapturedAt: "2026-08-23T00:00:00Z",
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "x:opaque-current", Author: "Current Actor", AvatarURL: "https://private.example/avatar.jpg", Text: "Current bounded evidence",
			Permalink: "https://x.com/private/status/current", PublishedAt: stringPointer("2026-08-23T00:00:00Z"), PlatformID: "private-platform-id",
			ContentKind: "post", RelationshipType: "original", ParentPermalink: "https://x.com/private/status/parent", QuotedPost: quoted,
			Engagement: map[string]any{"like": 999}, Presentation: map[string]any{"secret": "PRESENTATION_SENTINEL"},
			Attachments: []domain.Attachment{{Kind: "document", Title: "Bounded attachment", Detail: "Useful detail", URL: "https://private.example/document", ImageURL: "https://private.example/document.jpg"}},
			Media:       []map[string]any{{"kind": "image", "alt": "Current alt evidence", "width": 640.0, "height": 480.0, "provenance": "captured", "url": "https://private.example/media.jpg"}},
			Links:       []map[string]any{{"url": "https://private.example/link"}}, MediaRecovery: map[string]any{"secret": "RECOVERY_SENTINEL"}, CaptureQuality: map[string]any{"secret": "QUALITY_SENTINEL"}, FeedPosition: 99,
		}}}},
		Coverage: map[string]any{"diagnostics": "COVERAGE_SENTINEL", "mediaRecovery": map[string]any{"secret": true}},
	}
	request := buildEvaluationRequest(domain.Run{ID: "private-run", SessionID: "private-session", Source: domain.SourceX}, observation, nil)
	for _, forbidden := range []string{
		"https://", "private-platform-id", "private-quoted-platform", "PRESENTATION_SENTINEL", "RECOVERY_SENTINEL", "QUALITY_SENTINEL", "COVERAGE_SENTINEL",
		`"avatarUrl"`, `"permalink"`, `"platformId"`, `"links"`, `"mediaRecovery"`, `"presentation"`, `"captureQuality"`, `"engagement"`, `"feedPosition"`, `"url"`, `"imageUrl"`,
	} {
		if strings.Contains(request.prompt, forbidden) {
			t.Fatalf("evaluation prompt leaked %q: %s", forbidden, request.prompt)
		}
	}
	for _, required := range []string{"candidate_001", "Current Actor", "Current bounded evidence", "Quoted bounded evidence", "Quoted alt evidence", "Bounded attachment", "Current alt evidence"} {
		if !strings.Contains(request.prompt, required) {
			t.Fatalf("evaluation prompt missing %q: %s", required, request.prompt)
		}
	}
}

func TestRelevantKnowledgeIsDeterministicBoundedAndFindsOlderMaterialOverlap(t *testing.T) {
	blocks := []domain.Block{{EvidenceKey: "x:new-0", Author: "Rare Research Lab", Text: "Rare Research Lab released Aurora inference architecture."}}
	for index := 1; index < 8; index++ {
		blocks = append(blocks, domain.Block{
			EvidenceKey: fmt.Sprintf("x:new-%d", index), Author: fmt.Sprintf("Current Actor %d", index),
			Text:      fmt.Sprintf("Distinct current evidence candidate %d about a bounded topic. %s", index, strings.Repeat("Current evidence detail. ", 10)),
			AvatarURL: fmt.Sprintf("https://private.example/avatar-%d.jpg", index), Permalink: fmt.Sprintf("https://x.com/private/status/current-%d", index),
			PlatformID: fmt.Sprintf("private-platform-%d", index), MediaRecovery: map[string]any{"diagnostic": strings.Repeat("recovery metadata ", 20)},
			Presentation: map[string]any{"layout": strings.Repeat("presentation metadata ", 10)},
		})
	}
	observation := domain.Observation{Source: domain.SourceX, PageURL: "https://x.com/home", PageTitle: "Private home", Snapshots: []domain.Snapshot{{Blocks: blocks}}, Coverage: map[string]any{"diagnostics": strings.Repeat("coverage metadata ", 40)}}
	knowledge := make([]domain.ReasonedItem, 200)
	for index := range knowledge {
		knowledge[index] = domain.ReasonedItem{
			WhatChanged:  fmt.Sprintf("Generic unrelated historical entry %03d %s", index, strings.Repeat("filler ", 20)),
			WhyItMatters: strings.Repeat("Unrelated context remains bounded. ", 5), Source: domain.SourceX,
			SourceURL: fmt.Sprintf("https://x.com/private/status/%d", index), KnowledgeDelta: "new_event", Author: fmt.Sprintf("Generic Author %03d", index), Confidence: .8, EvidenceState: "primary",
		}
	}
	knowledge[199] = domain.ReasonedItem{WhatChanged: "Rare Research Lab previously announced Aurora training", WhyItMatters: "Material overlap with the current Aurora inference architecture claim", Source: domain.SourceX, SourceURL: "https://x.com/private/status/older-relevant", KnowledgeDelta: "new_event", Author: "Rare Research Lab", Confidence: .95, EvidenceState: "primary"}
	knowledge[0] = domain.ReasonedItem{WhatChanged: "Rare Research Lab cross-source sentinel", Source: domain.SourceLinkedIn, Author: "Rare Research Lab", EvidenceState: "primary"}

	first := relevantKnowledge(observation, knowledge)
	second := relevantKnowledge(observation, knowledge)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("relevant knowledge selection is not deterministic")
	}
	if len(first) > maxEvaluationKnowledgeItems || len(first) < minRecentKnowledgeFallback {
		t.Fatalf("knowledge count=%d", len(first))
	}
	if first[0].Author != "Rare Research Lab" {
		t.Fatalf("older material overlap was not ranked first: %+v", first[0])
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > maxEvaluationKnowledgeBytes || strings.Contains(string(raw), "https://") || strings.Contains(string(raw), "sourceUrl") || strings.Contains(string(raw), "cross-source sentinel") {
		t.Fatalf("knowledge payload bytes=%d payload=%s", len(raw), raw)
	}
	request := buildEvaluationRequest(domain.Run{Source: domain.SourceX}, observation, knowledge)
	legacy, _ := json.Marshal(knowledge)
	legacyObservation, _ := json.Marshal(observation)
	t.Logf("representative knowledge bytes before=%d after=%d count_before=%d count_after=%d; observation bytes before=%d after=%d; total optimized prompt=%d", len(legacy), request.knowledgeBytes, len(knowledge), request.knowledgeCount, len(legacyObservation), request.observationBytes, len(request.prompt))
}

func stringPointer(value string) *string { return &value }

func TestBindEvidenceKeysByPositionOverridesModelIdentity(t *testing.T) {
	result := domain.ReasoningResult{
		Items:                []domain.ReasonedItem{{ID: "invented", EvidenceKey: "x:invented"}},
		CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "linkedin:invented"}},
	}
	if err := bindEvidenceKeysByPosition(&result, []string{"x:real"}); err != nil {
		t.Fatal(err)
	}
	if result.Items[0].ID != "x:real" || result.Items[0].EvidenceKey != "x:real" || result.CandidateAssessments[0].EvidenceKey != "x:real" {
		t.Fatalf("model identity was not replaced: %+v", result)
	}
}

func TestBindEvidenceKeysByPositionRejectsCardinalityMismatch(t *testing.T) {
	result := domain.ReasoningResult{Items: []domain.ReasonedItem{{}}, CandidateAssessments: nil}
	if err := bindEvidenceKeysByPosition(&result, []string{"x:key"}); err == nil {
		t.Fatal("expected assessment cardinality mismatch to fail")
	}
}

func filepathRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{current, filepath.Dir(os.Args[0])} {
		for candidate := start; ; candidate = filepath.Dir(candidate) {
			module, err := os.ReadFile(filepath.Join(candidate, "go.mod"))
			if err == nil && strings.Contains(string(module), "module github.com/abangkis/AkuSidecar") {
				return filepathSlash(candidate)
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break
			}
		}
	}
	t.Fatalf("AkuSidecar go.mod not found from cwd=%s executable=%s", current, os.Args[0])
	return ""
}

func filepathSlash(value string) string { return strings.ReplaceAll(value, "\\", "/") }
