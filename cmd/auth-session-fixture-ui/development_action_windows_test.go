//go:build windows && auth_session_fixture

package main

import (
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDevelopmentActionHelperRejectionCancelsPendingAPI(t *testing.T) {
	entered, cancelled := make(chan struct{}), make(chan struct{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	request := readerbroker.Request{RequestID: "broker_" + readerbroker.Secret()[:32], Source: "x", URL: "https://x.com/aku_fixture_only/status/1"}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Type, Source, URL, RequestID string }
		if r.URL.Path != "/api/split-capture/actions" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Type != "open_native_post" || body.RequestID != request.RequestID || r.Header.Get("X-Aku-Bridge-Contract") != domain.BridgeContractVersion || r.Header.Get("X-Aku-Bridge-Token") != "invented-test-token" || r.Header.Get("X-Aku-Capture-Instance") != "invented-instance" || r.Header.Get("X-Aku-Split-Epoch") != "invented-epoch" {
			t.Error("API correlation/header contract changed")
			http.Error(w, "denied", 403)
			return
		}
		close(entered)
		<-r.Context().Done()
		close(cancelled)
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	started := time.Now()
	result := developmentActionResult(command{Development: true, Request: request, APIAddress: server.URL, Token: "invented-test-token", CaptureKey: "invented-instance", EngineEpoch: "invented-epoch"}, func(command) readerbroker.Reply {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("API did not start before rejection")
		}
		return readerbroker.Reply{Message: "Reader broker server identity rejected"}
	})
	if result["apiCancelledAfterHelperRejection"] != true || result["error"] != "context_cancelled" || result["apiStatus"] != 0 || time.Since(started) > time.Second {
		t.Fatal("helper rejection waited for unmatched API timeout", result["error"])
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("pending HTTP action was not cancelled")
	}
	t.Logf("nativeHelperRejectedBeforeBrokerRequest=true pendingCorrelatedAPIContextCancelled=true safeError=context_cancelled elapsedMS=%d no30SecondWait=true UIStarted=false", time.Since(started).Milliseconds())
}
