package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestXManualStartConsentAndSecurity(t *testing.T) {
	s, _ := openLibraryHTTPFixture(t)
	for _, tc := range []struct {
		name, body, origin, epoch string
		dev                       bool
		code                      int
	}{
		{"nondev", `{}`, "", "", false, 404},
		{"missing_epoch", `{}`, "", "", true, 409},
		{"foreign_origin", `{}`, "https://example.com", s.engine.Epoch(), true, 403},
		{"missing_consent", `{}`, "", s.engine.Epoch(), true, 400},
		{"unknown_field", `{"acknowledgeLimitations":true,"allowOneSessionDerivation":true,"profile":"outside"}`, "", s.engine.Epoch(), true, 400},
		{"duplicate", `{"acknowledgeLimitations":false,"acknowledgeLimitations":true,"allowOneSessionDerivation":true}`, "", s.engine.Epoch(), true, 400},
		{"trailing", `{"acknowledgeLimitations":true,"allowOneSessionDerivation":true}{}`, "", s.engine.Epoch(), true, 400},
		{"oversized", strings.Repeat(" ", 1025) + `{}`, "", s.engine.Epoch(), true, 400},
		{"false_consent", `{"acknowledgeLimitations":false,"allowOneSessionDerivation":true}`, "", s.engine.Epoch(), true, 409},
		{"no_provider", `{"acknowledgeLimitations":true,"allowOneSessionDerivation":true}`, "", s.engine.Epoch(), true, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.config.Dev = tc.dev
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/development/x-manual/start", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Aku-Split-Epoch", tc.epoch)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			s.security(s.api()).ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}
