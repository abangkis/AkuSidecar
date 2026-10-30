//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

type sourceAnalysisFailure struct {
	reasoning.Deterministic
	mu            sync.Mutex
	source        domain.Source
	failed        bool
	beforeFailure func(domain.Run)
	failure       error
}

func (p *sourceAnalysisFailure) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.mu.Lock()
	fail := run.Source == p.source && !p.failed
	if fail {
		if p.beforeFailure != nil {
			p.beforeFailure(run)
		}
		p.failed = true
	}
	p.mu.Unlock()
	if fail {
		err := p.failure
		if err == nil {
			err = errors.New("synthetic provider unavailable")
		}
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: "candidate_evaluation", Provider: "synthetic", Model: "fixture", Status: "failed", CreatedAt: domain.Now()}, err
	}
	return p.Deterministic.Analyze(ctx, run, observation, knowledge)
}

func TestDailyReasoningQuotaFailureTerminatesQueuedPeers(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		t.Run(string(source), func(t *testing.T) {
			e, _ := dailyRuntimeFixture(t)
			attachInstagramOffline(t, e)
			e.provider = &sourceAnalysisFailure{source: source, failure: errors.New("You've hit your usage limit. Try again later.")}
			ctx := context.Background()
			if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
				t.Fatal(err)
			}
			settings, err := e.store.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			peer := domain.SourceInstagram
			if source == peer {
				peer = domain.SourceX
			}
			settings.ActiveSources = []domain.Source{source, peer}
			if err := e.store.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			session, err := e.StartVisibleUpdate(ctx, "synthetic quota failure")
			if err != nil {
				t.Fatal(err)
			}
			awaitIsolationBatch(t, e, session.ID)
			if !e.WaitForIdle(time.Second) {
				t.Fatal("quota failure did not drain")
			}
			session, err = e.store.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, run := range session.Runs {
				if run.Status != "failed" || run.Error == nil || run.Error.Code != "codex_usage_limit" {
					t.Fatalf("quota failure was not terminal: %+v", run)
				}
				observations, err := e.store.Observations(ctx, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if run.Source == peer && len(observations) != 0 {
					t.Fatal("queued peer captured after quota exhaustion")
				}
			}
		})
	}
}

// Reproduce the real failure after native capture committed and collector
// cleanup revoked its permit. Reasoning failure must still become terminal.
func TestDailyReasoningFailureTerminatesAndAllowsNextBatch(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		for _, order := range [][]domain.Source{{domain.SourceX, domain.SourceInstagram}, {domain.SourceInstagram, domain.SourceX}} {
			t.Run(string(source)+"_failed_"+string(order[0])+"_first", func(t *testing.T) {
				e, _ := dailyRuntimeFixture(t)
				attachInstagramOffline(t, e)
				released := make(chan struct{})
				e.provider = &sourceAnalysisFailure{source: source, beforeFailure: func(run domain.Run) {
					// Model the shared capture window's session intent. It must be
					// released after terminal state and worker drain, not earlier.
					e.captureMu.Lock()
					defer e.captureMu.Unlock()
					if e.collectionIntents == nil {
						e.collectionIntents = map[string]func(){}
					}
					e.collectionIntents[run.SessionID] = func() { close(released) }
				}}
				ctx := context.Background()
				if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
					t.Fatal(err)
				}
				settings, err := e.store.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.ActiveSources, settings.AutoUpdateEnabled = order, true
				if err := e.store.SaveSettings(ctx, settings); err != nil {
					t.Fatal(err)
				}
				for attempt := 0; attempt < 2; attempt++ {
					session, err := e.StartVisibleUpdate(ctx, "synthetic post-capture failure")
					if err != nil {
						t.Fatal(err)
					}
					awaitIsolationBatch(t, e, session.ID)
					if !e.WaitForIdle(time.Second) {
						t.Fatal("terminal session retained a reasoning worker")
					}
					select {
					case <-released:
					default:
						t.Fatal("terminal session retained capture window intent")
					}
					session, err = e.store.GetSession(ctx, session.ID)
					if err != nil {
						t.Fatal(err)
					}
					if len(session.Runs) != 2 {
						t.Fatalf("lost source admission: %d runs", len(session.Runs))
					}
					for _, run := range session.Runs {
						want := "completed"
						if attempt == 0 && run.Source == source {
							want = "failed"
						}
						if run.Status != want {
							t.Fatalf("source=%s status=%s want=%s", run.Source, run.Status, want)
						}
						if want == "failed" && (run.Error == nil || run.Error.Code != "reasoning_failed" || run.Error.Message != "synthetic provider unavailable") {
							t.Fatalf("first failure lost: %+v", run.Error)
						}
						observations, err := e.store.Observations(ctx, run.ID)
						if err != nil || len(observations) != 1 {
							t.Fatalf("accepted evidence lost: %d %v", len(observations), err)
						}
					}
					for _, s := range dailySources {
						status := e.dailyController(s).status()
						if !status.DerivationAuthorized || !status.RememberedPermission || status.Cleanup != "verified" {
							t.Fatalf("reasoning failure disabled source %s: %+v", s, status)
						}
					}
					current, err := e.store.GetSettings(ctx)
					if err != nil || !current.AutoUpdateEnabled {
						t.Fatal("Auto Update changed")
					}
				}
			})
		}
	}
}
