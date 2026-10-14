//go:build windows && auth_session_dev && auth_session_fixture

package httpapi

import (
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Actual Server routes and typed result metadata, with inert unit callbacks.
// This verifies the fixture's observation predicate, not native activation.
func TestDevelopmentFrontendNativeObservedTypedServerResult(t *testing.T) {
	s, token := splitTestServer(t)
	s.config.Dev = true
	if err := s.LabelDevelopmentFrontend(true); err != nil {
		t.Fatal(err)
	}
	if s.developmentNativeReaderClosed {
		t.Fatal("positive presentation silently retained closed gate")
	}
	o, err := s.ObserveDevelopmentFrontend()
	if err != nil {
		t.Fatal(err)
	}
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("e", 32), Source: "x", URL: "https://x.com/aku_fixture_only/status/1"}
	s.SetSplitDirectNativeReader(func(context.Context, string, string, string) (readerbroker.NativePostPreparation, error) {
		return readerbroker.NativePostPreparation{Target: readerbroker.Target{HWND: 1, PID: 2, Expires: time.Now().Add(time.Second)}, VerifyForeground: func(context.Context) error { return nil }, NavigatePost: func(context.Context) error { return nil }}, nil
	}, func(context.Context) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRequest("POST", "http://127.0.0.1:11122/api/split-capture/actions", strings.NewReader(`{"type":"open_native_post","requestId":"`+req.RequestID+`","source":"x","url":"`+req.URL+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aku-Bridge-Token", token)
		r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		done <- w
	}()
	if err = s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) {
		return readerbroker.Reply{OK: true, Readback: true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		var result splitActionResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.OK {
			t.Fatal("actual native typed server result missing", w.Code, w.Body)
		}
	case <-ctx.Done():
		t.Fatal("actual server result timed out")
	}
	stats := o.Snapshot()
	if stats.NativeAccepted != 1 || stats.NativeRequests != 1 || stats.NativeDenied != 0 || stats.RequestID != req.RequestID {
		t.Fatalf("native observation predicate disagrees with actual Server: %+v", stats)
	}
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:11122/", nil))
	if !strings.Contains(w.Body.String(), "<title>"+DevelopmentFrontendNativeTitle+"</title>") || strings.Contains(w.Body.String(), DevelopmentFrontendTitle) {
		t.Fatal("positive QA presentation is not truthful")
	}
}

func TestDevelopmentFrontendClosedAdmissionPreservesExistingValidationAndOwner(t *testing.T) {
	s, token := splitTestServer(t)
	if s.developmentNativeReaderClosed {
		t.Fatal("ordinary server default changed")
	}
	if s.CloseDevelopmentFrontendNativeActivation() == nil {
		t.Fatal("non-development setter admitted")
	}
	s.config.Dev = true
	if err := s.CloseDevelopmentFrontendNativeActivation(); err != nil {
		t.Fatal(err)
	}
	owner, err := captureruntime.New(&collectionUIFixtureProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, mode: "headless"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Terminate)
	if err = s.engine.AttachCaptureRuntime(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	launches := 0
	c := collection.NewCoordinator(owner, func(context.Context, string, uint64) (captureruntime.Process, error) {
		launches++
		panic("closed native action borrowed owner")
	}, func() error { return nil })
	s.engine.AttachCollectionCoordinator(c)
	if err = s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	before := owner.Snapshot()
	body := `{"type":"open_native_post","requestId":"broker_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source":"x","url":"https://x.com/aku_fixture_only/status/1"}`
	w := splitRequest(s, "wrong", "", "POST", "/api/split-capture/actions", body)
	if w.Code != 401 || strings.Contains(w.Body.String(), "development_native_activation_closed") {
		t.Fatal("closed gate bypassed credentials")
	}
	w = splitRequest(s, token, "", "POST", "/api/split-capture/actions", strings.Replace(body, "https://x.com/aku_fixture_only/status/1", "https://foreign.invalid", 1))
	if w.Code != 400 {
		t.Fatal("closed gate bypassed URL validation", w.Code, w.Body)
	}
	w = splitRequest(s, token, "", "POST", "/api/split-capture/actions", body)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "development_native_activation_closed") {
		t.Fatal("typed closed denial missing", w.Code, w.Body)
	}
	if launches != 0 || owner.Snapshot() != before || owner.Snapshot().ActiveLeases != 0 || len(s.splitCapture.actions) != 0 {
		t.Fatal("closed action mutated owner or queue")
	}
	audit, err := s.store.ReadSplitActionAudit(context.Background(), "", 20)
	if err != nil || len(audit) != 1 || audit[0].Phase != "result" || audit[0].Outcome != "rejected" {
		t.Fatal("explicit closed audit missing", err)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:11122/", nil)
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != 200 || !developmentFrontendTitlePresent(w.Body.String()) || !strings.Contains(w.Body.String(), `src="/split-ui-bridge.js"`) || strings.Contains(w.Body.String(), "aku_fixture_only") {
		t.Fatal("actual page/title/relay contract changed")
	}
}

func TestDevelopmentFrontendRealRelayAndServerContract(t *testing.T) {
	node := os.Getenv("AKU_TEST_NODE_PATH")
	if node == "" {
		t.Skip("requires canonical frontend preparation")
	}
	s, _ := splitTestServer(t)
	s.config.Dev = true
	if err := s.CloseDevelopmentFrontendNativeActivation(); err != nil {
		t.Fatal(err)
	}
	owner, err := captureruntime.New(&collectionUIFixtureProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, mode: "headless"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Terminate)
	if err = s.engine.AttachCaptureRuntime(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	c := collection.NewCoordinator(owner, func(context.Context, string, uint64) (captureruntime.Process, error) { panic("closed relay borrowed") }, func() error { return nil })
	s.engine.AttachCollectionCoordinator(c)
	observation, err := s.ObserveDevelopmentFrontend()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.http.Handler)
	defer server.Close()
	script, err := filepath.Abs("../../scripts/auth-session-fixture/frontend-bridge-probe.mjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, script, server.URL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real relay/server probe failed: %v %s", err, output)
	}
	got := observation.Snapshot()
	if got.Bootstrap != 1 || got.RelayAsset != 1 || got.NativeDenied != 1 || got.Cancelled != 1 || got.RequestID != "broker_"+strings.Repeat("b", 32) {
		t.Fatalf("real relay/cancel metadata invalid: %+v", got)
	}
	t.Log(strings.TrimSpace(string(output)))
}
