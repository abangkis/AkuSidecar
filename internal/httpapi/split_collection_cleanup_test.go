package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestBrowserCollectionCleanupKeepsClaimThroughTimeoutAndLateAck(t *testing.T) {
	s, token := splitTestServer(t)
	owner, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	generation := owner.Snapshot().Generation
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- s.ReleaseBrowserCollectionSurfaces(ctx, "session_fixture", generation) }()
	response := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	var payload struct {
		Action splitCaptureAction `json:"action"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &payload) != nil {
		t.Fatalf("claim: %d %s", response.Code, response.Body.String())
	}
	if payload.Action.Type != "release" || payload.Action.LeaseID != "session_fixture" || payload.Action.Source != "" {
		t.Fatalf("cleanup must release the whole exact session: %+v", payload.Action)
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
	if owner.Snapshot().ActiveLeases != 1 {
		t.Fatal("claimed cleanup must pin its Browser generation until acknowledgement")
	}
	if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID,
		`{"ok":true,"result":{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}}`).Code; got != 204 {
		t.Fatalf("late acknowledgement: %d", got)
	}
	if owner.Snapshot().ActiveLeases != 0 {
		t.Fatal("acknowledged cleanup lease did not drain")
	}
	if err := s.ReleaseBrowserCollectionSurfaces(context.Background(), "session_fixture", generation); err != nil {
		t.Fatalf("late acknowledgement readback: %v", err)
	}
	if len(s.splitCapture.actions) != 1 {
		t.Fatal("cleanup was replayed after acknowledgement")
	}
}

func TestBrowserCollectionCleanupRejectsWrongOwnerBeforeAdmission(t *testing.T) {
	s, _ := splitTestServer(t)
	owner, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.ReleaseBrowserCollectionSurfaces(ctx, "session_fixture", owner.Snapshot().Generation+1); err == nil {
		t.Fatal("cleanup accepted a different profile owner")
	}
	if len(s.splitCapture.actions) != 0 || owner.Snapshot().ActiveLeases != 0 {
		t.Fatal("rejected cleanup leaked ownership")
	}
}

func TestBrowserCollectionCleanupRequiresVerifiedLeaseOutcome(t *testing.T) {
	for _, raw := range []string{``, `null`, `{"released":true,"mode":"owned_window_closed"}`, `{"outcome":null}`, `{"outcome":{"reason":"lease_mismatch"}}`, `{"outcome":{"reason":"cleanup_pending"}}`, `{"outcome":{"reason":"unknown_reason","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"reason":null,"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"reason":"no_owned_surface"}}`, `{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"other","source":null,"expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":"x","expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":1,"closed":0,"transferred":0,"retained":0,"pending":1,"complete":false}}}`, `{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":2,"closed":1,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":4097,"closed":4097,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"reason":"no_owned_source_surface","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`, `{"outcome":{"reason":"no_owned_surface","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":1,"closed":0,"transferred":0,"retained":1,"pending":0,"complete":true}}}`} {
		if err := browserCollectionCleanupResult(&splitActionResult{OK: true, Result: json.RawMessage(raw)}, "session_fixture", nil); err == nil {
			t.Errorf("unverified outcome accepted: %q", raw)
		}
	}
	for _, raw := range []string{
		`{"outcome":{"cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":2,"closed":1,"transferred":1,"retained":0,"pending":0,"complete":true}}}`,
		`{"outcome":{"reason":"no_owned_surface","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`,
		`{"outcome":{"reason":"no_owned_surface","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":1,"closed":1,"transferred":0,"retained":0,"pending":0,"complete":true}}}`,
		`{"outcome":{"reason":"surface_already_closed","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":null,"expected":1,"closed":1,"transferred":0,"retained":0,"pending":0,"complete":true}}}`,
	} {
		if err := browserCollectionCleanupResult(&splitActionResult{OK: true, Result: json.RawMessage(raw)}, "session_fixture", nil); err != nil {
			t.Errorf("valid cleanup rejected: %s: %v", raw, err)
		}
	}
	source := "x"
	raw := `{"outcome":{"reason":"no_owned_source_surface","cleanup":{"schema":"aku.surface-cleanup.v1","leaseId":"session_fixture","source":"x","expected":0,"closed":0,"transferred":0,"retained":0,"pending":0,"complete":true}}}`
	if err := browserCollectionCleanupResult(&splitActionResult{OK: true, Result: json.RawMessage(raw)}, "session_fixture", &source); err != nil {
		t.Fatalf("valid source-specific no-owned cleanup rejected: %v", err)
	}
}

