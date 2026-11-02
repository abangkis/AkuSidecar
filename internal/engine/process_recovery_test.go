package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

type recoveryFixture struct {
	e       *Engine
	state   *store.Store
	db      *sql.DB
	session domain.Session
	log     *provenanceTestLog
}

func newRecoveryFixture(t *testing.T) recoveryFixture {
	t.Helper()
	e, state := testEngine(t)
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn, domain.SourceFacebook}
	settings.AIDetectionEnabled = false
	if err := state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateUpdateSession(context.Background(), "offline recovery characterization", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil || len(session.Runs) != 2 {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	log := &provenanceTestLog{}
	e.logger = log
	f := recoveryFixture{e, state, db, session, log}
	f.exec(t, `UPDATE sessions SET status='running' WHERE id=?`, session.ID)
	f.exec(t, `UPDATE runs SET status='reasoning',stage='candidate_evaluation' WHERE id=?`, session.Runs[0].ID)
	// testEngine records a heartbeat, which may already have notified the scheduler.
	f.wake()
	return f
}

func (f recoveryFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f recoveryFixture) wake() bool {
	select {
	case <-f.e.scheduler.wake:
		return true
	default:
		return false
	}
}

func TestEngineProcessRecoveryCharacterizationFormatting(t *testing.T) {
	for _, test := range []struct {
		name, value, want string
		limit             int
	}{
		{"trim", " \tmessage\n", "message", 500},
		{"boundary", strings.Repeat("a", 500), strings.Repeat("a", 500), 500},
		{"truncate", strings.Repeat("a", 501), strings.Repeat("a", 500), 500},
		{"zero", "message", "", 0},
		{"bytes", "éé", "é\xc3", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := boundedText(test.value, test.limit); got != test.want {
				t.Fatalf("got=%q want=%q", got, test.want)
			}
			if test.name == "bytes" && utf8.ValidString(boundedText(test.value, test.limit)) {
				t.Fatal("byte truncation changed")
			}
		})
	}
	if got := boundedFailureMessage(errors.New("  " + strings.Repeat("b", 501) + "  ")); got != strings.Repeat("b", 500) {
		t.Fatalf("failure message=%q", got)
	}
	failure := reasoning.ProviderFailure{Code: "provider", Stage: "provider", Retry: "transient", Category: "capacity", Reason: "busy", ProviderResponseStatus: "incomplete", PartialOutputSeen: true, DiagnosticCodes: []string{"safe"}, ProviderStatus: 503, RequestID: "request", Operation: "turn", RPCCode: 5, ProcessExitCode: 23}
	details := providerFailureDetails(failure)
	want := map[string]any{"sdkCode": "provider", "sdkStage": "provider", "sdkRetry": "transient", "sdkCategory": "capacity", "sdkReason": "busy", "sdkProviderResponseStatus": "incomplete", "sdkPartialOutputSeen": true, "sdkDiagnosticCodes": []string{"safe"}, "sdkProviderStatus": 503, "sdkRequestID": "request", "sdkOperation": "turn", "sdkRPCCode": 5, "sdkProcessExitCode": 23}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("details=%+v", details)
	}
	failure.DiagnosticCodes[0] = "mutated"
	if details["sdkDiagnosticCodes"].([]string)[0] != "safe" {
		t.Fatal("diagnostic codes alias input")
	}
	if got := providerFailureDetails(reasoning.ProviderFailure{}); !reflect.DeepEqual(got, map[string]any{"sdkCode": "", "sdkStage": "", "sdkRetry": ""}) {
		t.Fatalf("empty projection=%+v", got)
	}
}

