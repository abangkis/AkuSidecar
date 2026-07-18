package reasoning

import (
	"os"
	"path/filepath"
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
