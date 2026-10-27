package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestXDailyStatusAndReaderNoFallback(t *testing.T) {
	s, token := splitTestServer(t)
	cfg := s.config
	cfg.Dev = true
	cfg.Capture.XMode = config.XCaptureConcurrent
	s.engine = engine.New(s.store, reasoning.Deterministic{}, cfg, s.logger)
	out := httptest.NewRecorder()
	s.security(s.api()).ServeHTTP(out, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11122/api/collection/runtime", nil))
	var status struct{ XCapture engine.XCaptureStatus }
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &status) != nil || status.XCapture.SelectedMode != "concurrent" || status.XCapture.Reason != "batch-adapter-required" || status.XCapture.DerivationAuthorized {
		t.Fatalf("status %d %s", out.Code, out.Body.String())
	}
	for _, kind := range []string{"open_source", "open_native_post"} {
		body := `{"type":"` + kind + `","source":"x","url":"https://x.com/synthetic/status/1","requestId":"broker_` + strings.Repeat("a", 32) + `"}`
		w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", body)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "x_concurrent_not_ready") {
			t.Fatalf("%s: %d %s", kind, w.Code, w.Body.String())
		}
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
	if len(s.splitCapture.actions) != 0 {
		t.Fatal("unavailable daily reader entered ordinary queue")
	}
}
