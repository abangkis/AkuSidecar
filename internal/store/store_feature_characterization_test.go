package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func storeFeatureCount(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := s.db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func storeFeatureSession(t *testing.T) (*Store, domain.Session, domain.Run) {
	t.Helper()
	s := openTestStore(t)
	settings, err := s.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, context.Background(), "Store feature characterization", settings)
	if err != nil || len(session.Runs) != 1 {
		t.Fatal(session, err)
	}
	return s, session, session.Runs[0]
}

func storeFeatureObservation() domain.Observation {
	return domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Coverage: map[string]any{"fixture": "accepted"}, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:store-feature", PlatformID: "x:status:45123", Permalink: "https://x.com/fixture/status/45123", Text: "Bounded native evidence for the Store feature contract."}}}}}
}

func storeFeatureClaim(t *testing.T, s *Store, run domain.Run) domain.BridgeCommand {
	t.Helper()
	command, err := s.StartRun(context.Background(), run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimCommandForCollector(context.Background(), run.ID, "store-fixture", "bridge")
	if err != nil || claimed == nil || claimed.ID != command.ID {
		t.Fatal(claimed, err)
	}
	return *claimed
}

// Characterize durable behavior before automatic file separation. Each failure
// is injected into a disposable SQLite fixture; production bodies stay intact.
func TestStoreFeatureCharacterizationFeedback(t *testing.T) {
	for _, mode := range []string{"more", "less", "client-origin", "validation", "lookup", "feedback-failure", "ledger-failure", "memory-failure", "less-failure", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			s, f := timelineReadFixture(t)
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "store-fixture", "context-retained"))
			defer cancel()
			timing := &FeedbackTiming{Lookup: -1, Hydration: -1, BeginTx: -1, Transaction: -1, Commit: -1, Total: -1, PoolWaitCountDelta: -1, PoolWaitDurationDelta: -1}
			ctx = WithFeedbackTiming(ctx, timing)
			if ctx.Value("store-fixture") != "context-retained" {
				t.Fatal("timing wrapper dropped context values")
			}
			input := domain.Feedback{Direction: "more"}
			wantFeedback, wantMemory, wantLedger := 1, 1, 1
			id := f.Item.ID
			switch mode {
			case "less":
				reason := "not_interested"
				input.Direction, input.Reason = "less", &reason
				wantMemory = 0
			case "client-origin":
				input.Origin = "calibration"
			case "validation":
				input.Direction = "invalid"
			case "lookup":
				id = "missing"
			case "feedback-failure":
				timelineReadExec(t, s, `CREATE TRIGGER feature_feedback_fail BEFORE INSERT ON feedback_events BEGIN SELECT RAISE(ABORT,'feature feedback rejected'); END`)
			case "ledger-failure", "less-failure":
				if mode == "less-failure" {
					if _, err := s.AddFeedback(ctx, id, input); err != nil {
						t.Fatal(err)
					}
					reason := "not_interested"
					input.Direction, input.Reason = "less", &reason
				}
				timelineReadExec(t, s, `CREATE TRIGGER feature_ledger_fail BEFORE INSERT ON preference_learning_ledger BEGIN SELECT RAISE(ABORT,'feature ledger rejected'); END`)
			case "memory-failure":
				timelineReadExec(t, s, `CREATE TRIGGER feature_memory_fail BEFORE INSERT ON memory_items BEGIN SELECT RAISE(ABORT,'feature memory rejected'); END`)
			case "cancelled":
				cancel()
			}
			feedback, err := s.AddFeedback(ctx, id, input)
			failure := mode != "more" && mode != "less" && mode != "client-origin"
			if failure {
				if err == nil || !reflect.DeepEqual(feedback, domain.Feedback{}) {
					t.Fatal("feedback failure returned success", feedback, err)
				}
				wantFeedback, wantMemory, wantLedger = 0, 0, 0
				if mode == "less-failure" {
					wantFeedback, wantMemory, wantLedger = 1, 1, 1
				}
				if mode == "cancelled" && !errors.Is(err, context.Canceled) || mode == "lookup" && !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("feedback context/missing-row error changed", err)
				}
				prefix := map[string]string{"ledger-failure": "persist routine preference learning: ", "less-failure": "persist routine preference learning: ", "memory-failure": "project routine More memory: "}[mode]
				if prefix != "" && !strings.HasPrefix(err.Error(), prefix) {
					t.Fatal("feedback error wrapping changed", err)
				}
			} else if err != nil || feedback.ID == "" || feedback.Origin != "routine" || feedback.TimelineID != id || feedback.SessionID != f.Session.ID || feedback.RunID != f.Run.ID {
				t.Fatal("feedback normalization/result changed", feedback, err)
			}
			if timing.Total < 0 || timing.Lookup < 0 || timing.PoolWaitCountDelta < 0 || timing.PoolWaitDurationDelta < 0 {
				t.Fatal("feedback timing lifecycle changed", timing)
			}
			if mode == "validation" || mode == "lookup" || mode == "cancelled" {
				if timing.Hydration != -1 || timing.BeginTx != -1 || timing.Transaction != -1 || timing.Commit != -1 {
					t.Fatal("early failure reached a later timed stage", timing)
				}
			} else if timing.Hydration < 0 || timing.BeginTx < 0 || timing.Transaction < 0 {
				t.Fatal("transaction timing stages were skipped", timing)
			}
			if storeFeatureCount(t, s, `SELECT COUNT(*) FROM feedback_events`) != wantFeedback || storeFeatureCount(t, s, `SELECT COUNT(*) FROM memory_items WHERE lifecycle_state='active'`) != wantMemory || storeFeatureCount(t, s, `SELECT COUNT(*) FROM preference_learning_ledger`) != wantLedger {
				t.Fatal("feedback/learning/memory transaction atomicity changed")
			}
		})
	}
}

