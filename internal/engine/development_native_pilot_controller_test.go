//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testNativeManualController(t *testing.T, f *testNativePilotFixture, capture nativePilotCapture) *nativePilotManualController {
	t.Helper()
	testNativeStartupSource(t, f, capture)
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c, d := newNativePilotManualController(parent, nativePilotManualConfig{enabled: true, offlineTest: true, pilot: f.cfg}, f.ports)
	assertNativePilotClosed(t, d, "admission-closed")
	if c == nil {
		t.Fatal("offline manual controller missing")
	}
	return c
}
func startNativeManualController(t *testing.T, c *nativePilotManualController) {
	t.Helper()
	d := c.start(context.Background())
	assertNativePilotClosed(t, d, "manual-attempt-valid")
	if !d.OfflineValidated {
		t.Fatal("offline startup did not validate")
	}
}
func dispatchNativeManual(t *testing.T, c *nativePilotManualController, work string) *nativePilotManualTicket {
	t.Helper()
	ticket, d := c.dispatch(context.Background(), work)
	assertNativePilotClosed(t, d, "work-dispatched")
	if ticket == nil {
		t.Fatal("ticket missing")
	}
	return ticket
}

func TestNativePilotManualControllerDefaultOffAndOfflineBoundary(t *testing.T) {
	f := newTestNativePilotFixture()
	c, d := newNativePilotManualController(nil, nativePilotManualConfig{}, f.ports)
	assertNativePilotClosed(t, d, "admission-disabled")
	if c != nil || f.calls != 0 {
		t.Fatal("disabled controller had effects")
	}
	testNativeStartupSource(t, f, nil)
	for _, cfg := range []nativePilotManualConfig{{enabled: true, pilot: f.cfg}, {enabled: true, offlineTest: true, pilot: f.cfg}} {
		parent := context.Background()
		if cfg.offlineTest {
			parent = nil
		}
		c, d = newNativePilotManualController(parent, cfg, f.ports)
		assertNativePilotClosed(t, d, "invalid-configuration")
		if c != nil {
			t.Fatal("offline source admitted without explicit offline mode or parent")
		}
	}
}

func TestNativePilotManualControllerOfflineFlowAndCollectorPreserved(t *testing.T) {
	f := newTestNativePilotFixture()
	var captures atomic.Int32
	c := testNativeManualController(t, f, func(_ context.Context, req nativePilotCaptureChallenge) ([]byte, error) {
		captures.Add(1)
		return marshalNativeStartupWire(req)
	})
	if ticket, d := c.dispatch(context.Background(), "early"); ticket != nil || d.Reason != "admission-closed" {
		t.Fatal("work admitted before startup")
	}
	startNativeManualController(t, c)
	ticket := dispatchNativeManual(t, c, "capture-1")
	collector := ticket.collector
	claims, results := 0, 0
	assertNativePilotClosed(t, c.claim(context.Background(), ticket, func() bool { claims++; return true }), "work-claimed")
	// Non-final inventory changes preserve collector and work. Sample freshness
	// expires, while the explicitly manual attempt remains valid without probing.
	f.mu.Lock()
	f.state.readerWindows++
	f.mu.Unlock()
	f.advance(12 * time.Second)
	assertNativePilotClosed(t, c.check(context.Background()), "manual-attempt-valid")
	assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { results++; return true }), "result-completed")
	assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { results++; return true }), "result-duplicate")
	if claims != 1 || results != 1 || captures.Load() != 2 || ticket.collector != collector || c.vector.binding.collector != collector || ticket.version != 1 {
		t.Fatal("collector/work changed or source renewed")
	}
	assertNativePilotClosed(t, c.start(context.Background()), "startup-already-attempted")
	for _, value := range []any{c, ticket, nativePilotManualConfig{pilot: f.cfg}} {
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private controller serialized")
		}
		if strings.Contains(fmt.Sprintf("%#v", value), f.cfg.binding.account) || strings.Contains(fmt.Sprintf("%+v", value), collector.document) {
			t.Fatal("private controller formatting leaked")
		}
	}
}

func TestNativePilotManualControllerCancelAndTicketFences(t *testing.T) {
	f := newTestNativePilotFixture()
	c := testNativeManualController(t, f, nil)
	startNativeManualController(t, c)
	assertNativePilotClosed(t, c.cancel("never-issued"), "work-canceled")
	if ticket, d := c.dispatch(context.Background(), "never-issued"); ticket != nil || d.Reason != "duplicate-work" {
		t.Fatal("canceled command revived")
	}
	ticket := dispatchNativeManual(t, c, "capture-1")
	forged := *ticket
	assertNativePilotClosed(t, c.claim(context.Background(), &forged, func() bool { t.Fatal("forged ticket callback"); return true }), "stale-ticket")
	assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { t.Fatal("unclaimed result callback"); return true }), "work-not-claimed")
	assertNativePilotClosed(t, c.claim(context.Background(), ticket, func() bool { return false }), "local-effect-rejected")
	assertNativePilotClosed(t, c.claim(context.Background(), ticket, func() bool { return true }), "work-claimed")
	assertNativePilotClosed(t, c.claim(context.Background(), ticket, func() bool { t.Fatal("duplicate claim callback"); return true }), "work-already-claimed")
	assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { return false }), "local-effect-rejected")
	assertNativePilotClosed(t, c.cancel(ticket.work), "work-canceled")
	assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { t.Fatal("canceled result callback"); return true }), "work-canceled")
	if another, d := c.dispatch(context.Background(), ticket.work); another != nil || d.Reason != "duplicate-work" {
		t.Fatal("canceled work redispatched")
	}
	for i := 0; i < 30; i++ {
		c.cancel(fmt.Sprintf("bounded-%d", i))
	}
	assertNativePilotClosed(t, c.cancel("capacity-overflow"), "work-capacity")
}

