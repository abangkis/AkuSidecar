package httpapi

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/capturestartup"
)

func TestCaptureStartupRequiresDevelopmentAndCurrentHost(t *testing.T) {
	s, _ := splitTestServer(t)
	const path = "/api/bridge/split-capture/startup-sample"
	const sample = `{"doc":1,"phase":"early","reload":false,"scriptStarted":false,"workerReceived":false}`
	key := s.splitCapture.key
	post := func(key, body string) int { return splitRequest(s, "", key, "POST", path, body).Code }
	host := func() string {
		w := httptest.NewRecorder()
		s.serveSplitCaptureAsset(w, httptest.NewRequest("GET", "/split-capture-host", nil))
		return w.Body.String()
	}
	if post(key, sample) != 404 || strings.Contains(host(), "split-capture-startup.js") {
		t.Fatal("development diagnostic exposed by default")
	}
	var b bytes.Buffer
	s.captureStartup = capturestartup.New(log.New(&b, "", 0))
	if !strings.Contains(host(), "split-capture-startup.js") {
		t.Fatal("host probe missing")
	}
	if post("stale", sample) != 403 || post(key, `{"phase":"early"}`) != 400 || post(key, sample) != 204 || post(key, sample) != 204 {
		t.Fatal("unexpected sample admission")
	}
	if strings.Count(b.String(), "phase=early_sample") != 1 || !strings.Contains(b.String(), "script_started=false worker_receipt=false") || strings.Contains(b.String(), key) {
		t.Fatal("unexpected diagnostic projection")
	}
}
