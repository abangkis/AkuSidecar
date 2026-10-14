//go:build windows && auth_session_dev && auth_session_fixture

package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

const DevelopmentFrontendTitle = "AkuBrowser QA - Native activation DISABLED"
const DevelopmentFrontendNativeTitle = "AkuBrowser QA - Synthetic native reader"

type DevelopmentFrontendSnapshot struct {
	Bootstrap, AppAsset, RelayAsset, Runtime, NativeDenied, Cancelled int
	NativeRequests, NativeAccepted                                    int
	RequestID                                                         string
}
type DevelopmentFrontendObservation struct {
	mu    sync.Mutex
	value DevelopmentFrontendSnapshot
}

func (o *DevelopmentFrontendObservation) Snapshot() DevelopmentFrontendSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.value
}

// ObserveDevelopmentFrontend collects fixed request metadata only, never bridge
// credentials, URLs or content. It wraps the real server, not a second backend.
func (s *Server) ObserveDevelopmentFrontend() (*DevelopmentFrontendObservation, error) {
	if !s.config.Dev || s.splitCapture == nil || s.listener != nil {
		return nil, errors.New("frontend observation requires closed unstarted fixture")
	}
	observation := &DevelopmentFrontendObservation{}
	original := s.http.Handler
	s.http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var action struct{ Type, RequestID string }
		if r.Method == http.MethodPost && (r.URL.Path == "/api/split-capture/actions" || r.URL.Path == "/api/split-capture/reader-broker/cancel") {
			body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
			if err != nil || len(body) > 65536 {
				http.Error(w, "invalid fixture request", 400)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			_ = json.Unmarshal(body, &action)
		}
		if r.URL.Path == "/api/split-capture/actions" || r.URL.Path == "/api/split-capture/reader-broker/cancel" {
			if action.Type == "open_native_post" {
				observation.mu.Lock()
				observation.value.NativeRequests++
				observation.value.RequestID = action.RequestID
				observation.mu.Unlock()
			}
			recorded := httptest.NewRecorder()
			original.ServeHTTP(recorded, r)
			for k, values := range recorded.Header() {
				for _, value := range values {
					w.Header().Add(k, value)
				}
			}
			w.WriteHeader(recorded.Code)
			_, _ = w.Write(recorded.Body.Bytes())
			observation.mu.Lock()
			defer observation.mu.Unlock()
			if action.Type == "open_native_post" && recorded.Code == 409 && strings.Contains(recorded.Body.String(), "development_native_activation_closed") {
				observation.value.NativeDenied++
				observation.value.RequestID = action.RequestID
			}
			if action.Type == "open_native_post" && recorded.Code == 200 {
				var result struct {
					OK     bool
					Result struct{ State, Foreground string }
				}
				if json.Unmarshal(recorded.Body.Bytes(), &result) == nil && result.OK && result.Result.State == "native_post_opened" && result.Result.Foreground == "" {
					observation.value.NativeAccepted++
				}
			}
			if r.URL.Path == "/api/split-capture/reader-broker/cancel" && recorded.Code == 200 && action.RequestID == observation.value.RequestID {
				observation.value.Cancelled++
			}
			return
		}
		original.ServeHTTP(w, r)
		observation.mu.Lock()
		defer observation.mu.Unlock()
		switch r.URL.Path {
		case "/api/bootstrap":
			observation.value.Bootstrap++
		case "/app.js":
			observation.value.AppAsset++
		case "/split-ui-bridge.js":
			observation.value.RelayAsset++
		case "/api/collection/runtime":
			observation.value.Runtime++
		}
	})
	return observation, nil
}

// CloseDevelopmentFrontendNativeActivation can only close native admission.
// It installs no broker or activation callback. The production route still
// checks bridge credentials, epoch and typed action before the closed gate.
func (s *Server) CloseDevelopmentFrontendNativeActivation() error {
	if !s.config.Dev || !s.config.WindowsCaptureSplit || s.splitCapture == nil || s.listener != nil {
		return errors.New("development frontend requires an unstarted isolated split server")
	}
	s.developmentNativeReaderClosed = true
	return s.LabelDevelopmentFrontend(false)
}

func (s *Server) LabelDevelopmentFrontend(native bool) error {
	if !s.config.Dev || s.splitCapture == nil || s.listener != nil {
		return errors.New("development presentation requires unstarted isolated split server")
	}
	title := DevelopmentFrontendTitle
	if native {
		title = DevelopmentFrontendNativeTitle
	}
	original := s.http.Handler
	s.http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || (r.URL.Path != "/" && r.URL.Path != "/index.html") {
			original.ServeHTTP(w, r)
			return
		}
		recorded := httptest.NewRecorder()
		original.ServeHTTP(recorded, r)
		for k, values := range recorded.Header() {
			for _, value := range values {
				w.Header().Add(k, value)
			}
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(recorded.Code)
		// Test-only presentation metadata, never a replacement link or event.
		body := bytes.Replace(recorded.Body.Bytes(), []byte("<title>AkuBrowser</title>"), []byte("<title>"+title+"</title>"), 1)
		_, _ = w.Write(body)
	})
	return nil
}

func developmentFrontendTitlePresent(body string) bool {
	return strings.Contains(body, "<title>"+DevelopmentFrontendTitle+"</title>")
}
