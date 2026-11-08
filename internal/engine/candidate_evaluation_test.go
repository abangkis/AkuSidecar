package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

type candidateEvaluationProvider struct {
	reasoning.Deterministic
	analyze func(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error)
}

func (p *candidateEvaluationProvider) Analyze(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	return p.analyze(ctx, run, obs, knowledge)
}

func newCandidateEvaluationFixture(t *testing.T) bridgeCommandFixture {
	t.Helper()
	f := newBridgeCommandFixture(t, domain.SourceLinkedIn)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.CalibrationEnabled, settings.AIDetectionEnabled = false, false
	settings.PreferenceEligibilityMode, settings.SemanticEventMode = "rank_only", "show_all"
	settings.MaxItemsPerSource = 4
	if err := f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	command := f.claim(t)
	obs := domain.Observation{Source: domain.SourceLinkedIn, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "linkedin:evaluation", PlatformID: "41", Text: "Independent primary research evidence for candidate evaluation.", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:41"}}}}, Coverage: map[string]any{"status": "complete"}}
	if err := validateObservation(obs); err != nil {
		t.Fatal(err)
	}
	if err := f.state.SaveObservation(context.Background(), command.ID, f.run.ID, obs); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `DELETE FROM preference_model`)
	for _, source := range []string{"linkedin", "x"} {
		f.exec(t, `INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at) VALUES(?,?,'known-event',?,'{"whatChanged":"local knowledge"}',?,?)`, "evaluation-knowledge-"+source, source, source+":known", domain.Now(), domain.Now())
	}
	return f
}

func candidateEvaluationResult(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	result, telemetry, err := (reasoning.Deterministic{}).Analyze(ctx, run, obs, knowledge)
	for i := range result.CandidateAssessments {
		a := &result.CandidateAssessments[i]
		a.Novelty, a.Urgency, a.Actionability, a.Materiality, a.EvidenceStrength = 1, 1, 1, 1, 1
	}
	for i := range result.Items {
		result.Items[i].EventKey = "fresh-event"
	}
	return result, telemetry, err
}