func TestSplitReleaseWithoutBrowserOwnerReturnsStrictCorrelatedReceipt(t *testing.T) {
	t.Run("headless", func(t *testing.T) {
		s, token := splitTestServer(t)
		attachHeadlessSplitOwner(t, s)
		assertNoBrowserOwnerSplitReleaseReceipt(t, s, token)
	})
	t.Run("direct reader", func(t *testing.T) {
		s, token := splitTestServer(t)
		s.splitCapture.directReader = func(context.Context, string, string, string) (readerbroker.NativePostPreparation, error) {
			return readerbroker.NativePostPreparation{}, nil
		}
		assertNoBrowserOwnerSplitReleaseReceipt(t, s, token)
	})
}

func attachHeadlessSplitOwner(t *testing.T, s *Server) {
	t.Helper()
	owner, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	s.engine.RecordHeartbeat(engine.ExpectedHeartbeat())
	coordinator := collection.NewCoordinator(owner, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		if mode != "headless" {
			return nil, errors.New("unexpected test capture mode")
		}
		return &splitCleanupHeadlessProcess{splitLeaseProcess: &splitLeaseProcess{done: make(chan error, 1)}}, nil
	}, func() error { return nil })
	s.engine.AttachCollectionCoordinator(coordinator)
	ctx, cancel := context.WithCancel(context.Background())
	done := coordinator.StartWithDone(ctx)
	coordinator.Request("headless")
	t.Cleanup(func() {
		cancel()
		<-done
		owner.Terminate()
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.engine.CollectionRuntime().Effective == "headless" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("test collection owner did not become headless")
}

type splitCleanupHeadlessProcess struct{ *splitLeaseProcess }

func (*splitCleanupHeadlessProcess) Driver() string { return "headless" }

func assertNoBrowserOwnerSplitReleaseReceipt(t *testing.T, s *Server, token string) {
	t.Helper()
	for _, testCase := range []struct {
		body           string
		leaseID        string
		source         *string
		expectedSource string
	}{
		{body: `{"type":"release","leaseId":"lease_fixture"}`, leaseID: "lease_fixture", expectedSource: "null"},
		{body: `{"type":"release","leaseId":"lease_fixture","source":"x"}`, leaseID: "lease_fixture", source: stringPointer("x"), expectedSource: `"x"`},
	} {
		response := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/split-capture/actions", testCase.body)
		if response.Code != http.StatusOK {
			t.Fatalf("split release relay: status=%d body=%s", response.Code, response.Body.String())
		}
		var result splitActionResult
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.OK {
			t.Fatalf("split release relay response: err=%v body=%s", err, response.Body.String())
		}
		var envelope struct {
			Outcome struct {
				Reason  string `json:"reason"`
				Cleanup struct {
					Schema   string          `json:"schema"`
					LeaseID  string          `json:"leaseId"`
					Source   json.RawMessage `json:"source"`
					Expected int             `json:"expected"`
					Closed   int             `json:"closed"`
					Complete bool            `json:"complete"`
				} `json:"cleanup"`
			} `json:"outcome"`
		}
		if err := json.Unmarshal(result.Result, &envelope); err != nil {
			t.Fatalf("split release receipt: %v", err)
		}
		cleanup := envelope.Outcome.Cleanup
		if envelope.Outcome.Reason != "no_owned_surface" || cleanup.Schema != "aku.surface-cleanup.v1" ||
			cleanup.LeaseID != testCase.leaseID || string(cleanup.Source) != testCase.expectedSource ||
			cleanup.Expected != 0 || cleanup.Closed != 0 || !cleanup.Complete {
			t.Fatalf("split release receipt did not echo the validated request: %+v", envelope.Outcome)
		}
		if err := browserCollectionCleanupResult(&result, testCase.leaseID, testCase.source); err != nil {
			t.Fatalf("strict receipt rejected: %v", err)
		}
		if err := browserCollectionCleanupResult(&result, "another_lease", testCase.source); err == nil {
			t.Fatal("strict receipt accepted a different lease")
		}
		otherSource := "facebook"
		if err := browserCollectionCleanupResult(&result, testCase.leaseID, &otherSource); err == nil {
			t.Fatal("strict receipt accepted a different source")
		}
	}
	if response := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/split-capture/actions", `{"type":"release"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("release without a lease should be rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}

func stringPointer(value string) *string { return &value }
