package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const corroborationHTTPReply = `{"schema":"aku.instagram-bootstrap-corroboration.v1","reason":"corroborated","bodyBound":true,"documentBound":true,"serverFresh":true,"idMatched":true,"cookieMatched":true,"scanComplete":true,"qualified":false,"cleanup":"verified"}`
const corroborationHTTPConsent = `{"allowBootstrapCorroboration":true,"acknowledgeBoundedDocumentBody":true,"acknowledgeAccountCookieComparison":true}`

func TestInstagramBootstrapCorroborationOperator(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("development build required")
	}
	calls := 0
	s.SetInstagramBootstrapCorroborationDiagnosticAction(func(_ context.Context, nonce string) (json.RawMessage, error) {
		calls++
		if len(nonce) != 48 {
			t.Fatal("server nonce missing")
		}
		return json.RawMessage(corroborationHTTPReply), nil
	})
	request := func(body, epoch, query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-bootstrap-corroboration"+query, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aku-Split-Epoch", epoch)
		w := httptest.NewRecorder()
		s.security(s.api()).ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{}`, `null`, `{"allowPassiveBootstrapInspection":true}`, strings.Replace(corroborationHTTPConsent, `true`, `false`, 1), strings.TrimSuffix(corroborationHTTPConsent, `}`) + `,"nonce":"PRIVATE"}`, strings.TrimSuffix(corroborationHTTPConsent, `}`) + `,"cookie":"ds_user_id"}`, strings.TrimSuffix(corroborationHTTPConsent, `}`) + `,"allowBootstrapCorroboration":true}`} {
		if w := request(body, s.engine.Epoch(), ""); w.Code != 400 {
			t.Fatalf("invalid consent accepted: %d", w.Code)
		}
	}
	if w := request(corroborationHTTPConsent, "old", ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := request(corroborationHTTPConsent, s.engine.Epoch(), "?url=PRIVATE"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if calls != 0 {
		t.Fatal("rejected request dispatched")
	}
	if w := request(corroborationHTTPConsent, s.engine.Epoch(), ""); w.Code != 200 {
		t.Fatalf("unexpected operator result: %d", w.Code)
	}
	if w := request(corroborationHTTPConsent, s.engine.Epoch(), ""); w.Code != 409 || calls != 1 {
		t.Fatal("one-shot replay dispatched")
	}
}

func TestInstagramBootstrapCorroborationOperatorRejectsUnheldOrPrivateOutput(t *testing.T) {
	for _, held := range []bool{false, true} {
		s := instagramDiagnosticServer(t, held)
		if !s.engine.InstagramReaderDiagnosticConfigured() {
			t.Skip("development build required")
		}
		calls := 0
		s.SetInstagramBootstrapCorroborationDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
			calls++
			return json.RawMessage(strings.TrimSuffix(corroborationHTTPReply, `}`) + `,"cookie":"PRIVATE"}`), nil
		})
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-bootstrap-corroboration", strings.NewReader(corroborationHTTPConsent))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
		w := httptest.NewRecorder()
		s.security(s.api()).ServeHTTP(w, r)
		if w.Code != 409 || strings.Contains(w.Body.String(), "PRIVATE") || !held && calls != 0 || held && calls != 1 {
			t.Fatal("unsafe operator boundary")
		}
	}
}

func TestInstagramBootstrapCorroborationUnavailableWithoutBuildSupport(t *testing.T) {
	s := instagramDiagnosticServer(t, false)
	if s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("default-build negative path")
	}
	calls := 0
	s.SetInstagramBootstrapCorroborationDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		calls++
		return json.RawMessage(corroborationHTTPReply), nil
	})
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-bootstrap-corroboration", strings.NewReader(corroborationHTTPConsent))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	w := httptest.NewRecorder()
	s.security(s.api()).ServeHTTP(w, r)
	if w.Code != 404 || calls != 0 {
		t.Fatal("default build exposed diagnostic")
	}
}

func TestInstagramBootstrapCorroborationTrailingSlashKeepsItsConsentAndLane(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("development build required")
	}
	s.SetInstagramReaderDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		t.Fatal("corroboration route selected legacy reader")
		return nil, nil
	})
	s.SetInstagramBootstrapCorroborationDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		return json.RawMessage(corroborationHTTPReply), nil
	})
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-bootstrap-corroboration/", strings.NewReader(corroborationHTTPConsent))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	w := httptest.NewRecorder()
	s.security(s.api()).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
