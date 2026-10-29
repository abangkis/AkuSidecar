//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestDailyRetryPolicyAllowsOnlyVerifiedOperationalTransients(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		cause                      error
		phase                      string
		cleanupFailed, ready, want bool
	}{
		{"timeout", context.DeadlineExceeded, "capture", false, true, true},
		{"network", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("private network detail")}, "capture", false, true, true},
		{"IG abort timeout", sourceNativeFail(nil, "evidence", "reader_identity_fetch_timeout"), "startup", false, true, true},
		{"IG ambiguous fetch", sourceNativeFail(nil, "evidence", "reader_identity_fetch_failed"), "startup", false, true, false},
		{"IG HTTP denial", sourceNativeFail(nil, "evidence", "collector_identity_http_forbidden"), "startup", false, true, false},
		{"IG server ambiguity", sourceNativeFail(nil, "evidence", "reader_identity_http_server"), "startup", false, true, false},
		{"account mismatch", sourceNativeFail(context.DeadlineExceeded, "evidence", "account_mismatch"), "startup", false, true, false},
		{"ownership", sourceNativeFail(context.DeadlineExceeded, "ownership", "changed"), "capture", false, true, false},
		{"challenge", errors.New("challenge_required"), "capture", false, true, false},
		{"challenge with timeout", errors.Join(&headless.CaptureError{Code: "challenge_required"}, context.DeadlineExceeded), "capture", false, true, false},
		{"ownership with timeout", xDailyStartupFail(context.DeadlineExceeded, "reader_bind", "ownership"), "startup", false, true, false},
		{"diagnostic-looking text", errors.New("network_failed timeout"), "capture", false, true, false},
		{"unknown", errors.New("unknown failure"), "startup", false, true, false},
		{"cleanup uncertain", context.DeadlineExceeded, "capture", true, true, false},
		{"provider not recovered", context.DeadlineExceeded, "capture", false, false, false},
		{"reader check", context.DeadlineExceeded, "reader-check", false, true, false},
		{"canceled", context.Canceled, "capture", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := testEngine(t)
			d := e.dailyController(domain.SourceX)
			d.state.authorized = true
			d.state.parent = context.Background()
			d.finishRetryPolicy(tc.cause, tc.phase, tc.cleanupFailed, tc.ready)
			if d.state.authorized != tc.want || (d.state.retryCount == 1) != tc.want || (!d.state.retryAt.IsZero()) != tc.want {
				t.Fatalf("retry decision authorized=%t count=%d reason=%s", d.state.authorized, d.state.retryCount, d.state.reason)
			}
		})
	}
}

func TestDailyRetryPolicyExhaustionAndSuccessReset(t *testing.T) {
	e, _ := testEngine(t)
	d := e.dailyController(domain.SourceX)
	d.state.authorized = true
	d.state.parent = context.Background()
	for attempt := 1; attempt <= 3; attempt++ {
		before := e.store.Now()
		d.finishRetryPolicy(context.DeadlineExceeded, "capture", false, true)
		if attempt <= 2 {
			if d.state.retryCount != attempt || d.state.retryAt.Before(before.Add(time.Duration(attempt)*30*time.Second)) || !d.state.authorized {
				t.Fatal("cooldown or bounded allowance lost")
			}
		} else if d.state.authorized || d.state.reason != "batch-failed" {
			t.Fatal("retry limit bypassed")
		}
	}
	d.state.authorized = true
	d.state.reason = ""
	d.finishRetryPolicy(nil, "capture", false, true)
	if d.state.retryCount != 0 || !d.state.retryAt.IsZero() {
		t.Fatal("success did not reset consecutive failure budget")
	}
}

type retryFixtureBatch struct {
	xDailyNativeBatch
	failure error
	closed  bool
}

func awaitRetryRunRetired(t *testing.T, e *Engine, session string) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		v, err := e.store.GetSession(context.Background(), session)
		e.xDaily.mu.RLock()
		retired := e.xDaily.done == nil && e.xDaily.workers == 0
		e.xDaily.mu.RUnlock()
		if err == nil && (v.Status == "completed" || v.Status == "failed") && retired {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("retry attempt did not retire")
}

