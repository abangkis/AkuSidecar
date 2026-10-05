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
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestRotationFencesOldBootstrapAndResetsHeartbeat(t *testing.T) {
	s, token := splitTestServer(t)
	transport := s.splitCapture
	old := transport.key
	if w := splitRequest(s, token, old, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	s.engine.RecordHeartbeat(domain.BridgeHeartbeat{BridgeID: "old"})
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	if s.splitCapture != transport || transport.key == old {
		t.Fatal("rotation replaced stable transport or kept key")
	}
	if transport.sourceTrackingSupported || transport.untrackedSourceOutcome {
		t.Fatal("new owner inherited stale readiness")
	}
	if w := splitRequest(s, token, old, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1}`); w.Code != 403 {
		t.Fatal("old owner bootstrapped", w.Code)
	}
	if w := splitRequest(s, token, transport.key, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if !transport.sourceTrackingSupported {
		t.Fatal("new owner cannot negotiate")
	}
}
func TestRotationRefusesUnfinishedAction(t *testing.T) {
	s, token := splitTestServer(t)
	attachSplitLeaseManager(t, s)
	cancel, done, _ := startCancellableSplitAction(t, s, token)
	old := s.splitCapture.key
	if err := s.RotateSplitCapture(); err == nil {
		t.Fatal("rotation discarded unfinished ownership")
	}
	if s.splitCapture.key != old {
		t.Fatal("refusal rotated key")
	}
	cancel()
	awaitSplitClient(t, done)
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
}
func TestStalePollCannotClaimAfterRotation(t *testing.T) {
	s, token := splitTestServer(t)
	old := s.splitCapture.key
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	w := splitRequest(s, token, old, "GET", "/api/bridge/split-capture/next", "")
	if w.Code != 403 && w.Code != 409 {
		t.Fatal("old poll accepted", w.Code)
	}
	// The fresh key, not the persistent token alone, authorizes capture routes.
	if strings.Contains(w.Body.String(), `"action"`) {
		t.Fatal("stale action leaked")
	}
}

type splitRotationHeadlessProcess struct{ *splitLeaseProcess }

func (*splitRotationHeadlessProcess) Driver() string { return "headless" }

func TestRotationPreservesLiveReaderBrokerDuringNativeBorrow(t *testing.T) {
	s, token := splitTestServer(t)
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("c", 32), Source: "x", URL: "https://x.com/a/status/123"}
	brokerCtx, cancelBroker := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelBroker()
	brokerDone := make(chan error, 1)
	activated := make(chan string, 2)
	prepared := make(chan string, 2)
	verified := make(chan struct{}, 2)
	go func() {
		brokerDone <- s.HandleReaderBroker(brokerCtx, request, func(target readerbroker.Target) (readerbroker.Reply, error) {
			if target.HWND != 123 || target.Action == "" {
				t.Errorf("wrong broker target: %+v", target)
			}
			activated <- target.Action
			return readerbroker.Reply{OK: true, Readback: true}, nil
		})
	}()
	waitForSplitTest(t, "reader broker conversation registration", func() bool {
		s.splitCapture.mu.Lock()
		defer s.splitCapture.mu.Unlock()
		conversation := s.splitCapture.readerBrokers[request.RequestID]
		return conversation != nil && conversation.cancel != nil && !conversation.cancelled
	})

	owner, err := captureruntime.New(&splitRotationHeadlessProcess{&splitLeaseProcess{done: make(chan error, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	launchEntered := make(chan struct{})
	finishLaunch := make(chan struct{})
	var releaseLaunch sync.Once
	coordinator := collection.NewCoordinator(owner, func(ctx context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		if err := s.RotateSplitCapture(); err != nil {
			return nil, err
		}
		switch mode {
		case "native_reader":
			// Match main.go: bind direct helper preparation to the new reader process.
			s.SetSplitDirectNativeReader(func(_ context.Context, id, url, marker string) (readerbroker.Target, func(context.Context) error, error) {
				if url != request.URL || marker != "/split-reader-intent?id="+id {
					t.Errorf("wrong direct reader correlation: id=%q url=%q marker=%q", id, url, marker)
				}
				prepared <- id
				return readerbroker.Target{HWND: 123, PID: 456, Action: id, Expires: time.Now().Add(3 * time.Second)}, func(context.Context) error {
					verified <- struct{}{}
					return nil
				}, nil
			}, func(context.Context) error { return nil })
			close(launchEntered)
			select {
			case <-finishLaunch:
				return &splitLeaseProcess{done: make(chan error, 1)}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case "headless":
			// After the native reader lease returns, reconciliation may restore the
			// saved mode. It must not enter or signal the blocked reader launch.
			return &splitRotationHeadlessProcess{&splitLeaseProcess{done: make(chan error, 1)}}, nil
		default:
			return nil, errors.New("unexpected capture mode: " + mode)
		}
	}, func() error { return nil })
	s.engine.AttachCollectionCoordinator(coordinator)
	coordinator.Request("headless")
	coordinatorCtx, cancelCoordinator := context.WithCancel(context.Background())
	defer func() {
		cancelCoordinator()
		releaseLaunch.Do(func() { close(finishLaunch) })
		owner.Terminate()
	}()

	actionCtx, cancelAction := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAction()
	actionRequest := httptest.NewRequest("POST", "http://127.0.0.1:11122/api/split-capture/actions", strings.NewReader(`{"type":"open_native_post","source":"x","url":"`+request.URL+`","requestId":"`+request.RequestID+`"}`)).WithContext(actionCtx)
	actionRequest.Header.Set("Content-Type", "application/json")
	actionRequest.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	actionRequest.Header.Set("X-Aku-Bridge-Token", token)
	actionRequest.Header.Set("X-Aku-Capture-Instance", s.splitCapture.key)
	actionRequest.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	type actionResponse struct {
		status int
		body   []byte
	}
	actionDone := make(chan actionResponse, 1)
	go func() {
		w := httptest.NewRecorder()
		s.api().ServeHTTP(w, actionRequest)
		actionDone <- actionResponse{status: w.Code, body: append([]byte(nil), w.Body.Bytes()...)}
	}()
	waitForSplitTest(t, "native reader borrow admission", func() bool {
		status := coordinator.Status()
		return status.Pending && status.Effective == "headless" && status.State == captureruntime.Ready
	})
	coordinator.Start(coordinatorCtx)

	select {
	case <-launchEntered:
	case <-actionCtx.Done():
		t.Fatal("native reader borrow did not enter the replacement factory")
	case <-time.After(2 * time.Second):
		t.Fatal("native reader borrow did not enter the replacement factory")
	}
	s.splitCapture.mu.Lock()
	conversation := s.splitCapture.readerBrokers[request.RequestID]
	conversationPreserved := conversation != nil && conversation.cancel != nil && !conversation.cancelled
	s.splitCapture.mu.Unlock()
	releaseLaunch.Do(func() { close(finishLaunch) })
	if !conversationPreserved {
		t.Fatal("ordinary rotation canceled the live UI reader broker conversation")
	}

	var preparedActionID string
	select {
	case preparedActionID = <-prepared:
	case <-actionCtx.Done():
		t.Fatal("matching native action did not attach to the waiting reader broker")
	}
	select {
	case err := <-brokerDone:
		if err != nil {
			t.Fatalf("reader broker failed after rotation: %v", err)
		}
	case <-actionCtx.Done():
		t.Fatal("reader broker did not complete")
	}
	select {
	case response := <-actionDone:
		if response.status != 200 || !strings.Contains(string(response.body), `"ok":true`) {
			t.Fatalf("native action did not complete successfully: %d %s", response.status, response.body)
		}
	case <-actionCtx.Done():
		t.Fatal("native action did not return")
	}
	select {
	case actionID := <-activated:
		if actionID != preparedActionID {
			t.Fatalf("activated reader %q differs from prepared action %q", actionID, preparedActionID)
		}
	default:
		t.Fatal("matching reader helper was not activated")
	}
	select {
	case <-activated:
		t.Fatal("reader helper activation ran more than once")
	default:
	}
	select {
	case <-verified:
	default:
		t.Fatal("reader helper verification did not run")
	}
	select {
	case <-verified:
		t.Fatal("reader helper verification ran more than once")
	default:
	}
}

func TestRotationPreservesExplicitReaderBrokerCancellationTombstone(t *testing.T) {
	s, _ := splitTestServer(t)
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("d", 32), Source: "x", URL: "https://x.com/a/status/456"}
	if err := s.CancelReaderBroker(request); err != nil {
		t.Fatal(err)
	}
	s.splitCapture.mu.Lock()
	before := s.splitCapture.readerBrokers[request.RequestID]
	if before == nil || !before.cancelled {
		s.splitCapture.mu.Unlock()
		t.Fatal("explicit cancel did not create a tombstone")
	}
	expiresBefore := before.expires
	s.splitCapture.mu.Unlock()
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	s.splitCapture.mu.Lock()
	after := s.splitCapture.readerBrokers[request.RequestID]
	preserved := after != nil && after.cancelled && after.expires.Equal(expiresBefore) && s.splitCapture.readerBrokerCancelledLocked(request, time.Now())
	s.splitCapture.mu.Unlock()
	if !preserved {
		t.Fatal("ordinary rotation changed or removed an explicit cancellation tombstone")
	}
}

func TestSplitCaptureCloseCancelsReaderBrokerWaiter(t *testing.T) {
	s, _ := splitTestServer(t)
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("e", 32), Source: "x", URL: "https://x.com/a/status/789"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.HandleReaderBroker(ctx, request, func(readerbroker.Target) (readerbroker.Reply, error) {
			t.Error("closed broker waiter reached activation")
			return readerbroker.Reply{}, nil
		})
	}()
	waitForSplitTest(t, "reader broker waiter registration", func() bool {
		s.splitCapture.mu.Lock()
		defer s.splitCapture.mu.Unlock()
		conversation := s.splitCapture.readerBrokers[request.RequestID]
		return conversation != nil && conversation.cancel != nil
	})
	s.splitCapture.close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("close did not cancel reader broker waiter: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("reader broker waiter remained after transport close")
	}
}

func waitForSplitTest(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
