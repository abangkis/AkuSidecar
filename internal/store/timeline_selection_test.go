package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Frozen reference to the previous full-hydration path. This compares complete
// public items, not just IDs, including semantic, evidence and memory fields.
func legacyTimelinePage(ctx context.Context, s *Store, mode string, limit, offset int) ([]domain.TimelineItem, error) {
	items, err := s.listItems(ctx, `WHERE COALESCE((SELECT state FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),batch_state) IN ('','visible')`+timelinePresentationOrderSQL+` LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	result := make([]domain.TimelineItem, 0, limit)
	seen, included := 0, 0
	for _, item := range items {
		if item.SemanticEvent != nil && item.SemanticEvent.Relation == "duplicate_report" {
			if mode == "collapse" && seen >= offset && included <= limit {
				result = append(result, item)
			}
			continue
		}
		if seen < offset {
			seen++
			continue
		}
		if included >= limit {
			break
		}
		result = append(result, item)
		seen++
		included++
	}
	return result, nil
}

func TestTimelinePageSelectionPreservesCompleteResultsAndDuplicateBoundaries(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	session, first := insertSemanticTimelineFixture(t, s, "duplicate_report")
	runs, err := s.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	relations := []string{"new_event", "duplicate_report", "duplicate_report", "material_update", "duplicate_report", "new_event"}
	ids := []string{first}
	for i, relation := range relations {
		id := fmt.Sprintf("timeline-select-%d", i+1)
		ids = append(ids, id)
		evidence := fmt.Sprintf("x:select-%d", i+1)
		item, _ := json.Marshal(domain.ReasonedItem{Source: domain.SourceX, EvidenceKey: evidence, WhatChanged: fmt.Sprintf("Report %d", i)})
		if _, err := s.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, session.ID, runs[0].ID, "x", evidence, i+1, string(item), "{}", "{}", domain.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("report-select-%d", i), "event-existing", id, session.ID, runs[0].ID, evidence, "x", relation, .9, "fixture", domain.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"collapse", "hide"} {
		settings, _ := s.GetSettings(ctx)
		settings.SemanticEventMode = mode
		if err := s.SaveSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		for _, page := range []struct{ limit, offset int }{{1, 0}, {1, 1}, {2, 0}, {2, 1}, {10, 0}, {2, 10}, {0, 0}} {
			want, err := legacyTimelinePage(ctx, s, mode, page.limit, page.offset)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.ListTimeline(ctx, page.limit, page.offset)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("complete result changed: mode=%s limit=%d offset=%d", mode, page.limit, page.offset)
			}
		}
	}
	selected, err := s.timelinePageIDs(ctx, "collapse", 1, 0)
	if err != nil || !reflect.DeepEqual(selected, ids[:4]) {
		t.Fatal("leading/trailing duplicate boundary changed", selected, err)
	}
	selected, err = s.timelinePageIDs(ctx, "collapse", 1, 1)
	if err != nil || !reflect.DeepEqual(selected, ids[2:6]) {
		t.Fatal("unique offset boundary changed", selected, err)
	}
	// Corrections and hidden/prepared items must affect selection before hydration.
	if _, err := s.db.ExecContext(ctx, `UPDATE semantic_event_reports SET relation='new_event',corrected=1 WHERE timeline_id=?`, ids[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE timeline_items SET batch_state='prepared' WHERE id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.GetSettings(ctx)
	settings.SemanticEventMode = "collapse"
	if err := s.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	want, err := legacyTimelinePage(ctx, s, "collapse", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ListTimeline(ctx, 2, 0)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("correction/visibility changed output", err)
	}
}

func TestTimelinePageDoesNotHydrateHistoryOutsidePage(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	old := createRoutineMemoryTimelineFixture(t, s, ctx, "completed", "x:routine-memory:11111")
	current := createRoutineMemoryTimelineFixture(t, s, ctx, "completed", "x:routine-memory:22222")
	for _, row := range []struct{ id, at string }{{old.Session.ID, "2026-10-03T10:00:00Z"}, {current.Session.ID, "2026-10-04T10:00:00Z"}} {
		if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET completed_at=? WHERE id=?`, row.at, row.id); err != nil {
			t.Fatal(err)
		}
	}
	// An unreadable observation outside the page detects accidental broad detail
	// hydration deterministically, without a fragile timing assertion.
	if _, err := s.db.ExecContext(ctx, `UPDATE observations SET observation_json='{' WHERE run_id=?`, old.Run.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListTimeline(ctx, 1, 0)
	if err != nil || len(got) != 1 || got[0].ID != current.Item.ID || got[0].Evidence == nil {
		t.Fatal("outside-page evidence was hydrated", err)
	}
	if _, err := s.ListTimeline(ctx, 1, 1); err == nil {
		t.Fatal("selected evidence errors were hidden")
	}
}

func TestTimelinePagePreservesExistingCandidateHorizon(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	session, _ := insertSemanticTimelineFixture(t, s, "duplicate_report")
	runs, err := s.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<1005)
		INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at)
		SELECT 'timeline-horizon-'||i,?,?,'x','x:horizon-'||i,i,'{}','{}','{}',? FROM n`, session.ID, runs[0].ID, domain.Now()); err != nil {
		t.Fatal(err)
	}
	ids, err := s.timelinePageIDs(ctx, "collapse", 3, 998)
	if err != nil || len(ids) != 1 || ids[0] != "timeline-horizon-999" {
		t.Fatal("candidate horizon changed", ids, err)
	}
	ids, err = s.timelinePageIDs(ctx, "hide", 3, 999)
	if err != nil || len(ids) != 0 {
		t.Fatal("outside-horizon item included", ids, err)
	}
}

// Explicit opt-in read-only performance/equivalence check of an existing DB.
// OpenReadOnly does not initialize, migrate, retain or modify the user's data.
func TestTimelinePageReadOnlyPerformance(t *testing.T) {
	path := os.Getenv("AKU_TIMELINE_PERF_READONLY_DB")
	if path == "" {
		t.Skip("requires explicit read-only database path")
	}
	if !filepath.IsAbs(path) {
		t.Fatal("absolute database path required")
	}
	s, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	settings, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.SemanticEventMode == "show_all" {
		t.Skip("show_all already hydrates its bounded page")
	}
	start := time.Now()
	want, err := legacyTimelinePage(ctx, s, settings.SemanticEventMode, 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := time.Since(start)
	start = time.Now()
	got, err := s.ListTimeline(ctx, 12, 0)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Since(start)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("complete item projection changed or database changed during the comparison")
	}
	t.Logf("read-only equivalence verified: requested=12 returned=%d old_ms=%d new_ms=%d", len(got), before.Milliseconds(), after.Milliseconds())
}
