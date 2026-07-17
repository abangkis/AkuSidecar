package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestMediaRecaptureReplacesEvidenceWithoutCreatingTimelineItems(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, evidenceKey := insertUnavailableMediaFixture(t, state)

	job, err := state.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" || job.Payload["mode"] != "recapture_media" {
		t.Fatalf("job=%+v", job)
	}
	if _, err := state.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground); err == nil {
		t.Fatal("a second active recapture must be rejected")
	}
	job, err = state.ClaimMediaRecapture(ctx, job.ID, "bridge-test")
	if err != nil {
		t.Fatal(err)
	}
	observation := domain.Observation{
		Source:     domain.SourceX,
		PageURL:    "https://x.com/example/status/123",
		PageTitle:  "Post",
		CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{
			Index:      0,
			CapturedAt: domain.Now(),
			Blocks: []domain.Block{{
				EvidenceKey:   evidenceKey,
				Author:        "Example",
				Text:          "A sufficiently long post body for a deterministic media recapture fixture.",
				Permalink:     "https://x.com/example/status/123",
				Media:         []map[string]any{{"kind": "image", "url": "https://pbs.twimg.com/media/example.jpg"}},
				MediaRecovery: map[string]any{"outcome": "recovered", "attempts": 1},
			}},
		}},
	}
	job, err = state.CompleteMediaRecapture(ctx, job.ID, observation)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" || job.Outcome != "recovered" {
		t.Fatalf("job=%+v", job)
	}
	items, err := state.ListTimeline(ctx, 24, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Evidence == nil || len(items[0].Evidence.Media) != 1 {
		t.Fatalf("items=%+v", items)
	}
	var count int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("timeline count=%d err=%v", count, err)
	}
}

func TestForegroundMediaRecaptureRequiresUnavailableBackgroundAttempt(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, evidenceKey := insertUnavailableMediaFixture(t, state)

	if _, err := state.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureForeground); err == nil {
		t.Fatal("foreground recapture must require a completed unavailable background attempt")
	}
	background, err := state.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	background, err = state.ClaimMediaRecapture(ctx, background.ID, "bridge-test")
	if err != nil {
		t.Fatal(err)
	}
	observation := domain.Observation{
		Source: domain.SourceX,
		Snapshots: []domain.Snapshot{{
			Index: 0,
			Blocks: []domain.Block{{
				EvidenceKey:   evidenceKey,
				Author:        "Example",
				Text:          "A sufficiently long post body whose media remains unavailable in the background.",
				Permalink:     "https://x.com/example/status/123",
				MediaRecovery: map[string]any{"outcome": "unavailable", "attempts": 1},
			}},
		}},
	}
	background, err = state.CompleteMediaRecapture(ctx, background.ID, observation)
	if err != nil {
		t.Fatal(err)
	}
	if background.Outcome != "unavailable" {
		t.Fatalf("background=%+v", background)
	}
	foreground, err := state.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureForeground)
	if err != nil {
		t.Fatal(err)
	}
	if foreground.Payload["foregroundAuthorized"] != true || foreground.Payload["captureVisibilityPolicy"] != "quiet" {
		t.Fatalf("foreground payload=%+v", foreground.Payload)
	}
}

func insertUnavailableMediaFixture(t *testing.T, state *Store) (string, string) {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := state.CreateSession(ctx, "fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	evidenceKey := "x:status:123"
	block := domain.Block{
		EvidenceKey:   evidenceKey,
		Author:        "Example",
		Text:          "A sufficiently long post body for a deterministic unavailable-media fixture.",
		Permalink:     "https://x.com/example/status/123",
		MediaRecovery: map[string]any{"outcome": "unavailable", "attempts": 1},
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Index: 0, CapturedAt: domain.Now(), Blocks: []domain.Block{block}}}}
	observationRaw, _ := json.Marshal(observation)
	itemRaw, _ := json.Marshal(domain.ReasonedItem{Source: domain.SourceX, SourceURL: block.Permalink, SourceURLKind: "native_post", EvidenceKey: evidenceKey})
	assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: evidenceKey})
	now := domain.Now()
	commandID := "command-recapture-fixture"
	if _, err := state.db.ExecContext(ctx, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at,completed_at) VALUES(?,?,?,'completed','{}',?,?)`, commandID, run.ID, "collect_visible", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES(?,?,?,?,?,?,?)`, "observation-recapture-fixture", run.ID, commandID, run.Source, string(observationRaw), now, now); err != nil {
		t.Fatal(err)
	}
	timelineID := "timeline-recapture-fixture"
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, timelineID, session.ID, run.ID, run.Source, evidenceKey, 0, string(itemRaw), string(assessmentRaw), now); err != nil {
		t.Fatal(err)
	}
	return timelineID, evidenceKey
}
