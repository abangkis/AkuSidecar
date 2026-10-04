package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestReaderOnlyRejectsPassiveProbesBeforeAcquiringLeases(t *testing.T) {
	s, token := splitTestServer(t)
	m := attachSplitLeaseManager(t, s)
	s.SetSplitDirectNativeReader(func(context.Context, string, string, string) (readerbroker.Target, func(context.Context) error, error) {
		t.Fatal("passive probe reached reader")
		return readerbroker.Target{}, nil, nil
	}, func(context.Context) error { return nil })
	for _, action := range []string{"ping", "probe_source_sessions", "reload_self"} {
		w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", `{"type":"`+action+`","actionId":"reload_test"}`)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "browser_handoff_required") {
			t.Fatalf("%s status=%d", action, w.Code)
		}
	}
	if m.Snapshot().ActiveLeases != 0 || len(s.splitCapture.actions) != 0 {
		t.Fatal("reader-only request held a collector lease")
	}
}

func TestReaderExitRetiresOnlyMatchingGenerationPassiveProbes(t *testing.T) {
	s, token := splitTestServer(t)
	process := &splitLeaseProcess{done: make(chan error, 1)}
	m, err := captureruntime.New(process)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Terminate)
	_, done, probe := startCancellableSplitAction(t, s, token)
	// Model probes admitted by the old runtime before the reader-only gate.
	s.SetSplitDirectNativeReader(func(context.Context, string, string, string) (readerbroker.Target, func(context.Context) error, error) {
		return readerbroker.Target{}, nil, nil
	}, func(context.Context) error { return nil })
	userLease, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	user := &pendingSplitAction{action: splitCaptureAction{ID: "split_user", Type: "open_native_post"}, runtimeLease: userLease, result: make(chan splitActionResult, 1)}
	s.splitCapture.mu.Lock()
	probe.claimed = true
	s.splitCapture.actions = append(s.splitCapture.actions, user)
	s.splitCapture.mu.Unlock()
	s.retireReaderPassiveActions(m.Snapshot().Generation + 1)
	if m.Snapshot().ActiveLeases != 2 {
		t.Fatal("wrong-generation probe retired")
	}
	// The fake process naturally exits; the manager's observer drains the probe.
	process.Terminate()
	awaitSplitClient(t, done)
	if m.Snapshot().ActiveLeases != 1 {
		t.Fatal("user action lost its lease or passive lease leaked")
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
	if len(s.splitCapture.actions) != 1 || s.splitCapture.actions[0] != user || !probe.completed {
		t.Fatal("incorrect retirement")
	}
	s.splitCapture.removeAction(user)
}

type splitLeaseProcess struct {
	done chan error
	once sync.Once
}

func (p *splitLeaseProcess) Done() <-chan error                         { return p.done }
func (p *splitLeaseProcess) PID() int                                   { return 123 }
func (p *splitLeaseProcess) Terminate()                                 { p.once.Do(func() { p.done <- nil }) }
func (p *splitLeaseProcess) CloseForRetry(context.Context) error        { p.Terminate(); return nil }
func (p *splitLeaseProcess) OpenExtensionsPage(context.Context) error   { return nil }
func (p *splitLeaseProcess) ReplacementReadiness(context.Context) error { return nil }

func attachSplitLeaseManager(t *testing.T, s *Server) *captureruntime.Manager {
	t.Helper()
	m, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Terminate)
	return m
}

func startCancellableSplitAction(t *testing.T, s *Server, token string) (context.CancelFunc, <-chan struct{}, *pendingSplitAction) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r := httptest.NewRequest("POST", "http://127.0.0.1:11122/api/split-capture/actions", strings.NewReader(`{"type":"ping"}`)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", token)
	r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	done := make(chan struct{})
	go func() { s.api().ServeHTTP(httptest.NewRecorder(), r); close(done) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.splitCapture.mu.Lock()
		var entry *pendingSplitAction
		if len(s.splitCapture.actions) > 0 {
			entry = s.splitCapture.actions[0]
		}
		s.splitCapture.mu.Unlock()
		if entry != nil {
			return cancel, done, entry
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("action was not admitted")
	return nil, nil, nil
}

func awaitSplitClient(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("client did not finish")
	}
}

func TestClaimedSplitActionLeaseSurvivesClientCancellationAndLateResult(t *testing.T) {
	s, token := splitTestServer(t)
	m := attachSplitLeaseManager(t, s)
	cancel, done, entry := startCancellableSplitAction(t, s, token)
	if w := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", ""); w.Code != 200 {
		t.Fatalf("claim=%d", w.Code)
	}
	cancel()
	awaitSplitClient(t, done)
	s.splitCapture.mu.Lock()
	retained := len(s.splitCapture.actions) == 1 && entry.claimed && entry.detached && !entry.completed
	s.splitCapture.mu.Unlock()
	if !retained || m.Snapshot().ActiveLeases != 1 {
		t.Fatal("claimed action ownership was released with client")
	}
	if err := m.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) {
		t.Fatal("unknown active action replaced")
		return nil, nil
	}); !errors.Is(err, captureruntime.ErrBusy) {
		t.Fatalf("active action did not block replacement: %v", err)
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+entry.action.ID, `{"ok":true}`); w.Code != 204 {
		t.Fatalf("late result=%d", w.Code)
	}
	if m.Snapshot().ActiveLeases != 0 {
		t.Fatal("late result did not release lease")
	}
	s.splitCapture.mu.Lock()
	remaining := len(s.splitCapture.actions)
	s.splitCapture.mu.Unlock()
	if remaining != 0 {
		t.Fatal("completed orphan retained queue slot")
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+entry.action.ID, `{"ok":true}`); w.Code < 400 {
		t.Fatal("late result replay accepted")
	}
}

