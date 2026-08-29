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
	if _, err := state.db.Exec(`
		INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at,resolved_at)
		VALUES(?,?,?,NULL)`, large.ID, "saved", "2026-08-29T00:00:00Z"); err != nil {
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
	if len(recommendations) != 2 || recommendations[0].ID == large.ID || recommendations[0].ContentBytes != 40 || recommendations[0].ReclaimableBytes != 40 {
		t.Fatalf("recommendations=%+v", recommendations)
	}
	if recommendations[1].ContentBytes != 40 {
		t.Fatalf("equal-size recommendations=%+v", recommendations)
	}
	wantTieOrder := []string{medium.ID, tie.ID}
	sort.Strings(wantTieOrder)
	// ASC is the stable id tie-breaker, after equal updated_at values.
	if recommendations[0].ID != wantTieOrder[1] || recommendations[1].ID != wantTieOrder[0] {
		t.Fatalf("tie order=%v want descending ids=%v", []string{recommendations[0].ID, recommendations[1].ID}, wantTieOrder)
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
	if limited, err := state.MemoryStorageRecommendations(ctx, 1); err != nil || len(limited) != 1 || limited[0].ID != wantTieOrder[1] {
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
	if report.SavedRecommendations == nil || len(report.SavedRecommendations) != 0 {
		t.Fatalf("empty Saved recommendations=%#v", report.SavedRecommendations)
	}
	if report.SavedPressure.ActiveItems != 0 || report.SavedPressure.LocalCopyItems != 0 ||
		report.SavedPressure.SourceDependentItems != 0 || report.SavedPressure.ContentBytes != 0 ||
		report.SavedPressure.OldestClaimedAt != "" {
		t.Fatalf("empty Saved pressure=%+v", report.SavedPressure)
	}
}

func TestMemorySavedPressureAndRecommendationsAreCurrentAndFIFO(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	first, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("saved-first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("saved-second"))
	if err != nil {
		t.Fatal(err)
	}
	full, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("saved-full"))
	if err != nil {
		t.Fatal(err)
	}
	const fullText = "temporarily saved full text"
	if _, err := state.KeepMemoryFullCopy(ctx, full.ID, domain.MemoryFullCopyInput{Content: fullText}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at,resolved_at)
		VALUES(?,?,?,NULL),(?,?,?,NULL),(?,?,?,NULL)`,
		first.ID, "saved", "2026-08-30T00:00:01Z",
		second.ID, "saved", "2026-08-30T00:00:01Z",
		full.ID, "saved", "2026-08-30T00:00:02Z"); err != nil {
		t.Fatal(err)
	}
	resolved, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("saved-resolved"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at,resolved_at)
		VALUES(?,?,?,?)`, resolved.ID, "saved", "2026-08-29T00:00:00Z", "2026-08-30T00:00:03Z"); err != nil {
		t.Fatal(err)
	}

	pressure, err := state.MemorySavedPressure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pressure.ActiveItems != 3 || pressure.LocalCopyItems != 1 || pressure.SourceDependentItems != 2 ||
		pressure.ContentBytes != int64(len(fullText)) || pressure.OldestClaimedAt != "2026-08-30T00:00:01Z" {
		t.Fatalf("Saved pressure=%+v", pressure)
	}

	recommendations, err := state.MemorySavedRecommendations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recommendations) != 3 {
		t.Fatalf("Saved recommendations=%+v", recommendations)
	}
	wantFirst, wantSecond := first.ID, second.ID
	if wantFirst > wantSecond {
		wantFirst, wantSecond = wantSecond, wantFirst
	}
	if recommendations[0].ID != wantFirst || recommendations[1].ID != wantSecond || recommendations[2].ID != full.ID {
		t.Fatalf("Saved FIFO order=%v want=%v,%v,%v", []string{recommendations[0].ID, recommendations[1].ID, recommendations[2].ID}, wantFirst, wantSecond, full.ID)
	}
	if recommendations[0].SourceDependent != true || recommendations[1].SourceDependent != true || recommendations[2].SourceDependent || recommendations[2].ContentBytes != int64(len(fullText)) {
		t.Fatalf("Saved recommendation retention=%+v", recommendations)
	}
	for _, recommendation := range recommendations {
		if recommendation.ReasonCode != "oldest_saved" || recommendation.ReviewAction != "review_saved" {
			t.Fatalf("Saved recommendation metadata=%+v", recommendation)
		}
	}
	report, err := state.MemoryStorageReport(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Recommendations) != 0 {
		t.Fatalf("general storage recommendations included current Saved full copy=%+v", report.Recommendations)
	}
}

func TestMemorySavedPressureTreatsZeroByteFullCopyAsSourceDependent(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	mediaOnly, err := state.CreateMemoryRecallStub(ctx, memoryStorageInput("saved-media-only"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		UPDATE memory_items SET retention_tier='full_copy',content_bytes=0,full_content_version_id=''
		WHERE id=?`, mediaOnly.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at,resolved_at)
		VALUES(?,?,?,NULL)`, mediaOnly.ID, "saved", "2026-08-30T00:00:03Z"); err != nil {
		t.Fatal(err)
	}

	pressure, err := state.MemorySavedPressure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pressure.ActiveItems != 1 || pressure.LocalCopyItems != 0 || pressure.SourceDependentItems != 1 || pressure.ContentBytes != 0 {
		t.Fatalf("zero-byte Saved full copy pressure=%+v", pressure)
	}
	recommendations, err := state.MemorySavedRecommendations(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recommendations) != 1 || !recommendations[0].SourceDependent || recommendations[0].RetentionTier != domain.MemoryTierFullCopy || recommendations[0].ContentBytes != 0 {
		t.Fatalf("zero-byte Saved full copy recommendation=%+v", recommendations)
	}
}

func TestMemoryStorageUsageIncludesRetentionClaimMetadata(t *testing.T) {
	state := openTestStore(t)
	const memoryID = "memory-claim"
	const claimKind = "saved"
	const claimedAt = "2026-08-30T00:00:00Z"
	if _, err := state.db.Exec(`
		INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at,resolved_at)
		VALUES(?,?,?,NULL)`, memoryID, claimKind, claimedAt); err != nil {
		t.Fatal(err)
	}
	usage, err := state.MemoryStorageUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := int64(len(memoryID) + len(claimKind) + len(claimedAt))
	if usage.MetadataBytes != want || usage.LogicalBytes != want {
		t.Fatalf("retention claim usage=%+v want metadata/logical=%d", usage, want)
	}
}
