//go:build auth_session_dev

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func capturedReasoningFixture(t *testing.T, source domain.Source, protected bool, peers ...domain.Source) (*Store, domain.Run, *ManualPilotPermit) {
	t.Helper()
	ctx := context.Background()
	s := openTestStore(t)
	settings, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = append([]domain.Source{source}, peers...)
	settings.CollectionMode = "headless"
	session, err := createVisibleUpdateSession(s, ctx, "offline reasoning failure", settings)
	if err != nil {
		t.Fatal(err)
	}
	command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	var permit *ManualPilotPermit
	if protected {
		now := s.Now()
		binding := DevelopmentManualPilotBinding{Digest: strings.Repeat("a", 64), Attempt: "synthetic:reasoning-failure", OwnerGeneration: 1, StartedAt: now, Deadline: now.Add(time.Minute)}
		permit, err = s.NewDevelopmentSourceDailyPilotIssuer(source, func(context.Context) (DevelopmentManualPilotBinding, error) { return binding, nil })(ctx, session.ID, command.RunID)
		if err != nil {
			t.Fatal(err)
		}
		claim, err := s.ClaimManualPilot(ctx, permit, command.ID, "reasoning-claim")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CommitManualPilotResult(ctx, claim.Handle, "reasoning-result", domain.Observation{Source: source}); err != nil {
			t.Fatal(err)
		}
	} else {
		// Ordinary malformed-evidence failure must retain generic terminalization.
		if _, err = s.db.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning' WHERE id=?`, command.RunID); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.SetRunPipelineStage(ctx, command.RunID, "candidate_evaluation"); err != nil {
		t.Fatal(err)
	}
	run, err := s.GetRun(ctx, command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return s, run, permit
}

func TestCapturedReasoningFailureClosedOrExpiredDailyPermit(t *testing.T) {
	ctx := context.Background()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		for _, closed := range []bool{false, true} {
			t.Run(string(source)+map[bool]string{false: "_expired", true: "_closed"}[closed], func(t *testing.T) {
				s, run, permit := capturedReasoningFixture(t, source, true)
				if closed {
					if _, err := s.RevokeManualPilot(ctx, permit, "reasoning-revoke"); err != nil {
						t.Fatal(err)
					}
				} else {
					s.clock = &manualTestClock{s.Now().Add(2 * time.Minute)}
				}
				failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning", Message: "synthetic provider unavailable", Retryable: true}
				if err := s.FailRun(ctx, run.ID, failure); err == nil {
					t.Fatal("generic fence removed")
				}
				if err := s.FailCapturedRunReasoning(ctx, run, failure); err != nil {
					t.Fatal(err)
				}
				first, err := s.GetRun(ctx, run.ID)
				if err != nil || first.Status != "failed" || first.Error == nil || first.Error.Code != failure.Code {
					t.Fatalf("failure not recorded: %+v %v", first, err)
				}
				if err := s.FailCapturedRunReasoning(ctx, run, failure); err != nil {
					t.Fatal("identical replay rejected", err)
				}
				second, _ := s.GetRun(ctx, run.ID)
				if *first.CompletedAt != *second.CompletedAt {
					t.Fatal("replay changed terminal time")
				}
				if err := s.FailRun(ctx, run.ID, failure); err == nil {
					t.Fatal("generic fence removed after failure")
				}
				var count int
				if err := s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE run_id=?`, run.ID).Scan(&count); err != nil || count != 1 {
					t.Fatal("accepted evidence changed", err)
				}
			})
		}
	}
}

func TestCapturedReasoningFailureRejectsUnprovenOrStaleTransitions(t *testing.T) {
	ctx := context.Background()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		for _, scenario := range []string{"no_observation", "no_receipt", "wrong_receipt_binding", "wrong_receipt_command", "wrong_fence_source", "wrong_fence_run", "wrong_fence_session", "wrong_source", "wrong_session", "wrong_stage", "completed", "cancelled", "active_capture", "wrong_failure_stage"} {
			t.Run(string(source)+"_"+scenario, func(t *testing.T) {
				s, run, _ := capturedReasoningFixture(t, source, true)
				failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning"}
				var query string
				var args []any
				switch scenario {
				case "no_observation":
					query = `DELETE FROM observations WHERE run_id=?`
					args = []any{run.ID}
				case "no_receipt":
					query = `DELETE FROM meta WHERE key=?`
					args = []any{manualPilotOpPrefix + "reasoning-result"}
				case "wrong_receipt_binding":
					query = `UPDATE meta SET value=json_set(value,'$.binding','foreign') WHERE key=?`
					args = []any{manualPilotOpPrefix + "reasoning-result"}
				case "wrong_receipt_command":
					query = `UPDATE meta SET value=json_set(value,'$.commandId','foreign') WHERE key=?`
					args = []any{manualPilotOpPrefix + "reasoning-result"}
				case "wrong_fence_source":
					query = `UPDATE meta SET value=json_set(value,'$.source','foreign') WHERE key=?`
					args = []any{manualPilotSessionPrefix + manualScopeID(run.SessionID, source)}
				case "wrong_fence_run":
					query = `UPDATE meta SET value=json_set(value,'$.runId','foreign') WHERE key=?`
					args = []any{manualPilotSessionPrefix + manualScopeID(run.SessionID, source)}
				case "wrong_fence_session":
					query = `UPDATE meta SET value=json_set(value,'$.sessionId','foreign') WHERE key=?`
					args = []any{manualPilotSessionPrefix + manualScopeID(run.SessionID, source)}
				case "wrong_source":
					run.Source = domain.SourceLinkedIn
				case "wrong_session":
					run.SessionID = "foreign"
				case "wrong_stage":
					query = `UPDATE runs SET stage='capture' WHERE id=?`
					args = []any{run.ID}
				case "completed", "cancelled":
					query = `UPDATE runs SET status=? WHERE id=?`
					args = []any{scenario, run.ID}
				case "active_capture":
					query = `UPDATE bridge_commands SET status='claimed' WHERE run_id=?`
					args = []any{run.ID}
				case "wrong_failure_stage":
					failure.Stage = "capture"
				}
				if query != "" {
					if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
						t.Fatal(err)
					}
				}
				before, err := s.GetRun(ctx, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.FailCapturedRunReasoning(ctx, run, failure); err == nil {
					t.Fatal("unsafe transition accepted")
				}
				after, err := s.GetRun(ctx, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if after.Status != before.Status || after.Stage != before.Stage || after.Error != nil {
					t.Fatal("rejection mutated run")
				}
			})
		}
	}
}

func TestCapturedReasoningFailurePreservesOrdinaryFailure(t *testing.T) {
	s, run, _ := capturedReasoningFixture(t, domain.SourceLinkedIn, false)
	failure := domain.Failure{Code: "codex_usage_limit", Stage: "reasoning"}
	if err := s.FailCapturedRunReasoning(context.Background(), run, failure); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetRun(context.Background(), run.ID)
	if err != nil || stored.Status != "failed" {
		t.Fatal("ordinary reasoning failure stranded", err)
	}
}

func TestQueuedAfterReasoningFailureStopsOnlyUnstartedPeers(t *testing.T) {
	ctx := context.Background()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		t.Run(string(source), func(t *testing.T) {
			peer := domain.SourceInstagram
			if source == domain.SourceInstagram {
				peer = domain.SourceX
			}
			s, run, permit := capturedReasoningFixture(t, source, true, peer, domain.SourceLinkedIn)
			if _, err := s.RevokeManualPilot(ctx, permit, "quota-revoke"); err != nil {
				t.Fatal(err)
			}
			failure := domain.Failure{Code: "codex_usage_limit", Stage: "reasoning", Message: "synthetic quota exhausted"}
			if err := s.FailCapturedRunReasoning(ctx, run, failure); err != nil {
				t.Fatal(err)
			}
			failed, err := s.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.FailQueuedRuns(ctx, run.SessionID, failure); err == nil {
				t.Fatal("generic batch fence removed")
			}
			if err := s.FailQueuedAfterReasoningFailure(ctx, failed, failure); err != nil {
				t.Fatal(err)
			}
			if err := s.FailQueuedAfterReasoningFailure(ctx, failed, failure); err != nil {
				t.Fatal("replay rejected", err)
			}
			session, err := s.GetSession(ctx, run.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range session.Runs {
				if r.Status != "failed" || r.Error == nil || r.Error.Code != "codex_usage_limit" {
					t.Fatal("queued peer not failed")
				}
			}
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&count); err != nil || count != 1 {
				t.Fatal("accepted evidence changed", err)
			}
		})
	}
}

func TestQueuedAfterReasoningFailureRejectsStaleOrTouchedWork(t *testing.T) {
	ctx := context.Background()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		for _, scenario := range []string{"wrong_source", "wrong_session", "wrong_failure", "no_receipt", "active_command", "observation", "started", "cancelled_session", "terminal_peers"} {
			t.Run(string(source)+"_"+scenario, func(t *testing.T) {
				peer := domain.SourceInstagram
				if source == domain.SourceInstagram {
					peer = domain.SourceX
				}
				s, run, _ := capturedReasoningFixture(t, source, true, peer, domain.SourceLinkedIn)
				failure := domain.Failure{Code: "codex_usage_limit", Stage: "reasoning"}
				if err := s.FailCapturedRunReasoning(ctx, run, failure); err != nil {
					t.Fatal(err)
				}
				failed, err := s.GetRun(ctx, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				session, err := s.GetSession(ctx, run.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				peerID := session.Runs[1].ID
				var query string
				var args []any
				switch scenario {
				case "wrong_source":
					failed.Source = domain.SourceLinkedIn
				case "wrong_session":
					failed.SessionID = "foreign"
				case "wrong_failure":
					failure.Message = "changed"
				case "no_receipt":
					query = `DELETE FROM meta WHERE key=?`
					args = []any{manualPilotOpPrefix + "reasoning-result"}
				case "active_command":
					if _, err = s.StartRun(ctx, peerID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}}); err != nil {
						t.Fatal(err)
					}
					query = `UPDATE runs SET status='queued',started_at=NULL WHERE id=?`
					args = []any{peerID}
				case "observation":
					command, createErr := s.StartRun(ctx, peerID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
					if createErr != nil {
						t.Fatal(createErr)
					}
					if _, err = s.db.ExecContext(ctx, `UPDATE bridge_commands SET status='completed' WHERE id=?`, command.ID); err != nil {
						t.Fatal(err)
					}
					if _, err = s.db.ExecContext(ctx, `UPDATE runs SET status='queued',started_at=NULL WHERE id=?`, peerID); err != nil {
						t.Fatal(err)
					}
					query = `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) SELECT 'synthetic-peer-observation',?,?,?,observation_json,captured_at,created_at FROM observations WHERE run_id=?`
					args = []any{peerID, command.ID, peer, run.ID}
				case "started":
					query = `UPDATE runs SET started_at=? WHERE id=?`
					args = []any{domain.Now(), peerID}
				case "cancelled_session":
					if err = s.CancelSession(ctx, run.SessionID); err != nil {
						t.Fatal(err)
					}
				case "terminal_peers":
					query = `UPDATE runs SET status=CASE WHEN id=? THEN 'completed' ELSE 'cancelled' END WHERE session_id=? AND status='queued'`
					args = []any{peerID, run.SessionID}
				}
				if query != "" {
					if _, err = s.db.ExecContext(ctx, query, args...); err != nil {
						t.Fatal(err)
					}
				}
				before, err := s.GetSession(ctx, run.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				err = s.FailQueuedAfterReasoningFailure(ctx, failed, failure)
				if scenario == "terminal_peers" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil {
					t.Fatal("unsafe queued transition accepted")
				}
				after, err := s.GetSession(ctx, run.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				for i := range before.Runs {
					if before.Runs[i].Status != after.Runs[i].Status {
						t.Fatal("existing state overwritten")
					}
				}
			})
		}
	}
}