func candidateEvaluationCount(t *testing.T, f bridgeCommandFixture, query string, args ...any) int {
	t.Helper()
	var count int
	if err := f.db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// Characterize process before extraction; the fixture records real SQLite
// effects and injects failures without adding a production seam.
func TestEngineCandidateEvaluationCharacterizationEffects(t *testing.T) {
	for _, mode := range []string{"success", "knowledge-failure", "stage-failure", "provider-failure", "validation-failure", "preference-load-failure", "preference-save-failure", "evidence-lookup-failure", "completion-failure", "telemetry-failure", "timing-failure", "caller-cancellation"} {
		t.Run(mode, func(t *testing.T) {
			f := newCandidateEvaluationFixture(t)
			f.exec(t, `CREATE TABLE evaluation_effects(sequence INTEGER PRIMARY KEY,effect TEXT NOT NULL)`)
			f.exec(t, `CREATE TRIGGER evaluation_stage AFTER UPDATE OF stage ON runs WHEN NEW.stage='candidate_evaluation' BEGIN INSERT INTO evaluation_effects(effect) VALUES('stage'); END`)
			f.exec(t, `CREATE TRIGGER evaluation_telemetry AFTER INSERT ON reasoning_invocations WHEN NEW.phase='candidate_evaluation' BEGIN INSERT INTO evaluation_effects(effect) VALUES('telemetry'); END`)
			f.exec(t, `CREATE TRIGGER evaluation_timing AFTER INSERT ON run_stage_timings BEGIN INSERT INTO evaluation_effects(effect) VALUES('timing:'||NEW.stage); END`)
			f.exec(t, `CREATE TRIGGER evaluation_preference AFTER INSERT ON preference_model BEGIN INSERT INTO evaluation_effects(effect) VALUES('preference'); END`)
			f.exec(t, `CREATE TRIGGER evaluation_assessment AFTER INSERT ON candidate_assessments BEGIN INSERT INTO evaluation_effects(effect) VALUES('assessment'); END`)
			f.exec(t, `CREATE TRIGGER evaluation_item AFTER INSERT ON timeline_items BEGIN INSERT INTO evaluation_effects(effect) VALUES('item'); END`)
			f.exec(t, `CREATE TRIGGER evaluation_completion AFTER UPDATE OF status ON runs WHEN NEW.status='completed' BEGIN INSERT INTO evaluation_effects(effect) VALUES('completed'); END`)
			switch mode {
			case "knowledge-failure":
				f.exec(t, `UPDATE knowledge_events SET item_json='invalid-json' WHERE source='linkedin'`)
			case "stage-failure":
				f.exec(t, `CREATE TRIGGER evaluation_fail_stage BEFORE UPDATE OF stage ON runs WHEN NEW.stage='candidate_evaluation' BEGIN SELECT RAISE(ABORT,'evaluation stage rejected'); END`)
			case "preference-save-failure":
				f.exec(t, `CREATE TRIGGER evaluation_fail_preference BEFORE INSERT ON preference_model BEGIN SELECT RAISE(ABORT,'evaluation preference rejected'); END`)
			case "completion-failure":
				f.exec(t, `CREATE TRIGGER evaluation_fail_item BEFORE INSERT ON timeline_items BEGIN SELECT RAISE(ABORT,'evaluation item rejected'); END`)
			case "telemetry-failure":
				f.exec(t, `CREATE TRIGGER evaluation_fail_telemetry BEFORE INSERT ON reasoning_invocations BEGIN SELECT RAISE(ABORT,'evaluation telemetry rejected'); END`)
			case "timing-failure":
				f.exec(t, `CREATE TRIGGER evaluation_fail_timing BEFORE INSERT ON run_stage_timings BEGIN SELECT RAISE(ABORT,'evaluation timing rejected'); END`)
			}
			key := struct{}{}
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), key, "evaluation-context"), time.Minute)
			defer cancel()
			providerFailure := errors.New("offline candidate evaluation rejected")
			called := 0
			f.e.provider = &candidateEvaluationProvider{analyze: func(actual context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
				called++
				if actual != ctx || actual.Value(key) != "evaluation-context" || run.ID != f.run.ID || len(knowledge) != 1 || knowledge[0].WhatChanged != "local knowledge" {
					t.Fatal("evaluation inputs/context/source-scoped Knowledge changed")
				}
				stored, err := f.state.GetRun(actual, run.ID)
				if err != nil || stored.Stage != "candidate_evaluation" {
					t.Fatal("provider called before stage", stored, err)
				}
				result, telemetry, err := candidateEvaluationResult(actual, run, obs, knowledge)
				switch mode {
				case "provider-failure":
					err = providerFailure
				case "validation-failure":
					result.CandidateAssessments[0].EvidenceKey = "foreign-evidence"
				case "preference-load-failure":
					f.exec(t, `INSERT INTO preference_model(id,model_json,feedback_count,updated_at) VALUES(1,'invalid-json',0,'fixture')`)
				case "evidence-lookup-failure":
					// Preference projection needs id/source/assessment_json, whereas
					// the subsequent evidence lookup evaluates the failing expression.
					f.exec(t, `ALTER TABLE timeline_items RENAME TO evaluation_original_timeline`)
					f.exec(t, `CREATE VIEW timeline_items AS SELECT 'prior-id' AS id,'linkedin' AS source,'{}' AS assessment_json,json_extract('invalid-json','$') AS evidence_key`)
				case "caller-cancellation":
					cancel()
				}
				return result, telemetry, err
			}}
			err := f.e.process(ctx, f.run.ID, false)
			want := []string{"stage", "telemetry", "timing:evaluated", "preference", "timing:selected", "assessment", "item", "completed", "timing:added"}
			wantCalls, wantAssessments, wantItems := 1, 1, 1
			switch mode {
			case "knowledge-failure", "stage-failure":
				want = nil
				wantCalls, wantAssessments, wantItems = 0, 0, 0
				if err == nil {
					t.Fatal("expected pre-provider failure")
				}
			case "provider-failure", "validation-failure", "caller-cancellation":
				want = want[:3]
				wantAssessments, wantItems = 0, 0
				if mode == "provider-failure" && err != providerFailure {
					t.Fatal("provider error identity changed", err)
				}
				if mode == "validation-failure" && (err == nil || err.Error() != "assessment references unknown evidence foreign-evidence") {
					t.Fatal(err)
				}
				if mode == "caller-cancellation" && !errors.Is(err, context.Canceled) {
					t.Fatal("caller cancellation lost", err)
				}
			case "preference-load-failure":
				// The malformed cache insertion itself is observed by the fixture.
				want = []string{"stage", "preference", "telemetry", "timing:evaluated"}
				wantAssessments, wantItems = 0, 0
				if err == nil {
					t.Fatal("malformed preference cache accepted")
				}
			case "preference-save-failure":
				want = want[:3]
				wantAssessments, wantItems = 0, 0
				if err == nil || !strings.Contains(err.Error(), "evaluation preference rejected") {
					t.Fatal(err)
				}
			case "evidence-lookup-failure":
				want = want[:4]
				wantAssessments, wantItems = 0, 0
				if err == nil || candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM preference_model`) != 1 {
					t.Fatal("lookup failure rolled back preference", err)
				}
			case "completion-failure":
				want = want[:5]
				wantAssessments, wantItems = 0, 0
				if err == nil || !strings.Contains(err.Error(), "evaluation item rejected") {
					t.Fatal(err)
				}
			case "telemetry-failure":
				want = []string{"stage", "timing:evaluated", "preference", "timing:selected", "assessment", "item", "completed", "timing:added"}
			case "timing-failure":
				want = []string{"stage", "telemetry", "preference", "assessment", "item", "completed"}
			}
			if mode == "success" || mode == "telemetry-failure" || mode == "timing-failure" {
				if err != nil {
					t.Fatal("best-effort behavior changed", err)
				}
			}
			var effects []string
			rows, qerr := f.db.Query(`SELECT effect FROM evaluation_effects ORDER BY sequence`)
			if qerr != nil {
				t.Fatal(qerr)
			}
			for rows.Next() {
				var value string
				if err := rows.Scan(&value); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				effects = append(effects, value)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			rows.Close()
			if !reflect.DeepEqual(effects, want) || called != wantCalls {
				t.Fatalf("effects=%v want=%v calls=%d/%d err=%v", effects, want, called, wantCalls, err)
			}
			if candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM candidate_assessments WHERE run_id=?`, f.run.ID) != wantAssessments {
				t.Fatal("completion assessment transaction changed")
			}
			if mode != "evidence-lookup-failure" && candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, f.run.ID) != wantItems {
				t.Fatal("completion item transaction changed")
			}
			stored, loadErr := f.state.GetRun(context.Background(), f.run.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if (stored.Status == "completed") != (wantAssessments == 1) {
				t.Fatal("run completion state changed", stored.Status)
			}
		})
	}
}

