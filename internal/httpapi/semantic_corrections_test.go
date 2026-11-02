package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const semanticCorrectionSubmit = "/api/timeline/semantic-item/event-correction"
const semanticCorrectionSuggestions = "/api/timeline/semantic-item/event-suggestions"
const semanticCorrectionMerge = `{"action":"same_event","targetEventId":"event-target-1","relation":"material_update"}`

type semanticCorrectionsFixture struct{ sessionUpdatesFixture }

type semanticCorrectionBody struct {
	io.Reader
	reads, closes int
}

func (b *semanticCorrectionBody) Read(p []byte) (int, error) {
	b.reads++
	return b.Reader.Read(p)
}
func (b *semanticCorrectionBody) Close() error { b.closes++; return nil }

func newSemanticCorrectionsFixture(t *testing.T) semanticCorrectionsFixture {
	t.Helper()
	f := semanticCorrectionsFixture{newSessionUpdatesFixture(t)}
	f.seed(t, "semantic", "completed", false)
	now := f.s.store.Now().UTC()
	f.exec(t, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('semantic-run','semantic','linkedin',0,'completed','completed',?)`, now.Format(time.RFC3339Nano))
	item := domain.ReasonedItem{Source: domain.SourceLinkedIn, EvidenceKey: "PRIVATE_EVIDENCE_KEY", WhatChanged: "Acme launches Widget release", Author: "Acme", EventKey: "widget-release"}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,evidence_snapshot_json,created_at) VALUES('semantic-item','semantic','semantic-run','linkedin','PRIVATE_EVIDENCE_KEY',0,?,'{"topicTags":["widget"]}','{"private":"PRIVATE_COVERAGE"}','{"text":"PRIVATE_CAPTURE_TEXT"}',?)`, string(raw), now.Format(time.RFC3339Nano))
	f.event(t, "event-current", now)
	for i, id := range []string{"event-target-1", "event-target-2", "event-target-3", "event-target-4"} {
		f.event(t, id, now.Add(time.Duration(i+1)*time.Minute))
	}
	f.exec(t, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES('semantic-report','event-current','semantic-item','semantic','semantic-run','PRIVATE_EVIDENCE_KEY','linkedin','duplicate_report',0.98,'PRIVATE_REPORT_REASON',?)`, now.Format(time.RFC3339Nano))
	return f
}

func (f semanticCorrectionsFixture) event(t *testing.T, id string, at time.Time) {
	t.Helper()
	f.exec(t, `INSERT INTO semantic_events(id,canonical_claim,actor,action,object,event_kind,aliases_json,first_seen_at,last_seen_at) VALUES(?,'Acme launches Widget release','Acme','launches','Widget','release','["PRIVATE_ALIAS"]',?,?)`, id, at.Format(time.RFC3339Nano), at.Format(time.RFC3339Nano))
}

func (f semanticCorrectionsFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f semanticCorrectionsFixture) correction(t *testing.T, w *httptest.ResponseRecorder, status int) domain.EventCorrection {
	t.Helper()
	payload := f.json(t, w, status)
	var fields map[string]json.RawMessage
	if len(payload) != 1 || json.Unmarshal(payload["correction"], &fields) != nil || len(fields) != 7 {
		t.Fatal("correction envelope changed", w.Body)
	}
	for _, name := range strings.Fields("id timelineId action fromEventId toEventId toRelation createdAt") {
		if _, ok := fields[name]; !ok {
			t.Fatal("missing correction field", name)
		}
	}
	var value domain.EventCorrection
	if json.Unmarshal(payload["correction"], &value) != nil || value.ID == "" || value.TimelineID != "semantic-item" || value.FromEventID != "event-current" || value.UndoneAt != nil {
		t.Fatal("correction projection changed", w.Body)
	}
	if _, err := time.Parse(time.RFC3339Nano, value.CreatedAt); err != nil {
		t.Fatal(err)
	}
	f.public(t, w.Body.String())
	return value
}

func (f semanticCorrectionsFixture) public(t *testing.T, raw string) {
	t.Helper()
	for _, sentinel := range []string{"PRIVATE_EVIDENCE_KEY", "PRIVATE_CAPTURE_TEXT", "PRIVATE_COVERAGE", "PRIVATE_REPORT_REASON", "PRIVATE_ALIAS"} {
		if strings.Contains(raw, sentinel) {
			t.Fatal("HTTP projection exposed internal evidence", sentinel)
		}
	}
}

func semanticCorrectionUndo(id string) string { return "/api/event-corrections/" + id + "/undo" }

func (f semanticCorrectionsFixture) report(t *testing.T, event, relation string, corrected int) {
	t.Helper()
	var gotEvent, gotRelation string
	var gotCorrected int
	if err := f.db.QueryRow(`SELECT event_id,relation,corrected FROM semantic_event_reports WHERE id='semantic-report'`).Scan(&gotEvent, &gotRelation, &gotCorrected); err != nil {
		t.Fatal(err)
	}
	if gotEvent != event || gotRelation != relation || gotCorrected != corrected {
		t.Fatalf("report=%s/%s/%d want=%s/%s/%d", gotEvent, gotRelation, gotCorrected, event, relation, corrected)
	}
}

func TestSemanticCorrectionsCharacterizationSuggestions(t *testing.T) {
	f := newSemanticCorrectionsFixture(t)
	for _, tc := range []struct {
		query string
		count int
	}{{"", 3}, {"?limit=1", 1}, {"?limit=2", 2}, {"?limit=0", 1}, {"?limit=-2", 1}, {"?limit=20", 3}, {"?limit=invalid", 3}} {
		t.Run(tc.query, func(t *testing.T) {
			w := f.serve(sessionUpdatesRequest("GET", semanticCorrectionSuggestions+tc.query, ""))
			payload := f.json(t, w, 200)
			var values []map[string]json.RawMessage
			if len(payload) != 1 || json.Unmarshal(payload["suggestions"], &values) != nil || len(values) != tc.count {
				t.Fatal("suggestion envelope/limit changed", w.Body)
			}
			for i, value := range values {
				var id string
				_ = json.Unmarshal(value["eventId"], &id)
				if len(value) != 6 || id != []string{"event-target-4", "event-target-3", "event-target-2"}[i] {
					t.Fatal("suggestion projection/rank/current exclusion changed", w.Body)
				}
				for _, key := range strings.Fields("eventId canonicalClaim actor object reportCount lastSeenAt") {
					if _, ok := value[key]; !ok {
						t.Fatal("suggestion field missing", key)
					}
				}
			}
			f.public(t, w.Body.String())
		})
	}
	missing := f.serve(sessionUpdatesRequest("GET", "/api/timeline/missing/event-suggestions", ""))
	if payload := f.json(t, missing, 200); len(payload) != 1 || string(payload["suggestions"]) != "null" {
		t.Fatal("missing Timeline item no longer projects null", missing.Body)
	}
	f.exec(t, `DELETE FROM semantic_events WHERE id!='event-current'`)
	empty := f.serve(sessionUpdatesRequest("GET", semanticCorrectionSuggestions, ""))
	if payload := f.json(t, empty, 200); len(payload) != 1 || string(payload["suggestions"]) != "[]" {
		t.Fatal("empty suggestions no longer projects array", empty.Body)
	}
	if f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections`) != 0 {
		t.Fatal("suggestions changed durable corrections")
	}
}

