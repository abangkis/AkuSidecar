package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/observability"
)

type observedDiagnostic struct {
	operation observability.Operation
	outcome   observability.Outcome
	finished  int
}

type diagnosticObserverFixture struct{ calls []*observedDiagnostic }

func (o *diagnosticObserverFixture) Begin(ctx context.Context, operation observability.Operation) (context.Context, func(observability.Outcome)) {
	call := &observedDiagnostic{operation: operation}
	o.calls = append(o.calls, call)
	return ctx, func(outcome observability.Outcome) { call.outcome = outcome; call.finished++ }
}

func TestDiagnosticTelemetryPreservesTimelineHTTPContract(t *testing.T) {
	valid, err := json.Marshal(validUIPerformanceReport())
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name, method, mime string
		body               []byte
		production         bool
		status             int
		outcome            observability.Outcome
	}{
		{"empty read", http.MethodGet, "", nil, false, 200, observability.Success},
		{"accepted write", http.MethodPost, "application/json", valid, false, 200, observability.Success},
		{"invalid report", http.MethodPost, "application/json", []byte(`{"private":"post and token"}`), false, 400, observability.Rejected},
		{"unsupported mime", http.MethodPost, "text/plain", valid, false, 415, observability.Rejected},
		{"production gate", http.MethodPost, "application/json", valid, true, 404, observability.Rejected},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := newUIPerformanceDevServer(t)
			if fixture.production {
				server.config.Dev = false
			}
			invoke := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest(fixture.method, uiPerformancePath, bytes.NewReader(fixture.body))
				if fixture.mime != "" {
					r.Header.Set("Content-Type", fixture.mime)
				}
				w := httptest.NewRecorder()
				server.api().ServeHTTP(w, r)
				return w
			}
			before := invoke()
			observer := &diagnosticObserverFixture{}
			server.SetDiagnosticsObserver(observer)
			after := invoke()
			if before.Code != fixture.status || after.Code != before.Code || after.Body.String() != before.Body.String() || !reflect.DeepEqual(after.Header(), before.Header()) {
				t.Fatalf("telemetry changed HTTP contract: before=%d %s after=%d %s", before.Code, before.Body.String(), after.Code, after.Body.String())
			}
			if len(observer.calls) != 1 || observer.calls[0].outcome != fixture.outcome || observer.calls[0].finished != 1 {
				t.Fatalf("unexpected bounded observation: %+v", observer.calls)
			}
			expected := observability.UITimelineWrite
			if fixture.method == http.MethodGet {
				expected = observability.UITimelineRead
			}
			if observer.calls[0].operation != expected {
				t.Fatal("operation classification changed")
			}
		})
	}
}

func TestDiagnosticTelemetryUsesWrittenIGErrorStatus(t *testing.T) {
	server := instagramDiagnosticServer(t, false)
	if !server.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("positive operator route requires the supported development build")
	}
	observer := &diagnosticObserverFixture{}
	server.SetDiagnosticsObserver(observer)
	request := httptest.NewRequest(http.MethodPost, "/api/diagnostics/instagram-reader", bytes.NewBufferString(instagramDiagnosticConsent))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Split-Epoch", server.engine.Epoch())
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected unavailable diagnostic: %d %s", response.Code, response.Body.String())
	}
	if len(observer.calls) != 1 || observer.calls[0].operation != observability.InstagramReader || observer.calls[0].outcome != observability.Rejected {
		t.Fatal("a handler-written error was incorrectly reported as success")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body) != 3 || body["error"] == nil || body["message"] == nil || body["details"] == nil {
		t.Fatal("diagnostic error envelope changed")
	}
	server.SetDiagnosticsObserver(nil)
	if server.diagnostics != nil {
		t.Fatal("nil did not restore uninstrumented behavior")
	}
}

func TestDiagnosticTelemetryIgnoresUnselectedRoutesAndPreflight(t *testing.T) {
	server := newUIPerformanceDevServer(t)
	observer := &diagnosticObserverFixture{}
	server.SetDiagnosticsObserver(observer)
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		request := httptest.NewRequest(method, "/api/health", nil)
		server.api().ServeHTTP(httptest.NewRecorder(), request)
	}
	request := httptest.NewRequest(http.MethodOptions, uiPerformancePath, nil)
	server.api().ServeHTTP(httptest.NewRecorder(), request)
	if len(observer.calls) != 0 {
		t.Fatal("an unselected request started telemetry")
	}
	for _, path := range []string{"/api/health", "/api/diagnostics/export", "/api/diagnostics/instagram/status", "/api/diagnostics/private-post-secret"} {
		if diagnosticOperation(http.MethodGet, path) != 0 {
			t.Fatal("arbitrary route entered operation vocabulary")
		}
	}
}

func TestDiagnosticResponseWriterTracksStatusAndUnwraps(t *testing.T) {
	original := httptest.NewRecorder()
	writer := &diagnosticResponseWriter{ResponseWriter: original}
	writer.WriteHeader(409)
	writer.WriteHeader(200)
	if _, err := writer.Write([]byte("unchanged response")); err != nil {
		t.Fatal(err)
	}
	if writer.status != 409 || original.Code != 409 || writer.Unwrap() != original || original.Body.String() != "unchanged response" {
		t.Fatal("response status, body or writer identity changed")
	}
}

func TestDiagnosticTelemetryExporterFailureLeavesStoredContractIntact(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer collector.Close()
	telemetry, err := observability.New(context.Background(), observability.Config{OTLPEndpoint: collector.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background()) })
	server := newUIPerformanceDevServer(t)
	server.SetDiagnosticsObserver(telemetry)
	report := validUIPerformanceReport()
	postUIPerformanceReport(t, server, report)
	if got := readUIPerformanceSnapshot(t, server); !reflect.DeepEqual(got.Report, report) {
		t.Fatal("telemetry changed the authoritative saved report")
	}
	if err := telemetry.Shutdown(context.Background()); err != nil {
		t.Fatal("export failure escaped best-effort telemetry")
	}
	if health := telemetry.Diagnostics(); !health.ExportConfigured || health.ExportFailures == 0 {
		t.Fatalf("export failure was not retained as separate bounded health: %+v", health)
	}
	if got := readUIPerformanceSnapshot(t, server); !reflect.DeepEqual(got.Report, report) {
		t.Fatal("export failure changed the diagnostic response")
	}
}
