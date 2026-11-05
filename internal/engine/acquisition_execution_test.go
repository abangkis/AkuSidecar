package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

type acquisitionExecutionProvider struct {
	reasoning.Deterministic
	plan    func(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error)
	analyze func(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error)
}

func (p *acquisitionExecutionProvider) Plan(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return p.plan(ctx, run, obs, knowledge)
}

func (p *acquisitionExecutionProvider) Analyze(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	return p.analyze(ctx, run, obs, knowledge)
}

// Exercise the existing process entry point before and after extraction. SQL
// triggers affect only disposable fixture data and observe durable effect order.
func TestEngineAcquisitionExecutionCharacterizationModel(t *testing.T) {
	for _, mode := range []string{"finish", "follow-up", "continuation-not-ready", "continuation-not-ready-headless", "stage-failure", "queue-failure", "receipt-failure", "follow-up-receipt-failure", "telemetry-failure", "incomplete", "untyped-failure", "authentication-failure", "joined-cancellation", "joined-deadline", "caller-cancellation"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t, domain.SourceX)
			settings, err := f.state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings.CalibrationEnabled, settings.AIDetectionEnabled = false, false
			if err := f.state.SaveSettings(context.Background(), settings); err != nil {
				t.Fatal(err)
			}
			command := f.claim(t)
			obs := acquisitionPolicyObservation(domain.SourceX)
			obs.Coverage["captureQuality"] = map[string]any{"verdict": "usable_degraded"}
			obs.Coverage["captureMode"] = "browser"
			if mode == "continuation-not-ready-headless" {
				obs.Coverage["captureMode"] = "headless_worker"
				mode = "continuation-not-ready"
			}
			if mode == "continuation-not-ready" {
				obs.Coverage["frontier"].(map[string]any)["continuationReady"] = false
			}
			if err := validateObservation(obs); err != nil {
				t.Fatal(err)
			}
			if err := f.state.SaveObservation(context.Background(), command.ID, f.run.ID, obs); err != nil {
				t.Fatal(err)
			}
			f.exec(t, `CREATE TABLE acquisition_effect_order(sequence INTEGER PRIMARY KEY, effect TEXT NOT NULL)`)
			f.exec(t, `CREATE TRIGGER acquisition_stage_order AFTER UPDATE OF stage ON runs WHEN NEW.id=OLD.id AND NEW.stage IN ('acquisition_planning','candidate_evaluation') BEGIN INSERT INTO acquisition_effect_order(effect) VALUES(NEW.stage); END`)
			f.exec(t, `CREATE TRIGGER acquisition_telemetry_order AFTER INSERT ON reasoning_invocations BEGIN INSERT INTO acquisition_effect_order(effect) VALUES('telemetry'); END`)
			f.exec(t, `CREATE TRIGGER acquisition_queue_order AFTER INSERT ON bridge_commands WHEN json_extract(NEW.payload_json,'$.acquisitionRound')=2 BEGIN INSERT INTO acquisition_effect_order(effect) VALUES('queue'); END`)
			f.exec(t, `CREATE TRIGGER acquisition_receipt_order AFTER UPDATE OF coverage_json ON runs WHEN json_type(NEW.coverage_json,'$.acquisitionPlanning')='object' BEGIN INSERT INTO acquisition_effect_order(effect) VALUES('receipt'); END`)
			switch mode {
			case "stage-failure":
				f.exec(t, `CREATE TRIGGER acquisition_stage_failure BEFORE UPDATE OF stage ON runs WHEN NEW.stage='acquisition_planning' BEGIN SELECT RAISE(ABORT,'acquisition stage rejected'); END`)
			case "queue-failure":
				f.exec(t, `CREATE TRIGGER acquisition_queue_failure BEFORE INSERT ON bridge_commands WHEN json_extract(NEW.payload_json,'$.acquisitionRound')=2 BEGIN SELECT RAISE(ABORT,'acquisition queue rejected'); END`)
			case "receipt-failure", "follow-up-receipt-failure":
				f.exec(t, `CREATE TRIGGER acquisition_receipt_failure BEFORE UPDATE OF coverage_json ON runs WHEN json_type(NEW.coverage_json,'$.acquisitionPlanning')='object' BEGIN SELECT RAISE(ABORT,'acquisition receipt rejected'); END`)
			case "telemetry-failure":
				f.exec(t, `CREATE TRIGGER acquisition_telemetry_failure BEFORE INSERT ON reasoning_invocations BEGIN SELECT RAISE(ABORT,'acquisition telemetry rejected'); END`)
			}
			key := struct{}{}
			parent, cancel := context.WithTimeout(context.WithValue(context.Background(), key, "planning-context"), time.Minute)
			defer cancel()
			stop := errors.New("acquisition evaluation reached")
			incomplete := &inference.Error{Code: inference.FailureCodeIncomplete, Category: inference.FailureCategoryResponseMissing, Stage: inference.FailureStageProvider, Retry: inference.RetryCaller, PartialOutputSeen: true}
			var planFailure error
			switch mode {
			case "incomplete", "caller-cancellation":
				planFailure = incomplete
			case "untyped-failure":
				planFailure = errors.New("acquisition provider rejected")
			case "authentication-failure":
				planFailure = &inference.Error{Code: inference.FailureCode("authentication_failed"), Stage: inference.FailureStageProvider}
			case "joined-cancellation":
				planFailure = errors.Join(incomplete, context.Canceled)
			case "joined-deadline":
				planFailure = errors.Join(incomplete, context.DeadlineExceeded)
			}
			planned, analyzed := 0, 0
			f.e.provider = &acquisitionExecutionProvider{
				plan: func(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
					planned++
					if ctx != parent || ctx.Value(key) != "planning-context" || knowledge != nil || run.ID != f.run.ID {
						t.Fatal("planner inputs/context changed")
					}
					stored, err := f.state.GetRun(ctx, run.ID)
					if err != nil || stored.Stage != "acquisition_planning" {
						t.Fatal("planner ran before stage persistence", stored, err)
					}
					_, telemetry, _ := (reasoning.Deterministic{}).Plan(ctx, run, observation, knowledge)
					decision := "finish"
					if mode == "follow-up" || mode == "continuation-not-ready" || mode == "queue-failure" || mode == "follow-up-receipt-failure" || planFailure != nil {
						decision = "request_follow_up"
					}
					if mode == "caller-cancellation" {
						cancel()
					}
					return reasoning.AcquisitionPlan{Decision: decision, Reason: "offline acquisition decision"}, telemetry, planFailure
				},
				analyze: func(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
					analyzed++
					if ctx != parent || ctx.Value(key) != "planning-context" {
						t.Fatal("evaluation context changed")
					}
					stored, err := f.state.GetRun(ctx, run.ID)
					if err != nil || stored.Stage != "candidate_evaluation" || observation.Coverage["acquisitionPlanning"] == nil {
						t.Fatal("evaluation ran before receipt/stage", stored, err)
					}
					return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, stop
				},
			}
			err = f.e.process(parent, f.run.ID, true)
			stored, loadErr := f.state.GetRun(context.Background(), f.run.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			var commands, telemetry int
			if e := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&commands); e != nil {
				t.Fatal(e)
			}
			if e := f.db.QueryRow(`SELECT COUNT(*) FROM reasoning_invocations WHERE run_id=? AND phase='acquisition_planning'`, f.run.ID).Scan(&telemetry); e != nil {
				t.Fatal(e)
			}
			var order string
			if e := f.db.QueryRow(`SELECT COALESCE(group_concat(effect,','),'') FROM (SELECT effect FROM acquisition_effect_order ORDER BY sequence)`).Scan(&order); e != nil {
				t.Fatal(e)
			}
			wantOrder := "acquisition_planning,telemetry,receipt,candidate_evaluation"
			wantCommands, wantPlan, wantAnalyze, wantTelemetry := 1, 1, 1, 1
			receipt, _ := stored.Coverage["acquisitionPlanning"].(map[string]any)
			if mode == "continuation-not-ready" {
				wantPlan, wantTelemetry = 0, 0
				wantOrder = "receipt,candidate_evaluation"
			}
			switch mode {
			case "stage-failure":
				wantOrder = ""
				wantPlan, wantAnalyze, wantTelemetry = 0, 0, 0
				if err == nil || !strings.Contains(err.Error(), "acquisition stage rejected") {
					t.Fatal(err)
				}
			case "queue-failure":
				wantOrder = "acquisition_planning,telemetry"
				wantAnalyze = 0
				if err == nil || !strings.Contains(err.Error(), "acquisition queue rejected") {
					t.Fatal(err)
				}
			case "receipt-failure", "follow-up-receipt-failure":
				wantOrder = "acquisition_planning,telemetry"
				wantAnalyze = 0
				if mode == "follow-up-receipt-failure" {
					wantCommands = 2
					wantOrder += ",queue"
				}
				if err == nil || !strings.HasPrefix(err.Error(), "save model acquisition planning receipt: ") {
					t.Fatal(err)
				}
			case "follow-up":
				wantCommands, wantAnalyze = 2, 0
				wantOrder = "acquisition_planning,telemetry,queue,receipt"
				if err != nil {
					t.Fatal(err)
				}
			case "telemetry-failure":
				wantTelemetry = 0
				wantOrder = "acquisition_planning,receipt,candidate_evaluation"
				if !errors.Is(err, stop) {
					t.Fatal("telemetry failure became fatal", err)
				}
			case "untyped-failure", "authentication-failure", "joined-cancellation", "joined-deadline", "caller-cancellation":
				wantAnalyze = 0
				wantOrder = "acquisition_planning,telemetry"
				if err != planFailure {
					t.Fatal("planner error identity changed", err, planFailure)
				}
			default:
				if !errors.Is(err, stop) {
					t.Fatal("evaluation handoff changed", err)
				}
			}
			if planned != wantPlan || analyzed != wantAnalyze || commands != wantCommands || telemetry != wantTelemetry || order != wantOrder {
				t.Fatalf("calls=%d/%d commands=%d telemetry=%d order=%q want=%q err=%v", planned, analyzed, commands, telemetry, order, wantOrder, err)
			}
			if wantAnalyze == 0 && mode != "follow-up" && receipt != nil {
				t.Fatal("failed planning persisted a success receipt", receipt)
			}
			if mode == "incomplete" && (receipt["mode"] != "fallback" || receipt["decision"] != "finish" || receipt["fallbackPolicy"] != "evaluate_captured_skip_follow_up") {
				t.Fatal("partial planner decision was used", receipt)
			}
			if mode == "continuation-not-ready" && (receipt["mode"] != "local_frontier" || receipt["decision"] != "finish" || receipt["followUpQueued"] != false) {
				t.Fatal("missing continuation did not finish locally", receipt)
			}
			if wantCommands == 2 && stored.Status != "waiting_for_bridge" {
				t.Fatal("durable follow-up lost", stored)
			}
		})
	}
}
