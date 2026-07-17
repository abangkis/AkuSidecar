package eventengine

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestShortlistIsGloballyBounded(t *testing.T) {
	candidates := []domain.SemanticCandidate{{Alias: "candidate_001", EvidenceKey: "x:1", WhatChanged: "OpenAI launches Codex App Server"}}
	events := make([]domain.SemanticEvent, 0, 20)
	for index := 0; index < 20; index++ {
		events = append(events, domain.SemanticEvent{ID: domain.NewID("event"), CanonicalClaim: "OpenAI launches Codex App Server", LastSeenAt: "2026-07-16T10:00:00Z"})
	}
	shortlist, signal := rankShortlist(candidates, events, 5)
	if len(shortlist) != 5 {
		t.Fatalf("shortlist=%d want=5", len(shortlist))
	}
	if !signal.Strong || signal.Overlap < 2 {
		t.Fatalf("signal=%+v", signal)
	}
}

func TestHistoricalShortlistRejectsGenericHiringOverlap(t *testing.T) {
	candidates := []domain.SemanticCandidate{{Alias: "candidate_001", WhatChanged: "PT ALTO Network announced that it is expanding its team and hiring applicants.", EventKey: "alto-network-team-expansion", TopicTags: []string{"hiring", "fintech", "Indonesia"}}}
	events := []domain.SemanticEvent{{ID: "event-google-hiring", CanonicalClaim: "Logan Kilpatrick announced that Google AI Studio is hiring a TPM lead.", Actor: "Logan Kilpatrick", Object: "Google AI Studio TPM lead", Aliases: []string{"Google AI Studio", "TPM lead", "AI hiring"}}}
	shortlist, signal := rankShortlist(candidates, events, 5)
	if len(shortlist) != 0 || signal.Strong {
		t.Fatalf("generic hiring overlap reached resolver: shortlist=%+v signal=%+v", shortlist, signal)
	}
}

func TestObservedGenericAndPlatformTokensDoNotTriggerResolver(t *testing.T) {
	candidates := []domain.SemanticCandidate{
		{Alias: "candidate_001", WhatChanged: "Logan shared a LinkedIn post toward a hiring goal https://lnkd.in/example", EventKey: "google-ai-studio-hiring", TopicTags: []string{"hiring", "google-ai-studio"}},
		{Alias: "candidate_002", WhatChanged: "A Phase 3 result was reported with a full status update https://x.com/example/status/1", EventKey: "soficitinib-phase-3-result", TopicTags: []string{"biotech", "clinical-trial"}},
		{Alias: "candidate_003", WhatChanged: "A writer said you should think about the year ahead", EventKey: "workplace-opinion", TopicTags: []string{"workplace", "leadership"}},
	}
	signal := strongestIntraCheckSignal(candidates)
	if signal.Strong {
		t.Fatalf("generic overlap triggered resolver: %+v", signal)
	}
}

func TestFirstLiveSemanticRunRemainsOnLocalFastPath(t *testing.T) {
	candidates := []domain.SemanticCandidate{
		{WhatChanged: "Logan Kilpatrick announced that Google AI Studio is hiring a TPM lead to help accelerate progress toward AGI.", EventKey: "linkedin:share:7482858514764455936", TopicTags: []string{"Google AI Studio", "TPM lead", "AI hiring", "AGI"}},
		{WhatChanged: "The Hacker News reported that a Zimbra Classic Web Client stored-XSS flaw can persist in content and execute when rendered in a user session; it said Zimbra had not reported exploitation in the wild.", EventKey: "x:status:2077055767832883239", TopicTags: []string{"Zimbra", "stored XSS", "email security", "vulnerability"}},
		{WhatChanged: "Minhua Chu reported that InnoCare's TYK2 inhibitor soficitinib met a Phase 3 endpoint in moderate-to-severe atopic dermatitis, with secondary endpoints met, no new safety signals, and a planned regulatory filing.", EventKey: "x:status:2077700267634774042", TopicTags: []string{"atopic dermatitis", "TYK2 inhibitor", "soficitinib", "Phase 3"}},
		{WhatChanged: "The Jakarta Post shared a society story on the Indonesian habit of opening messages with mohon izin bertanya and its perceived military-hierarchical cultural nuance.", EventKey: "linkedin:text:22a1947d", TopicTags: []string{"Indonesia", "communication culture", "hierarchy", "society"}},
		{WhatChanged: "Token Gremlin speculated that a Kimi K3 release with 2.5 trillion parameters, a 1M context window, and open weights could be a leading release of the year.", EventKey: "x:status:2077546739003744310", TopicTags: []string{"Kimi K3", "open weights", "model release"}},
		{WhatChanged: "Tyler said Codex works while he sleeps whereas Claude forces him out of bed.", EventKey: "x:status:2077679638797516872", TopicTags: []string{"Codex", "Claude", "developer workflow"}},
		{WhatChanged: "Prakash Kumar argued that CTOs should delegate internal dashboards, data pipelines, and operations scripts to specialists rather than build and maintain them themselves.", EventKey: "linkedin:share:7479457065573298176", TopicTags: []string{"CTO", "internal tools", "startup operations", "outsourcing"}},
	}
	if signal := strongestIntraCheckSignal(candidates); signal.Strong {
		t.Fatalf("observed live run would still invoke resolver: %+v", signal)
	}
}

