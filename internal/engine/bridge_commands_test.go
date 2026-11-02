package engine

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type bridgeCommandFixture struct {
	e       *Engine
	state   *store.Store
	db      *sql.DB
	session domain.Session
	run     domain.Run
	manager *captureruntime.Manager
}

// SQL changes only disposable fixture state; all command operations use Engine.
func newBridgeCommandFixture(t *testing.T, sources ...domain.Source) bridgeCommandFixture {
	t.Helper()
	e, state := testEngine(t)
	manager := attachTestCapture(t, e)
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		e.Shutdown()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("command workers did not retire")
		}
	})
	if len(sources) > 0 {
		settings, err := e.Settings(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		settings.ActiveSources = sources
		if _, err := e.SaveSettings(context.Background(), settings); err != nil {
			t.Fatal(err)
		}
	}
	session, err := e.StartVisibleUpdate(context.Background(), "offline command characterization")
	if err != nil {
		t.Fatal(err)
	}
	return bridgeCommandFixture{e, state, db, session, session.Runs[0], manager}
}

func (f bridgeCommandFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f bridgeCommandFixture) claim(t *testing.T) *domain.BridgeCommand {
	t.Helper()
	command, err := f.e.ClaimCommand(context.Background(), f.run.ID, "offline-bridge")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	return command
}

func (f bridgeCommandFixture) commandID(t *testing.T) string {
	t.Helper()
	var id string
	if err := f.db.QueryRow(`SELECT id FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f bridgeCommandFixture) commandStatus(t *testing.T) string {
	t.Helper()
	var status string
	if err := f.db.QueryRow(`SELECT status FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f bridgeCommandFixture) observation() domain.Observation {
	return domain.Observation{Source: f.run.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{Author: "Fixture", Text: "Offline native evidence"}}}}, Coverage: map[string]any{"quality": "complete"}}
}