func (b *retryFixtureBatch) Capture(ctx context.Context, command string) (domain.Run, error) {
	if b.failure != nil {
		return domain.Run{}, b.failure
	}
	return b.xDailyNativeBatch.Capture(ctx, command)
}
func (b *retryFixtureBatch) Close(ctx context.Context) error {
	err := b.xDailyNativeBatch.Close(ctx)
	b.closed = err == nil
	return err
}
func (b *retryFixtureBatch) dailyRetryReady() bool { return b.closed }

func TestDailyRetryUsesNewRunsAndGenerationsThenClearsFailure(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	plain := &syntheticDailyOwner{e: e}
	attempt := 0
	o.begin = func(ctx, parent context.Context, generation uint64, run domain.Run) (xDailyNativeBatch, error) {
		attempt++
		base, err := plain.BeginBatch(ctx, parent, generation, run)
		if err != nil {
			return base, err
		}
		b := &retryFixtureBatch{xDailyNativeBatch: base}
		if attempt <= 2 {
			b.failure = &dailyTransientFailure{class: "network"}
		}
		return b, nil
	}
	ids := map[string]bool{}
	for i := 1; i <= 3; i++ {
		if i > 1 {
			e.xDaily.mu.Lock()
			e.xDaily.retryAt = e.store.Now().Add(-time.Second)
			e.xDaily.mu.Unlock()
		}
		session, err := e.StartVisibleUpdate(context.Background(), "offline limited retry")
		if err != nil {
			t.Fatal(err)
		}
		if ids[session.ID] {
			t.Fatal("old session replayed")
		}
		ids[session.ID] = true
		awaitRetryRunRetired(t, e, session.ID)
		status := e.XCaptureStatus()
		if i <= 2 {
			if status.State != "retry-wait" || status.Cleanup != "verified" || status.RetryCount != i || !status.DerivationAuthorized || status.FailureCode != "daily_transient_network" {
				t.Fatalf("retry status %+v", status)
			}
		} else if status.State != "ready" || status.RetryCount != 0 || status.FailureCode != "" {
			t.Fatalf("successful retry status %+v", status)
		}
	}
	if len(plain.generations) != 3 || plain.generations[2] != 3 || plain.collectorCloses != 3 {
		t.Fatal("collector lifetime or fresh generation missing")
	}
}

func TestDailyRetryStopDuringCooldownDoesNotStartCollector(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	plain := &syntheticDailyOwner{e: e}
	begins := 0
	o.begin = func(ctx, parent context.Context, generation uint64, run domain.Run) (xDailyNativeBatch, error) {
		begins++
		base, err := plain.BeginBatch(ctx, parent, generation, run)
		return &retryFixtureBatch{xDailyNativeBatch: base, failure: context.DeadlineExceeded}, err
	}
	session, err := e.StartVisibleUpdate(context.Background(), "offline first timeout")
	if err != nil {
		t.Fatal(err)
	}
	awaitRetryRunRetired(t, e, session.ID)
	if _, err = e.StartVisibleUpdate(context.Background(), "offline queued retry"); err != nil {
		t.Fatal(err)
	}
	if err = e.RevokeXDailyPermission(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !e.WaitForIdle(time.Second) {
		t.Fatal("cooldown worker did not stop")
	}
	if begins != 1 || e.XCaptureStatus().RememberedPermission || e.xDailyReady() {
		t.Fatal("Stop allowed derivation or restored permission")
	}
}

type retryOperationalWire struct{ code string }

func (e retryOperationalWire) Error() string            { return e.code }
func (e retryOperationalWire) dailyFailureCode() string { return e.code }

func TestDailyRetryXOperationalCodeIsIndependentOfDiagnosticSideband(t *testing.T) {
	for _, class := range []string{"timeout", "network"} {
		code := "manual_capture_transient_" + class
		got := nativePilotCaptureTransportError(context.Background(), retryOperationalWire{code})
		if dailyTransientClass(got) != class {
			t.Fatal("trusted operational code lost")
		}
		fake := nativePilotCaptureTransportError(context.Background(), errors.New(code))
		if dailyTransientClass(fake) != "" {
			t.Fatal("arbitrary error text granted retry")
		}
	}
}
