package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestFirstParentFeedDoesNotInferAliasFromIdenticalContent(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _, _ := insertFacebookPlaybackFixture(t, state)
	var runID string
	if err := state.db.QueryRowContext(ctx, `SELECT run_id FROM timeline_items WHERE id=?`, timelineID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	_, photo, parent := photoParentFixture()
	photo.Text = strings.Repeat("Identical content still does not prove a photo-to-parent identity relation. ", 3)
	parent.Snapshots[0].Blocks[0].Text = photo.Text
	original := domain.Observation{Source: domain.SourceFacebook, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{photo}}}}
	tx, err := state.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := state.resolveObservationContentIdentity(ctx, tx, runID, &original, domain.Now()); err != nil {
		t.Fatal(err)
	}
	summary, err := state.resolveObservationContentIdentity(ctx, tx, runID, &parent, domain.Now())
	if err != nil {
		t.Fatal(err)
	}
	if parent.Snapshots[0].Blocks[0].EvidenceKey == photo.EvidenceKey || summary.AliasesReused != 0 || summary.NativeConflicts != 1 {
		t.Fatalf("first encounter guessed an alias: %+v", summary)
	}
}

func photoParentFixture() (domain.MediaRecapture, domain.Block, domain.Observation) {
	original := domain.Block{EvidenceKey: "saved-photo", PlatformID: "facebook:post:123", Permalink: "https://www.facebook.com/photo?fbid=123",
		Author: "Fixture", Text: "An exact saved photo caption for this fixture.", MediaRecovery: map[string]any{"outcome": "unavailable"}}
	job := domain.MediaRecapture{Source: domain.SourceFacebook, EvidenceKey: original.EvidenceKey, TargetURL: original.Permalink,
		Payload: map[string]any{"captureCollector": map[string]any{"version": 1, "backend": "headless"}}}
	parent := original
	parent.EvidenceKey = "parent-key"
	parent.PlatformID = "facebook:post:pfbidABC"
	parent.Permalink = "https://www.facebook.com/fixture/posts/pfbidABC"
	parent.Media = []map[string]any{{"kind": "image", "url": "https://media.fbcdn.net/photo.jpg"}}
	observation := domain.Observation{Source: domain.SourceFacebook, PageURL: parent.Permalink, Coverage: map[string]any{
		"photoParentResolution": map[string]any{"status": "verified", "photoId": "123", "parentPlatformId": parent.PlatformID, "provenance": "structured_photo_parent_and_matching_native_post"}},
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{parent}}}}
	return job, original, observation
}

func TestPhotoParentRecaptureRequiresExactSavedEvidence(t *testing.T) {
	mutations := map[string]func(*domain.MediaRecapture, *domain.Block, *domain.Observation){
		"bridge": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) { j.Payload = nil },
		"wrong photo": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			j.TargetURL = "https://www.facebook.com/photo?fbid=999"
		},
		"duplicate IDs": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) { j.TargetURL += "&fbid=999" },
		"forged status": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) { o.Coverage = nil },
		"wrong author": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.Snapshots[0].Blocks[0].Author = "Other"
		},
		"wrong text": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.Snapshots[0].Blocks[0].Text = "Other"
		},
		"no image": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.Snapshots[0].Blocks[0].Media = nil
		},
		"wrong existing image": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			b.Media = []map[string]any{{"kind": "image", "url": "https://media.fbcdn.net/other.jpg"}}
		},
		"wrong parent": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.Snapshots[0].Blocks[0].PlatformID = "facebook:post:999"
		},
		"wrong page": func(j *domain.MediaRecapture, b *domain.Block, o *domain.Observation) {
			o.PageURL = "https://www.facebook.com/fixture/posts/pfbidOTHER"
		},
	}
	j, b, o := photoParentFixture()
	got, ok := recapturedPhotoParent(o, j, b)
	if !ok || got.EvidenceKey != j.EvidenceKey || got.PlatformID == b.PlatformID {
		t.Fatal("valid relation did not preserve separate identities")
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			j, b, o := photoParentFixture()
			mutate(&j, &b, &o)
			if _, ok := recapturedPhotoParent(o, j, b); ok {
				t.Fatal("unverified relation accepted")
			}
		})
	}
}

