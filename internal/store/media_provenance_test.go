package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestMediaProvenanceKeepsLargeCarouselsOnABoundedBackgroundBudget(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	_, item := insertAIDetectionTimelineItem(t, state)
	item.Evidence = &domain.Block{}
	for index := 0; index < 8; index++ {
		item.Evidence.Media = append(item.Evidence.Media, map[string]any{
			"kind": "image",
			"url":  fmt.Sprintf("https://pbs.twimg.com/media/carousel-%d.jpg", index),
		})
	}

	queued, err := state.QueueMediaProvenance(ctx, []domain.TimelineItem{item}, "c2patool", "c2pa-image-v1")
	if err != nil {
		t.Fatal(err)
	}
	if queued != maxAutomaticMediaProvenancePerItem {
		t.Fatalf("queued=%d want=%d", queued, maxAutomaticMediaProvenancePerItem)
	}
	var persisted int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_provenance_assessments WHERE timeline_id=?`, item.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != maxAutomaticMediaProvenancePerItem {
		t.Fatalf("persisted=%d want=%d", persisted, maxAutomaticMediaProvenancePerItem)
	}
}