func TestSemanticCorrectionsCharacterizationWorkflow(t *testing.T) {
	for _, tc := range []struct{ name, body, action, relation string }{
		{"split", `{"action":"not_same_event","targetEventId":"ignored","relation":"ignored"}`, "not_same_event", "new_event"},
		{"merge_default", `{"action":"same_event","targetEventId":"event-target-1"}`, "same_event", "duplicate_report"},
		{"merge_update", semanticCorrectionMerge, "same_event", "material_update"},
		{"trailing_existing_behavior", semanticCorrectionMerge + `{}`, "same_event", "material_update"},
		{"duplicate_existing_behavior", `{"action":"invalid","action":"same_event","targetEventId":"event-target-1"}`, "same_event", "duplicate_report"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSemanticCorrectionsFixture(t)
			before, err := f.s.store.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			value := f.correction(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionSubmit, tc.body)), 201)
			if value.Action != tc.action || value.ToRelation != tc.relation || value.ToEventID == "" || tc.action == "same_event" && value.ToEventID != "event-target-1" || tc.action == "not_same_event" && value.ToEventID == value.FromEventID {
				t.Fatal("correction action/relation changed", value)
			}
			f.report(t, value.ToEventID, tc.relation, 1)
			if f.count(t, `SELECT COUNT(*) FROM semantic_event_constraints`) != 1 {
				t.Fatal("correction constraint missing")
			}
			f.error(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionSubmit, tc.body)), 400, "invalid_request", "existing event correction must be undone first")
			request := sessionUpdatesRequest("POST", semanticCorrectionUndo(value.ID), "not JSON")
			request.Header.Set("Content-Type", "text/plain")
			body := &semanticCorrectionBody{Reader: strings.NewReader("not JSON")}
			request.Body = body
			undone := f.correction(t, f.serve(request), 200)
			if !reflect.DeepEqual(value, undone) || body.reads != 0 || body.closes != 0 {
				t.Fatal("undo response or ignored body changed", undone)
			}
			f.report(t, "event-current", "duplicate_report", 0)
			if f.count(t, `SELECT COUNT(*) FROM semantic_event_constraints`) != 0 || f.count(t, `SELECT COUNT(*) FROM semantic_novelty_constraints`) != 0 || f.count(t, `SELECT COUNT(*) FROM semantic_event_deltas`) != 0 || f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections WHERE undone_at IS NOT NULL`) != 1 {
				t.Fatal("undo failed to restore durable relationships")
			}
			f.error(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionUndo(value.ID), "")), 404, "not_found", "event correction not found")
			after, err := f.s.store.GetSettings(context.Background())
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("correction changed settings")
			}
		})
	}
}

func TestSemanticCorrectionsCharacterizationValidation(t *testing.T) {
	f := newSemanticCorrectionsFixture(t)
	for _, tc := range []struct {
		name, path, body, media string
		status                  int
		code                    string
	}{
		{"missing_action", semanticCorrectionSubmit, `{}`, "application/json", 400, "invalid_request"},
		{"null", semanticCorrectionSubmit, `null`, "application/json", 400, "invalid_request"},
		{"invalid_action", semanticCorrectionSubmit, `{"action":"other"}`, "application/json", 400, "invalid_request"},
		{"missing_target", semanticCorrectionSubmit, `{"action":"same_event"}`, "application/json", 400, "invalid_request"},
		{"unknown_target", semanticCorrectionSubmit, `{"action":"same_event","targetEventId":"absent"}`, "application/json", 400, "invalid_request"},
		{"invalid_relation", semanticCorrectionSubmit, `{"action":"same_event","relation":"new_event"}`, "application/json", 400, "invalid_request"},
		{"unknown_field", semanticCorrectionSubmit, `{"action":"not_same_event","extra":true}`, "application/json", 400, "invalid_request"},
		{"wrong_type", semanticCorrectionSubmit, `{"action":3}`, "application/json", 400, "invalid_request"},
		{"malformed", semanticCorrectionSubmit, `{`, "application/json", 400, "invalid_request"},
		{"media", semanticCorrectionSubmit, semanticCorrectionMerge, "text/plain", 415, "unsupported_media_type"},
		{"missing_report", "/api/timeline/absent/event-correction", semanticCorrectionMerge, "application/json", 404, "not_found"},
		{"validation_before_missing", "/api/timeline/absent/event-correction", `{}`, "application/json", 400, "invalid_request"},
		{"missing_undo", semanticCorrectionUndo("absent"), "", "", 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sessionUpdatesRequest("POST", tc.path, tc.body)
			r.Header.Set("Content-Type", tc.media)
			f.error(t, f.serve(r), tc.status, tc.code, "")
			f.report(t, "event-current", "duplicate_report", 0)
			if f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections`) != 0 {
				t.Fatal("invalid request committed a correction")
			}
		})
	}
}