func TestEngineBridgeCommandsCharacterizationClaimGuards(t *testing.T) {
	for _, mode := range []string{"development-driver", "development-collector", "paused", "unsupported-source", "collector", "reserved-source", "frozen-driver", "stale-owner"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t)
			driver, collector, want := "browser", collection.BackendBridge, ""
			switch mode {
			case "development-driver", "development-collector":
				f.e.developmentSources = map[domain.Source]bool{f.run.Source: true}
				f.e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
				if mode == "development-collector" {
					driver = "headless"
				}
				want = "development session collector unavailable"
			case "paused":
				f.e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
				f.exec(t, `UPDATE bridge_commands SET created_at=? WHERE run_id=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), f.run.ID)
			case "unsupported-source":
				f.e.developmentSources = map[domain.Source]bool{}
				driver, collector = "headless", collection.BackendHeadless
				want = "development session source unsupported"
			case "collector":
				collector = collection.BackendQuiet
			case "frozen-driver":
				f.exec(t, `UPDATE sessions SET coverage_json=json_set(coverage_json,'$.collectionPolicy',?,'$.collectionSourceDrivers',json(?)) WHERE id=?`, domain.SessionCollectionPolicyHybridHeadlessV1, `{"x":"headless"}`, f.session.ID)
				want = "requested driver conflicts with the frozen hybrid source plan"
			case "reserved-source":
				// The ordinary route must not borrow the manual X reservation.
				f.exec(t, `UPDATE runs SET source='x' WHERE id=?`, f.run.ID)
				f.e.xManual.starting = true
			case "stale-owner":
				f.e.epoch = "replacement-epoch"
				want = errStaleCaptureRuntime.Error()
			}
			command, err := f.e.claimCommandForCollector(context.Background(), f.run.ID, "offline", driver, collector)
			if command != nil {
				t.Fatalf("guard dispatched command: %+v", command)
			}
			if want != "" {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("err=%v want=%q", err, want)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			status := "queued"
			if mode == "stale-owner" {
				status = "failed"
			}
			if got := f.commandStatus(t); got != status {
				t.Fatalf("status=%s want=%s", got, status)
			}
		})
	}
}

func TestEngineBridgeCommandsCharacterizationPollingRecovery(t *testing.T) {
	for _, status := range []string{"queued", "claimed"} {
		for _, poll := range []string{"claim-missing", "pending"} {
			t.Run(status+"/"+poll, func(t *testing.T) {
				f := newBridgeCommandFixture(t)
				if status == "claimed" {
					f.claim(t)
				}
				old := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
				f.exec(t, `UPDATE bridge_commands SET created_at=?,claimed_at=CASE WHEN status='claimed' THEN ? ELSE NULL END WHERE run_id=?`, old, old, f.run.ID)
				if poll == "claim-missing" {
					command, err := f.e.ClaimCommand(context.Background(), "missing-run", "offline")
					if command != nil || !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("missing claim=%+v %v", command, err)
					}
				} else {
					id, err := f.e.PendingBridgeRunID(context.Background())
					if err != nil || id != f.session.Runs[1].ID {
						t.Fatalf("pending=%s err=%v", id, err)
					}
				}
				run, err := f.state.GetRun(context.Background(), f.run.ID)
				code := "capture_dispatch_timeout"
				if status == "claimed" {
					code = "capture_lease_expired"
				}
				if err != nil || run.Status != "failed" || run.Error == nil || run.Error.Code != code || !run.Error.Retryable || run.Error.Stage != "capture" {
					t.Fatalf("recovery=%+v %v", run, err)
				}
				next, err := f.state.GetRun(context.Background(), f.session.Runs[1].ID)
				if err != nil || next.Status != "waiting_for_bridge" {
					t.Fatalf("next=%+v %v", next, err)
				}
			})
		}
	}
}

func TestEngineBridgeCommandsCharacterizationObservation(t *testing.T) {
	for _, mode := range []string{"owner-before-source", "source-before-validation", "invalid-before-write", "persisted-before-error"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t)
			command := f.claim(t)
			value := f.observation()
			want := ""
			switch mode {
			case "owner-before-source":
				f.e.epoch = "replacement-epoch"
				value = domain.Observation{}
				want = errStaleCaptureRuntime.Error()
			case "source-before-validation":
				value = domain.Observation{}
				want = "observation source does not match run"
			case "invalid-before-write":
				value.Snapshots = nil
				want = "observation has no snapshots"
			case "persisted-before-error":
				// Corrupt only the session projection after capture persists. GetRun,
				// owner validation and the observation transaction still succeed.
				f.exec(t, `CREATE TRIGGER command_observation_projection_failure AFTER INSERT ON observations BEGIN UPDATE sessions SET max_items_total=1.5 WHERE id=(SELECT session_id FROM runs WHERE id=NEW.run_id); END`)
				want = "converting driver.Value"
			}
			_, err := f.e.AcceptObservation(context.Background(), command.ID, command.RunID, value)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err=%v want=%q", err, want)
			}
			observations, err := f.state.Observations(context.Background(), f.run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "persisted-before-error" {
				if len(observations) != 0 || f.commandStatus(t) != "claimed" {
					t.Fatal("rejected evidence mutated durable command")
				}
				return
			}
			if len(observations) != 1 || observations[0].Snapshots[0].Blocks[0].EvidenceKey == "" || f.commandStatus(t) != "completed" {
				t.Fatalf("durable normalized evidence missing: %+v", observations)
			}
			if _, err := f.e.AcceptObservation(context.Background(), command.ID, command.RunID, value); err == nil {
				t.Fatal("completed command accepted replay")
			}
			observations, err = f.state.Observations(context.Background(), f.run.ID)
			if err != nil || len(observations) != 1 {
				t.Fatalf("replay changed evidence: %d %v", len(observations), err)
			}
			f.exec(t, `UPDATE sessions SET max_items_total=20 WHERE id=?`, f.session.ID)
		})
	}
}

func TestEngineBridgeCommandsCharacterizationFailure(t *testing.T) {
	for _, mode := range []string{"defaults", "explicit", "queued-rejected", "store-error"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t)
			id := f.commandID(t)
			if mode != "queued-rejected" {
				f.claim(t)
			}
			failure := domain.Failure{Message: "offline failure"}
			if mode == "explicit" {
				failure.Code = "custom_failure"
				failure.Stage = "custom_stage"
			}
			if mode == "store-error" {
				f.exec(t, `CREATE TRIGGER command_failure_abort BEFORE UPDATE OF status ON bridge_commands WHEN NEW.status='failed' BEGIN SELECT RAISE(ABORT,'fixture failure write rejected'); END`)
			}
			_, err := f.e.FailCommand(context.Background(), id, f.run.ID, failure)
			if mode == "queued-rejected" || mode == "store-error" {
				if err == nil {
					t.Fatal("rejected failure succeeded")
				}
				want := "bridge command is not active"
				status := "queued"
				if mode == "store-error" {
					want = "fixture failure write rejected"
					status = "claimed"
				}
				if !strings.Contains(err.Error(), want) || f.commandStatus(t) != status {
					t.Fatalf("failure=%v status=%s", err, f.commandStatus(t))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			run, err := f.state.GetRun(context.Background(), f.run.ID)
			code, stage := "bridge_failure", "capture"
			if mode == "explicit" {
				code, stage = failure.Code, failure.Stage
			}
			if err != nil || run.Error == nil || run.Error.Code != code || run.Error.Stage != stage {
				t.Fatalf("durable failure=%+v %v", run, err)
			}
			if _, err := f.e.FailCommand(context.Background(), id, f.run.ID, failure); err == nil {
				t.Fatal("terminal failure replay accepted")
			}
		})
	}
}

func TestEngineBridgeCommandsCharacterizationCancellationOrder(t *testing.T) {
	for _, mode := range []string{"x-challenge", "other-source", "ordinary-failure", "store-error"} {
		t.Run(mode, func(t *testing.T) {
			source := domain.SourceX
			if mode == "other-source" {
				source = domain.SourceLinkedIn
			}
			f := newBridgeCommandFixture(t, source)
			command := f.claim(t)
			calls, durable, locked := 0, false, false
			f.e.captureWork = map[string]context.CancelFunc{"offline": func() {
				calls++
				run, err := f.state.GetRun(context.Background(), f.run.ID)
				durable = err == nil && run.Status == "failed" && run.Error != nil && domain.IsAccessChallenge(run.Error.Code)
				locked = !f.e.captureBehaviorMu.TryLock()
				if !locked {
					f.e.captureBehaviorMu.Unlock()
				}
			}}
			if mode == "store-error" {
				f.exec(t, `CREATE TRIGGER command_challenge_abort BEFORE UPDATE OF status ON bridge_commands WHEN NEW.status='failed' BEGIN SELECT RAISE(ABORT,'fixture challenge write rejected'); END`)
			}
			code := "challenge_required"
			if mode == "ordinary-failure" {
				code = "offline_failure"
			}
			_, err := f.e.FailCommand(context.Background(), command.ID, command.RunID, domain.Failure{Code: code})
			delete(f.e.captureWork, "offline")
			if mode == "store-error" {
				if err == nil {
					t.Fatal("write error lost")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "x-challenge" {
				if calls != 1 || !durable || !locked {
					t.Fatalf("cancellation calls=%d durable=%t lockHeld=%t", calls, durable, locked)
				}
			} else if calls != 0 {
				t.Fatalf("unrelated failure cancelled work: %d", calls)
			}
		})
	}
}

func TestEngineBridgeCommandsCharacterizationUnlockBeforeScheduling(t *testing.T) {
	f := newBridgeCommandFixture(t)
	command := f.claim(t)
	f.e.schedule.Lock()
	locked := true
	defer func() {
		if locked {
			f.e.schedule.Unlock()
		}
	}()
	done := make(chan error, 1)
	go func() {
		_, err := f.e.FailCommand(context.Background(), command.ID, command.RunID, domain.Failure{})
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for f.commandStatus(t) != "failed" {
		if time.Now().After(deadline) {
			t.Fatal("failure did not persist before scheduling")
		}
		time.Sleep(time.Millisecond)
	}
	for !f.e.captureBehaviorMu.TryLock() {
		if time.Now().After(deadline) {
			t.Fatal("capture lock held while scheduler blocked")
		}
		time.Sleep(time.Millisecond)
	}
	f.e.captureBehaviorMu.Unlock()
	select {
	case err := <-done:
		t.Fatalf("failure skipped schedule lock: %v", err)
	default:
	}
	f.e.schedule.Unlock()
	locked = false
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("failure did not finish after schedule unlock")
	}
}

func TestEngineBridgeCommandsCharacterizationContext(t *testing.T) {
	for _, operation := range []string{"claim", "pending", "recovery", "observation", "failure", "terminal-claim-cleanup"} {
		t.Run(operation, func(t *testing.T) {
			f := newBridgeCommandFixture(t)
			manager := f.manager
			id := f.commandID(t)
			if operation == "observation" || operation == "failure" {
				id = f.claim(t).ID
			}
			if operation == "terminal-claim-cleanup" {
				if err := f.state.CancelSession(context.Background(), f.session.ID); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var err error
			switch operation {
			case "claim", "terminal-claim-cleanup":
				_, err = f.e.ClaimCommand(ctx, f.run.ID, "offline")
			case "pending":
				_, err = f.e.PendingBridgeRunID(ctx)
			case "recovery":
				err = f.e.recoverExpiredBridgeCommands(ctx, time.Now().Add(time.Hour))
			case "observation":
				_, err = f.e.AcceptObservation(ctx, id, f.run.ID, f.observation())
			case "failure":
				_, err = f.e.FailCommand(ctx, id, f.run.ID, domain.Failure{})
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("context error=%v", err)
			}
			if operation == "terminal-claim-cleanup" {
				waitCaptureLeases(t, manager, 0)
				return
			}
			want := "queued"
			if operation == "observation" || operation == "failure" {
				want = "claimed"
			}
			if got := f.commandStatus(t); got != want {
				t.Fatalf("cancelled request mutated command: %s", got)
			}
			waitCaptureLeases(t, manager, 1)
		})
	}
}
