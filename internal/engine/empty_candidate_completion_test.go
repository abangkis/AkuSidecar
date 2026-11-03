package engine

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Exercise process before and after extraction using accepted evidence and real
// Store transactions. SQL initializes only disposable history and fault probes.
func emptyCompletionFixture(t *testing.T, resurfaced, visual int) (bridgeCommandFixture, *visionPersistenceProvider) {
	t.Helper()
	f, p := newVisionPersistenceFixture(t)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ResurfaceMode = "smart"
	if err := f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	blocks := []domain.Block{}
	for i := 0; i < resurfaced; i++ {
		blocks = append(blocks, visionLaneBlock(fmt.Sprintf("empty-old-%d", i), "Unchanged native text evidence within the configured cooldown."))
	}
	if resurfaced > 0 {
		prior := visionPersistencePriorRun(t, f)
		obs := domain.Observation{Source: f.run.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: blocks}}}
		if _, err := f.state.ClassifyContentContinuity(context.Background(), prior, obs, settings); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < visual; i++ {
		blocks = append(blocks, visionLaneBlock(fmt.Sprintf("empty-visual-%d", i), ""))
	}
	acceptVisionPersistenceObservation(t, f, blocks)
	return f, p
}

func emptyCompletionAssertNoReasoning(t *testing.T, f bridgeCommandFixture, p *visionPersistenceProvider) {
	t.Helper()
	if p.planned != 0 || p.analyzed != 0 {
		t.Fatal("empty candidate run entered text reasoning", p)
	}
	for _, table := range []string{"candidate_assessments", "timeline_items", "reasoning_invocations"} {
		if visionPersistenceCount(t, f, "SELECT COUNT(*) FROM "+table+" WHERE run_id=?", f.run.ID) != 0 {
			t.Fatal("empty candidate run wrote " + table)
		}
	}
	if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM run_stage_timings WHERE run_id=? AND stage IN ('evaluated','selected')`, f.run.ID) != 0 || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID) != 1 {
		t.Fatal("empty candidate run evaluated, selected or requested follow-up")
	}
}

func TestEngineEmptyCandidateCompletionCharacterizationResults(t *testing.T) {
	for _, mode := range []string{"resurface-one", "resurface-two", "vision-one", "vision-two", "resurface-and-vision", "existing-map", "existing-null", "two-rounds"} {
		t.Run(mode, func(t *testing.T) {
			resurfaced, visual := 1, 0
			switch mode {
			case "resurface-two":
				resurfaced = 2
			case "vision-one":
				resurfaced, visual = 0, 1
			case "vision-two":
				resurfaced, visual = 0, 2
			case "resurface-and-vision":
				visual = 2
			}
			f, p := emptyCompletionFixture(t, resurfaced, visual)
			ctx := context.Background()
			wantReceipt := map[string]any{"mode": "continuity_bypass", "decision": "finish", "reason": "All captured candidates were resolved as unchanged native resurfaces before acquisition planning.", "followUpQueued": false, "followUpNewCandidates": float64(0)}
			var receipt any = wantReceipt
			if mode == "existing-map" || mode == "two-rounds" {
				wantReceipt = map[string]any{"mode": "model", "decision": "request_follow_up", "followUpQueued": true, "followUpNewCandidates": float64(99), "fixtureMarker": "preserved"}
				receipt = wantReceipt
			}
			if mode == "existing-null" {
				receipt = nil
			}
			if strings.HasPrefix(mode, "existing-") || mode == "two-rounds" {
				if err := f.state.SetRunCoverageField(ctx, f.run.ID, "acquisitionPlanning", receipt); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "two-rounds" {
				observations, err := f.state.Observations(ctx, f.run.ID)
				if err != nil || len(observations) != 1 {
					t.Fatal(observations, err)
				}
				if _, err := f.state.QueueFollowUp(ctx, f.run.ID, map[string]any{"acquisitionRound": 2}); err != nil {
					t.Fatal(err)
				}
				command := f.claim(t)
				second := observations[0]
				second.CapturedAt = time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)
				if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, second); err != nil {
					t.Fatal(err)
				}
				wantReceipt["followUpNewCandidates"] = float64(0)
			}
			if err := f.e.process(ctx, f.run.ID, true); err != nil {
				t.Fatal(err)
			}
			run, err := f.state.GetRun(ctx, f.run.ID)
			if err != nil || run.Status != "completed" || run.Stage != "completed" || run.CompletedAt == nil || *run.CompletedAt == "" {
				t.Fatal("empty run not completed", run, err)
			}
			actual, exists := run.Coverage["acquisitionPlanning"]
			if !exists || !reflect.DeepEqual(actual, receipt) {
				t.Fatal("bypass overwrote existing receipt or changed output", actual, receipt)
			}
			if mode == "two-rounds" {
				if run.Coverage["acquisitionRounds"] != float64(2) || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID) != 2 {
					t.Fatal("rounds/yield or follow-up count changed", run.Coverage)
				}
				if p.planned != 0 || p.analyzed != 0 {
					t.Fatal("empty second round entered reasoning")
				}
			} else {
				emptyCompletionAssertNoReasoning(t, f, p)
			}
			noun := "items"
			if resurfaced == 1 {
				noun = "item"
			}
			wantSummary := fmt.Sprintf("%d unchanged resurfaced %s skipped before reasoning inside the configured cooldown.", resurfaced, noun)
			if visual > 0 {
				noun = "candidates"
				if visual == 1 {
					noun = "candidate"
				}
				wantSummary = fmt.Sprintf("%d Instagram media-only %s held for bounded vision evaluation; no text-bearing candidate entered reasoning.", visual, noun)
			}
			if run.Summary != wantSummary || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE run_id=?`, f.run.ID) != visual || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM run_stage_timings WHERE run_id=? AND stage='added'`, f.run.ID) != 1 {
				t.Fatal("summary, vision persistence or added timing changed", run)
			}
			if _, ok := run.Coverage["visionEvaluation"]; ok != (visual > 0) {
				t.Fatal("vision receipt presence changed", run.Coverage)
			}
		})
	}
}

func emptyCompletionRecordEffects(t *testing.T, f bridgeCommandFixture) {
	t.Helper()
	f.exec(t, `CREATE TABLE empty_completion_effects(sequence INTEGER PRIMARY KEY,effect TEXT NOT NULL)`)
	f.exec(t, `CREATE TRIGGER empty_effect_continuity AFTER INSERT ON content_continuity_occurrences BEGIN INSERT INTO empty_completion_effects(effect) VALUES('continuity'); END`)
	f.exec(t, `CREATE TRIGGER empty_effect_job AFTER INSERT ON vision_evaluation_jobs BEGIN INSERT INTO empty_completion_effects(effect) VALUES('vision-job'); END`)
	f.exec(t, `CREATE TRIGGER empty_effect_vision AFTER UPDATE OF coverage_json ON runs WHEN json_type(OLD.coverage_json,'$.visionEvaluation') IS NULL AND json_type(NEW.coverage_json,'$.visionEvaluation')='object' BEGIN INSERT INTO empty_completion_effects(effect) VALUES('vision-receipt'); END`)
	f.exec(t, `CREATE TRIGGER empty_effect_bypass AFTER UPDATE OF coverage_json ON runs WHEN json_type(OLD.coverage_json,'$.acquisitionPlanning') IS NULL AND json_extract(NEW.coverage_json,'$.acquisitionPlanning.mode')='continuity_bypass' BEGIN INSERT INTO empty_completion_effects(effect) VALUES('bypass-receipt'); END`)
	f.exec(t, `CREATE TRIGGER empty_effect_completion AFTER UPDATE OF status ON runs WHEN NEW.status='completed' BEGIN INSERT INTO empty_completion_effects(effect) VALUES('completed'); END`)
	f.exec(t, `CREATE TRIGGER empty_effect_timing AFTER INSERT ON run_stage_timings WHEN NEW.stage='added' BEGIN INSERT INTO empty_completion_effects(effect) VALUES('timing:added'); END`)
	f.exec(t, `CREATE TRIGGER empty_effect_session AFTER UPDATE OF status ON sessions WHEN NEW.status='completed' BEGIN INSERT INTO empty_completion_effects(effect) VALUES('session-completed'); END`)
}

func TestEngineEmptyCandidateCompletionCharacterizationEffects(t *testing.T) {
	for _, mode := range []string{"success", "receipt-failure", "completion-failure", "timing-failure", "cancel-before-read"} {
		t.Run(mode, func(t *testing.T) {
			f, p := emptyCompletionFixture(t, 1, 1)
			emptyCompletionRecordEffects(t, f)
			switch mode {
			case "receipt-failure":
				f.exec(t, `CREATE TRIGGER empty_fail_receipt BEFORE UPDATE OF coverage_json ON runs WHEN json_extract(NEW.coverage_json,'$.acquisitionPlanning.mode')='continuity_bypass' BEGIN SELECT RAISE(ABORT,'empty bypass rejected'); END`)
			case "completion-failure":
				f.exec(t, `CREATE TRIGGER empty_fail_completion BEFORE UPDATE OF status ON runs WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'empty completion rejected'); END`)
			case "timing-failure":
				f.exec(t, `CREATE TRIGGER empty_fail_timing BEFORE INSERT ON run_stage_timings WHEN NEW.stage='added' BEGIN SELECT RAISE(ABORT,'empty timing rejected'); END`)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel-before-read" {
				cancel()
			}
			err := f.e.process(ctx, f.run.ID, true)
			want := []string{"continuity", "continuity", "vision-job", "vision-receipt"}
			completed, timing := false, false
			switch mode {
			case "receipt-failure":
				if err == nil || !strings.HasPrefix(err.Error(), "save continuity bypass receipt: ") || !strings.Contains(err.Error(), "empty bypass rejected") {
					t.Fatal("bypass error contract changed", err)
				}
			case "completion-failure":
				if err == nil || !strings.HasPrefix(err.Error(), "constraint failed: empty completion rejected") {
					t.Fatal("completion error gained wrapping or changed", err)
				}
				want = append(want, "bypass-receipt")
			case "cancel-before-read":
				if !errors.Is(err, context.Canceled) {
					t.Fatal("caller context was lost", err)
				}
				want = []string{}
			default:
				if err != nil {
					t.Fatal(err)
				}
				completed, timing = true, mode != "timing-failure"
				want = append(want, "bypass-receipt", "completed")
				if timing {
					want = append(want, "timing:added")
				}
				want = append(want, "session-completed")
			}
			rows, loadErr := f.db.Query(`SELECT effect FROM empty_completion_effects ORDER BY sequence`)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			actual := []string{}
			for rows.Next() {
				var effect string
				if err := rows.Scan(&effect); err != nil {
					t.Fatal(err)
				}
				actual = append(actual, effect)
			}
			loadErr = rows.Err()
			rows.Close()
			if loadErr != nil || !reflect.DeepEqual(actual, want) {
				t.Fatal("effect order changed", actual, want, loadErr)
			}
			run, loadErr := f.state.GetRun(context.Background(), f.run.ID)
			if loadErr != nil || (run.Status == "completed") != completed {
				t.Fatal("completion status changed", run, loadErr)
			}
			wantTiming := 0
			if timing {
				wantTiming = 1
			}
			if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM run_stage_timings WHERE run_id=? AND stage='added'`, f.run.ID) != wantTiming {
				t.Fatal("added timing changed")
			}
			if mode != "cancel-before-read" && (run.Coverage["visionEvaluation"] == nil || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE run_id=?`, f.run.ID) != 1 || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM content_continuity_occurrences WHERE run_id=?`, f.run.ID) != 2) {
				t.Fatal("earlier durable effects were lost", run.Coverage)
			}
			_, receipt := run.Coverage["acquisitionPlanning"]
			if receipt != (mode != "receipt-failure" && mode != "cancel-before-read") {
				t.Fatal("receipt commit boundary changed", run.Coverage)
			}
			emptyCompletionAssertNoReasoning(t, f, p)
		})
	}
}