func TestEngineProcessRecoveryCharacterizationFailure(t *testing.T) {
	for _, mode := range []string{"plain", "typed-transient", "typed-never", "quota", "generic-quota", "primary-error", "pause-error", "readback-error", "peer-error", "advance-error", "missing", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecoveryFixture(t)
			ctx := context.Background()
			runID, peerID := f.session.Runs[0].ID, f.session.Runs[1].ID
			processErr := error(errors.New("You've hit your usage limit. Try again later."))
			quota := true
			if mode == "plain" || strings.HasPrefix(mode, "typed-") {
				quota = false
				processErr = errors.New("  offline process failed  ")
				f.exec(t, `UPDATE runs SET status='cancelled' WHERE id=?`, peerID)
				if mode != "plain" {
					retry := inference.RetryTransient
					if mode == "typed-never" {
						retry = inference.RetryNever
					}
					processErr = fmt.Errorf("outer: %w", &inference.Error{Code: inference.FailureCodeProvider, Stage: inference.FailureStageProvider, Retry: retry, Message: "safe provider failure", Cause: errors.New("private-cause")})
				}
			}
			if mode == "generic-quota" {
				f.exec(t, `UPDATE runs SET status='waiting_for_bridge',stage='capture' WHERE id=?`, runID)
			}
			// SQL fences observe ordering on the disposable database, without production hooks.
			f.exec(t, fmt.Sprintf(`CREATE TRIGGER recovery_primary_order BEFORE UPDATE OF status ON runs WHEN OLD.id='%s' AND NEW.status='failed' AND EXISTS(SELECT 1 FROM meta WHERE key='auto_update_usage_limit_pause') BEGIN SELECT RAISE(ABORT,'pause preceded primary failure'); END`, runID))
			f.exec(t, fmt.Sprintf(`CREATE TRIGGER recovery_pause_order BEFORE INSERT ON meta WHEN NEW.key='auto_update_usage_limit_pause' AND NOT EXISTS(SELECT 1 FROM runs WHERE id='%s' AND status='failed') BEGIN SELECT RAISE(ABORT,'pause preceded durable failure'); END`, runID))
			if mode != "pause-error" {
				f.exec(t, fmt.Sprintf(`CREATE TRIGGER recovery_peer_order BEFORE UPDATE OF status ON runs WHEN OLD.id='%s' AND NEW.status='failed' AND NOT EXISTS(SELECT 1 FROM meta WHERE key='auto_update_usage_limit_pause') BEGIN SELECT RAISE(ABORT,'peer failure preceded pause'); END`, peerID))
			}
			switch mode {
			case "primary-error":
				f.exec(t, fmt.Sprintf(`CREATE TRIGGER recovery_primary_abort BEFORE UPDATE OF status ON runs WHEN OLD.id='%s' AND NEW.status='failed' BEGIN SELECT RAISE(ABORT,'stop-primary'); END`, runID))
			case "pause-error":
				f.exec(t, `CREATE TRIGGER recovery_pause_abort BEFORE INSERT ON meta WHEN NEW.key='auto_update_usage_limit_pause' BEGIN SELECT RAISE(ABORT,'stop-pause'); END`)
			case "readback-error":
				f.exec(t, fmt.Sprintf(`CREATE TRIGGER recovery_readback_abort AFTER INSERT ON meta WHEN NEW.key='auto_update_usage_limit_pause' BEGIN UPDATE runs SET ordinal='bad-ordinal' WHERE id='%s'; END`, runID))
			case "peer-error":
				f.exec(t, fmt.Sprintf(`CREATE TRIGGER recovery_peer_abort BEFORE UPDATE OF status ON runs WHEN OLD.id='%s' AND NEW.status='failed' BEGIN SELECT RAISE(ABORT,'stop-peer'); END`, peerID))
			case "advance-error":
				f.exec(t, `CREATE TRIGGER recovery_advance_abort BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT,'stop-advance'); END`)
			case "missing":
				runID = "missing-run"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := f.e.failRunAfterProcessError(ctx, runID, processErr)
			wantError := map[string]string{"primary-error": "stop-primary", "readback-error": "bad-ordinal", "peer-error": "stop-peer", "advance-error": "stop-advance"}[mode]
			if mode == "missing" {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("missing error=%v", err)
				}
			} else if mode == "cancelled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled error=%v", err)
				}
			} else if wantError != "" {
				if err == nil || !strings.Contains(err.Error(), wantError) {
					t.Fatalf("err=%v want=%s", err, wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			var status, peer, session string
			var failureRaw sql.NullString
			if err := f.db.QueryRow(`SELECT status,error_json FROM runs WHERE id=?`, f.session.Runs[0].ID).Scan(&status, &failureRaw); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(`SELECT status FROM runs WHERE id=?`, peerID).Scan(&peer); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(`SELECT status FROM sessions WHERE id=?`, f.session.ID).Scan(&session); err != nil {
				t.Fatal(err)
			}
			early := mode == "primary-error" || mode == "missing" || mode == "cancelled"
			wantStatus := "failed"
			if early {
				wantStatus = "reasoning"
			}
			if status != wantStatus {
				t.Fatalf("status=%s want=%s", status, wantStatus)
			}
			pause, pauseErr := f.state.AutoUpdateUsageLimitPause(context.Background())
			if pauseErr != nil {
				t.Fatal(pauseErr)
			}
			wantPause := quota && !early && mode != "pause-error"
			if (pause != nil) != wantPause || f.wake() != wantPause {
				t.Fatalf("pause=%+v want=%t", pause, wantPause)
			}
			wantPeer := "failed"
			if !quota {
				wantPeer = "cancelled"
			}
			if early || mode == "readback-error" || mode == "peer-error" {
				wantPeer = "queued"
			}
			if peer != wantPeer {
				t.Fatalf("peer=%s want=%s", peer, wantPeer)
			}
			if early || mode == "readback-error" || mode == "peer-error" || mode == "advance-error" {
				if session != "running" {
					t.Fatalf("advanced despite error: %s", session)
				}
			} else if session != "failed" {
				t.Fatalf("session=%s", session)
			}
			if mode == "pause-error" && !strings.Contains(f.log.String(), "stop-pause") {
				t.Fatal("best-effort pause failure not logged")
			}
			if !early && mode != "readback-error" {
				run, err := f.state.GetRun(context.Background(), f.session.Runs[0].ID)
				if err != nil || run.Error == nil {
					t.Fatalf("run=%+v err=%v", run, err)
				}
				wantCode := "reasoning_failed"
				if quota {
					wantCode = "codex_usage_limit"
				}
				wantRetry := !quota && mode != "typed-never"
				if run.Error.Code != wantCode || run.Error.Stage != "reasoning" || run.Error.Retryable != wantRetry {
					t.Fatalf("failure=%+v", run.Error)
				}
				if strings.Contains(failureRaw.String, "private-cause") {
					t.Fatal("raw provider cause persisted")
				}
				if mode == "plain" && run.Error.Message != "offline process failed" {
					t.Fatalf("message=%q", run.Error.Message)
				}
				if strings.HasPrefix(mode, "typed-") && !strings.HasPrefix(run.Error.Message, "safe provider failure (outer: ") {
					t.Fatalf("typed message=%q", run.Error.Message)
				}
			}
		})
	}
}

