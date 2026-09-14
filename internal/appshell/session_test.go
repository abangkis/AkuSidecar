package appshell

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSessionWindow struct {
	done          chan error
	once          sync.Once
	closed        atomic.Bool
	closeForRetry func(context.Context) error
}

func newFakeSessionWindow() *fakeSessionWindow  { return &fakeSessionWindow{done: make(chan error, 1)} }
func (w *fakeSessionWindow) Done() <-chan error { return w.done }
func (w *fakeSessionWindow) Terminate()         { w.once.Do(func() { w.closed.Store(true); w.done <- nil }) }
func (w *fakeSessionWindow) CloseForRetry(ctx context.Context) error {
	if w.closeForRetry != nil {
		if err := w.closeForRetry(ctx); err != nil {
			return err
		}
	}
	w.Terminate()
	return nil
}
func (w *fakeSessionWindow) OpenExtensionsPage(context.Context) error { return nil }

func waitSessionCondition(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("session transition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}
func sessionToken(target string) string {
	u, _ := url.Parse(target)
	return strings.TrimPrefix(u.Fragment, "aku-startup=")
}

func TestSessionRetryWaitsForCleanupAndResetsAcknowledgement(t *testing.T) {
	old, replacement := newFakeSessionWindow(), newFakeSessionWindow()
	closing, allowCleanup, newLaunch, allowLaunch := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	old.closeForRetry = func(ctx context.Context) error {
		close(closing)
		select {
		case <-allowCleanup:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	options := LaunchOptions{URL: "http://127.0.0.1:11122/", Executable: "pinned-chromium", UserDataDir: "same-profile", ExtensionPath: "same-extension", ExtraArgs: []string{"--example"}}
	var startup *Startup
	var calls atomic.Int32
	var initialToken, newToken string
	s, err := launchSession(context.Background(), options, func(value *Startup) { startup = value }, nil,
		func(ctx context.Context, actual LaunchOptions) (sessionWindow, error) {
			if calls.Add(1) == 1 {
				initialToken = sessionToken(actual.URL)
				return old, nil
			}
			if !old.closed.Load() {
				t.Error("replacement launched before cleanup")
			}
			newToken = sessionToken(actual.URL)
			actual.URL = options.URL
			if !reflect.DeepEqual(actual, options) {
				t.Errorf("launch settings changed: %#v", actual)
			}
			if newToken == initialToken || startup.Acknowledge(startupRequest(initialToken)) {
				t.Error("old launch capability reused")
			}
			if !startup.Acknowledge(startupRequest(newToken)) {
				t.Error("replacement acknowledgement rejected during launch")
			}
			close(newLaunch)
			select {
			case <-allowLaunch:
				return replacement, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	if !startup.Acknowledge(startupRequest(initialToken)) {
		t.Fatal("initial acknowledgement failed")
	}
	if !startup.requestRetry() {
		t.Fatal("retry rejected")
	}
	<-closing
	for range 10 {
		if startup.requestRetry() {
			t.Fatal("duplicate retry accepted")
		}
	}
	if calls.Load() != 1 || startup.Acknowledge(startupRequest(initialToken)) {
		t.Fatal("old window acknowledged or replacement overlapped")
	}
	close(allowCleanup)
	<-newLaunch
	if startup.requestRetry() {
		t.Fatal("duplicate retry accepted during launch")
	}
	close(allowLaunch)
	waitSessionCondition(t, func() bool { busy, _ := startup.retryStatus(); return !busy })
	select {
	case <-s.Done():
		t.Fatal("manual replacement shut down Sidecar session")
	default:
	}
	replacement.Terminate()
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("normal close failed to end session")
	}
}

func TestSessionFailedRelaunchRequiresAnotherManualClick(t *testing.T) {
	old, replacement := newFakeSessionWindow(), newFakeSessionWindow()
	var calls atomic.Int32
	var reports atomic.Int32
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, func(error) { reports.Add(1) },
		func(context.Context, LaunchOptions) (sessionWindow, error) {
			switch calls.Add(1) {
			case 1:
				return old, nil
			case 2:
				return nil, errors.New("simulated failed start")
			default:
				return replacement, nil
			}
		}, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	s.startup.requestRetry()
	waitSessionCondition(t, func() bool { busy, failed := s.startup.retryStatus(); return !busy && failed })
	if calls.Load() != 2 || reports.Load() != 1 {
		t.Fatal("unexpected automatic retry or missing diagnostic")
	}
	if _, _, finished, _ := s.startup.status(time.Now()); finished {
		t.Fatal("failure dismissed native recovery")
	}
	select {
	case <-s.Done():
		t.Fatal("failed relaunch stopped service")
	default:
	}
	if !s.startup.requestRetry() {
		t.Fatal("manual recovery unavailable")
	}
	waitSessionCondition(t, func() bool { busy, failed := s.startup.retryStatus(); return !busy && !failed })
	if calls.Load() != 3 {
		t.Fatal("manual recovery did not launch exactly once")
	}
}

func TestSessionShutdownDuringCleanupDoesNotRelaunch(t *testing.T) {
	old := newFakeSessionWindow()
	entered := make(chan struct{})
	old.closeForRetry = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	var calls atomic.Int32
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
		func(context.Context, LaunchOptions) (sessionWindow, error) { calls.Add(1); return old, nil }, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	s.startup.requestRetry()
	<-entered
	s.Terminate()
	if calls.Load() != 1 {
		t.Fatal("shutdown launched replacement")
	}
}

func TestSessionNormalCloseWinsQueuedRetry(t *testing.T) {
	old := newFakeSessionWindow()
	old.Terminate()
	var calls atomic.Int32
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
		func(context.Context, LaunchOptions) (sessionWindow, error) { calls.Add(1); return old, nil }, func(startup *Startup) { startup.requestRetry() })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("normal close did not win")
	}
	if calls.Load() != 1 {
		t.Fatal("queued click reopened a normally closed app")
	}
}

func TestSessionUnverifiedCleanupNeverLaunchesReplacement(t *testing.T) {
	old := newFakeSessionWindow()
	old.closeForRetry = func(context.Context) error { return errors.New("cleanup not verified") }
	var calls atomic.Int32
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
		func(context.Context, LaunchOptions) (sessionWindow, error) { calls.Add(1); return old, nil }, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	for range 2 {
		if !s.startup.requestRetry() {
			t.Fatal("manual retry rejected")
		}
		waitSessionCondition(t, func() bool { busy, failed := s.startup.retryStatus(); return !busy && failed })
	}
	if calls.Load() != 1 {
		t.Fatal("unverified cleanup allowed overlapping launch")
	}
}

func TestSessionDismissDuringRetryDoesNotCancelRequestedReplacement(t *testing.T) {
	old, next := newFakeSessionWindow(), newFakeSessionWindow()
	entered, release := make(chan struct{}), make(chan struct{})
	old.closeForRetry = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	var calls atomic.Int32
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
		func(context.Context, LaunchOptions) (sessionWindow, error) {
			if calls.Add(1) == 1 {
				return old, nil
			}
			return next, nil
		}, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	s.startup.requestRetry()
	<-entered
	s.startup.Stop()
	close(release)
	waitSessionCondition(t, func() bool { busy, _ := s.startup.retryStatus(); return !busy })
	if calls.Load() != 2 {
		t.Fatal("status dismissal cancelled requested replacement")
	}
	if show, _, finished, _ := s.startup.status(time.Now().Add(time.Minute)); show || !finished {
		t.Fatal("retry reopened dismissed native status")
	}
}

func TestWindowCloseForRetryRequiresCleanupBarrier(t *testing.T) {
	window := &Window{closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := window.CloseForRetry(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("unconfirmed cleanup returned %v", err)
	}
	window.cleanupErr = errors.New("unverified ownership cleanup")
	close(window.closed)
	if err := window.CloseForRetry(context.Background()); err == nil {
		t.Fatal("cleanup error ignored")
	}
	window.cleanupErr = nil
	if err := window.CloseForRetry(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSessionUnsafeFailedLaunchBlocksFurtherLaunches(t *testing.T) {
	old := newFakeSessionWindow()
	var calls atomic.Int32
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
		func(context.Context, LaunchOptions) (sessionWindow, error) {
			if calls.Add(1) == 1 {
				return old, nil
			}
			return nil, errCleanupUnverified
		}, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	for range 2 {
		if !s.startup.requestRetry() {
			t.Fatal("manual click rejected")
		}
		waitSessionCondition(t, func() bool { busy, failed := s.startup.retryStatus(); return !busy && failed })
	}
	if calls.Load() != 2 {
		t.Fatal("unverified failed launch allowed another browser")
	}
}

func TestSessionRecoveryCloseEndsFailedRelaunch(t *testing.T) {
	for _, failure := range []error{errors.New("failed launch"), errCleanupUnverified} {
		t.Run(failure.Error(), func(t *testing.T) {
			old := newFakeSessionWindow()
			var calls atomic.Int32
			s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
				func(context.Context, LaunchOptions) (sessionWindow, error) {
					if calls.Add(1) == 1 {
						return old, nil
					}
					return nil, failure
				}, func(*Startup) {})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Terminate()
			s.startup.requestRetry()
			waitSessionCondition(t, func() bool { busy, failed := s.startup.retryStatus(); return !busy && failed })
			s.startup.Stop()
			select {
			case <-s.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("last recovery close stranded Sidecar")
			}
			if calls.Load() != 2 {
				t.Fatal("close triggered another launch")
			}
		})
	}
}

func TestSessionStatusClosePreservesLiveShell(t *testing.T) {
	old := newFakeSessionWindow()
	s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
		func(context.Context, LaunchOptions) (sessionWindow, error) { return old, nil }, func(*Startup) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	s.startup.Stop()
	select {
	case <-s.Done():
		t.Fatal("status-only close ended a live shell")
	case <-time.After(10 * time.Millisecond):
	}
	if old.closed.Load() {
		t.Fatal("status close terminated Chromium")
	}
	old.Terminate()
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("normal shell close did not end session")
	}
}

func TestSessionPendingCleanupObservesLateExitAfterStatusClose(t *testing.T) {
	for _, exitsDuringCleanup := range []bool{false, true} {
		t.Run(map[bool]string{false: "late exit", true: "already exited"}[exitsDuringCleanup], func(t *testing.T) {
			old := newFakeSessionWindow()
			old.closeForRetry = func(context.Context) error {
				if exitsDuringCleanup {
					old.Terminate()
				}
				return errors.New("cleanup unverified")
			}
			var calls atomic.Int32
			s, err := launchSession(context.Background(), LaunchOptions{URL: "http://127.0.0.1:11122/"}, nil, nil,
				func(context.Context, LaunchOptions) (sessionWindow, error) { calls.Add(1); return old, nil }, func(*Startup) {})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Terminate()
			s.startup.requestRetry()
			waitSessionCondition(t, func() bool { busy, failed := s.startup.retryStatus(); return !busy && failed })
			s.startup.Stop()
			if !exitsDuringCleanup {
				select {
				case <-s.Done():
					t.Fatal("status close terminated remaining shell")
				case <-time.After(10 * time.Millisecond):
				}
				old.Terminate()
			}
			select {
			case <-s.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("late owner exit left a headless session")
			}
			if calls.Load() != 1 {
				t.Fatal("unverified cleanup relaunched a browser")
			}
		})
	}
}