func TestInternalPhotoParentRecaptureUpdatesExistingItemOnly(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _, _ := insertFacebookPlaybackFixture(t, state)
	_, original, observation := photoParentFixture()
	raw, _ := json.Marshal(domain.Observation{Source: domain.SourceFacebook, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{original}}}})
	if _, err := state.db.ExecContext(ctx, `UPDATE observations SET observation_json=? WHERE id='observation-facebook-playback-fixture'`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE timeline_items SET evidence_key=? WHERE id=?`, original.EvidenceKey, timelineID); err != nil {
		t.Fatal(err)
	}
	job, err := state.CreateOwnedMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, map[string]any{"driver": "headless"}, "headless")
	if err != nil {
		t.Fatal(err)
	}
	job, err = state.ClaimMediaRecaptureForCollector(ctx, job.ID, "internal-worker", "headless")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.CompleteMediaRecapture(ctx, job.ID, observation); err == nil {
		t.Fatal("Bridge entrypoint accepted parent relation")
	}
	completed, err := state.CompleteHeadlessMediaRecapture(ctx, job.ID, observation)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Outcome != "recovered" {
		t.Fatalf("outcome %s", completed.Outcome)
	}
	stored, err := state.timelineEvidence(ctx, timelineID, original.EvidenceKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.EvidenceKey != original.EvidenceKey || stored.PlatformID != "facebook:post:pfbidABC" || len(stored.Media) != 1 {
		t.Fatal("wrong saved evidence")
	}
	tx, err := state.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	summary, err := state.resolveObservationContentIdentity(ctx, tx, "fixture-repeat", &observation, domain.Now())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshots[0].Blocks[0].EvidenceKey != original.EvidenceKey || summary.AliasesReused != 1 {
		t.Fatal("verified parent did not reuse saved item key")
	}
	wrong := observation.Snapshots[0].Blocks[0]
	wrong.Author = "Unrelated"
	if key, err := savedPhotoParentKey(ctx, tx, wrong); err != nil || key != "" {
		t.Fatal("wrong owner reused a relation")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// Re-run the claimed fixture as a direct parent capture to verify that
	// refreshing media cannot erase the previously persisted relation.
	if _, err := state.db.ExecContext(ctx, `UPDATE media_recaptures SET status='claimed',target_url=? WHERE id=?`, stored.Permalink, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteHeadlessMediaRecapture(ctx, job.ID, observation); err != nil {
		t.Fatal(err)
	}
	refreshed, err := state.timelineEvidence(ctx, timelineID, original.EvidenceKey)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.MediaRecovery["photoParentResolution"] == nil || refreshed.EvidenceKey != original.EvidenceKey {
		t.Fatal("direct refresh erased established relation")
	}
	var count int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("timeline count %d, err %v", count, err)
	}
}

func TestCallerPhotoParentClaimDoesNotCreateAlias(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _, _ := insertFacebookPlaybackFixture(t, state)
	_, original, observation := photoParentFixture()
	// A caller-controlled recovery map cannot create authority through the ordinary route.
	original.MediaRecovery["photoParentResolution"] = map[string]any{"status": "verified", "provenance": "internal_headless_and_saved_photo_evidence", "parentPlatformId": observation.Snapshots[0].Blocks[0].PlatformID}
	observation.Snapshots[0].Blocks[0].MediaRecovery = original.MediaRecovery
	tx, err := state.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if key, err := savedPhotoParentKey(ctx, tx, observation.Snapshots[0].Blocks[0]); err != nil || key != "" {
		t.Fatalf("unpersisted claim became alias for %s", timelineID)
	}
}

func TestOrdinaryRecaptureStripsCallerIssuedPhotoAuthority(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, key := insertUnavailableMediaFixture(t, state)
	job, err := state.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	job, err = state.ClaimMediaRecapture(ctx, job.ID, "bridge-test")
	if err != nil {
		t.Fatal(err)
	}
	_, _, observation := photoParentFixture()
	observation.Source = domain.SourceX
	observation.Snapshots[0].Blocks[0].EvidenceKey = key
	observation.Snapshots[0].Blocks[0].MediaRecovery = map[string]any{"photoParentResolution": map[string]any{"status": "verified", "provenance": "internal_headless_and_saved_photo_evidence"}}
	if _, err := state.CompleteMediaRecapture(ctx, job.ID, observation); err != nil {
		t.Fatal(err)
	}
	stored, err := state.timelineEvidence(ctx, timelineID, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := stored.MediaRecovery["photoParentResolution"]; exists {
		t.Fatal("caller authority persisted")
	}
}
