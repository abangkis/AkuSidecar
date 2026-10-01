package appshell

import (
	"context"
	"errors"
	"testing"
	"time"
)

type replacementSnapshotGuard struct {
	CaptureContainment
	err error
}

func (g replacementSnapshotGuard) ReplacementReadiness(context.Context) error { return g.err }

func TestLiveCaptureSnapshotDoesNotAuthorizeShutdown(t *testing.T) {
	startup, err := NewStartup("http://127.0.0.1:11122/")
	if err != nil {
		t.Fatal(err)
	}
	window := &Window{captureHost: true, closed: make(chan struct{}), startup: startup,
		containment: replacementSnapshotGuard{}}
	if err := window.ReplacementReadiness(context.Background()); !errors.Is(err, ErrCaptureHandoffUnsafe) {
		t.Fatalf("host-only snapshot authorized shutdown: %v", err)
	}
	if err := window.CloseForRetry(context.Background()); !errors.Is(err, ErrCaptureHandoffUnsafe) {
		t.Fatalf("live capture shutdown admitted: %v", err)
	}
	select {
	case <-startup.stopped:
		t.Fatal("refused handoff stopped live owner")
	default:
	}
	// An owner that has independently exited may expose its cleanup evidence.
	close(window.closed)
	if err := window.ReplacementReadiness(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := window.CloseForRetry(context.Background()); err != nil {
		t.Fatal(err)
	}
	window.cleanupErr = errors.New("owned process tree remains")
	if !errors.Is(window.ReplacementReadiness(context.Background()), window.cleanupErr) ||
		!errors.Is(window.CloseForRetry(context.Background()), window.cleanupErr) {
		t.Fatal("exited owner bypassed cleanup evidence")
	}
}

func TestCancelledRetryLeavesLiveOwnerUntouched(t *testing.T) {
	startup, err := NewStartup("http://127.0.0.1:11122/")
	if err != nil {
		t.Fatal(err)
	}
	window := &Window{closed: make(chan struct{}), startup: startup}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := window.CloseForRetry(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-startup.stopped:
		t.Fatal("cancelled retry terminated owner")
	default:
	}
}

func TestHostOnlyRetirementRetainsLatePopupUntilNaturalExit(t *testing.T) {
	startup, err := NewStartup("http://127.0.0.1:11122/")
	if err != nil {
		t.Fatal(err)
	}
	window := &Window{captureHost: true, closed: make(chan struct{}), startup: startup,
		containment: replacementSnapshotGuard{}}
	closeRequests := 0
	if err := window.SetCaptureHandoff(func(context.Context) error { closeRequests++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := window.ReplacementReadiness(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Model a popup arriving after readiness. Host closure succeeds, but Chrome
	// remains alive; an ACK is not sufficient to reuse its profile.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := window.CloseForRetry(ctx); !errors.Is(err, ErrCaptureHandoffPending) {
		t.Fatalf("live tree authorized profile reuse: %v", err)
	}
	if closeRequests != 1 || !window.Retiring() {
		t.Fatal("scoped retirement was not retained")
	}
	select {
	case <-startup.stopped:
		t.Fatal("pending retirement forcibly terminated owner")
	default:
	}
	window.containment = replacementSnapshotGuard{err: errors.New("host no longer exists")}
	if err := window.ReplacementReadiness(context.Background()); err != nil {
		t.Fatalf("retired host presence blocked natural-exit retry: %v", err)
	}
	close(window.closed) // User closes the final popup; verified natural exit.
	if err := window.CloseForRetry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if closeRequests != 1 {
		t.Fatal("already-exited owner replayed host closure")
	}
}

func TestReaderLifetimeOnlyReleasesAfterNativeWindowCloses(t *testing.T) {
	readers := map[uintptr]struct{}{11: {}, 22: {}}
	// A reader remains live even when its foreground token was consumed or it
	// is minimized. Neither condition participates in this existence boundary.
	if !retainLiveReaders(readers, func(hwnd uintptr) bool { return hwnd == 22 }) || len(readers) != 1 {
		t.Fatalf("live reader released: %+v", readers)
	}
	if retainLiveReaders(readers, func(uintptr) bool { return false }) || len(readers) != 0 {
		t.Fatalf("closed reader retained: %+v", readers)
	}
}

func TestCapturePopupReadiness(t *testing.T) {
	host := captureZWindow{hwnd: 1, owned: true, chromeWindow: true, host: true}
	for _, tc := range []struct {
		name    string
		windows []captureZWindow
		ready   bool
	}{
		{"host only", []captureZWindow{host}, true},
		{"other user Chrome", []captureZWindow{host, {hwnd: 2, chromeWindow: true}}, true},
		{"native helper", []captureZWindow{host, {hwnd: 2, owned: true}}, true},
		{"visible popup", []captureZWindow{host, {hwnd: 2, owned: true, chromeWindow: true, visible: true}}, false},
		{"hidden or minimized popup after parent closed", []captureZWindow{host, {hwnd: 3, owned: true, chromeWindow: true}}, false},
		{"enumeration failed", nil, false},
		{"host missing", []captureZWindow{}, false},
		{"duplicate host marker", []captureZWindow{host, host}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := capturePopupReadiness(tc.windows) == nil; got != tc.ready {
				t.Fatalf("ready=%t want %t", got, tc.ready)
			}
		})
	}
}