func TestEngineCandidateEvaluationCharacterizationSelection(t *testing.T) {
	for _, mode := range []string{"reused-event-label", "material-update", "contradiction", "prior-delivered", "prior-delivered-material", "resurfaced-changed", "resurfaced-after-cooldown", "foreign-source-event", "weak-evidence-material"} {
		t.Run(mode, func(t *testing.T) {
			f := newCandidateEvaluationFixture(t)
			if strings.HasPrefix(mode, "prior-delivered") || strings.HasPrefix(mode, "resurfaced-") {
				f.exec(t, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at,coverage_json) SELECT 'evaluation-prior-session','historical fixture','completed',max_items_per_source,max_items_total,created_at,coverage_json FROM sessions WHERE id=?`, f.session.ID)
				f.exec(t, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('evaluation-prior-run','evaluation-prior-session','linkedin',0,'completed','completed',?)`, domain.Now())
				f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES('prior-item','evaluation-prior-session','evaluation-prior-run','linkedin','linkedin:evaluation',0,'{}','{}','{}',?)`, domain.Now())
			}
			if strings.HasPrefix(mode, "resurfaced-") {
				observations, err := f.state.Observations(context.Background(), f.run.ID)
				if err != nil {
					t.Fatal(err)
				}
				settings, err := f.state.GetSettings(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.state.ClassifyContentContinuity(context.Background(), f.run, observations[0], settings); err != nil {
					t.Fatal(err)
				}
				status := "resurfaced_changed"
				if mode == "resurfaced-after-cooldown" {
					status = "resurfaced_after_cooldown"
				}
				f.exec(t, `UPDATE content_continuity_occurrences SET status=?,action='evaluate' WHERE run_id=? AND evidence_key='linkedin:evaluation'`, status, f.run.ID)
			}
			f.e.provider = &candidateEvaluationProvider{analyze: func(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
				result, telemetry, err := candidateEvaluationResult(ctx, run, obs, knowledge)
				result.Items[0].EventKey = "known-event"
				switch mode {
				case "reused-event-label":
					f.exec(t, `UPDATE knowledge_events SET event_key='event_003' WHERE source='linkedin'`)
					result.Items[0].EventKey = "event_003"
				case "material-update", "prior-delivered-material", "weak-evidence-material":
					result.Items[0].KnowledgeDelta = "material_update"
				case "contradiction":
					result.Items[0].KnowledgeDelta = "contradiction"
				case "resurfaced-changed", "resurfaced-after-cooldown":
					result.Items[0].EventKey = "fresh-event"
				case "foreign-source-event":
					f.exec(t, `UPDATE knowledge_events SET event_key='foreign-event' WHERE source='x'`)
					result.Items[0].EventKey = "foreign-event"
				}
				if mode == "weak-evidence-material" {
					result.CandidateAssessments[0].EvidenceStrength = 0.1
				}
				return result, telemetry, err
			}}
			if err := f.e.process(context.Background(), f.run.ID, false); err != nil {
				t.Fatal(err)
			}
			want := 1
			wantReason := domain.SelectionProtected
			switch mode {
			case "prior-delivered", "prior-delivered-material":
				want = 0
				wantReason = domain.SelectionPreviouslyDelivered
			case "weak-evidence-material":
				want = 0
				wantReason = domain.SelectionInsufficientEvidence
			}
			if got := candidateEvaluationCount(t, f, `SELECT selected FROM candidate_assessments WHERE run_id=?`, f.run.ID); got != want {
				t.Fatalf("selected=%d want=%d", got, want)
			}
			trace, err := f.state.InboxRunTrace(context.Background(), f.run.ID, "evaluated", 5, 0)
			if err != nil || len(trace.Items) != 1 {
				t.Fatalf("evaluated trace=%+v err=%v", trace, err)
			}
			decision := trace.Items[0].SelectionDecision
			if decision == nil || decision.PolicyVersion == "" || decision.Reason != wantReason || decision.Selected != (want == 1) {
				t.Fatalf("persisted decision=%+v, want %q and selected=%v", decision, wantReason, want == 1)
			}
			if mode == "reused-event-label" {
				if got := candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM timeline_items WHERE run_id=? AND evidence_key='linkedin:evaluation'`, f.run.ID); got != 1 {
					t.Fatal("unrelated historical label kept new evidence out of the Timeline")
				}
				if got := candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM knowledge_events WHERE source='linkedin' AND (event_key='event_003' AND evidence_key='linkedin:known' OR evidence_key='linkedin:evaluation')`); got != 2 {
					t.Fatal("new evidence overwrote the unrelated historical knowledge entry")
				}
			}
		})
	}
}

func TestEngineCandidateEvaluationCharacterizationCompletionBeforeAdvance(t *testing.T) {
	for _, mode := range []string{"success", "cancel-after-commit", "finalization-failure"} {
		t.Run(mode, func(t *testing.T) {
			f := newCandidateEvaluationFixture(t)
			f.e.provider = &candidateEvaluationProvider{analyze: candidateEvaluationResult}
			if mode == "finalization-failure" {
				f.exec(t, `CREATE TRIGGER evaluation_fail_finalize BEFORE UPDATE OF status ON sessions WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'evaluation finalization rejected'); END`)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.e.schedule.Lock()
			var once sync.Once
			unlock := func() { once.Do(f.e.schedule.Unlock) }
			defer unlock()
			result, done := make(chan error, 1), make(chan struct{})
			go func() { defer close(done); result <- f.e.process(ctx, f.run.ID, false) }()
			t.Cleanup(func() {
				unlock()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("evaluation worker did not finish")
				}
			})
			current := waitSession(t, f.e, f.session.ID, func(s domain.Session) bool { return s.Runs[0].Status == "completed" })
			if current.Status == "completed" || candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, f.run.ID) != 1 {
				t.Fatal("advancement preceded run completion", current.Status)
			}
			select {
			case err := <-result:
				t.Fatal("process bypassed schedule lock", err)
			default:
			}
			if mode == "cancel-after-commit" {
				cancel()
			}
			unlock()
			var err error
			select {
			case err = <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("process did not return after schedule release")
			}
			switch mode {
			case "success":
				if err != nil {
					t.Fatal(err)
				}
			case "cancel-after-commit":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case "finalization-failure":
				if err == nil || !strings.HasPrefix(err.Error(), "finalize unified session: ") {
					t.Fatal(err)
				}
			}
			stored, loadErr := f.state.GetRun(context.Background(), f.run.ID)
			if loadErr != nil || stored.Status != "completed" {
				t.Fatal("advancement failure changed committed run", stored, loadErr)
			}
			if candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, f.run.ID) != 1 || candidateEvaluationCount(t, f, `SELECT COUNT(*) FROM run_stage_timings WHERE run_id=? AND stage='added'`, f.run.ID) != 1 {
				t.Fatal("advancement lost committed output/timing")
			}
		})
	}
}

func TestEngineCandidateEvaluationCharacterizationRoutedProfile(t *testing.T) {
	f := newCandidateEvaluationFixture(t)
	provider := &routedProfileProvider{}
	f.e.provider = provider
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ReasoningEvaluationProfile = "sol_medium"
	if err := f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if err := f.e.process(context.Background(), f.run.ID, false); err != nil {
		t.Fatal(err)
	}
	if provider.evaluationModel.Model != "gpt-5.6-sol" || provider.evaluationModel.Effort != "medium" || provider.planningModel.Model != "" {
		t.Fatal("configured evaluation profile dispatch changed", provider)
	}
	// The deterministic routed provider above is offline; no model service runs.
	var raw string
	if err := f.db.QueryRow(`SELECT item_json FROM timeline_items WHERE run_id=?`, f.run.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var item domain.ReasonedItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil || item.Source != domain.SourceLinkedIn {
		t.Fatal("result binding changed", raw, err)
	}
}

func TestEngineCandidateEvaluationCharacterizationKnowledgeBound(t *testing.T) {
	f := newCandidateEvaluationFixture(t)
	base := time.Now().UTC().Add(time.Hour)
	for i := 0; i < 205; i++ {
		key := fmt.Sprintf("bounded-%03d", i)
		raw, err := json.Marshal(domain.ReasonedItem{WhatChanged: key})
		if err != nil {
			t.Fatal(err)
		}
		at := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		f.exec(t, `INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at) VALUES(?,'linkedin',?,?,?,?,?)`, key, key, key, string(raw), at, at)
	}
	f.e.provider = &candidateEvaluationProvider{analyze: func(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
		if len(knowledge) != 200 || knowledge[0].WhatChanged != "bounded-204" || knowledge[199].WhatChanged != "bounded-005" {
			t.Fatal("source-local Knowledge order/cap changed", len(knowledge))
		}
		return candidateEvaluationResult(ctx, run, obs, knowledge)
	}}
	if err := f.e.process(context.Background(), f.run.ID, false); err != nil {
		t.Fatal(err)
	}
}
