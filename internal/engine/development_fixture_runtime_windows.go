//go:build windows && auth_session_fixture

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"sync"
	"sync/atomic"
)

// DevelopmentFixtureRuntime exposes only the scoped synthetic composition. It
// receives no user configuration and supports no arbitrary browser URL or CDP RPC.
type DevelopmentFixtureRuntime struct {
	NativeHost     *readerbroker.FixtureNativeHostReceipt
	Engine         *Engine
	State          *store.Store
	Owner          *captureruntime.Manager
	Coordinator    *collection.Coordinator
	Root           string
	backend        *processFixtureBackend
	bridge         *chromeFixtureBridge
	reasoning      *developmentFixtureReasoningGate
	scanner        *bufio.Scanner
	encoder        *json.Encoder
	authority      func(context.Context) (SyntheticSessionEvidence, error)
	pumpCancel     context.CancelFunc
	captureRelease sync.Once
}

type DevelopmentFixtureRect struct{ Left, Top, Right, Bottom int32 }

type developmentFixtureReasoningGate struct {
	reasoning.Deterministic
	entered, release         chan struct{}
	enteredOnce, releaseOnce sync.Once
}

func (p *developmentFixtureReasoningGate) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.enteredOnce.Do(func() { close(p.entered) })
	select {
	case <-p.release:
		return p.Deterministic.Analyze(ctx, run, observation, knowledge)
	case <-ctx.Done():
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, ctx.Err()
	}
}

func (r *DevelopmentFixtureRuntime) Emit(value any) error { return r.encoder.Encode(value) }
func (r *DevelopmentFixtureRuntime) ReadControl() (string, error) {
	if !r.scanner.Scan() {
		return "", errors.New("development controller closed")
	}
	return r.scanner.Text(), nil
}
func (r *DevelopmentFixtureRuntime) StartBatch(ctx context.Context) (domain.Session, error) {
	session, err := r.Engine.StartVisibleUpdate(ctx, "actual dev main UI/native broker batch")
	if err != nil {
		return session, err
	}
	pumpCtx, cancel := context.WithCancel(ctx)
	r.pumpCancel = cancel
	r.Engine.StartHeadlessCollection(pumpCtx)
	select {
	case <-r.backend.entered:
		return session, nil
	case <-ctx.Done():
		return session, ctx.Err()
	}
}
func (r *DevelopmentFixtureRuntime) ReleaseCapture() {
	r.captureRelease.Do(func() { close(r.backend.release) })
}
func (r *DevelopmentFixtureRuntime) WaitReasoning(ctx context.Context) error {
	select {
	case <-r.reasoning.entered:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (r *DevelopmentFixtureRuntime) ReleaseReasoning() {
	r.reasoning.releaseOnce.Do(func() { close(r.reasoning.release) })
}
func (r *DevelopmentFixtureRuntime) StopCollection() {
	if r.pumpCancel != nil {
		r.pumpCancel()
	}
}
func (r *DevelopmentFixtureRuntime) CaptureCount() int64 { return r.backend.calls.Load() }
func (r *DevelopmentFixtureRuntime) CurrentPID() int {
	p := r.backend.current.Load()
	if p == nil {
		return 0
	}
	return p.PID()
}

func (r *DevelopmentFixtureRuntime) PrepareNativePost(ctx context.Context, id, url, marker string) (readerbroker.NativePostPreparation, error) {
	if url != "https://x.com/aku_fixture_only/status/1" || marker != "/split-reader-intent?id="+id {
		return readerbroker.NativePostPreparation{}, errors.New("synthetic post mapping denied")
	}
	owner := r.backend.current.Load()
	if owner == nil || owner.PID() != r.Owner.PID() || r.Owner.Snapshot().Driver != "browser" || !owner.nativeReaderStartup {
		return readerbroker.NativePostPreparation{}, errors.New("synthetic reader owner unavailable")
	}
	if r.Engine.developmentVerifyBrowser == nil {
		return readerbroker.NativePostPreparation{}, errors.New("synthetic reader verifier unavailable")
	}
	if err := r.Engine.developmentVerifyBrowser(ctx); err != nil {
		return readerbroker.NativePostPreparation{}, err
	}
	generation, epoch := r.Owner.Snapshot().Generation, r.Engine.Epoch()
	title := "AkuBrowser reader split_fixture_" + readerbroker.Secret()[:32]
	if err := r.bridge.rpc(ctx, "reader_marker", map[string]any{"owner": owner.owner, "marker": title}, nil); err != nil {
		return readerbroker.NativePostPreparation{}, err
	}
	binding, err := appshell.NewReaderBrokerFixtureBinding(r.bridge.job, owner.pid)
	if err != nil {
		return readerbroker.NativePostPreparation{}, err
	}
	target, verify, err := binding.Prepare(ctx, title)
	if err != nil {
		return readerbroker.NativePostPreparation{}, err
	}
	var navigated atomic.Bool
	return readerbroker.NativePostPreparation{Target: target, VerifyForeground: verify, NavigatePost: func(c context.Context) error {
		if !navigated.CompareAndSwap(false, true) || r.backend.current.Load() != owner || owner.PID() != r.Owner.PID() || r.Owner.Snapshot().Generation != generation || r.Engine.Epoch() != epoch {
			return errors.New("synthetic post completion stale or duplicated")
		}
		if err := r.Engine.developmentVerifyBrowser(c); err != nil {
			return err
		}
		var result struct {
			LocalPost bool `json:"localPost"`
		}
		if err := r.bridge.rpc(c, "reader_post", map[string]any{"owner": owner.owner}, &result); err != nil {
			return err
		}
		if !result.LocalPost || r.backend.current.Load() != owner || owner.PID() != r.Owner.PID() || r.Owner.Snapshot().Generation != generation || r.Engine.Epoch() != epoch {
			return errors.New("synthetic local post readback failed")
		}
		return r.Engine.developmentVerifyBrowser(c)
	}}, nil
}
func (r *DevelopmentFixtureRuntime) ReaderWindows(ctx context.Context) ([]bool, error) {
	p := r.backend.current.Load()
	if p == nil {
		return nil, errors.New("reader missing")
	}
	return p.FixtureBootstrapWindows(ctx)
}
func (r *DevelopmentFixtureRuntime) OpenIndependentWindow(ctx context.Context) error {
	return r.backend.current.Load().OpenFixtureIndependentWindow(ctx)
}
func (r *DevelopmentFixtureRuntime) CloseWindow(ctx context.Context, index int) error {
	return r.backend.current.Load().CloseFixtureWindow(ctx, index)
}
func (r *DevelopmentFixtureRuntime) OwnerReleased(pid int) bool {
	p := r.backend.current.Load()
	return p != nil && p.PID() != pid && r.Coordinator.Status().Effective == "headless" && !r.Coordinator.Status().BatchPaused
}
