package capture

import (
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestReconcileSnapshotsNeverMergesDistinctNativeIdentities(t *testing.T) {
	text := strings.Repeat("A substantial exact social post body can still be published under distinct native identities. ", 2)
	snapshots := []domain.Snapshot{
		{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:native-a",
			Author:      "Shared Author",
			Text:        text,
			PlatformID:  "urn:li:activity:7411111111111111111",
			Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111",
		}}},
		{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:native-b",
			Author:      "Shared Author",
			Text:        text,
			PlatformID:  "urn:li:activity:7422222222222222222",
			Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7422222222222222222",
		}}},
	}
	result := ReconcileSnapshots(domain.SourceLinkedIn, snapshots)
	if result[0].Blocks[0].EvidenceKey != "linkedin:native-a" || result[1].Blocks[0].EvidenceKey != "linkedin:native-b" {
		t.Fatalf("distinct native identities were merged: %+v", result)
	}
}

func TestReconcileSnapshotsPromotesOneRecoveredNativeIdentity(t *testing.T) {
	text := strings.Repeat("A substantial exact social post body gains one verified native identity later in capture. ", 2)
	snapshots := []domain.Snapshot{
		{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:fallback",
			Author:      "Shared Author",
			Text:        text,
		}}},
		{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:native",
			Author:      "Shared Author",
			Text:        text,
			PlatformID:  "urn:li:activity:7411111111111111111",
			Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111",
		}}},
	}
	result := ReconcileSnapshots(domain.SourceLinkedIn, snapshots)
	for _, snapshot := range result {
		if snapshot.Blocks[0].EvidenceKey != "linkedin:native" || snapshot.Blocks[0].PlatformID == "" {
			t.Fatalf("recovered native identity was not promoted: %+v", result)
		}
	}
}

func TestReconcileSnapshotsDoesNotMergeRepublishedOrDifferentContentKinds(t *testing.T) {
	text := strings.Repeat("An exact author and body can still represent a later source-native publication. ", 2)
	firstPublishedAt := "2026-07-26T17:07:00Z"
	secondPublishedAt := "2026-07-27T17:07:00Z"
	for _, test := range []struct {
		name   string
		first  domain.Block
		second domain.Block
	}{
		{
			name:   "different publication time",
			first:  domain.Block{ContentKind: "post", PublishedAt: &firstPublishedAt},
			second: domain.Block{ContentKind: "post", PublishedAt: &secondPublishedAt},
		},
		{
			name:   "different content kind",
			first:  domain.Block{ContentKind: "post"},
			second: domain.Block{ContentKind: "video"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := test.first
			first.EvidenceKey = "linkedin:fallback"
			first.Author = "Repeat Author"
			first.Text = text
			second := test.second
			second.EvidenceKey = "linkedin:native"
			second.Author = "Repeat Author"
			second.Text = text
			second.PlatformID = "linkedin:activity:7444444444444444444"
			second.Permalink = "https://www.linkedin.com/feed/update/urn:li:activity:7444444444444444444"

			result := ReconcileSnapshots(domain.SourceLinkedIn, []domain.Snapshot{
				{Blocks: []domain.Block{first}},
				{Blocks: []domain.Block{second}},
			})
			if got := result[0].Blocks[0].EvidenceKey; got != first.EvidenceKey {
				t.Fatalf("fallback evidence was rewritten to %q", got)
			}
			if got := result[1].Blocks[0].EvidenceKey; got != second.EvidenceKey {
				t.Fatalf("native evidence was rewritten to %q", got)
			}
		})
	}
}

func TestReconcileSnapshotsIgnoresEstimatedTimestampDrift(t *testing.T) {
	text := strings.Repeat("A relative LinkedIn timestamp estimate can drift while the native post remains the same. ", 2)
	firstPublishedAt := "2026-07-26T17:00:00Z"
	secondPublishedAt := "2026-07-26T18:00:00Z"
	result := ReconcileSnapshots(domain.SourceLinkedIn, []domain.Snapshot{
		{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:fallback",
			Author:      "Estimated Author",
			Text:        text,
			PublishedAt: &firstPublishedAt,
			ContentKind: "post",
			Presentation: map[string]any{
				"timestampEstimated": true,
				"timestampPrecision": "hour",
			},
		}}},
		{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:native",
			Author:      "Estimated Author",
			Text:        text,
			PublishedAt: &secondPublishedAt,
			ContentKind: "post",
			PlatformID:  "linkedin:activity:7488888888888888888",
			Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7488888888888888888",
			Presentation: map[string]any{
				"timestampEstimated": true,
				"timestampPrecision": "hour",
			},
		}}},
	})
	for _, snapshot := range result {
		if got := snapshot.Blocks[0].EvidenceKey; got != "linkedin:native" {
			t.Fatalf("estimated timestamp drift prevented native promotion: %q", got)
		}
	}
}
