package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
)

type selectionCorrectionLog struct {
	provenanceTestLog
	observe func(string)
}

func (l *selectionCorrectionLog) Printf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if l.observe != nil {
		l.observe(message)
	}
	l.provenanceTestLog.Printf("%s", message)
}

type selectionCorrectionFixture struct {
	bridgeCommandFixture
	ref string
	log *selectionCorrectionLog
}

func newSelectionCorrectionFixture(t *testing.T) selectionCorrectionFixture {
	t.Helper()
	f := newBridgeCommandFixture(t, domain.SourceX)
	ctx := context.Background()
	settings, err := f.state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled, settings.CalibrationEnabled, settings.SemanticEventMode = false, false, "show_all"
	if err := f.state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	command := f.claim(t)
	key := "x:selection-correction-fixture"
	block := domain.Block{EvidenceKey: key, PlatformID: "x:status:1234567890123456789", Author: "Fixture", Text: "This post was written by ChatGPT. It describes an independently captured source update for the correction workflow.", Permalink: "https://x.com/fixture/status/1234567890123456789", Media: []map[string]any{{"kind": "image", "url": "https://pbs.twimg.com/media/correction-fixture.jpg"}}}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}, Coverage: map[string]any{"quality": "complete"}}
	if err := validateObservation(observation); err != nil {
		t.Fatal(err)
	}
	if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, observation); err != nil {
		t.Fatal(err)
	}
	assessment := domain.CandidateAssessment{EvidenceKey: key, TopicTags: []string{"correction"}, TopicFacets: []string{"other"}, ContentType: "news", Novelty: .6, Materiality: .5, EvidenceStrength: .8, Rationale: "Evaluated but outside the original bounded selection."}
	item := domain.ReasonedItem{ID: "fixture-reasoned-item", EvidenceKey: key, Source: domain.SourceX, WhatChanged: block.Text, WhyItMatters: "Offline correction evidence", SourceURL: block.Permalink, SourceURLKind: "native_post", EventKey: "fixture-correction-event", KnowledgeDelta: "new_event", Author: block.Author, Confidence: .8, EvidenceState: "primary"}
	assessmentJSON, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	itemJSON, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,0,?,0,?)`, f.run.ID, key, domain.SourceX, string(assessmentJSON), string(itemJSON), .4, .4, domain.Now())
	f.exec(t, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id=?`, domain.Now(), f.run.ID)
	f.exec(t, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, domain.Now(), f.session.ID)
	if _, err := f.state.CreateLivingTopic(ctx, "Offline correction topic"); err != nil {
		t.Fatal(err)
	}
	// Keep routing queued so tests observe synchronous admission, not worker speed.
	f.e.mu.Lock()
	f.e.work.registerActive("living-topics-routing", func() {})
	f.e.mu.Unlock()
	t.Cleanup(func() { f.e.mu.Lock(); f.e.work.retireActive("living-topics-routing"); f.e.mu.Unlock() })
	log := &selectionCorrectionLog{}
	f.e.logger = log
	sum := sha256.Sum256([]byte(f.run.ID + "\x00" + key))
	return selectionCorrectionFixture{f, "candidate_" + hex.EncodeToString(sum[:8]), log}
}

func (f selectionCorrectionFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	if err := f.db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func correctionOperationHeld(t *testing.T, e *Engine) {
	t.Helper()
	if e.operation.TryLock() {
		e.operation.Unlock()
		t.Error("operation lock was released inside correction")
	}
}

func correctionLocksReleased(t *testing.T, e *Engine) {
	t.Helper()
	if !e.operation.TryLock() {
		t.Fatal("operation lock was retained after return")
	}
	e.operation.Unlock()
	if !e.mu.TryLock() {
		t.Fatal("mu was retained after return")
	}
	e.mu.Unlock()
}

func TestEngineSelectionCorrectionsCharacterizationAdmission(t *testing.T) {
	for _, action := range []string{"correct", "undo"} {
		for _, mode := range []string{"active", "cancelled", "missing"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newSelectionCorrectionFixture(t)
				ctx := context.Background()
				id := "missing"
				if action == "undo" && mode != "missing" {
					correction, _, err := f.e.CorrectSelection(ctx, f.run.ID, f.ref)
					if err != nil {
						t.Fatal(err)
					}
					id = correction.ID
				}
				if mode == "active" {
					settings, err := f.state.GetSettings(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := f.state.CreateUpdateSession(ctx, "active guard", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser}); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "cancelled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				before := f.count(t, `SELECT COUNT(*) FROM selection_corrections WHERE undone_at IS NULL`)
				var correction domain.SelectionCorrection
				var item domain.TimelineItem
				var err error
				if action == "correct" {
					ref := f.ref
					if mode == "missing" {
						ref = "missing"
					}
					correction, item, err = f.e.CorrectSelection(ctx, f.run.ID, ref)
				} else {
					correction, err = f.e.UndoSelectionCorrection(ctx, id)
				}
				if !reflect.DeepEqual(correction, domain.SelectionCorrection{}) || !reflect.DeepEqual(item, domain.TimelineItem{}) {
					t.Fatalf("guard returned partial results: %+v %+v", correction, item)
				}
				switch mode {
				case "active":
					want := "finish the active update before correcting an earlier selection"
					if action == "undo" {
						want = "finish the active update before undoing a selection correction"
					}
					if err == nil || err.Error() != want {
						t.Fatalf("error=%v want=%q", err, want)
					}
				case "cancelled":
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("error=%v", err)
					}
				case "missing":
					if !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("error=%v", err)
					}
				}
				if f.count(t, `SELECT COUNT(*) FROM selection_corrections WHERE undone_at IS NULL`) != before {
					t.Fatal("guard mutated correction")
				}
				correctionLocksReleased(t, f.e)
			})
		}
	}
}

