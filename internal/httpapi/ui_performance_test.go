package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const uiPerformancePath = "/api/diagnostics/ui-performance"

func TestUIPerformanceDiagnosticsRoundTripAndReplacement(t *testing.T) {
	server, _ := openLibraryHTTPFixture(t)
	server.config.Dev = true
	server.config.Deployment.Mode = "development"

	response := serveUIPerformanceRequest(server, http.MethodGet, nil)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"trace":null}` {
		t.Fatalf("initial read status=%d body=%s", response.Code, response.Body.String())
	}

	first := validUIPerformanceReport()
	postUIPerformanceReport(t, server, first)
	got := readUIPerformanceSnapshot(t, server)
	if _, err := time.Parse(time.RFC3339Nano, got.RecordedAt); err != nil {
		t.Fatalf("recordedAt=%q is not a server UTC timestamp: %v", got.RecordedAt, err)
	}
	if !reflect.DeepEqual(got.Report, first) {
		t.Fatalf("stored report differs from accepted report:\n got: %#v\nwant: %#v", got.Report, first)
	}

	second := validUIPerformanceReport()
	second.DurationMs = 1_200
	second.Reason = "manual"
	postUIPerformanceReport(t, server, second)
	got = readUIPerformanceSnapshot(t, server)
	if got.Report.DurationMs != second.DurationMs || got.Report.Reason != second.Reason {
		t.Fatalf("latest report was not replaced: %+v", got.Report)
	}
}

func TestUIPerformanceDiagnosticsRequireDevelopmentMode(t *testing.T) {
	for _, test := range []struct {
		name string
		dev  bool
		mode string
	}{
		{name: "dev flag off", dev: false, mode: "development"},
		{name: "production mode", dev: true, mode: "production-installed-app"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, _ := openLibraryHTTPFixture(t)
			server.config.Dev = test.dev
			server.config.Deployment.Mode = test.mode
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				var body []byte
				if method == http.MethodPost {
					body, _ = json.Marshal(validUIPerformanceReport())
				}
				response := serveUIPerformanceRequest(server, method, body)
				if response.Code != http.StatusNotFound {
					t.Fatalf("%s status=%d body=%s", method, response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestUIPerformanceRejectsUnknownAndPrivacyFields(t *testing.T) {
	server := newUIPerformanceDevServer(t)
	body, err := json.Marshal(validUIPerformanceReport())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		body []byte
	}{
		{name: "unknown content field", body: bytes.Replace(body, []byte(`"version":1`), []byte(`"version":1,"content":"private marker"`), 1)},
		{name: "event script attribution", body: bytes.Replace(body, []byte(`"kind":"loaf"`), []byte(`"kind":"loaf","scriptAttribution":"private marker"`), 1)},
		{name: "event URL", body: bytes.Replace(body, []byte(`"kind":"loaf"`), []byte(`"kind":"loaf","url":"https://example.invalid/private"`), 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveUIPerformanceRequest(server, http.MethodPost, test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private marker") || strings.Contains(response.Body.String(), "example.invalid") {
				t.Fatalf("response echoed private input: %s", response.Body.String())
			}
		})
	}
}

func TestUIPerformanceRejectsBoundsNaNEventLimitOversizeAndTrailingJSON(t *testing.T) {
	server := newUIPerformanceDevServer(t)
	valid, err := json.Marshal(validUIPerformanceReport())
	if err != nil {
		t.Fatal(err)
	}
	nan := bytes.Replace(valid, []byte(`"durationMs":1500`), []byte(`"durationMs":NaN`), 1)
	oversized := append(append([]byte(nil), valid...), bytes.Repeat([]byte(" "), uiPerformanceMaxBodyBytes)...)
	tooManyEvents := validUIPerformanceReport()
	tooManyEvents.Events = make([]uiPerformanceEvent, 501)
	tooManyEventsBody, err := json.Marshal(tooManyEvents)
	if err != nil {
		t.Fatal(err)
	}
	tooLong := validUIPerformanceReport()
	tooLong.DurationMs = 60_001
	tooLongBody, _ := json.Marshal(tooLong)
	tooMany := validUIPerformanceReport()
	tooMany.FrameGaps.Count = uiPerformanceMaxCount + 1
	tooManyBody, _ := json.Marshal(tooMany)
	tooLargeTotal := validUIPerformanceReport()
	tooLargeTotal.Stages[0].TotalMs = uiPerformanceMaxTotalMs + 1
	tooLargeTotalBody, _ := json.Marshal(tooLargeTotal)
	missingRequired := bytes.Replace(valid, []byte(`,"version":1`), nil, 1)
	if bytes.Equal(missingRequired, valid) {
		missingRequired = bytes.Replace(valid, []byte(`"version":1,`), nil, 1)
	}

	for _, test := range []struct {
		name string
		body []byte
	}{
		{name: "oversized body", body: oversized},
		{name: "trailing JSON", body: append(append([]byte(nil), valid...), []byte(` {}`)...)},
		{name: "NaN", body: nan},
		{name: "duration bound", body: tooLongBody},
		{name: "count bound", body: tooManyBody},
		{name: "aggregate total bound", body: tooLargeTotalBody},
		{name: "event limit", body: tooManyEventsBody},
		{name: "missing required field", body: missingRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveUIPerformanceRequest(server, http.MethodPost, test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func validUIPerformanceReport() uiPerformanceReport {
	duration := 80.0
	script := 21.0
	recentInput := false
	name := "scroll_frame"
	return uiPerformanceReport{
		Version:    1,
		DurationMs: 1_500,
		Reason:     "timeout",
		Support: uiPerformanceSupport{
			LongAnimationFrame: true,
			LongTask:           true,
			LayoutShift:        false,
		},
		FrameGaps:           uiPerformanceFrameGaps{Count: 3, MaxMs: 44, P50Ms: 20, P95Ms: 44, Over32Ms: 1},
		ScrollEvents:        12,
		DroppedEvents:       0,
		DroppedFrameSamples: 0,
		Stages:              []uiPerformanceStage{{Name: "scroll_frame", Count: 4, TotalMs: 100, MaxMs: 50}},
		Events:              []uiPerformanceEvent{{Kind: "loaf", Name: &name, AtMs: 240, DurationMs: &duration, ScriptMs: &script, HadRecentInput: &recentInput}},
	}
}

func newUIPerformanceDevServer(t *testing.T) *Server {
	t.Helper()
	server, _ := openLibraryHTTPFixture(t)
	server.config.Dev = true
	server.config.Deployment.Mode = "development"
	return server
}

func serveUIPerformanceRequest(server *Server, method string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, uiPerformancePath, bytes.NewReader(body))
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	return response
}

func postUIPerformanceReport(t *testing.T, server *Server, report uiPerformanceReport) {
	t.Helper()
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	response := serveUIPerformanceRequest(server, http.MethodPost, body)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"saved":true}` {
		t.Fatalf("post status=%d body=%s", response.Code, response.Body.String())
	}
}

func readUIPerformanceSnapshot(t *testing.T, server *Server) uiPerformanceSnapshot {
	t.Helper()
	response := serveUIPerformanceRequest(server, http.MethodGet, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Trace *uiPerformanceSnapshot `json:"trace"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Trace == nil {
		t.Fatal("expected a saved report")
	}
	return *body.Trace
}
