package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstagramBootstrapOperator(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("development build required")
	}
	calls := 0
	const reply = `{"schema":"aku.instagram-self-bootstrap-probe.v2","reason":"missing","shapeReason":"none","usernameReason":"none","modulePresent":false,"idString":false,"idSafeNumber":false,"usernameString":false,"duplicateDetected":false,"conflictDetected":false,"documentBound":true,"serverFresh":false,"qualified":false,"scanComplete":true,"budgetExpanded":false,"failureStage":"none","failureCondition":"none","cleanup":"verified","cleanupStage":"none","cleanupCondition":"none"}`
	s.SetInstagramBootstrapDiagnosticAction(func(context.Context, string) (json.RawMessage, error) { calls++; return json.RawMessage(reply), nil })
	request := func(body, epoch string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-bootstrap", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aku-Split-Epoch", epoch)
		w := httptest.NewRecorder()
		s.security(s.api()).ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{}`, `{"allowPassiveBootstrapInspection":false}`, `{"allowPassiveBootstrapInspection":true,"url":"PRIVATE"}`} {
		if w := request(body, s.engine.Epoch()); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if w := request(`{"allowPassiveBootstrapInspection":true}`, "old"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if calls != 0 {
		t.Fatal("rejected request dispatched")
	}
	if w := request(`{"allowPassiveBootstrapInspection":true}`, s.engine.Epoch()); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w := request(`{"allowPassiveBootstrapInspection":true}`, s.engine.Epoch()); w.Code != 409 || calls != 1 {
		t.Fatal("replay admitted")
	}
}
