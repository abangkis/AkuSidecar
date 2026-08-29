package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func memoryStorageInput(id string) domain.MemoryItemInput {
	return domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{
			Source:               domain.SourceX,
			CanonicalEvidenceKey: "x:storage:" + id,
			CanonicalPlatformID:  id,
			ContentFingerprint:   "storage-fingerprint-" + id,
		},
		Title: "Storage title " + id, Author: "Storage author " + id,
	}
}

func TestMemoryStorageRecommendationsAreBoundedDeterministicAndPrivate(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	large, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("large"))
	if err != nil {
		t.Fatal(err)
	}
	medium, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("medium"))
	if err != nil {
		t.Fatal(err)
	}
	tie, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("tie"))
	if err != nil {
		t.Fatal(err)
	}
	recall, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("recall"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, large.ID, domain.MemoryFullCopyInput{Content: strings.Repeat("l", 100)}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, medium.ID, domain.MemoryFullCopyInput{Content: strings.Repeat("m", 40)}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, tie.ID, domain.MemoryFullCopyInput{Content: strings.Repeat("t", 40)}); err != nil {
		t.Fatal(err)
	}
	// Equal content sizes use the documented updated/id tie-breakers.
	if _, err := state.db.Exec(`UPDATE memory_items SET updated_at=? WHERE id IN (?,?)`, "2026-08-29T00:00:00Z", medium.ID, tie.ID); err != nil {
		t.Fatal(err)
	}

	recommendations, err := state.MemoryStorageRecommendations(ctx, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(recommendations) != 3 || recommendations[0].ID != large.ID || recommendations[0].ContentBytes != 100 || recommendations[0].ReclaimableBytes != 100 {
		t.Fatalf("recommendations=%+v", recommendations)
	}
	if recommendations[1].ContentBytes != 40 || recommendations[2].ContentBytes != 40 {
		t.Fatalf("equal-size recommendations=%+v", recommendations)
	}
	wantTieOrder := []string{medium.ID, tie.ID}
	sort.Strings(wantTieOrder)
	// DESC is the stable id tie-breaker, after equal updated_at values.
	if recommendations[1].ID != wantTieOrder[1] || recommendations[2].ID != wantTieOrder[0] {
		t.Fatalf("tie order=%v want descending ids=%v", []string{recommendations[1].ID, recommendations[2].ID}, wantTieOrder)
	}
	for _, recommendation := range recommendations {
		if recommendation.ReasonCode != "largest_full_copy" || recommendation.ReviewAction != "review_full_copy" {
			t.Fatalf("recommendation codes=%+v", recommendation)
		}
		encoded, err := json.Marshal(recommendation)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		for field := range fields {
			switch field {
			case "id", "source", "title", "author", "contentBytes", "reclaimableBytes", "updatedAt", "reasonCode", "reviewAction":
			default:
				t.Fatalf("recommendation exposed unexpected field %q: %s", field, encoded)
			}
		}
	}
	if limited, err := state.MemoryStorageRecommendations(ctx, 1); err != nil || len(limited) != 1 || limited[0].ID != large.ID {
		t.Fatalf("limited recommendations=%+v err=%v", limited, err)
	}
	for _, limit := range []int{-1, 13} {
		if _, err := state.MemoryStorageRecommendations(ctx, limit); err != ErrMemoryStorageRecommendationLimit {
			t.Fatalf("limit=%d err=%v", limit, err)
		}
	}

	if _, err := state.DeleteMemory(ctx, large.ID); err != nil {
		t.Fatal(err)
	}
	recommendations, err = state.MemoryStorageRecommendations(ctx, 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, recommendation := range recommendations {
		if recommendation.ID == large.ID {
			t.Fatalf("tombstone was recommended: %+v", recommendations)
		}
	}
	usage, err := state.MemoryStorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Tombstones != 1 || usage.FullCopyItems != 2 || usage.RecallItems != 1 {
		t.Fatalf("usage=%+v", usage)
	}
	_ = recall
}

func TestMemoryStorageReportReturnsEmptyRecommendations(t *testing.T) {
	state := openTestStore(t)
	report, err := state.MemoryStorageReport(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Recommendations == nil || len(report.Recommendations) != 0 {
		t.Fatalf("empty report recommendations=%#v", report.Recommendations)
	}
}