func TestEngineEmptyCandidateCompletionCharacterizationAdvance(t *testing.T) {
	for _, mode := range []string{"success", "cancel-after-commit", "finalization-failure"} {
		t.Run(mode, func(t *testing.T) {
			f, p := emptyCompletionFixture(t, 1, 0)
			if mode == "finalization-failure" {
				f.exec(t, `CREATE TRIGGER empty_fail_finalize BEFORE UPDATE OF status ON sessions WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'empty finalization rejected'); END`)
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
					t.Error("empty completion worker did not finish")
				}
			})
			current := waitSession(t, f.e, f.session.ID, func(s domain.Session) bool { return s.Runs[0].Status == "completed" })
			if current.Status == "completed" {
				t.Fatal("session advanced before schedule lock")
			}
			select {
			case err := <-result:
				t.Fatal("empty completion bypassed schedule lock", err)
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
				t.Fatal("empty completion did not return after schedule release")
			}
			switch mode {
			case "success":
				if err != nil {
					t.Fatal(err)
				}
			case "cancel-after-commit":
				if !errors.Is(err, context.Canceled) {
					t.Fatal("advance lost caller context", err)
				}
			case "finalization-failure":
				if err == nil || !strings.HasPrefix(err.Error(), "finalize unified session: ") || !strings.Contains(err.Error(), "empty finalization rejected") {
					t.Fatal("advance error contract changed", err)
				}
			}
			stored, loadErr := f.state.GetRun(context.Background(), f.run.ID)
			if loadErr != nil || stored.Status != "completed" || stored.Coverage["acquisitionPlanning"] == nil || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM run_stage_timings WHERE run_id=? AND stage='added'`, f.run.ID) != 1 {
				t.Fatal("advance failure lost committed empty result/timing", stored, loadErr)
			}
			emptyCompletionAssertNoReasoning(t, f, p)
		})
	}
}
