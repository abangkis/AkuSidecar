//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type manualRetryFixtureOwner struct {
	*syntheticDailyOwner
	calls int
	retry func(context.Context, int) error
}

func (o *manualRetryFixtureOwner) manualRetry(ctx context.Context, count int) error {
	o.calls++
	if o.retry != nil {
		return o.retry(ctx, count)
	}
	return nil
}

func manualRetryFixture(t *testing.T) (*Engine, *manualRetryFixtureOwner) {
	t.Helper()
	e, _ := dailyRuntimeFixture(t)
	e.config.Capture.InstagramMode = "concurrent"
	e.config.Capture.InstagramTrial = true
	owner := &manualRetryFixtureOwner{syntheticDailyOwner: &syntheticDailyOwner{e: e}}
	d := e.dailyController(domain.SourceInstagram)
	scope := strings.Repeat("a", 64)
	if err := e.store.SaveDailySourcePermission(context.Background(), domain.SourceInstagram, scope, true); err != nil {
		t.Fatal(err)
	}
	d.state.mu.Lock()
	d.state.attached, d.state.startUsed = true, true
	d.state.parent, d.state.owner = context.Background(), owner
	d.state.profileScope, d.state.rememberedPermission = scope, true
	d.state.reason, d.state.failurePhase = "batch-failed", "startup"
	d.state.firstFailure = sourceNativeFail(nil, "evidence", "document_changed")
	d.state.manualRetryEligible = true
	d.state.mu.Unlock()
	t.Cleanup(func() {
		d.state.mu.Lock()
		d.state.cleanupUncertain = false
		d.state.done = nil
		d.state.workers = 0
		d.state.manualRetryInProgress = false
		d.state.mu.Unlock()
	})
	return e, owner
}

func TestInstagramManualRetryRearmsOnlyExplicitlyAndPreservesX(t *testing.T) {
	e, owner := manualRetryFixture(t)
	ctx := context.Background()
	if status := e.SourceCaptureStatus(domain.SourceInstagram); !status.ManualRetryAvailable || status.DerivationAuthorized {
		t.Fatalf("manual retry not offered after verified failure: %+v", status)
	}
	if err := e.RetryInstagramTrial(ctx, XDailyStartRequest{true, true}, true); err != nil {
		t.Fatal(err)
	}
	if owner.calls != 1 || !e.dailySourceReady(domain.SourceInstagram) || !e.xDailyReady() {
		t.Fatal("retry failed to rearm IG or disturbed X")
	}
	if status := e.SourceCaptureStatus(domain.SourceInstagram); status.ManualRetryAvailable || status.State != "ready" {
		t.Fatalf("rearmed status invalid: %+v", status)
	}
	if active, err := e.ActiveSession(ctx); err != nil || active != nil {
		t.Fatal("manual retry unexpectedly started an update")
	}
	if err := e.RetryInstagramTrial(ctx, XDailyStartRequest{true, true}, true); err == nil || owner.calls != 1 {
		t.Fatal("manual retry was replayed")
	}
}

func TestInstagramManualRetryRejectsUnsafeOrUnconsentedStates(t *testing.T) {
	for _, mode := range []string{"cleanup", "pending", "worker", "wrong-cause", "opt-out", "limit", "no-permission", "no-consent", "maintenance", "parent-canceled"} {
		t.Run(mode, func(t *testing.T) {
			e, owner := manualRetryFixture(t)
			d := e.dailyController(domain.SourceInstagram)
			request, acknowledged := XDailyStartRequest{true, true}, true
			d.state.mu.Lock()
			switch mode {
			case "cleanup":
				d.state.cleanupUncertain = true
			case "pending":
				d.state.done = make(chan struct{})
			case "worker":
				d.state.workers = 1
			case "wrong-cause":
				d.state.firstFailure = sourceNativeFail(nil, "evidence", "account_mismatch")
			case "opt-out":
				d.state.reason = "opted-out"
			case "limit":
				d.state.manualRetryCount = dailyRetryLimit
			case "no-permission":
				d.state.rememberedPermission = false
			case "no-consent":
				acknowledged = false
			case "maintenance":
				d.state.closed = true
			case "parent-canceled":
				parent, cancel := context.WithCancel(context.Background())
				cancel()
				d.state.parent = parent
			}
			d.state.mu.Unlock()
			if err := e.RetryInstagramTrial(context.Background(), request, acknowledged); err == nil || owner.calls != 0 {
				t.Fatal("unsafe or unconsented retry reached owner")
			}
			if !e.xDailyReady() {
				t.Fatal("IG refusal disturbed X")
			}
		})
	}
}