func TestNativePilotManualControllerInvalidationFencesResults(t *testing.T) {
	for _, event := range []string{"opted-out", "reader-lost", "parent-exited", "owner-or-policy-changed", "evidence-channel-lost", "candidate-cookie-write", "identity-mismatch", "authentication-rejected", "cleanup-uncertain"} {
		t.Run(event, func(t *testing.T) {
			f := newTestNativePilotFixture()
			c := testNativeManualController(t, f, nil)
			startNativeManualController(t, c)
			ticket := dispatchNativeManual(t, c, "capture-1")
			assertNativePilotClosed(t, c.claim(context.Background(), ticket, func() bool { return true }), "work-claimed")
			assertNativePilotClosed(t, c.invalidate(event), event)
			assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { t.Fatal("revoked result committed"); return true }), event)
			if c.vector != nil {
				t.Fatal("revoked vector retained")
			}
		})
	}
	for _, drift := range []string{"expiry", "owner", "consent", "last-window", "parent", "canceled"} {
		t.Run(drift, func(t *testing.T) {
			f := newTestNativePilotFixture()
			c := testNativeManualController(t, f, nil)
			startNativeManualController(t, c)
			ticket := dispatchNativeManual(t, c, "capture-1")
			ctx := context.Background()
			want := "owner-or-policy-changed"
			switch drift {
			case "expiry":
				f.advance(nativePilotLife)
				want = "attempt-expired"
			case "owner":
				f.mu.Lock()
				f.state.binding.collector.generation++
				f.mu.Unlock()
			case "consent":
				f.mu.Lock()
				f.state.binding.consent++
				f.mu.Unlock()
			case "last-window":
				f.mu.Lock()
				f.state.readerWindows = 0
				f.mu.Unlock()
				want = "reader-lost"
			case "parent":
				parent, cancel := context.WithCancel(context.Background())
				c.parent = parent
				cancel()
				want = "parent-exited"
			case "canceled":
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx = canceled
				want = "canceled"
			}
			assertNativePilotClosed(t, c.claim(ctx, ticket, func() bool { t.Fatal("drifted claim committed"); return true }), want)
			if c.vector != nil {
				t.Fatal("invalid vector retained")
			}
		})
	}
}

func TestNativePilotManualControllerAtomicResultAndRevocation(t *testing.T) {
	f := newTestNativePilotFixture()
	c := testNativeManualController(t, f, nil)
	startNativeManualController(t, c)
	ticket := dispatchNativeManual(t, c, "capture-1")
	c.claim(context.Background(), ticket, func() bool { return true })
	entered, release := make(chan struct{}), make(chan struct{})
	resultDone, revokeDone := make(chan nativePilotDiagnostic, 1), make(chan nativePilotDiagnostic, 1)
	go func() {
		resultDone <- c.complete(context.Background(), ticket, func() bool { close(entered); <-release; return true })
	}()
	<-entered
	go func() { revokeDone <- c.invalidate("opted-out") }()
	select {
	case <-revokeDone:
		t.Fatal("revocation interleaved inside local atomic effect")
	default:
	}
	close(release)
	assertNativePilotClosed(t, <-resultDone, "result-completed")
	assertNativePilotClosed(t, <-revokeDone, "opted-out")
	assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { t.Fatal("post-revocation callback"); return true }), "opted-out")
}

func TestNativePilotManualControllerStartupRevocationRace(t *testing.T) {
	f := newTestNativePilotFixture()
	entered := make(chan struct{}, 2)
	c := testNativeManualController(t, f, func(ctx context.Context, _ nativePilotCaptureChallenge) ([]byte, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	done := make(chan nativePilotDiagnostic, 1)
	go func() { done <- c.start(context.Background()) }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("startup pair did not enter")
		}
	}
	assertNativePilotClosed(t, c.invalidate("opted-out"), "opted-out")
	select {
	case d := <-done:
		assertNativePilotClosed(t, d, "opted-out")
	case <-time.After(time.Second):
		t.Fatal("revocation did not cancel startup")
	}
	if c.vector != nil {
		t.Fatal("late startup revived vector")
	}
}
