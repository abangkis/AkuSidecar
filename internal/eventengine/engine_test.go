package eventengine

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestShortlistIsGloballyBounded(t *testing.T) {
	candidates := []domain.SemanticCandidate{{Alias: "candidate_001", EvidenceKey: "x:1", Text: "OpenAI launches Codex App Server"}}
	events := make([]domain.SemanticEvent, 0, 20)
	for index := 0; index < 20; index++ {
		events = append(events, domain.SemanticEvent{ID: domain.NewID("event"), CanonicalClaim: "OpenAI launches Codex App Server", LastSeenAt: "2026-07-16T10:00:00Z"})
	}
	shortlist := rankShortlist(candidates, events, 5, map[string]map[string]string{})
	if len(shortlist) != 5 {
		t.Fatalf("shortlist=%d want=5", len(shortlist))
	}
}

func TestOnlyHighConfidenceTrueDuplicateConsumesNoUniqueCapacity(t *testing.T) {
	event := domain.SemanticEvent{ID: "event-existing", CanonicalClaim: "OpenAI launches Codex App Server", EventKind: "release", FirstSeenAt: "2026-07-15T10:00:00Z", LastSeenAt: "2026-07-15T10:00:00Z"}
	candidate := domain.SemanticCandidate{Alias: "candidate_001", TimelineID: "timeline-1", EvidenceKey: "x:1", WhatChanged: "OpenAI launches Codex App Server"}
	target := "event_001"
	resolution := domain.SemanticResolution{Decisions: []domain.SemanticDecision{{CandidateAlias: candidate.Alias, Relation: "duplicate_report", TargetAlias: &target, Confidence: .96, Reason: "Same occurrence"}}}
	reports := resolveReports([]domain.SemanticCandidate{candidate}, []domain.SemanticEvent{event}, []domain.SemanticEvent{event}, resolution, nil, 30)
	if len(reports) != 1 || reports[0].Relation != "duplicate_report" || reports[0].Event.ID != event.ID {
		t.Fatalf("reports=%+v", reports)
	}

	resolution.Decisions[0].Confidence = .91
	reports = resolveReports([]domain.SemanticCandidate{candidate}, []domain.SemanticEvent{event}, []domain.SemanticEvent{event}, resolution, nil, 30)
	if reports[0].Relation != "new_event" || reports[0].Event.ID == event.ID {
		t.Fatalf("low-confidence merge was not rejected: %+v", reports[0])
	}
}

func TestMaterialUpdateRemainsUniqueEvenWhenAttachedToEvent(t *testing.T) {
	event := domain.SemanticEvent{ID: "event-existing", CanonicalClaim: "OpenAI launches Codex App Server", EventKind: "release", FirstSeenAt: "2026-07-15T10:00:00Z", LastSeenAt: "2026-07-15T10:00:00Z"}
	candidate := domain.SemanticCandidate{Alias: "candidate_001", TimelineID: "timeline-1", EvidenceKey: "x:1", WhatChanged: "App Server adds a new capability"}
	target := "event_001"
	resolution := domain.SemanticResolution{Decisions: []domain.SemanticDecision{{CandidateAlias: candidate.Alias, Relation: "material_update", TargetAlias: &target, Confidence: .98, Reason: "New information about the same occurrence"}}}
	reports := resolveReports([]domain.SemanticCandidate{candidate}, []domain.SemanticEvent{event}, []domain.SemanticEvent{event}, resolution, nil, 30)
	if reports[0].Relation != "material_update" || reports[0].Event.ID != event.ID {
		t.Fatalf("material update lost its event relationship: %+v", reports[0])
	}
}
