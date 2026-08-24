package eventengine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestBuildSemanticSignalReceiptStrongWeakAndUnavailable(t *testing.T) {
	published := "2026-08-01T00:00:00Z"
	candidate := domain.SemanticCandidate{
		Author:      "Alice",
		WhatChanged: "Alice launches project",
		EventKey:    "project launch",
		PublishedAt: &published,
	}
	shortlist := []domain.SemanticEvent{{
		Actor:      "Alice",
		Object:     "project",
		EventStart: &published,
		Aliases:    []string{"project launch"},
	}}
	catalog := append([]domain.SemanticEvent{}, shortlist...)
	catalog = append(catalog, domain.SemanticEvent{CanonicalClaim: "project update"})
	receipt := buildSemanticSignalReceipt([]domain.SemanticCandidate{candidate}, shortlist, catalog, triggerSignal{Overlap: 3, Tokens: []string{"project", "unknown"}})
	if receipt.Version != semanticSignalReceiptVersion || receipt.CandidateCount != 1 || receipt.ShortlistedEventCount != 1 || receipt.TopOverlap != 3 {
		t.Fatalf("receipt header=%+v", receipt)
	}
	if receipt.TriggerTokenTotal != 2 || receipt.TriggerCommonTokenCount != 1 || receipt.TriggerUnknownTokenCount != 1 {
		t.Fatalf("trigger counts=%+v", receipt)
	}
	if receipt.CandidateActorOverlapCount != 1 || receipt.CandidateObjectOverlapCount != 1 || receipt.CandidateTimeWithin24HoursCount != 1 || receipt.CandidateExactEventKeyMatchCount != 1 {
		t.Fatalf("strong signals=%+v", receipt)
	}
	weak := buildSemanticSignalReceipt([]domain.SemanticCandidate{{}}, nil, catalog, triggerSignal{})
	if weak.CandidateActorUnavailableCount != 1 || weak.CandidateObjectUnavailableCount != 1 || weak.CandidateTimeUnavailableCount != 1 || weak.CandidateExactEventKeyUnavailableCount != 1 {
		t.Fatalf("unavailable signals=%+v", weak)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Alice", "project launch", "unknown", "https://secret.invalid"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("receipt leaked raw value %q", secret)
		}
	}
}

func TestBuildSemanticSignalReceiptTimeDistanceBucketsUseNearestInterval(t *testing.T) {
	within24 := "2026-08-02T00:00:00Z"
	within7 := "2026-08-05T00:00:00Z"
	beyond7 := "2026-08-11T00:00:00Z"
	invalid := "not-a-time"
	start := "2026-08-03T00:00:00Z"
	end := "2026-08-01T00:00:00Z"
	farStart := "2026-09-01T00:00:00Z"
	receipt := buildSemanticSignalReceipt(
		[]domain.SemanticCandidate{
			{PublishedAt: &within24},
			{PublishedAt: &within7},
			{PublishedAt: &beyond7},
			{PublishedAt: &invalid},
		},
		[]domain.SemanticEvent{{EventStart: &start, EventEnd: &end}, {EventStart: &farStart}},
		nil,
		triggerSignal{},
	)
	if receipt.CandidateTimeWithin24HoursCount != 1 || receipt.CandidateTimeWithin7DaysCount != 1 || receipt.CandidateTimeBeyond7DaysCount != 1 || receipt.CandidateTimeUnavailableCount != 1 {
		t.Fatalf("time distance receipt=%+v", receipt)
	}
}
