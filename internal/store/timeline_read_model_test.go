package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func timelineReadExec(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func timelineReadFixture(t *testing.T) (*Store, routineMemoryTimelineFixture) {
	t.Helper()
	s := openTestStore(t)
	return s, createRoutineMemoryTimelineFixture(t, s, context.Background(), "completed", "x:routine-memory:45123")
}

// Characterize the public reads before moving their implementation. Fixtures
// touch only disposable SQLite data; no production injection seam is needed.
func TestStoreTimelineReadModelCharacterizationContracts(t *testing.T) {
	for _, mode := range []string{"show_all", "hide", "collapse"} {
		t.Run(mode, func(t *testing.T) {
			s := openTestStore(t)
			ctx := context.Background()
			settings, _ := s.GetSettings(ctx)
			settings.SemanticEventMode = mode
			if err := s.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			empty, err := s.ListSessionItems(ctx, "missing-session")
			if err != nil || empty != nil {
				t.Fatal("session empty result changed", empty, err)
			}
			missing, err := s.TimelineItem(ctx, "missing-item")
			if !errors.Is(err, sql.ErrNoRows) || !reflect.DeepEqual(missing, domain.TimelineItem{}) {
				t.Fatal("missing item contract changed", missing, err)
			}
			page, err := s.ListTimeline(ctx, 10, 0)
			if err != nil || len(page) != 0 || (page == nil) != (mode == "show_all") {
				t.Fatal("page empty representation changed", page, err)
			}
			f := createRoutineMemoryTimelineFixture(t, s, ctx, "completed", "x:routine-memory:45123")
			timelineReadExec(t, s, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) SELECT 'read-second',session_id,run_id,source,'x:second',1,item_json,assessment_json,coverage_json,created_at FROM timeline_items WHERE id=?`, f.Item.ID)
			items, err := s.ListSessionItems(ctx, f.Session.ID)
			if err != nil || len(items) != 2 || items[0].ID != f.Item.ID || items[1].ID != "read-second" {
				t.Fatal("rank ordering changed", items, err)
			}
			session, err := s.GetSession(ctx, f.Session.ID)
			if err != nil || !reflect.DeepEqual(session.Items, items) {
				t.Fatal("GetSession no longer shares hydrated item contract", err)
			}
		})
	}
}

func TestStoreTimelineReadModelCharacterizationEvidence(t *testing.T) {
	for _, mode := range []string{"snapshot", "first-observation", "override", "no-evidence", "malformed-base", "malformed-observation", "malformed-override"} {
		t.Run(mode, func(t *testing.T) {
			s, f := timelineReadFixture(t)
			timelineReadExec(t, s, `UPDATE timeline_items SET evidence_snapshot_json=json_object('evidenceKey',evidence_key,'text','snapshot') WHERE id=?`, f.Item.ID)
			timelineReadExec(t, s, `UPDATE observations SET observation_json=json_set(observation_json,'$.snapshots[0].blocks[0].text','first observation'),created_at='2026-01-01T00:00:00Z' WHERE run_id=?`, f.Run.ID)
			timelineReadExec(t, s, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('read-later-command',?,'collect_visible','completed','{}','2026-01-02T00:00:00Z')`, f.Run.ID)
			timelineReadExec(t, s, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) SELECT 'read-later-observation',run_id,'read-later-command',source,json_set(observation_json,'$.snapshots[0].blocks[0].text','later observation'),captured_at,'2026-01-02T00:00:00Z' FROM observations WHERE run_id=? AND command_id<>'read-later-command'`, f.Run.ID)
			want := "first observation"
			switch mode {
			case "snapshot", "no-evidence":
				timelineReadExec(t, s, `DELETE FROM observations WHERE run_id=?`, f.Run.ID)
				want = "snapshot"
				if mode == "no-evidence" {
					timelineReadExec(t, s, `UPDATE timeline_items SET evidence_snapshot_json=NULL WHERE id=?`, f.Item.ID)
				}
			case "override", "malformed-override":
				timelineReadExec(t, s, `INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,payload_json,created_at) VALUES('read-recapture',?,'x','https://pbs.twimg.com/media/fixture.jpg',?,'completed','{}','fixture')`, f.Item.ID, f.Item.EvidenceKey)
				raw := `{"evidenceKey":"x:routine-memory:45123","text":"override"}`
				want = "override"
				if mode == "malformed-override" {
					raw, want = "invalid-json", ""
				}
				timelineReadExec(t, s, `INSERT INTO timeline_evidence_overrides(timeline_id,recapture_id,evidence_json,updated_at) VALUES(?,'read-recapture',?,'fixture')`, f.Item.ID, raw)
			case "malformed-base":
				timelineReadExec(t, s, `UPDATE timeline_items SET item_json='invalid-json',assessment_json='invalid-json',coverage_json='invalid-json' WHERE id=?`, f.Item.ID)
			case "malformed-observation":
				timelineReadExec(t, s, `UPDATE observations SET observation_json='invalid-json' WHERE run_id=?`, f.Run.ID)
			}
			item, err := s.TimelineItem(context.Background(), f.Item.ID)
			if mode == "malformed-observation" {
				var syntax *json.SyntaxError
				if !errors.As(err, &syntax) || !reflect.DeepEqual(item, domain.TimelineItem{}) {
					t.Fatal("observation decode error contract changed", item, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "no-evidence" {
				if item.Evidence != nil {
					t.Fatal("fabricated missing evidence", item.Evidence)
				}
			} else if item.Evidence == nil || item.Evidence.Text != want {
				t.Fatal("evidence precedence changed", item.Evidence, want)
			}
			if mode == "malformed-base" && (item.Item.WhatChanged != "" || item.Assessment.EvidenceKey != "" || item.Coverage != nil) {
				t.Fatal("best-effort base JSON decoding changed", item)
			}
		})
	}
}

func TestStoreTimelineReadModelCharacterizationProjection(t *testing.T) {
	s, f := timelineReadFixture(t)
	ctx := context.Background()
	kept, _, err := s.KeepTimelineFullCopy(ctx, f.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	timelineReadExec(t, s, `INSERT INTO semantic_events(id,canonical_claim,actor,action,object,event_kind,aliases_json,first_seen_at,last_seen_at) VALUES('read-event','fixture claim','actor','action','object','release','[]','fixture','fixture')`)
	timelineReadExec(t, s, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES('read-report','read-event',?,?,?,?,'x','new_event',.9,'fixture','fixture')`, f.Item.ID, f.Session.ID, f.Run.ID, f.Item.EvidenceKey)
	if err := s.SaveAIAssessments(ctx, []domain.AIAssessment{{ID: "read-ai", TimelineID: f.Item.ID, SessionID: f.Session.ID, Stage: "fast", Status: "strong_signals", ConfidenceBand: "medium", EvidenceCodes: []string{"author_declared_ai"}, Provider: "local", DetectorVersion: "fixture", AssessedObject: "social_post", SignalScope: "social_post", CreatedAt: domain.Now()}}); err != nil {
		t.Fatal(err)
	}
	timelineReadExec(t, s, `INSERT INTO media_provenance_assessments(id,timeline_id,session_id,source,media_index,media_kind,target_url,target_url_hash,status,manifest_state,trust_state,ai_origin,provider,verifier_version,created_at) VALUES('read-media',?,?,'x',0,'image','https://pbs.twimg.com/media/example-45123.jpg','read-hash','completed','valid','trusted','generated','fixture','fixture-v1','fixture')`, f.Item.ID, f.Session.ID)
	timelineReadExec(t, s, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,created_at) VALUES('read-feedback',?,?,?,?,'more','2030-01-01T00:00:00Z')`, f.Item.ID, f.Session.ID, f.Run.ID, f.Item.EvidenceKey)
	for _, mode := range []string{"show_all", "hide", "collapse"} {
		t.Run(mode, func(t *testing.T) {
			settings, _ := s.GetSettings(ctx)
			settings.SemanticEventMode = mode
			if err := s.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err := s.db.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			item, err := s.TimelineItem(ctx, f.Item.ID)
			if err != nil || item.SemanticEvent == nil || item.SemanticEvent.EventID != "read-event" || item.AIDetection == nil || item.AIDetection.AssessmentID != "read-ai" || len(item.AIDetection.MediaSignals) != 1 || item.Feedback == nil || item.Feedback.ID != "read-feedback" {
				t.Fatal("combined semantic/AI/provenance/preference projection changed", item, err)
			}
			want := &domain.TimelineMemoryProjection{RetentionTier: kept.RetentionTier, Saved: kept.Saved, PermanentKeep: kept.PermanentKeep}
			if !reflect.DeepEqual(item.PersonalMemory, want) {
				t.Fatal("bounded memory projection changed", item.PersonalMemory, want)
			}
			raw, err := json.Marshal(item.PersonalMemory)
			if err != nil || strings.Contains(string(raw), kept.ID) || strings.Contains(string(raw), "fullContent") || strings.Contains(string(raw), *kept.FullContent) {
				t.Fatal("memory identity/full text leaked into projection", string(raw), err)
			}
			page, err := s.ListTimeline(ctx, 10, 0)
			if err != nil || len(page) != 1 || !reflect.DeepEqual(page[0].AIDetection, item.AIDetection) || !reflect.DeepEqual(page[0].PersonalMemory, item.PersonalMemory) || !reflect.DeepEqual(page[0].Feedback, item.Feedback) || (page[0].SemanticEvent == nil) != (mode == "show_all") {
				t.Fatal("page hydration contract changed", page, err)
			}
			if err := s.db.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil || after != before {
				t.Fatal("read-model wrote durable data", before, after, err)
			}
		})
	}
}

func TestStoreTimelineReadModelCharacterizationPreference(t *testing.T) {
	for _, mode := range []string{"calibration", "routine-tie", "id-tie", "newer-calibration"} {
		t.Run(mode, func(t *testing.T) {
			s, f := timelineReadFixture(t)
			timelineReadExec(t, s, `INSERT INTO calibration_sessions(id,session_id,trigger_kind,status,max_items,sample_count,created_at,updated_at) VALUES('read-calibration',?,'first_run','reviewing',2,1,'fixture','fixture')`, f.Session.ID)
			timelineReadExec(t, s, `INSERT INTO calibration_samples(calibration_session_id,ordinal,run_id,evidence_key,source,candidate_json,label,labeled_at) VALUES('read-calibration',0,?,?,'x','{}','less_like_this','2030-01-01T00:00:00Z')`, f.Run.ID, f.Item.EvidenceKey)
			wantID, wantOrigin, wantDirection := "calibration:read-calibration:0", "calibration", "less"
			if mode != "calibration" {
				timelineReadExec(t, s, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,created_at) VALUES('read-a',?,?,?,?,'more','2030-01-01T00:00:00Z')`, f.Item.ID, f.Session.ID, f.Run.ID, f.Item.EvidenceKey)
				wantID, wantOrigin, wantDirection = "read-a", "routine", "more"
			}
			if mode == "id-tie" {
				timelineReadExec(t, s, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,reason,created_at) VALUES('read-z',?,?,?,?,'less','not_interested','2030-01-01T00:00:00Z')`, f.Item.ID, f.Session.ID, f.Run.ID, f.Item.EvidenceKey)
				wantID, wantDirection = "read-z", "less"
			}
			if mode == "newer-calibration" {
				timelineReadExec(t, s, `UPDATE calibration_samples SET labeled_at='2030-01-02T00:00:00Z'`)
				wantID, wantOrigin, wantDirection = "calibration:read-calibration:0", "calibration", "less"
			}
			item, err := s.TimelineItem(context.Background(), f.Item.ID)
			if err != nil || item.Feedback == nil || item.Feedback.ID != wantID || item.Feedback.Origin != wantOrigin || item.Feedback.Direction != wantDirection || (item.Feedback.Reason != nil) != (wantDirection == "less") {
				t.Fatal("feedback timestamp/authority/id precedence changed", item.Feedback, err)
			}
		})
	}
}

func TestStoreTimelineReadModelCharacterizationFailures(t *testing.T) {
	for _, mode := range []string{"observation", "override", "semantic", "ai", "preference", "memory-resolve", "memory-read", "cancelled", "connection-wait"} {
		t.Run(mode, func(t *testing.T) {
			s, f := timelineReadFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want, prefix := "", "SQL logic error: "
			switch mode {
			case "observation":
				timelineReadExec(t, s, `UPDATE observations SET observation_json='invalid-json'`)
				timelineReadExec(t, s, `DROP TABLE timeline_evidence_overrides`)
				prefix, want = "invalid character", "invalid character"
			case "override":
				timelineReadExec(t, s, `DROP TABLE timeline_evidence_overrides`)
				timelineReadExec(t, s, `DROP TABLE semantic_event_reports`)
				want = "no such table: timeline_evidence_overrides"
			case "semantic":
				timelineReadExec(t, s, `DROP TABLE semantic_event_reports`)
				timelineReadExec(t, s, `DROP TABLE media_provenance_assessments`)
				want = "no such table: semantic_event_reports"
			case "ai":
				timelineReadExec(t, s, `DROP TABLE media_provenance_assessments`)
				timelineReadExec(t, s, `DROP TABLE feedback_events`)
				want = "no such table: media_provenance_assessments"
			case "preference":
				timelineReadExec(t, s, `DROP TABLE feedback_events`)
				timelineReadExec(t, s, `DROP TABLE memory_identity_aliases`)
				want = "no such table: feedback_events"
			case "memory-resolve":
				timelineReadExec(t, s, `DROP TABLE memory_identity_aliases`)
				prefix, want = "resolve Timeline memory projection: ", "no such table: memory_identity_aliases"
			case "memory-read":
				kept, _, err := s.KeepTimelineFullCopy(ctx, f.Item.ID)
				if err != nil {
					t.Fatal(err)
				}
				timelineReadExec(t, s, `UPDATE memory_items SET tags_json='invalid-json' WHERE id=?`, kept.ID)
				prefix, want = "read Timeline memory projection: ", "decode memory tags"
			case "cancelled":
				cancel()
			case "connection-wait":
				conn, err := s.db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 30*time.Millisecond)
				defer stop()
			}
			item, err := s.TimelineItem(ctx, f.Item.ID)
			if !reflect.DeepEqual(item, domain.TimelineItem{}) || err == nil {
				t.Fatal("failure returned a partial successful item", item, err)
			}
			if mode == "cancelled" || mode == "connection-wait" {
				wantErr := context.Canceled
				if mode == "connection-wait" {
					wantErr = context.DeadlineExceeded
				}
				if !errors.Is(err, wantErr) {
					t.Fatal("caller context lost", err)
				}
			} else if !strings.Contains(err.Error(), want) || !strings.HasPrefix(err.Error(), prefix) {
				t.Fatal("read/error ordering or wrapping changed", mode, err)
			}
		})
	}
}

func TestStoreTimelineReadModelCharacterizationMemoryAbsence(t *testing.T) {
	for _, mode := range []string{"missing", "invalid-identity", "forgotten"} {
		t.Run(mode, func(t *testing.T) {
			s, f := timelineReadFixture(t)
			if mode == "invalid-identity" {
				timelineReadExec(t, s, `DELETE FROM observations WHERE run_id=?`, f.Run.ID)
				timelineReadExec(t, s, `UPDATE timeline_items SET evidence_key='',item_json='{}',assessment_json='{}',evidence_snapshot_json=NULL WHERE id=?`, f.Item.ID)
			}
			if mode == "forgotten" {
				kept, _, err := s.KeepTimelineFullCopy(context.Background(), f.Item.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.ForgetMemory(context.Background(), kept.ID); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			item, err := s.TimelineItem(ctx, f.Item.ID)
			if err != nil || item.PersonalMemory != nil {
				t.Fatal("invalid/missing/nonactive memory became fatal or fabricated", item.PersonalMemory, err)
			}
			if stats := s.db.Stats(); stats.MaxOpenConnections != 1 {
				t.Fatal(fmt.Sprintf("fixture must exercise single-connection nested reads: %+v", stats))
			}
		})
	}
}