func TestSemanticCorrectionsCharacterizationRoutingAndSecurity(t *testing.T) {
	for _, action := range []string{"suggestions", "submit", "undo"} {
		t.Run(action, func(t *testing.T) {
			for _, mode := range []string{"canonical", "slash", "encoded_slash", "nested_id", "wrong_method", "foreign_host", "foreign_origin", "preflight"} {
				t.Run(mode, func(t *testing.T) {
					f := newSemanticCorrectionsFixture(t)
					method, target, body, status := "POST", semanticCorrectionSubmit, semanticCorrectionMerge, 201
					if action == "suggestions" {
						method, target, body, status = "GET", semanticCorrectionSuggestions, "", 200
					} else if action == "undo" {
						value := f.correction(t, f.serve(sessionUpdatesRequest("POST", target, body)), 201)
						target, body, status = semanticCorrectionUndo(value.ID), "", 200
					}
					if mode == "slash" {
						target += "/"
					}
					if mode == "encoded_slash" {
						target += "%2F"
					}
					if mode == "nested_id" {
						prefix := "/api/timeline/"
						if action == "undo" {
							prefix = "/api/event-corrections/"
						}
						target = strings.Replace(target, prefix, prefix+"nested/", 1)
					}
					r := sessionUpdatesRequest(method, target, body)
					switch mode {
					case "wrong_method":
						if action == "suggestions" {
							r.Method = "POST"
						} else {
							r.Method = "GET"
						}
						status = 404
					case "foreign_host":
						r.Host, status = "example.com", 403
					case "foreign_origin":
						r.Header.Set("Origin", "https://example.com")
						status = 403
					case "preflight":
						r.Method, status = "OPTIONS", 204
					}
					w := f.serve(r)
					if w.Code != status {
						t.Fatalf("route status=%d want=%d body=%s", w.Code, status, w.Body)
					}
					if status == 404 {
						f.error(t, w, 404, "not_found", "")
					}
					if status == 403 && w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
						t.Fatal("origin/host rejection reached API")
					}
					if status == 204 && w.Body.Len() != 0 {
						t.Fatal("preflight body changed")
					}
				})
			}
		})
	}
}