func TestInstagramManualRetryRejectsRevokedStoredPermission(t *testing.T) {
	e, owner := manualRetryFixture(t)
	d := e.dailyController(domain.SourceInstagram)
	if err := e.store.SaveDailySourcePermission(context.Background(), domain.SourceInstagram, d.state.profileScope, false); err != nil {
		t.Fatal(err)
	}
	if err := e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true); err == nil || owner.calls != 0 {
		t.Fatal("stale in-memory permission authorized retry")
	}
	if d.status().ManualRetryAvailable {
		t.Fatal("failed authorization remained replayable")
	}
}

func TestInstagramManualRetryStopCancelsLateSuccess(t *testing.T) {
	e, owner := manualRetryFixture(t)
	started := make(chan struct{})
	owner.retry = func(ctx context.Context, _ int) error { close(started); <-ctx.Done(); return nil }
	retryDone := make(chan error, 1)
	go func() { retryDone <- e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not start")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := e.dailyController(domain.SourceInstagram).stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := <-retryDone; err == nil {
		t.Fatal("late successful response reauthorized stopped source")
	}
	if e.dailySourceReady(domain.SourceInstagram) || !e.xDailyReady() {
		t.Fatal("stop failed to fence IG or disturbed X")
	}
}

func TestInstagramManualRetryRevocationCancelsLateSuccess(t *testing.T) {
	e, owner := manualRetryFixture(t)
	started := make(chan struct{})
	owner.retry = func(ctx context.Context, _ int) error { close(started); <-ctx.Done(); return nil }
	retryDone := make(chan error, 1)
	go func() { retryDone <- e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not start")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := e.RevokeInstagramDailyPermission(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := <-retryDone; err == nil {
		t.Fatal("late reply restored revoked authority")
	}
	d := e.dailyController(domain.SourceInstagram)
	granted, err := e.store.DailySourcePermission(context.Background(), domain.SourceInstagram, d.state.profileScope)
	if err != nil || granted || d.ready() || !e.xDailyReady() {
		t.Fatal("revocation failed or disturbed X")
	}
}

func TestInstagramManualRetryFailureDoesNotRearm(t *testing.T) {
	e, owner := manualRetryFixture(t)
	owner.retry = func(context.Context, int) error { return errors.New("synthetic lost acknowledgement") }
	if err := e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true); err == nil {
		t.Fatal("unverified retry accepted")
	}
	status := e.SourceCaptureStatus(domain.SourceInstagram)
	if status.ManualRetryAvailable || status.DerivationAuthorized || !e.xDailyReady() {
		t.Fatalf("lost retry response reopened authority: %+v", status)
	}
}

func TestInstagramManualRetryNotDispatchedRestoresReservation(t *testing.T) {
	e, owner := manualRetryFixture(t)
	owner.retry = func(context.Context, int) error { return errInstagramRetryNotDispatched }
	if err := e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true); err == nil {
		t.Fatal("undispatched retry was reported as successful")
	}
	d := e.dailyController(domain.SourceInstagram)
	status := d.status()
	d.state.mu.RLock()
	count := d.state.manualRetryCount
	d.state.mu.RUnlock()
	if !status.ManualRetryAvailable || status.DerivationAuthorized || count != 0 || !e.xDailyReady() {
		t.Fatalf("verified no-dispatch did not restore reservation: count=%d status=%+v", count, status)
	}
	owner.retry = nil
	if err := e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true); err != nil || owner.calls != 2 {
		t.Fatalf("subsequent explicit retry failed: %v", err)
	}
}