func TestEngineProcessRecoveryCharacterizationPause(t *testing.T) {
	for _, mode := range []string{"unrelated", "capacity", "cancelled-error", "cancelled-context", "store-error", "success", "duplicate", "full-wake"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecoveryFixture(t)
			ctx := context.Background()
			processErr := error(errors.New("  You've hit your usage limit. " + strings.Repeat("q", 600) + "  "))
			switch mode {
			case "unrelated":
				processErr = errors.New("offline failure")
			case "capacity":
				processErr = &inference.Error{Code: inference.FailureCodeProvider, Message: "You've hit your usage limit"}
			case "cancelled-error":
				processErr = context.Canceled
			case "cancelled-context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "store-error":
				f.exec(t, `CREATE TRIGGER recovery_pause_abort BEFORE INSERT ON meta WHEN NEW.key='auto_update_usage_limit_pause' BEGIN SELECT RAISE(ABORT,'stop-pause'); END`)
			case "duplicate":
				if err := f.state.PauseAutoUpdateForUsageLimit(ctx, domain.AutoUpdateUsageLimitPause{Message: "original", SessionID: "first", PausedAt: "2026-01-01T00:00:00Z"}); err != nil {
					t.Fatal(err)
				}
			case "full-wake":
				f.e.scheduler.notify()
			}
			want := mode == "success" || mode == "duplicate" || mode == "full-wake"
			if got := f.e.pauseAutoUpdateForUsageLimit(ctx, "current", processErr); got != want {
				t.Fatalf("paused=%t want=%t", got, want)
			}
			pause, err := f.state.AutoUpdateUsageLimitPause(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if (pause != nil) != want || f.wake() != want || f.wake() {
				t.Fatalf("pause/wake contract: %+v", pause)
			}
			if mode == "duplicate" {
				if pause.Message != "original" || pause.SessionID != "first" || pause.PausedAt != "2026-01-01T00:00:00Z" {
					t.Fatalf("pause overwritten: %+v", pause)
				}
			} else if want {
				if len(pause.Message) != 500 || pause.Message != strings.TrimSpace(processErr.Error())[:500] || pause.SessionID != "current" {
					t.Fatalf("pause=%+v", pause)
				}
			}
			if (mode == "store-error" || mode == "cancelled-context") && !strings.Contains(f.log.String(), "pause Auto Update") {
				t.Fatal("persistence failure not logged")
			}
		})
	}
}

