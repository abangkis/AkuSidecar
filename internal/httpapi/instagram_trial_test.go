package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstagramTrialConsentIsExact(t *testing.T) {
	for _, body := range []string{`{}`, `{"acknowledgeLimitations":true,"allowProcessSessionDerivation":true}`, `{"acknowledgeLimitations":true,"allowProcessSessionDerivation":true,"acknowledgeUnqualifiedRecipe":true,"acknowledgeUnqualifiedRecipe":true}`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if _, err := readConsentFields(r, []string{"acknowledgeLimitations", "allowProcessSessionDerivation", "acknowledgeUnqualifiedRecipe"}); err == nil {
			t.Fatal("ambiguous trial consent accepted")
		}
	}
}
func TestInstagramTrialRoutesRemainDevelopmentAndEpochBound(t *testing.T) {
	for _, trial := range []bool{false, true} {
		s, _ := splitTestServer(t)
		s.config.Dev = true
		s.config.Capture.InstagramTrial = trial
		for _, action := range []string{"start", "stop"} {
			r := httptest.NewRequest("POST", "http://127.0.0.1:11122/api/development/instagram-trial/"+action, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "http://127.0.0.1:11122")
			r.Header.Set("X-Aku-Split-Epoch", "old-process")
			w := httptest.NewRecorder()
			s.security(s.api()).ServeHTTP(w, r)
			want := http.StatusNotFound
			if trial {
				want = http.StatusConflict
			}
			if w.Code != want {
				t.Fatalf("route status %d", w.Code)
			}
		}
		w := httptest.NewRecorder()
		s.security(s.api()).ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:11122/api/collection/runtime", nil))
		var reply map[string]json.RawMessage
		if json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply["instagramCapture"] == nil {
			t.Fatal("IG runtime status missing")
		}
	}
}