func TestStoreFeatureCharacterizationSettingsAndAdmission(t *testing.T) {
	for _, mode := range []string{"onboarding", "onboarding-failure", "settings-decode", "token-error", "source-sync-failure", "session-run-failure", "session-batch-failure", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			s := openTestStore(t)
			ctx := context.Background()
			settings, err := s.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "onboarding", "onboarding-failure":
				if mode == "onboarding-failure" {
					timelineReadExec(t, s, `CREATE TRIGGER feature_onboarding_fail BEFORE INSERT ON meta WHEN NEW.key='onboarding_completed_at' BEGIN SELECT RAISE(ABORT,'feature onboarding rejected'); END`)
				}
				result, err := s.CompleteOnboarding(ctx, []domain.Source{domain.SourceX})
				if mode == "onboarding-failure" {
					current, loadErr := s.Onboarding(ctx)
					saved, settingsErr := s.GetSettings(ctx)
					if err == nil || !strings.Contains(err.Error(), "feature onboarding rejected") || loadErr != nil || settingsErr != nil || current.Status != "not_started" || !reflect.DeepEqual(saved.ActiveSources, settings.ActiveSources) {
						t.Fatal("onboarding transaction changed", result, err, current)
					}
				} else if err != nil || result.Status != "completed" || result.Profile == nil || !reflect.DeepEqual(result.Profile.ActiveSources, []domain.Source{domain.SourceX}) {
					t.Fatal(result, err)
				}
			case "settings-decode":
				timelineReadExec(t, s, `UPDATE settings SET value_json='invalid-json' WHERE key='runtime'`)
				if _, err := s.GetSettings(ctx); err == nil || !strings.HasPrefix(err.Error(), "decode settings: ") {
					t.Fatal(err)
				}
			case "token-error":
				token, err := s.BridgeToken(ctx)
				if err != nil || len(token) != 64 || !s.MatchesBridgeToken(ctx, token) || s.MatchesBridgeToken(ctx, token+"x") {
					t.Fatal("token comparison contract changed")
				}
				timelineReadExec(t, s, `DROP TABLE meta`)
				if s.MatchesBridgeToken(ctx, token) {
					t.Fatal("token read error failed open")
				}
			case "source-sync-failure":
				timelineReadExec(t, s, `UPDATE source_definitions SET display_name='retained'`)
				timelineReadExec(t, s, `CREATE TRIGGER feature_source_sync_fail BEFORE INSERT ON source_definitions WHEN NEW.id='instagram' BEGIN SELECT RAISE(ABORT,'feature registry rejected'); END`)
				if err := s.syncSourceDefinitions(ctx); err == nil || !strings.HasPrefix(err.Error(), "sync source definition instagram: ") {
					t.Fatal(err)
				}
				if storeFeatureCount(t, s, `SELECT COUNT(*) FROM source_definitions WHERE display_name<>'retained'`) != 0 {
					t.Fatal("partial registry sync committed")
				}
			case "session-run-failure", "session-batch-failure":
				if mode == "session-run-failure" {
					timelineReadExec(t, s, `CREATE TRIGGER feature_run_create_fail BEFORE INSERT ON runs BEGIN SELECT RAISE(ABORT,'feature run rejected'); END`)
				} else {
					timelineReadExec(t, s, `CREATE TRIGGER feature_batch_create_fail BEFORE INSERT ON auto_update_batches BEGIN SELECT RAISE(ABORT,'feature batch rejected'); END`)
				}
				_, err := createPreparedUpdateSession(s, ctx, "Store prepared fixture", settings)
				if err == nil || !strings.Contains(err.Error(), "feature") || storeFeatureCount(t, s, `SELECT COUNT(*) FROM sessions`) != 0 || storeFeatureCount(t, s, `SELECT COUNT(*) FROM runs`) != 0 || storeFeatureCount(t, s, `SELECT COUNT(*) FROM auto_update_batches`) != 0 {
					t.Fatal("session creation was not atomic", err)
				}
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				if _, err := createVisibleUpdateSession(s, cancelled, "Store cancelled fixture", settings); !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestStoreFeatureCharacterizationCaptureAtomicity(t *testing.T) {
	for _, mode := range []string{"start-failure", "observation-failure", "coverage-failure", "followup-failure", "followup-fallback", "cancel-failure", "failure-write-failure", "collector-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			s, session, run := storeFeatureSession(t)
			ctx := context.Background()
			if mode == "start-failure" {
				timelineReadExec(t, s, `CREATE TRIGGER feature_start_fail BEFORE INSERT ON bridge_commands BEGIN SELECT RAISE(ABORT,'feature command rejected'); END`)
				if _, err := s.StartRun(ctx, run.ID, map[string]any{}); err == nil {
					t.Fatal("StartRun unexpectedly succeeded")
				}
				current, err := s.GetSession(ctx, session.ID)
				if err != nil || current.Status != "queued" || current.Runs[0].Status != "queued" || storeFeatureCount(t, s, `SELECT COUNT(*) FROM bridge_commands`) != 0 {
					t.Fatal("StartRun partial transaction", current, err)
				}
				return
			}
			if mode == "collector-mismatch" {
				command, err := s.StartRun(ctx, run.ID, map[string]any{})
				if err != nil {
					t.Fatal(err)
				}
				claimed, err := s.ClaimCommandForCollector(ctx, run.ID, "fixture", "headless")
				if err != nil || claimed != nil || storeFeatureCount(t, s, `SELECT COUNT(*) FROM bridge_commands WHERE id=? AND status='queued'`, command.ID) != 1 {
					t.Fatal("collector mismatch changed durable claim", claimed, err)
				}
				return
			}
			command := storeFeatureClaim(t, s, run)
			switch mode {
			case "observation-failure", "coverage-failure":
				trigger := `CREATE TRIGGER feature_observation_fail BEFORE INSERT ON observations BEGIN SELECT RAISE(ABORT,'feature observation rejected'); END`
				if mode == "coverage-failure" {
					trigger = `CREATE TRIGGER feature_coverage_fail BEFORE UPDATE OF coverage_json ON runs BEGIN SELECT RAISE(ABORT,'feature coverage rejected'); END`
				}
				timelineReadExec(t, s, trigger)
				if err := s.SaveObservation(ctx, command.ID, run.ID, storeFeatureObservation()); err == nil {
					t.Fatal("SaveObservation unexpectedly succeeded")
				}
				if storeFeatureCount(t, s, `SELECT COUNT(*) FROM observations`) != 0 || storeFeatureCount(t, s, `SELECT COUNT(*) FROM content_identity_aliases`) != 0 || storeFeatureCount(t, s, `SELECT COUNT(*) FROM bridge_commands WHERE status='claimed'`) != 1 {
					t.Fatal("observation/identity/claim rollback changed")
				}
			case "followup-failure", "followup-fallback":
				if err := s.SaveObservation(ctx, command.ID, run.ID, storeFeatureObservation()); err != nil {
					t.Fatal(err)
				}
				if mode == "followup-failure" {
					timelineReadExec(t, s, `CREATE TRIGGER feature_followup_fail BEFORE INSERT ON bridge_commands BEGIN SELECT RAISE(ABORT,'feature followup rejected'); END`)
				}
				_, err := s.QueueFollowUp(ctx, run.ID, map[string]any{})
				if mode == "followup-failure" {
					if err == nil {
						t.Fatal("QueueFollowUp unexpectedly succeeded")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					claimed, err := s.ClaimCommand(ctx, run.ID, "fixture")
					if err != nil || claimed == nil {
						t.Fatal(claimed, err)
					}
					resume, err := s.FailCommand(ctx, claimed.ID, run.ID, domain.Failure{Code: "fixture_failure", Stage: "capture", Message: "fixture"})
					if err != nil || !resume {
						t.Fatal("accepted evidence fallback changed", resume, err)
					}
				}
				current, err := s.GetRun(ctx, run.ID)
				if err != nil || current.Status != "reasoning" || storeFeatureCount(t, s, `SELECT COUNT(*) FROM observations`) != 1 {
					t.Fatal("follow-up lost durable observation", current, err)
				}
			case "cancel-failure":
				timelineReadExec(t, s, `CREATE TRIGGER feature_cancel_fail BEFORE UPDATE OF status ON bridge_commands WHEN NEW.status='cancelled' BEGIN SELECT RAISE(ABORT,'feature cancellation rejected'); END`)
				if err := s.CancelSession(ctx, session.ID); err == nil {
					t.Fatal("CancelSession unexpectedly succeeded")
				}
				current, err := s.GetSession(ctx, session.ID)
				if err != nil || current.Status != "running" || current.Runs[0].Status != "waiting_for_bridge" || storeFeatureCount(t, s, `SELECT COUNT(*) FROM bridge_commands WHERE status='claimed'`) != 1 {
					t.Fatal("session/run/command cancellation did not roll back", current, err)
				}
			case "failure-write-failure":
				timelineReadExec(t, s, `CREATE TRIGGER feature_failure_fail BEFORE UPDATE OF status ON runs WHEN NEW.status='failed' BEGIN SELECT RAISE(ABORT,'feature run failure rejected'); END`)
				if err := s.FailRun(ctx, run.ID, domain.Failure{Code: "fixture_failure", Stage: "capture"}); err == nil {
					t.Fatal("FailRun unexpectedly succeeded")
				}
				current, err := s.GetRun(ctx, run.ID)
				if err != nil || current.Status != "waiting_for_bridge" || current.Error != nil {
					t.Fatal("failed failure-write changed run", current, err)
				}
			}
		})
	}
}

func TestStoreFeatureCharacterizationCompletionAndTelemetry(t *testing.T) {
	for _, mode := range []string{"completion", "completion-failure", "finalization-failure", "telemetry", "telemetry-failure", "coverage-failure", "retry-rejection"} {
		t.Run(mode, func(t *testing.T) {
			s, session, run := storeFeatureSession(t)
			ctx := context.Background()
			switch mode {
			case "completion", "completion-failure", "finalization-failure":
				if mode == "completion-failure" {
					timelineReadExec(t, s, `CREATE TRIGGER feature_complete_fail BEFORE UPDATE OF status ON runs WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'feature completion rejected'); END`)
				}
				err := s.CompleteRun(ctx, run, domain.ReasoningResult{Summary: "fixture empty completion"}, nil, nil, map[string]any{"fixture": true})
				if mode == "completion-failure" {
					current, loadErr := s.GetRun(ctx, run.ID)
					if err == nil || loadErr != nil || current.Status != "queued" || current.Summary != "" || current.Coverage["fixture"] != nil {
						t.Fatal("completion transaction changed", current, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if next, err := s.AdvanceSession(ctx, session.ID); err != nil || next != nil {
					t.Fatal("terminal run was advanced", next, err)
				}
				if mode == "finalization-failure" {
					timelineReadExec(t, s, `CREATE TRIGGER feature_finalize_fail BEFORE UPDATE OF status ON sessions WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'feature finalization rejected'); END`)
				}
				err = s.FinalizeSession(ctx, session.ID)
				if (err != nil) != (mode == "finalization-failure") {
					t.Fatal(err)
				}
				current, err := s.GetRun(ctx, run.ID)
				if err != nil || current.Status != "completed" || current.Summary != "fixture empty completion" || current.Coverage["fixture"] != true {
					t.Fatal("finalization changed committed completion", current, err)
				}
			case "telemetry", "telemetry-failure":
				if mode == "telemetry-failure" {
					timelineReadExec(t, s, `CREATE TRIGGER feature_telemetry_fail BEFORE INSERT ON reasoning_invocations BEGIN SELECT RAISE(ABORT,'feature telemetry rejected'); END`)
				}
				tokens := int64(11)
				err := s.SaveTelemetry(ctx, domain.ReasoningTelemetry{ID: "feature-telemetry", RunID: run.ID, Phase: "candidate_evaluation", Provider: "fixture", Model: "offline", DurationMS: 17, Status: "completed", InputTokens: &tokens, CreatedAt: domain.Now()})
				rows, loadErr := s.ReasoningInvocationsBySession(ctx, session.ID)
				if mode == "telemetry-failure" {
					if err == nil || loadErr != nil || len(rows) != 0 {
						t.Fatal("telemetry failure persisted a row", rows, err)
					}
				} else if err != nil || loadErr != nil || len(rows) != 1 || rows[0].CallerLatencyMS != 17 || rows[0].InputTokens == nil || *rows[0].InputTokens != tokens || rows[0].OutputTokens != nil {
					t.Fatal("telemetry projection/fallback/null contract changed", rows, err)
				}
			case "coverage-failure":
				timelineReadExec(t, s, `CREATE TRIGGER feature_receipt_fail BEFORE UPDATE OF coverage_json ON runs BEGIN SELECT RAISE(ABORT,'feature receipt rejected'); END`)
				if err := s.SetRunCoverageField(ctx, run.ID, "fixture", true); err == nil {
					t.Fatal("coverage failure succeeded")
				}
				current, err := s.GetRun(ctx, run.ID)
				if err != nil || current.Coverage["fixture"] != nil {
					t.Fatal(current, err)
				}
			case "retry-rejection":
				if _, err := s.RetryFailedRun(ctx, run.ID); err == nil || err.Error() != "only a failed run can be re-evaluated" {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestStoreFeatureCharacterizationReset(t *testing.T) {
	for _, mode := range []string{"active-guard", "learning-failure", "full-failure", "backup", "legacy-marker", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			s, f := timelineReadFixture(t)
			ctx := context.Background()
			settings, err := s.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "active-guard":
				timelineReadExec(t, s, `UPDATE sessions SET status='running' WHERE id=?`, f.Session.ID)
				if _, err := s.FullReset(ctx, settings); err == nil || err.Error() != "reset is unavailable while an update is running" {
					t.Fatal(err)
				}
			case "learning-failure":
				timelineReadExec(t, s, `INSERT INTO preference_model(id,model_json,feedback_count,updated_at) VALUES(1,'{}',0,'fixture')`)
				timelineReadExec(t, s, `CREATE TRIGGER feature_learning_fail BEFORE DELETE ON preference_model BEGIN SELECT RAISE(ABORT,'feature learning rejected'); END`)
				if err := s.ResetLearning(ctx); err == nil || storeFeatureCount(t, s, `SELECT COUNT(*) FROM preference_model`) != 1 {
					t.Fatal("learning reset rollback changed", err)
				}
			case "full-failure":
				timelineReadExec(t, s, `CREATE TRIGGER feature_full_reset_fail BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT,'feature reset rejected'); END`)
				if _, err := s.FullReset(ctx, settings); err == nil || storeFeatureCount(t, s, `SELECT COUNT(*) FROM timeline_items WHERE id=?`, f.Item.ID) != 1 || storeFeatureCount(t, s, `SELECT COUNT(*) FROM sessions WHERE id=?`, f.Session.ID) != 1 {
					t.Fatal("full reset rollback changed", err)
				}
			case "backup":
				path, err := s.createVerifiedBackup(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
				if err := verifySQLiteBackup(path); err != nil {
					t.Fatal(err)
				}
			case "legacy-marker":
				timelineReadExec(t, s, `INSERT INTO meta(key,value) VALUES('pending_app_profile_reset','1')`)
				pending, err := s.PendingAppProfileReset(ctx)
				if err != nil || !pending {
					t.Fatal(pending, err)
				}
				if err := s.ConsumePendingAppProfileReset(ctx); err != nil {
					t.Fatal(err)
				}
				if pending, err := s.PendingAppProfileReset(ctx); err != nil || pending {
					t.Fatal(pending, err)
				}
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				if err := s.ResetLearning(cancelled); !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
			if storeFeatureCount(t, s, `SELECT COUNT(*) FROM timeline_items WHERE id=?`, f.Item.ID) != 1 {
				t.Fatal("maintenance check removed Timeline data")
			}
		})
	}
}

func TestStoreFeatureCharacterizationQueuedExpiry(t *testing.T) {
	s, _, run := storeFeatureSession(t)
	ctx := context.Background()
	command, err := s.StartRun(ctx, run.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := time.Parse(time.RFC3339Nano, command.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if expiredQueuedBridgeCommand(command, created.Add(2*time.Minute-time.Nanosecond)) || !expiredQueuedBridgeCommand(command, created.Add(2*time.Minute)) {
		t.Fatal("dispatch deadline boundary changed")
	}
	if resume, err := s.ExpireQueuedBridgeCommand(ctx, command.ID, run.ID, domain.Failure{Code: "bridge_dispatch_timeout", Stage: "capture"}, created.Add(2*time.Minute)); err != nil || resume {
		t.Fatal("queued expiry/fallback changed", resume, err)
	}
	current, err := s.GetRun(ctx, run.ID)
	if err != nil || current.Status != "failed" {
		t.Fatal(current, err)
	}
}
