package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

func readIGDiagnosticStatus(s *Server, epoch string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11122/api/diagnostics/instagram/status", nil)
	r.Header.Set("X-Aku-Split-Epoch", epoch)
	w := httptest.NewRecorder()
	s.security(s.api()).ServeHTTP(w, r)
	return w
}

func TestInstagramDiagnosticHTTPPreservesSafeCauseAndReadOnlyStatus(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("development build required")
	}
	calls := 0
	s.SetInstagramReaderDiagnosticAction(func(ctx context.Context, _ string) (json.RawMessage, error) {
		calls++
		igdiagnostic.Note(ctx, "dispatch")
		igdiagnostic.Fail(ctx, "dispatch", "public_pending")
		igdiagnostic.SetDelivery(ctx, "unavailable", "terminal_missing")
		return nil, errors.New("PRIVATE_ACCOUNT_TOKEN")
	})
	w := instagramDiagnosticRequest(s, instagramDiagnosticConsent, s.engine.Epoch())
	if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatalf("unsafe failure response: %d", w.Code)
	}
	var response struct {
		Details struct {
			Diagnostic json.RawMessage `json:"diagnostic"`
		} `json:"details"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatal("missing safe details")
	}
	var trace igdiagnostic.Envelope
	if _, ok := igdiagnostic.ValidateJSON(response.Details.Diagnostic); !ok {
		t.Fatal("invalid failure envelope")
	}
	if json.Unmarshal(response.Details.Diagnostic, &trace) != nil || trace.Cause == nil || trace.Cause.Condition != "public_pending" || trace.Cause.Stage != "dispatch" || trace.Disposition != "not_started" {
		t.Fatal("generic HTTP rejection replaced inner cause")
	}
	if w.Header().Get("X-Aku-Diagnostic-ID") != trace.Attempt {
		t.Fatal("attempt header lost")
	}
	for i := 0; i < 2; i++ {
		status := readIGDiagnosticStatus(s, s.engine.Epoch())
		if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"response_written"`) || strings.Contains(status.Body.String(), "PRIVATE") {
			t.Fatal("retained safe delivery observation unavailable")
		}
		var retained struct {
			Diagnostic igdiagnostic.Envelope `json:"diagnostic"`
		}
		if json.Unmarshal(status.Body.Bytes(), &retained) != nil || retained.Diagnostic.Delivery.State != "unavailable" || retained.Diagnostic.HTTPDelivery.State != "response_written" {
			t.Fatal("HTTP delivery overwrote missing RPC terminal")
		}
	}
	if calls != 1 {
		t.Fatal("status replayed diagnostic")
	}
	if readIGDiagnosticStatus(s, "old").Code != http.StatusConflict {
		t.Fatal("status bypassed epoch binding")
	}
}

type brokenIGResponse struct{ header http.Header }

func (w *brokenIGResponse) Header() http.Header     { return w.header }
func (*brokenIGResponse) WriteHeader(int)           {}
func (*brokenIGResponse) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestInstagramDiagnosticHTTPWriteFailureKeepsOperationObservation(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("development build required")
	}
	s.SetInstagramReaderDiagnosticAction(func(ctx context.Context, _ string) (json.RawMessage, error) {
		igdiagnostic.SetCleanup(ctx, "verified", "none")
		igdiagnostic.SetDelivery(ctx, "observed", "none")
		return json.RawMessage(instagramDiagnosticReply), nil
	})
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-reader", strings.NewReader(instagramDiagnosticConsent))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	if err := s.runInstagramReaderDiagnostic(&brokenIGResponse{header: make(http.Header)}, r); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("write failure lost")
	}
	var trace igdiagnostic.Envelope
	if json.Unmarshal(s.engine.InstagramDiagnosticSnapshot(), &trace) != nil || trace.Cause != nil || trace.Cleanup.State != "verified" || trace.Delivery.State != "observed" || trace.HTTPDelivery.State != "unavailable" {
		t.Fatal("response failure replaced successful operation/cleanup observations")
	}
}

func TestInstagramDiagnosticHTTPInitialSnapshotDoesNotStartWork(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("development build required")
	}
	status := readIGDiagnosticStatus(s, s.engine.Epoch())
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"diagnostic":null`) {
		t.Fatal("initial snapshot is not empty")
	}
	w := instagramDiagnosticRequest(s, `{"allowAuthenticatedReaderDiagnostic":false}`, s.engine.Epoch())
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"condition":"consent_required"`) {
		t.Fatal("HTTP consent guard lacks safe condition")
	}
	if !strings.Contains(readIGDiagnosticStatus(s, s.engine.Epoch()).Body.String(), `"diagnostic":null`) {
		t.Fatal("invalid request became an admitted attempt")
	}
}