func TestSpecificSharedEventSignalsTriggerResolver(t *testing.T) {
	candidates := []domain.SemanticCandidate{
		{Alias: "candidate_001", WhatChanged: "Moonshot announced the Kimi K3 open-weight model launch", EventKey: "kimi-k3-model-launch", TopicTags: []string{"kimi-k3", "model-release"}},
		{Alias: "candidate_002", WhatChanged: "A second author reported Moonshot's Kimi K3 open-weight model launch", EventKey: "kimi-k3-model-launch", TopicTags: []string{"kimi-k3", "model-release"}},
	}
	signal := strongestIntraCheckSignal(candidates)
	if !signal.Strong || signal.Reason != "matching_event_key" {
		t.Fatalf("specific event did not trigger resolver: %+v", signal)
	}
}

func TestOnlyHighConfidenceTrueDuplicateConsumesNoUniqueCapacity(t *testing.T) {
	event := domain.SemanticEvent{ID: "event-existing", CanonicalClaim: "OpenAI launches Codex App Server", EventKind: "release", FirstSeenAt: "2026-07-15T10:00:00Z", LastSeenAt: "2026-07-15T10:00:00Z"}
	candidate := domain.SemanticCandidate{Alias: "candidate_001", TimelineID: "timeline-1", EvidenceKey: "x:1", WhatChanged: "OpenAI launches Codex App Server"}
	target := "event_001"
	resolution := domain.SemanticResolution{Decisions: []domain.SemanticDecision{{CandidateAlias: candidate.Alias, Relation: "duplicate_report", TargetAlias: &target, Confidence: .96, Reason: "Same occurrence"}}}
	reports := resolveReports([]domain.SemanticCandidate{candidate}, []domain.SemanticEvent{event}, []domain.SemanticEvent{event}, resolution, nil, 30, domain.DefaultSemanticMergeThreshold)
	if len(reports) != 1 || reports[0].Relation != "duplicate_report" || reports[0].Event.ID != event.ID {
		t.Fatalf("reports=%+v", reports)
	}

	resolution.Decisions[0].Confidence = .91
	reports = resolveReports([]domain.SemanticCandidate{candidate}, []domain.SemanticEvent{event}, []domain.SemanticEvent{event}, resolution, nil, 30, domain.DefaultSemanticMergeThreshold)
	if reports[0].Relation != "new_event" || reports[0].Event.ID == event.ID {
		t.Fatalf("low-confidence merge was not rejected: %+v", reports[0])
	}
}

func TestMaterialUpdateRemainsUniqueEvenWhenAttachedToEvent(t *testing.T) {
	event := domain.SemanticEvent{ID: "event-existing", CanonicalClaim: "OpenAI launches Codex App Server", EventKind: "release", FirstSeenAt: "2026-07-15T10:00:00Z", LastSeenAt: "2026-07-15T10:00:00Z"}
	candidate := domain.SemanticCandidate{Alias: "candidate_001", TimelineID: "timeline-1", EvidenceKey: "x:1", WhatChanged: "App Server adds a new capability"}
	target := "event_001"
	resolution := domain.SemanticResolution{Decisions: []domain.SemanticDecision{{CandidateAlias: candidate.Alias, Relation: "material_update", TargetAlias: &target, Confidence: .90, Reason: "New information about the same occurrence"}}}
	reports := resolveReports([]domain.SemanticCandidate{candidate}, []domain.SemanticEvent{event}, []domain.SemanticEvent{event}, resolution, nil, 30, domain.DefaultSemanticMergeThreshold)
	if reports[0].Relation != "material_update" || reports[0].Event.ID != event.ID {
		t.Fatalf("material update lost its event relationship: %+v", reports[0])
	}
}