func TestSemanticCorrectionsCharacterizationContextAndStoreErrors(t *testing.T) {
	for _, mode := range []string{"cancelled", "deadline", "closed_store"} {
		for _, action := range []string{"suggestions", "submit", "undo"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				f := newSemanticCorrectionsFixture(t)
				method, target, body, status := "POST", semanticCorrectionSubmit, semanticCorrectionMerge, 400
				if action == "suggestions" {
					method, target, body, status = "GET", semanticCorrectionSuggestions, "", 500
				}
				if action == "undo" {
					target, body = semanticCorrectionUndo("absent"), ""
				}
				r := sessionUpdatesRequest(method, target, body)
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				switch mode {
				case "cancelled":
					cancel()
					r = r.WithContext(ctx)
				case "deadline":
					ended, stop := context.WithDeadline(r.Context(), time.Now().Add(-time.Second))
					defer stop()
					r = r.WithContext(ended)
				case "closed_store":
					if err := f.s.store.Close(); err != nil {
						t.Fatal(err)
					}
				}
				code := "invalid_request"
				if status == 500 {
					code = "internal_error"
				}
				f.error(t, f.serve(r), status, code, "")
				f.report(t, "event-current", "duplicate_report", 0)
			})
		}
	}
}

func TestSemanticCorrectionsCharacterizationTransactions(t *testing.T) {
	for _, action := range []string{"submit", "undo"} {
		t.Run(action+"/rollback", func(t *testing.T) {
			f := newSemanticCorrectionsFixture(t)
			target := semanticCorrectionSubmit
			if action == "undo" {
				value := f.correction(t, f.serve(sessionUpdatesRequest("POST", target, semanticCorrectionMerge)), 201)
				target = semanticCorrectionUndo(value.ID)
				f.exec(t, `CREATE TRIGGER semantic_undo_fault BEFORE UPDATE OF undone_at ON semantic_event_corrections BEGIN SELECT RAISE(ABORT,'semantic-undo-fault'); END`)
			} else {
				f.exec(t, `CREATE TRIGGER semantic_submit_fault BEFORE INSERT ON semantic_event_corrections BEGIN SELECT RAISE(ABORT,'semantic-submit-fault'); END`)
			}
			f.error(t, f.serve(sessionUpdatesRequest("POST", target, semanticCorrectionMerge)), 400, "invalid_request", "semantic-"+action+"-fault")
			if action == "undo" {
				f.report(t, "event-target-1", "material_update", 1)
				if f.count(t, `SELECT COUNT(*) FROM semantic_novelty_constraints`) != 1 || f.count(t, `SELECT COUNT(*) FROM semantic_event_deltas`) != 1 || f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections WHERE undone_at IS NOT NULL`) != 0 {
					t.Fatal("failed undo did not roll back")
				}
			} else {
				f.report(t, "event-current", "duplicate_report", 0)
				if f.count(t, `SELECT COUNT(*) FROM semantic_event_constraints`) != 0 || f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections`) != 0 || f.count(t, `SELECT COUNT(*) FROM semantic_event_deltas`) != 0 {
					t.Fatal("failed correction did not roll back")
				}
			}
		})
	}
	t.Run("post_commit_cleanup_error", func(t *testing.T) {
		f := newSemanticCorrectionsFixture(t)
		f.event(t, "orphan-cleanup", f.s.store.Now())
		f.exec(t, `CREATE TRIGGER semantic_cleanup_fault BEFORE DELETE ON semantic_events WHEN OLD.id='orphan-cleanup' BEGIN SELECT RAISE(ABORT,'semantic-cleanup-fault'); END`)
		value := f.correction(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionSubmit, semanticCorrectionMerge)), 201)
		f.report(t, "event-target-1", "material_update", 1)
		f.correction(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionUndo(value.ID), "")), 200)
		f.report(t, "event-current", "duplicate_report", 0)
		if f.count(t, `SELECT COUNT(*) FROM semantic_events WHERE id='orphan-cleanup'`) != 1 {
			t.Fatal("cleanup failure fixture did not retain orphan")
		}
	})
}

