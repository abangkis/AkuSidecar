package store

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Operator-only replay of real read-only acquisition evidence into an isolated
// Store. No browser, production database, or network operation is performed.
func TestPhotoMediaLiveEvidencePreservesSavedBlock(t *testing.T) {
	baselinePath, capturePath := os.Getenv("AKU_PHOTO_BASELINE_EVIDENCE"), os.Getenv("AKU_PHOTO_CAPTURE_EVIDENCE")
	if baselinePath == "" || capturePath == "" {
		t.Skip("requires explicit local Browser baseline and headless media-only evidence")
	}
	var baseline struct {
		Sources map[string]struct {
			Status  string         `json:"status"`
			Drivers []string       `json:"drivers"`
			Blocks  []domain.Block `json:"blocks"`
		} `json:"sources"`
	}
	var capture struct {
		Captures []struct {
			Source string             `json:"source"`
			OK     bool               `json:"ok"`
			Result domain.Observation `json:"result"`
		} `json:"captures"`
	}
	read := func(path string, value any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, value) != nil {
			t.Fatal("local evidence file unreadable or invalid")
		}
	}
	read(baselinePath, &baseline)
	read(capturePath, &capture)
	entry := baseline.Sources["facebook"]
	if entry.Status != "completed" || len(entry.Drivers) != 1 || entry.Drivers[0] != "aku-bridge" || len(entry.Blocks) != 1 ||
		len(capture.Captures) != 1 || !capture.Captures[0].OK || capture.Captures[0].Source != "facebook" {
		t.Fatal("expected one completed Bridge photo baseline and successful headless capture")
	}
	b := entry.Blocks[0]
	if b.EvidenceKey == "" || exactFacebookPhotoID(b.Permalink) == "" || len(b.Media) != 1 {
		t.Fatal("baseline photo identity/media unverified")
	}
	// Simulate a saved missing-image item while retaining all real baseline body
	// fields. This is Store admission evidence, not a new UI Recapture journey.
	b.Media = nil
	b.MediaRecovery = map[string]any{"outcome": "unavailable"}
	o := capture.Captures[0].Result
	ctx := context.Background()
	s := openTestStore(t)
	id, _, _ := insertFacebookPlaybackFixture(t, s)
	raw, err := json.Marshal(domain.Observation{Source: domain.SourceFacebook, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{b}}}})
	if err != nil {
		t.Fatal("baseline serialization failed")
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE observations SET observation_json=? WHERE id='observation-facebook-playback-fixture'`, string(raw)); err != nil {
		t.Fatal("isolated observation setup failed")
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE timeline_items SET evidence_key=? WHERE id=?`, b.EvidenceKey, id); err != nil {
		t.Fatal("isolated timeline setup failed")
	}
	j, err := s.CreateOwnedMediaRecapture(ctx, id, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, nil, "headless")
	if err != nil {
		t.Fatal("media recovery job creation failed")
	}
	j, err = s.ClaimMediaRecaptureForCollector(ctx, j.ID, "live-evidence-replay", "headless")
	if err != nil {
		t.Fatal("media recovery ownership failed")
	}
	done, err := s.CompleteHeadlessMediaRecapture(ctx, j.ID, o)
	if err != nil || done.Outcome != "recovered" {
		t.Fatal("real photo evidence was not admitted as recovered media")
	}
	saved, err := s.timelineEvidence(ctx, id, b.EvidenceKey)
	if err != nil || len(saved.Media) != 1 || saved.Media[0]["url"] != o.Snapshots[0].Blocks[0].Media[0]["url"] {
		t.Fatal("saved recovered image differs from captured evidence")
	}
	saved.Media, saved.MediaRecovery = nil, nil
	b.MediaRecovery = nil
	if !reflect.DeepEqual(saved, b) {
		t.Fatal("recovery changed saved identity, body or other non-media fields")
	}
	var count int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items`).Scan(&count); err != nil || count != 1 {
		t.Fatal("recovery changed timeline item count")
	}
}
