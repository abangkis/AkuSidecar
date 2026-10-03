package store

import (
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"testing"
)

func photoMediaFixture() (domain.MediaRecapture, domain.Block, domain.Observation) {
	j, b, _ := photoParentFixture()
	b.Text = ""
	o := domain.Observation{Source: domain.SourceFacebook, PageURL: b.Permalink, Coverage: map[string]any{
		"photoMediaRecapture": map[string]any{"status": "verified", "photoId": "123", "ownerId": "456", "provenance": "exact_photo_metadata_and_visible_image"}},
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: "facebook:photo:123", Permalink: b.Permalink,
			Media: []map[string]any{{"kind": "image", "url": "https://media.fbcdn.net/photo.jpg"}}}}}}}
	return j, b, o
}

func TestPhotoMediaPreservesSavedContentAndRejectsMismatches(t *testing.T) {
	j, b, o := photoMediaFixture()
	got, ok := recapturedPhotoMedia(o, j, b)
	if !ok || got.Text != b.Text || got.Author != b.Author || got.PlatformID != b.PlatformID || got.EvidenceKey != b.EvidenceKey || got.Permalink != b.Permalink || len(got.Media) != 1 {
		t.Fatal("saved photo was not preserved")
	}
	for name, mutate := range map[string]func(*domain.MediaRecapture, *domain.Block, *domain.Observation){
		"bridge": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) { j.Payload = nil },
		"wrong target": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			j.TargetURL = "https://www.facebook.com/photo?fbid=999"
		},
		"wrong page": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.PageURL = "https://www.facebook.com/photo?fbid=999"
		},
		"wrong proof": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) { o.Coverage = nil },
		"extra media": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			v := &o.Snapshots[0].Blocks[0]
			v.Media = append(v.Media, v.Media[0])
		},
		"existing media": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			b.Media = o.Snapshots[0].Blocks[0].Media
		},
		"wrong host": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.Snapshots[0].Blocks[0].Media[0]["url"] = "https://evil.example/a"
		},
	} {
		t.Run(name, func(t *testing.T) {
			j, b, o := photoMediaFixture()
			mutate(&j, &b, &o)
			if _, ok := recapturedPhotoMedia(o, j, b); ok {
				t.Fatal("mismatch accepted")
			}
		})
	}
}

func TestInternalPhotoMediaCompletionPreservesTimelineItem(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	id, _, _ := insertFacebookPlaybackFixture(t, s)
	_, b, o := photoMediaFixture()
	raw, _ := json.Marshal(domain.Observation{Source: domain.SourceFacebook, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{b}}}})
	if _, err := s.db.ExecContext(ctx, `UPDATE observations SET observation_json=? WHERE id='observation-facebook-playback-fixture'`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE timeline_items SET evidence_key=? WHERE id=?`, b.EvidenceKey, id); err != nil {
		t.Fatal(err)
	}
	j, err := s.CreateOwnedMediaRecapture(ctx, id, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, nil, "headless")
	if err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimMediaRecaptureForCollector(ctx, j.ID, "worker", "headless")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteMediaRecapture(ctx, j.ID, o); err == nil {
		t.Fatal("Bridge accepted internal proof")
	}
	done, err := s.CompleteHeadlessMediaRecapture(ctx, j.ID, o)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.timelineEvidence(ctx, id, b.EvidenceKey)
	if err != nil {
		t.Fatal(err)
	}
	if done.Outcome != "recovered" || saved.PlatformID != b.PlatformID || saved.Text != b.Text || saved.Author != b.Author || len(saved.Media) != 1 || saved.MediaRecovery["photoParentResolution"] != nil {
		t.Fatal("photo identity or content changed")
	}
	var count int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("timeline count=%d err=%v", count, err)
	}
}