func TestSemanticCorrectionsCharacterizationWriteFailure(t *testing.T) {
	for _, action := range []string{"suggestions", "submit", "undo"} {
		t.Run(action, func(t *testing.T) {
			f := newSemanticCorrectionsFixture(t)
			method, target, body, status := "POST", semanticCorrectionSubmit, semanticCorrectionMerge, 201
			if action == "suggestions" {
				method, target, body, status = "GET", semanticCorrectionSuggestions, "", 200
			}
			if action == "undo" {
				value := f.correction(t, f.serve(sessionUpdatesRequest("POST", target, body)), 201)
				target, body, status = semanticCorrectionUndo(value.ID), "", 200
			}
			failure := errors.New("semantic-response-write-failed")
			w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
			if err := f.s.route(w, sessionUpdatesRequest(method, target, body)); !errors.Is(err, failure) || w.status != status {
				t.Fatal("response error contract changed", w.status, err)
			}
			if action == "submit" {
				f.report(t, "event-target-1", "material_update", 1)
				if f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections WHERE undone_at IS NULL`) != 1 {
					t.Fatal("write failure undid commit")
				}
			} else {
				f.report(t, "event-current", "duplicate_report", 0)
				if action == "undo" && f.count(t, `SELECT COUNT(*) FROM semantic_event_corrections WHERE undone_at IS NOT NULL`) != 1 {
					t.Fatal("write failure undid undo")
				}
			}
		})
	}
}

func TestSemanticCorrectionsCharacterizationRetention(t *testing.T) {
	f := newSemanticCorrectionsFixture(t)
	value := f.correction(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionSubmit, semanticCorrectionMerge)), 201)
	f.exec(t, `UPDATE timeline_items SET presented_at=?,batch_state='visible',origin_status='completed',presentation='prepend' WHERE id='semantic-item'`, f.s.store.Now().AddDate(0, 0, -40).UTC().Format(time.RFC3339Nano))
	settings, err := f.s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.s.store.EnforceRetention(context.Background(), settings)
	if err != nil || result.Timeline.RemovedItems != 1 {
		t.Fatal("Timeline retention fixture did not evict", result, err)
	}
	f.correction(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionUndo(value.ID), "")), 200)
	f.report(t, "event-current", "duplicate_report", 0)
	again := f.correction(t, f.serve(sessionUpdatesRequest("POST", semanticCorrectionSubmit, `{"action":"not_same_event"}`)), 201)
	if again.ToRelation != "new_event" {
		t.Fatal("retained evidence could not support correction")
	}
}
