//go:build auth_session_dev

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// A source-local identity failure must not revoke another source or prevent
// its queued successor from running once cleanup has been verified.
func TestDailySourceFailureDoesNotDisableOtherSource(t *testing.T) {
	for _, failed := range []domain.Source{domain.SourceInstagram, domain.SourceX} {
		for _, order := range [][]domain.Source{{domain.SourceInstagram, domain.SourceX}, {domain.SourceX, domain.SourceInstagram}} {
			t.Run(string(failed)+"_failed_"+string(order[0])+"_first", func(t *testing.T) {
				e, x := dailyRuntimeFixture(t)
				ig := attachInstagramOffline(t, e)
				ctx := context.Background()
				if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
					t.Fatal(err)
				}
				if failed == domain.SourceInstagram {
					ig.mutate = func(v *instagramSyntheticEvidence, collector bool) {
						if collector {
							v.Authenticated = false
						}
					}
				} else {
					x.begin = func(context.Context, context.Context, uint64, domain.Run) (xDailyNativeBatch, error) {
						return nil, xDailyStartupFail(nil, "reader_bind", "ownership")
					}
				}
				settings, err := e.store.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.ActiveSources = order
				settings.AutoUpdateEnabled = true
				if err := e.store.SaveSettings(ctx, settings); err != nil {
					t.Fatal(err)
				}
				for attempt := 0; attempt < 2; attempt++ {
					session, err := e.StartVisibleUpdate(ctx, "synthetic source failure isolation")
					if err != nil {
						t.Fatal(err)
					}
					awaitIsolationBatch(t, e, session.ID)
					session, err = e.store.GetSession(ctx, session.ID)
					if err != nil {
						t.Fatal(err)
					}
					if len(session.Runs) != 2-attempt {
						t.Fatalf("run count=%d at attempt=%d", len(session.Runs), attempt)
					}
					healthyRuns := 0
					for _, run := range session.Runs {
						observations, err := e.store.Observations(ctx, run.ID)
						if err != nil {
							t.Fatal(err)
						}
						if run.Source == failed {
							if run.Status != "failed" || len(observations) != 0 {
								t.Fatalf("failed source accepted work: status=%s observations=%d", run.Status, len(observations))
							}
						} else {
							healthyRuns++
							if run.Status != "completed" || len(observations) != 1 || observations[0].Source != run.Source {
								t.Fatalf("healthy source blocked: source=%s status=%s observations=%d", run.Source, run.Status, len(observations))
							}
						}
					}
					if healthyRuns != 1 {
						t.Fatalf("healthy source run count=%d", healthyRuns)
					}
					if failed == domain.SourceInstagram {
						ig.mu.Lock()
						derivations, closes, captures := ig.derivations, ig.closes, ig.captures
						ig.mu.Unlock()
						if derivations != 1 || closes != 1 || captures != 0 {
							t.Fatalf("failed IG collector lifetime: derivations=%d closes=%d captures=%d", derivations, closes, captures)
						}
					}
					for _, source := range dailySources {
						status := e.dailyController(source).status()
						if status.DerivationAuthorized != (source != failed) || !status.RememberedPermission {
							t.Fatalf("source failure crossed permission boundary: source=%s status=%+v", source, status)
						}
					}
					current, err := e.store.GetSettings(ctx)
					if err != nil || !current.AutoUpdateEnabled {
						t.Fatal("source failure changed Auto Update")
					}
				}
			})
		}
	}
}

func awaitIsolationBatch(t *testing.T, e *Engine, sessionID string) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		session, err := e.store.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		terminal := session.Status == "completed" || session.Status == "partial" || session.Status == "failed"
		drained := true
		for _, source := range dailySources {
			d := e.dailyController(source)
			d.state.mu.RLock()
			drained = drained && d.state.done == nil && d.state.workers == 0
			d.state.mu.RUnlock()
		}
		if terminal && drained {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("mixed failure session did not finish and drain")
}