type correctionDeepResolver struct {
	*aiTestResolver
	beforeModel func()
}

func (r *correctionDeepResolver) Model() config.ModelConfig {
	r.beforeModel()
	return r.aiTestResolver.Model()
}

type correctionInspector struct {
	*provenanceTestInspector
	beforeAvailable func()
}

func (i *correctionInspector) Available() bool {
	i.beforeAvailable()
	return i.provenanceTestInspector.Available()
}

func TestEngineSelectionCorrectionsCharacterizationSuccess(t *testing.T) {
	for _, semantic := range []string{"nil", "disabled", "enabled"} {
		for _, ai := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ai=%t", semantic, ai), func(t *testing.T) {
				f := newSelectionCorrectionFixture(t)
				ctx := context.Background()
				settings, err := f.state.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.AIDetectionEnabled = ai
				if semantic != "nil" {
					f.e.events = semanticengine.New(f.state, nil)
				}
				if semantic == "enabled" {
					settings.SemanticEventMode = "collapse"
				}
				if err := f.state.SaveSettings(ctx, settings); err != nil {
					t.Fatal(err)
				}
				f.exec(t, `CREATE TABLE correction_order(position INTEGER PRIMARY KEY,stage TEXT NOT NULL)`)
				for _, audit := range []struct{ table, stage, condition string }{
					{"selection_corrections", "correction", ""},
					{"semantic_event_reports", "semantic", ""},
					{"knowledge_events", "knowledge", ""},
					{"ai_assessments", "fast", "WHEN NEW.stage='fast'"},
					{"ai_detection_jobs", "deep", ""},
					{"media_provenance_assessments", "provenance", ""},
					{"living_topic_routing_jobs", "topics", ""},
				} {
					f.exec(t, fmt.Sprintf(`CREATE TRIGGER correction_order_%s AFTER INSERT ON %s %s BEGIN INSERT INTO correction_order(stage) VALUES('%s'); END`, audit.stage, audit.table, audit.condition, audit.stage))
				}
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				t.Cleanup(unblock)
				modelCalls, availableCalls := 0, 0
				r := &correctionDeepResolver{aiTestResolver: &aiTestResolver{}}
				r.beforeModel = func() {
					modelCalls++
					correctionOperationHeld(t, f.e)
					if f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != 1 || f.count(t, `SELECT COUNT(*) FROM ai_assessments WHERE stage='fast'`) != 1 || f.count(t, `SELECT COUNT(*) FROM media_provenance_assessments`) != 0 || f.count(t, `SELECT COUNT(*) FROM living_topic_routing_jobs`) != 0 {
						t.Error("deep launch order changed")
					}
				}
				r.resolve = func(_ context.Context, items []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
					<-release
					return aiTestResult(len(items)), domain.ModelUsage{}, time.Millisecond, nil
				}
				f.e.SetAIDeepResolver(r)
				inspector := &correctionInspector{provenanceTestInspector: &provenanceTestInspector{available: true, version: "offline-correction"}}
				inspector.beforeAvailable = func() {
					availableCalls++
					correctionOperationHeld(t, f.e)
					if f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs`) != 1 || f.count(t, `SELECT COUNT(*) FROM living_topic_routing_jobs`) != 0 {
						t.Error("provenance launch order changed")
					}
				}
				inspector.inspect = func(context.Context, domain.MediaProvenanceAssessment, []string) (mediaprovenance.Result, error) {
					<-release
					return neutralProvenanceResult(), nil
				}
				f.e.SetMediaProvenanceInspector(inspector)
				correction, item, err := f.e.CorrectSelection(ctx, f.run.ID, f.ref)
				if err != nil || correction.ID == "" || correction.TimelineID != item.ID || item.SessionID != f.session.ID || correction.Action != "should_select" {
					t.Fatalf("correction=%+v item=%+v err=%v", correction, item, err)
				}
				correctionLocksReleased(t, f.e)
				wantAI, wantSemantic := 0, 0
				if ai {
					wantAI = 1
				}
				if semantic == "enabled" {
					wantSemantic = 1
				}
				if modelCalls != wantAI || availableCalls != wantAI || f.count(t, `SELECT COUNT(*) FROM ai_assessments WHERE stage='fast'`) != wantAI || (item.AIDetection != nil) != ai || f.count(t, `SELECT COUNT(*) FROM semantic_event_reports`) != wantSemantic || f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != 1 || f.count(t, `SELECT COUNT(*) FROM living_topic_routing_jobs`) != 1 || f.count(t, `SELECT COUNT(*) FROM media_provenance_assessments`) != wantAI {
					t.Fatalf("unexpected enrichment counts: model=%d available=%d item=%+v", modelCalls, availableCalls, item)
				}
				wantOrder := []string{"correction"}
				if semantic == "enabled" {
					wantOrder = append(wantOrder, "semantic")
				}
				wantOrder = append(wantOrder, "knowledge")
				if ai {
					wantOrder = append(wantOrder, "fast", "deep", "provenance")
				}
				wantOrder = append(wantOrder, "topics")
				var order string
				if err := f.db.QueryRow(`SELECT group_concat(stage,',') FROM (SELECT stage FROM correction_order ORDER BY position)`).Scan(&order); err != nil {
					t.Fatal(err)
				}
				if order != strings.Join(wantOrder, ",") {
					t.Fatalf("side-effect order=%s want=%v", order, wantOrder)
				}
				unblock()
				// Remove only the fixture routing sentinel before draining real workers.
				f.e.mu.Lock()
				f.e.work.retireActive("living-topics-routing")
				f.e.mu.Unlock()
				if !f.e.WaitForIdle(3 * time.Second) {
					t.Fatal("enrichment workers did not retire")
				}
			})
		}
	}
}

func TestEngineSelectionCorrectionsCharacterizationErrors(t *testing.T) {
	for _, mode := range []string{"create", "settings", "semantic", "knowledge", "fast", "first-item", "second-item", "topics", "already-selected", "duplicate", "existing-timeline"} {
		t.Run(mode, func(t *testing.T) {
			f := newSelectionCorrectionFixture(t)
			ctx := context.Background()
			f.exec(t, `CREATE TABLE correction_audit(stage TEXT NOT NULL)`)
			switch mode {
			case "create":
				f.exec(t, `CREATE TRIGGER correction_abort BEFORE INSERT ON selection_corrections BEGIN SELECT RAISE(ABORT,'offline correction rejected'); END`)
			case "settings":
				f.exec(t, `CREATE TRIGGER correction_bad_settings AFTER INSERT ON selection_corrections BEGIN UPDATE settings SET value_json='{' WHERE key='runtime'; END`)
			case "semantic":
				settings, err := f.state.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.SemanticEventMode = "collapse"
				if err := f.state.SaveSettings(ctx, settings); err != nil {
					t.Fatal(err)
				}
				f.e.events = semanticengine.New(f.state, nil)
				f.exec(t, `CREATE TRIGGER correction_semantic_abort BEFORE INSERT ON semantic_event_reports BEGIN SELECT RAISE(ABORT,'offline semantic rejected'); END`)
			case "knowledge":
				f.exec(t, `CREATE TRIGGER correction_knowledge_abort BEFORE INSERT ON knowledge_events BEGIN SELECT RAISE(ABORT,'offline knowledge rejected'); END`)
			case "fast", "second-item":
				settings, err := f.state.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.AIDetectionEnabled = true
				if err := f.state.SaveSettings(ctx, settings); err != nil {
					t.Fatal(err)
				}
				if mode == "fast" {
					f.exec(t, `CREATE TRIGGER correction_fast_abort BEFORE INSERT ON ai_assessments BEGIN SELECT RAISE(ABORT,'offline fast rejected'); END`)
				} else {
					f.exec(t, `CREATE TRIGGER correction_second_item AFTER INSERT ON ai_assessments BEGIN INSERT INTO correction_audit VALUES('fast'); DELETE FROM timeline_items WHERE id=NEW.timeline_id; END`)
				}
			case "first-item":
				f.exec(t, `CREATE TRIGGER correction_first_item AFTER INSERT ON knowledge_events BEGIN DELETE FROM timeline_items WHERE evidence_key=NEW.evidence_key; END`)
			case "topics":
				f.exec(t, `CREATE TRIGGER correction_topic_abort BEFORE INSERT ON living_topic_routing_jobs BEGIN SELECT RAISE(ABORT,'offline topics rejected'); END`)
			case "already-selected":
				f.exec(t, `UPDATE candidate_assessments SET selected=1 WHERE run_id=?`, f.run.ID)
			case "duplicate", "existing-timeline":
				c, _, err := f.e.CorrectSelection(ctx, f.run.ID, f.ref)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "existing-timeline" {
					f.exec(t, `DELETE FROM selection_corrections WHERE id=?`, c.ID)
				}
			}
			f.log.observe = func(message string) {
				correctionOperationHeld(t, f.e)
				if f.count(t, `SELECT COUNT(*) FROM selection_corrections`) != 1 {
					t.Error("enrichment preceded durable correction")
				}
			}
			correction, item, err := f.e.CorrectSelection(ctx, f.run.ID, f.ref)
			correctionLocksReleased(t, f.e)
			if mode == "semantic" || mode == "knowledge" || mode == "fast" || mode == "topics" {
				if err != nil || correction.ID == "" || item.ID == "" || !strings.Contains(f.log.String(), "offline "+mode) {
					t.Fatalf("best-effort error=%v correction=%+v item=%+v log=%s", err, correction, item, f.log.String())
				}
				wantKnowledge := 1
				if mode == "knowledge" {
					wantKnowledge = 0
				}
				wantTopics := 1
				if mode == "topics" {
					wantTopics = 0
				}
				if f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != wantKnowledge || f.count(t, `SELECT COUNT(*) FROM living_topic_routing_jobs`) != wantTopics {
					t.Fatal("best-effort failure prevented later effects")
				}
				return
			}
			if mode == "settings" || mode == "first-item" || mode == "second-item" {
				if err == nil || correction.ID == "" || f.count(t, `SELECT COUNT(*) FROM selection_corrections`) != 1 || f.count(t, `SELECT COUNT(*) FROM living_topic_routing_jobs`) != 0 {
					t.Fatalf("partial result=%+v item=%+v err=%v", correction, item, err)
				}
				if mode == "settings" {
					if item.ID == "" || !strings.HasPrefix(err.Error(), "decode settings: ") || f.count(t, `SELECT COUNT(*) FROM timeline_items`) != 1 {
						t.Fatalf("settings partial item=%+v err=%v", item, err)
					}
				} else if !reflect.DeepEqual(item, domain.TimelineItem{}) || !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("item read result=%+v err=%v", item, err)
				}
				if mode == "second-item" && f.count(t, `SELECT COUNT(*) FROM correction_audit WHERE stage='fast'`) != 1 {
					t.Fatal("second item read moved before Fast save")
				}
				return
			}
			want := map[string]string{"create": "offline correction rejected", "already-selected": "candidate was already selected by the original run", "duplicate": "candidate already has an active selection correction", "existing-timeline": "candidate is already present in the Timeline"}[mode]
			if err == nil || !strings.Contains(err.Error(), want) || !reflect.DeepEqual(correction, domain.SelectionCorrection{}) || !reflect.DeepEqual(item, domain.TimelineItem{}) {
				t.Fatalf("rejection err=%v correction=%+v item=%+v", err, correction, item)
			}
			if mode == "create" && (f.count(t, `SELECT COUNT(*) FROM timeline_items`) != 0 || f.count(t, `SELECT COUNT(*) FROM selection_corrections`) != 0) {
				t.Fatal("creation failure was not atomic")
			}
		})
	}
}

func TestEngineSelectionCorrectionsCharacterizationUndo(t *testing.T) {
	for _, mode := range []string{"success", "undo-error", "cleanup-error", "refresh-error", "already-undone"} {
		t.Run(mode, func(t *testing.T) {
			f := newSelectionCorrectionFixture(t)
			ctx := context.Background()
			correction, _, err := f.e.CorrectSelection(ctx, f.run.ID, f.ref)
			if err != nil {
				t.Fatal(err)
			}
			f.exec(t, `INSERT INTO event_resolution_invocations(session_id,status,provider,model,effort,candidate_count,shortlist_count,unique_items,duplicate_reports,duration_ms,created_at) VALUES(?,'completed','fixture','fixture','none',99,0,99,0,0,?)`, f.session.ID, domain.Now())
			switch mode {
			case "undo-error":
				f.exec(t, `CREATE TRIGGER correction_undo_abort BEFORE UPDATE OF undone_at ON selection_corrections BEGIN SELECT RAISE(ABORT,'offline undo rejected'); END`)
			case "cleanup-error":
				f.exec(t, `INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('correction-orphan','Fixture orphan',?,?)`, domain.Now(), domain.Now())
				f.exec(t, `CREATE TRIGGER correction_cleanup_abort BEFORE DELETE ON semantic_events BEGIN SELECT RAISE(ABORT,'offline cleanup rejected'); END`)
			case "refresh-error":
				f.exec(t, `CREATE TRIGGER correction_refresh_abort BEFORE UPDATE ON event_resolution_invocations BEGIN SELECT RAISE(ABORT,'offline refresh rejected'); END`)
			case "already-undone":
				if _, err := f.state.UndoSelectionCorrection(ctx, correction.ID); err != nil {
					t.Fatal(err)
				}
			}
			calls := map[string]int{}
			keys := []string{"ai:" + f.session.ID + ":one", "ai:" + f.session.ID + ":two", "ai:" + f.session.ID + "-other:one", "ai:other:one", "media-provenance", "correction-unrelated"}
			f.e.mu.Lock()
			for _, key := range keys {
				key := key
				f.e.work.registerActive(key, func() {
					calls[key]++
					correctionOperationHeld(t, f.e)
					if f.e.mu.TryLock() {
						f.e.mu.Unlock()
						t.Error("mu released during cancellation")
					}
					if f.count(t, `SELECT COUNT(*) FROM selection_corrections WHERE id=? AND undone_at IS NOT NULL`, correction.ID) != 1 || f.count(t, `SELECT COUNT(*) FROM timeline_items WHERE id=?`, correction.TimelineID) != 0 || f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != 0 || f.count(t, `SELECT candidate_count FROM event_resolution_invocations WHERE session_id=?`, f.session.ID) != 99 {
						t.Error("undo/cancellation/count refresh order changed")
					}
				})
			}
			f.e.mu.Unlock()
			t.Cleanup(func() {
				f.e.mu.Lock()
				for _, key := range keys {
					f.e.work.retireActive(key)
				}
				f.e.mu.Unlock()
			})
			value, err := f.e.UndoSelectionCorrection(ctx, correction.ID)
			correctionLocksReleased(t, f.e)
			if mode == "undo-error" || mode == "cleanup-error" || mode == "already-undone" {
				if err == nil || !reflect.DeepEqual(value, domain.SelectionCorrection{}) || len(calls) != 0 {
					t.Fatalf("failed undo=%+v err=%v calls=%v", value, err, calls)
				}
				if mode == "cleanup-error" && (!strings.HasPrefix(err.Error(), "cleanup semantic event after undo: ") || !strings.Contains(err.Error(), "offline cleanup rejected")) {
					t.Fatalf("cleanup err=%v", err)
				}
				if mode == "undo-error" && !strings.Contains(err.Error(), "offline undo rejected") {
					t.Fatalf("undo err=%v", err)
				}
				if mode == "already-undone" && !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("repeated undo err=%v", err)
				}
			} else {
				if err != nil || value.ID != correction.ID || value.TimelineID != correction.TimelineID || value.UndoneAt == nil {
					t.Fatalf("undo=%+v err=%v", value, err)
				}
				want := map[string]int{keys[0]: 1, keys[1]: 1}
				if !reflect.DeepEqual(calls, want) {
					t.Fatalf("cancellation=%v want=%v", calls, want)
				}
			}
			wantRemaining := 0
			if mode == "undo-error" {
				wantRemaining = 1
			}
			if f.count(t, `SELECT COUNT(*) FROM timeline_items WHERE id=?`, correction.TimelineID) != wantRemaining || f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != wantRemaining {
				t.Fatal("undo persistence contract changed")
			}
			wantCount := 99
			if mode == "success" {
				wantCount = 0
			}
			if f.count(t, `SELECT candidate_count FROM event_resolution_invocations WHERE session_id=?`, f.session.ID) != wantCount {
				t.Fatal("event count refresh contract changed")
			}
			f.e.mu.Lock()
			for _, key := range keys {
				if _, ok := f.e.work.active[key]; !ok {
					t.Error("cancellation retired a worker synchronously")
				}
			}
			f.e.mu.Unlock()
		})
	}
}