func TestUnclaimedCancelledSplitActionReleasesQueueAndLease(t *testing.T) {
	s, token := splitTestServer(t)
	m := attachSplitLeaseManager(t, s)
	cancel, done, _ := startCancellableSplitAction(t, s, token)
	cancel()
	awaitSplitClient(t, done)
	s.splitCapture.mu.Lock()
	remaining := len(s.splitCapture.actions)
	s.splitCapture.mu.Unlock()
	if remaining != 0 || m.Snapshot().ActiveLeases != 0 {
		t.Fatal("unclaimed cancelled action retained ownership")
	}
}

func TestTransportCloseReleasesDetachedClaimedAction(t *testing.T) {
	s, token := splitTestServer(t)
	m := attachSplitLeaseManager(t, s)
	cancel, done, _ := startCancellableSplitAction(t, s, token)
	if w := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", ""); w.Code != 200 {
		t.Fatal("claim failed")
	}
	cancel()
	awaitSplitClient(t, done)
	s.splitCapture.close()
	s.splitCapture.close()
	if m.Snapshot().ActiveLeases != 0 {
		t.Fatal("transport close leaked action ownership")
	}
}

func TestRuntimeAttachmentAdoptsExistingActionsAndRejectsDuplicates(t *testing.T) {
	s, _ := splitTestServer(t)
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_existing", Type: "ping"}, claimed: true, result: make(chan splitActionResult, 1)}
	s.splitCapture.actions = []*pendingSplitAction{entry}
	m := attachSplitLeaseManager(t, s)
	if m.Snapshot().ActiveLeases != 1 {
		t.Fatal("existing action not adopted")
	}
	if err := s.SetSplitCaptureRuntime(m); err == nil {
		t.Fatal("duplicate runtime attachment accepted")
	}
	s.splitCapture.close()
	if m.Snapshot().ActiveLeases != 0 {
		t.Fatal("adopted lease not released")
	}
}

func TestSplitResultCannotReplayAfterChannelWasConsumed(t *testing.T) {
	s, token := splitTestServer(t)
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_result_once", Type: "ping"}, claimed: true, result: make(chan splitActionResult, 1)}
	s.splitCapture.actions = []*pendingSplitAction{entry}
	m := attachSplitLeaseManager(t, s)
	path := "/api/bridge/split-capture/results/" + entry.action.ID
	if w := splitRequest(s, token, s.splitCapture.key, "POST", path, `{"ok":true}`); w.Code != 204 {
		t.Fatalf("result=%d", w.Code)
	}
	<-entry.result
	if w := splitRequest(s, token, s.splitCapture.key, "POST", path, `{"ok":true}`); w.Code != 409 {
		t.Fatalf("consumed-channel replay=%d", w.Code)
	}
	if m.Snapshot().ActiveLeases != 0 {
		t.Fatal("completed action lease retained")
	}
}

func TestUnavailableRuntimeRejectsSplitAdmission(t *testing.T) {
	s, token := splitTestServer(t)
	m := attachSplitLeaseManager(t, s)
	m.Terminate()
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", `{"type":"ping"}`); w.Code != 503 {
		t.Fatalf("unavailable admission=%d", w.Code)
	}
	s.splitCapture.mu.Lock()
	remaining := len(s.splitCapture.actions)
	s.splitCapture.mu.Unlock()
	if remaining != 0 {
		t.Fatal("failed admission left queued action")
	}
}

func TestClaimedSplitActionTimeoutRetainsLeaseUntilLateResult(t *testing.T) {
	s, token := splitTestServer(t)
	m := attachSplitLeaseManager(t, s)
	s.splitCapture.actionTimeout = 200 * time.Millisecond
	_, done, entry := startCancellableSplitAction(t, s, token)
	if w := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", ""); w.Code != 200 {
		t.Fatal("claim failed")
	}
	// Do not cancel the client: the internal bounded request timer must fire.
	awaitSplitClient(t, done)
	s.splitCapture.mu.Lock()
	retained := entry.detached && entry.claimed && !entry.completed
	s.splitCapture.mu.Unlock()
	if !retained || m.Snapshot().ActiveLeases != 1 {
		t.Fatal("timeout released unknown claimed action")
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+entry.action.ID, `{"ok":false,"message":"controlled failure"}`); w.Code != 204 {
		t.Fatalf("late failure=%d", w.Code)
	}
	if m.Snapshot().ActiveLeases != 0 {
		t.Fatal("late failure retained lease")
	}
}
