//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type xQAClock struct{ fixture *manualRuntimeFixture }

func (c xQAClock) Now() time.Time {
	c.fixture.mu.Lock()
	defer c.fixture.mu.Unlock()
	return c.fixture.now
}

type xQASyntheticCapture struct{ *manualRuntimeFixtureProcess }

func (c xQASyntheticCapture) Capture(ctx context.Context, source domain.Source, payload map[string]any) (domain.Observation, error) {
	v, err := c.manualRuntimeFixtureProcess.Capture(ctx, source, payload)
	if err == nil {
		v.Coverage = map[string]any{"synthetic": true}
		v.Snapshots = []domain.Snapshot{{Blocks: []domain.Block{{Text: "Synthetic isolated QA post"}}}}
	}
	return v, err
}

type xQASyntheticAdmission struct {
	broker *nativePilotBroker
	store  *store.Store
}

func (xQASyntheticAdmission) syntheticStartupAllowed() bool { return true }
func (a xQASyntheticAdmission) validate(ctx context.Context) (store.DevelopmentManualPilotBinding, error) {
	var out store.DevelopmentManualPilotBinding
	if !a.broker.check(ctx).OfflineValidated {
		return out, errors.New("offline QA startup unavailable")
	}
	b := a.broker
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.controller
	if b.stopped || c == nil || !b.cfg.offlineTest {
		return out, errors.New("offline QA broker unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.vector
	if c.stopped || v == nil || !c.offlineTest || v.provenance != nativePilotOfflineOnly || v.seal != c.adapter.seal || v.binding != c.adapter.cfg.binding {
		return out, errors.New("offline QA vector rejected")
	}
	return store.DevelopmentManualPilotBinding{Digest: nativePilotStoreDigest(v.binding), Attempt: v.binding.attempt, OwnerGeneration: v.binding.collector.generation, StartedAt: b.cfg.pilot.attemptStarted, Deadline: b.deadline()}, nil
}
func (a xQASyntheticAdmission) issue(ctx context.Context, session, run string) (*store.ManualPilotPermit, error) {
	return a.store.NewDevelopmentManualPilotIssuer(a.validate)(ctx, session, run)
}

func init() {
	newXQASyntheticKit = func() (*xQASyntheticKit, error) {
		f, err := newManualRuntimeFixture(false)
		if err != nil {
			return nil, err
		}
		var release sync.Once
		kit := &xQASyntheticKit{clock: xQAClock{f}, close: f.close, foreground: func(ctx context.Context, _ readerbroker.Target) error { return ctx.Err() }}
		kit.factory = func(e *Engine) xManualApplicationFactory {
			return func(ctx, parent context.Context) (xManualApplicationPorts, error) {
				prepare := func(call context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
					if call.Err() != nil || req.Source != "x" || !xApplicationPostURL.MatchString(req.URL) || !xApplicationMarkerPath.MatchString(marker) {
						return readerbroker.NativePostPreparation{}, errors.New("synthetic post rejected")
					}
					f.mu.Lock()
					f.state.readerWindows++
					f.mu.Unlock()
					var verify, navigate sync.Once
					return readerbroker.NativePostPreparation{Target: readerbroker.Target{HWND: 42, PID: 4101, Expires: time.Now().Add(readerbroker.Lifetime)}, VerifyForeground: func(active context.Context) error {
						if active.Err() != nil {
							return active.Err()
						}
						verify.Do(func() {})
						return nil
					}, NavigatePost: func(active context.Context) error {
						if active.Err() != nil {
							return active.Err()
						}
						navigate.Do(func() { f.mu.Lock(); f.opens++; f.mu.Unlock(); release.Do(func() { close(f.backend.release) }) })
						return nil
					}}, nil
				}
				return xManualApplicationPorts{broker: f.broker, admission: xQASyntheticAdmission{f.broker, e.store}, prepare: prepare, dispose: func(context.Context) error { f.close(); return nil }, buildRuntime: func(call context.Context) (*nativePilotRuntime, error) {
					return newNativePilotRuntime(f.broker, f.reader, f.collector, xQASyntheticCapture{f.backend}, func(context.Context, readerbroker.Request) error { return nil })
				}}, nil
			}
		}
		return kit, nil
	}
}