func TestEngineProcessRecoveryCharacterizationRestore(t *testing.T) {
	for _, mode := range []string{"success", "other-provider", "status", "trigger", "delivery", "authority", "no-pause", "empty-usage", "failed-usage", "local-only", "read-error", "usage-error", "clear-error", "cancelled-context"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecoveryFixture(t)
			ctx := context.Background()
			session := f.session
			session.Status = "completed"
			if mode != "no-pause" {
				if err := f.state.PauseAutoUpdateForUsageLimit(ctx, domain.AutoUpdateUsageLimitPause{Message: "keep-until-proof", SessionID: "earlier"}); err != nil {
					t.Fatal(err)
				}
			}
			model, provider := "fixture-model", "codex-app-server"
			if mode == "other-provider" {
				provider = "another-provider"
			}
			if mode == "local-only" {
				model = "none"
			}
			if mode != "empty-usage" {
				if err := f.state.SaveTelemetry(ctx, domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: session.Runs[0].ID, Phase: "candidate_evaluation", Provider: provider, Model: model, Status: "completed", CreatedAt: domain.Now()}); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "status":
				session.Status = "running"
			case "trigger":
				session.Trigger = domain.UpdateTriggerScheduler
			case "delivery":
				session.Delivery = domain.UpdateDeliveryPrepared
			case "authority":
				session.BudgetAuthority = domain.BudgetAuthorityAutomatic
			case "failed-usage":
				if err := f.state.SaveTelemetry(ctx, domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: session.Runs[0].ID, Phase: "candidate_evaluation", Provider: provider, Model: model, Status: "failed", CreatedAt: domain.Now()}); err != nil {
					t.Fatal(err)
				}
			case "read-error":
				f.exec(t, `UPDATE meta SET value='invalid-json' WHERE key='auto_update_usage_limit_pause'`)
			case "usage-error":
				session.ID = "missing-session"
			case "clear-error":
				f.exec(t, `CREATE TRIGGER recovery_clear_abort BEFORE DELETE ON meta WHEN OLD.key='auto_update_usage_limit_pause' BEGIN SELECT RAISE(ABORT,'stop-clear'); END`)
			case "cancelled-context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			// Finalization already holds this lock; the helper must not acquire it again.
			f.e.schedule.Lock()
			f.e.clearAutoUpdateUsageLimitAfterSuccessfulUserSession(ctx, session)
			f.e.schedule.Unlock()
			wantClear := mode == "success" || mode == "other-provider"
			var count int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM meta WHERE key='auto_update_usage_limit_pause'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if wantClear || mode == "no-pause" {
				wantCount = 0
			}
			if count != wantCount || f.wake() != wantClear {
				t.Fatalf("pause count=%d want=%d log=%s", count, wantCount, f.log.String())
			}
			if mode == "read-error" || mode == "usage-error" || mode == "clear-error" || mode == "cancelled-context" {
				if f.log.String() == "" {
					t.Fatal("Store failure not logged")
				}
			}
		})
	}
}

func TestEngineProcessRecoveryCharacterizationModelProof(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries []domain.ModelUsageEntry
		want    bool
	}{
		{"empty", nil, false},
		{"success", []domain.ModelUsageEntry{{Status: "completed", Provider: " p ", Model: " m "}}, true},
		{"missing-provider", []domain.ModelUsageEntry{{Status: "completed", Model: "m"}}, false},
		{"missing-model", []domain.ModelUsageEntry{{Status: "completed", Provider: "p", Model: " "}}, false},
		{"none", []domain.ModelUsageEntry{{Status: "completed", Provider: "p", Model: " NoNe "}}, false},
		{"running-only", []domain.ModelUsageEntry{{Status: "running", Provider: "p", Model: "m"}}, false},
		{"cancelled-plus-success", []domain.ModelUsageEntry{{Status: "cancelled"}, {Status: "completed", Provider: "p", Model: "m"}}, true},
		{"failed-after-success", []domain.ModelUsageEntry{{Status: "completed", Provider: "p", Model: "m"}, {Status: "failed"}}, false},
		{"failed-before-success", []domain.ModelUsageEntry{{Status: "failed"}, {Status: "completed", Provider: "p", Model: "m"}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := domain.ModelUsageReport{Categories: []domain.ModelUsageCategory{{Entries: test.entries}}}
			if got := modelUsageProvesCodexRestored(report); got != test.want {
				t.Fatalf("proof=%t want=%t", got, test.want)
			}
		})
	}
	report := domain.ModelUsageReport{Categories: []domain.ModelUsageCategory{{Entries: []domain.ModelUsageEntry{{Status: "completed", Provider: "p", Model: "m"}}}, {Entries: []domain.ModelUsageEntry{{Status: "failed"}}}}}
	if modelUsageProvesCodexRestored(report) {
		t.Fatal("failed category ignored")
	}
}