func TestInstagramManualRetryNotDispatchedDoesNotUndoConcurrentStop(t *testing.T) {
	e, owner := manualRetryFixture(t)
	started := make(chan struct{})
	owner.retry = func(ctx context.Context, _ int) error {
		close(started)
		<-ctx.Done()
		return errInstagramRetryNotDispatched
	}
	done := make(chan error, 1)
	go func() { done <- e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := e.RevokeInstagramDailyPermission(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("canceled retry unexpectedly succeeded")
	}
	status := e.SourceCaptureStatus(domain.SourceInstagram)
	if status.ManualRetryAvailable || status.DerivationAuthorized || status.RememberedPermission || !e.xDailyReady() {
		t.Fatalf("no-dispatch restored revoked authority: %+v", status)
	}
}

func TestInstagramManualRetryCannotQueueBeforeRecoveryFinishes(t *testing.T) {
	e, _ := manualRetryFixture(t)
	d := e.dailyController(domain.SourceInstagram)
	d.state.mu.Lock()
	d.state.authorized = true
	d.state.reason = ""
	d.state.manualRetryInProgress = true
	d.state.mu.Unlock()
	if d.ready() {
		t.Fatal("IG became ready while recovery remained in progress")
	}
	if err := d.queue(domain.Run{ID: "synthetic-run", Source: domain.SourceInstagram}, nil); err == nil {
		t.Fatal("batch admitted before recovery finished")
	}
	if !e.xDailyReady() {
		t.Fatal("IG recovery blocked independent X readiness")
	}
}

type manualNativeFixtureTransport struct {
	*sourceNativeOffline
	calls   int
	fail    bool
	result  error
	onRetry func()
}

func (f *manualNativeFixtureTransport) retryDocument(_ context.Context, count int) error {
	f.calls++
	if f.onRetry != nil {
		f.onRetry()
	}
	if f.result != nil {
		return f.result
	}
	if f.fail || count != f.calls {
		return errors.New("synthetic retry rejected")
	}
	return nil
}

func TestNativeInstagramManualRetryDoesNotRestoreClosedOwner(t *testing.T) {
	o, base, run, _ := sourceNativeFixture(t)
	base.seedFailure = sourceNativeFail(nil, "evidence", "document_changed")
	transport := &manualNativeFixtureTransport{sourceNativeOffline: base, result: errInstagramRetryNotDispatched}
	o.transport = transport
	batch, err := o.BeginBatch(context.Background(), context.Background(), 1, run)
	if err == nil || batch == nil {
		t.Fatal("expected failed batch")
	}
	if err := batch.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.onRetry = func() {
		o.mu.Lock()
		o.closed = true
		o.mu.Unlock()
	}
	if err := o.manualRetry(context.Background(), 1); err == nil || errors.Is(err, errInstagramRetryNotDispatched) {
		t.Fatal("closed owner propagated the rollback signal")
	}
	if batch.(dailyManualRetryBatch).manualRetryReady() || o.manualRetryCount != 1 {
		t.Fatal("closed owner recovered retry authority")
	}
}

func TestNativeInstagramManualRetryNotDispatchedRestoresOwner(t *testing.T) {
	o, base, run, _ := sourceNativeFixture(t)
	base.seedFailure = sourceNativeFail(nil, "evidence", "document_changed")
	transport := &manualNativeFixtureTransport{sourceNativeOffline: base, result: errInstagramRetryNotDispatched}
	o.transport = transport
	batch, err := o.BeginBatch(context.Background(), context.Background(), 1, run)
	if err == nil || batch == nil {
		t.Fatal("expected failed batch")
	}
	if err := batch.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := o.manualRetry(context.Background(), 1); !errors.Is(err, errInstagramRetryNotDispatched) {
		t.Fatalf("wrong no-dispatch result: %v", err)
	}
	if !batch.(dailyManualRetryBatch).manualRetryReady() || o.manualRetryCount != 0 {
		t.Fatal("owner reservation was not restored")
	}
	transport.result = nil
	transport.calls = 0
	if err := o.manualRetry(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
}

func TestNativeInstagramManualRetryRequiresVerifiedBatchAndFreshGeneration(t *testing.T) {
	for _, mode := range []string{"document", "account", "cleanup", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			o, base, run, _ := sourceNativeFixture(t)
			transport := &manualNativeFixtureTransport{sourceNativeOffline: base}
			o.transport = transport
			base.seedFailure = sourceNativeFail(nil, "evidence", "document_changed")
			if mode == "account" {
				base.seedFailure = sourceNativeFail(nil, "evidence", "account_mismatch")
			}
			if mode == "cleanup" {
				base.cleanup = true
			}
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			batch, err := o.BeginBatch(context.Background(), parent, 1, run)
			if err == nil || batch == nil {
				t.Fatal("expected seed failure with cleanup obligation")
			}
			first := batch.(*sourceNativeBatch).challenge
			if mode == "canceled" {
				cancel()
			}
			closeErr := batch.Close(context.Background())
			if (closeErr != nil) != (mode == "cleanup") {
				t.Fatalf("unexpected cleanup result: %v", closeErr)
			}
			ready := batch.(dailyManualRetryBatch).manualRetryReady()
			if ready != (mode == "document") {
				t.Fatalf("incorrect retry readiness for %s", mode)
			}
			if mode != "document" {
				if err := o.manualRetry(context.Background(), 1); err == nil || transport.calls != 0 {
					t.Fatal("unsafe retry reached transport")
				}
				return
			}
			if err := o.manualRetry(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if err := o.manualRetry(context.Background(), 1); err == nil || transport.calls != 1 {
				t.Fatal("retry replay admitted")
			}
			base.seedFailure = nil
			if err := o.engine.store.CancelSession(context.Background(), run.SessionID); err != nil {
				t.Fatal(err)
			}
			settings, err := o.engine.store.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings.ActiveSources = []domain.Source{domain.SourceInstagram}
			settings.CollectionMode = "headless"
			settings.CalibrationEnabled = false
			settings.AIDetectionEnabled = false
			session, err := o.engine.store.CreateUpdateSession(context.Background(), "fresh retry", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := o.engine.store.StartRun(context.Background(), session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}}); err != nil {
				t.Fatal(err)
			}
			second, err := o.BeginBatch(context.Background(), parent, 2, session.Runs[0])
			if err != nil {
				for cause := err; cause != nil; cause = errors.Unwrap(cause) {
					t.Logf("%T: %v", cause, cause)
				}
				t.Fatal(err)
			}
			fresh := second.(*sourceNativeBatch).challenge
			if fresh.Nonce == first.Nonce || fresh.Generation != 2 || fresh.ProfileScope != first.ProfileScope {
				t.Fatal("retry did not create a fresh bound challenge")
			}
			if err := second.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInstagramManualRetryRunsNewBatchAfterVerifiedFailureAndPreservesX(t *testing.T) {
	e, _ := dailyRuntimeFixture(t)
	e.config.Capture.InstagramMode = "concurrent"
	e.config.Capture.InstagramTrial = true
	base := &sourceNativeOffline{engine: e, seedFailure: sourceNativeFail(nil, "evidence", "document_changed")}
	transport := &manualNativeFixtureTransport{sourceNativeOffline: base}
	scope := strings.Repeat("d", 64)
	owner := &sourceNativeOwner{engine: e, source: domain.SourceInstagram, recipe: instagramNativeRecipe, profile: scope, transport: transport}
	d := e.dailyController(domain.SourceInstagram)
	d.state.mu.Lock()
	d.state.attached = true
	d.state.mu.Unlock()
	if err := d.configure(context.Background(), func(context.Context, context.Context) (sourceDailyOwner, error) { return owner, nil }, scope); err != nil {
		t.Fatal(err)
	}
	if err := e.StartInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.closeInstagramDaily(context.Background()); err != nil {
			t.Error(err)
		}
	})
	settings, err := e.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceInstagram, domain.SourceX}
	if err := e.store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	first, err := e.StartVisibleUpdate(context.Background(), "offline failed IG then X")
	if err != nil {
		t.Fatal(err)
	}
	awaitIsolationBatch(t, e, first.ID)
	first, err = e.store.GetSession(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range first.Runs {
		if run.Source == domain.SourceInstagram {
			obs, err := e.store.Observations(context.Background(), run.ID)
			if err != nil || run.Status != "failed" || len(obs) != 0 {
				t.Fatal("failed IG proof escaped")
			}
		} else if run.Source == domain.SourceX && run.Status != "completed" {
			t.Fatal("failed IG blocked X")
		}
	}
	if status := d.status(); !status.ManualRetryAvailable || status.Cleanup != "verified" {
		t.Fatalf("retry unavailable after verified failure: %+v", status)
	}
	base.seedFailure = nil
	if err := e.RetryInstagramTrial(context.Background(), XDailyStartRequest{true, true}, true); err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Fatal("manual retry did not reach private transport exactly once")
	}
	second, err := e.StartVisibleUpdate(context.Background(), "offline fresh IG retry")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("old session replayed")
	}
	awaitIsolationBatch(t, e, second.ID)
	second, err = e.store.GetSession(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range second.Runs {
		if run.Status != "completed" {
			t.Fatalf("fresh %s retry did not complete: %s", run.Source, run.Status)
		}
		if run.Source == domain.SourceInstagram {
			obs, err := e.store.Observations(context.Background(), run.ID)
			if err != nil || len(obs) != 1 {
				t.Fatal("fresh IG observation was not accepted")
			}
		}
	}
	if d.status().State != "ready" || !e.xDailyReady() {
		t.Fatal("success did not preserve both owners")
	}
}
