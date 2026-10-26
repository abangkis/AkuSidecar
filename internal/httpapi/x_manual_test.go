package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXManualStatusAndStopDefaultOff(t *testing.T) {
	s, _ := openLibraryHTTPFixture(t)
	get := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/collection/runtime", nil)
	out := httptest.NewRecorder()
	s.security(s.api()).ServeHTTP(out, get)
	if out.Code != 200 {
		t.Fatalf("status %d", out.Code)
	}
	var status struct {
		XManual struct {
			State     string
			Available bool
			Source    string
		}
	}
	if json.Unmarshal(out.Body.Bytes(), &status) != nil || status.XManual.State != "disabled" || status.XManual.Available || status.XManual.Source != "x" {
		t.Fatal("default route not disabled")
	}
	for _, tc := range []struct {
		name          string
		dev           bool
		epoch, origin string
		code          int
	}{
		{"nondev", false, "", "", 404},
		{"missing epoch", true, "", "", 409},
		{"same instance", true, s.engine.Epoch(), "http://127.0.0.1", 200},
		{"foreign origin", true, s.engine.Epoch(), "https://example.com", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.config.Dev = tc.dev
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/development/x-manual/stop", bytes.NewBufferString("{}"))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Aku-Split-Epoch", tc.epoch)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			s.security(s.api()).ServeHTTP(w, r)
			if w.Code != tc.code {
				t.Fatalf("stop %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
