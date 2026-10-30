package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/activationguard"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

const instagramDiagnosticConsent = `{"allowAuthenticatedReaderDiagnostic":true,"acknowledgeBoundedErrorBody":true}`
const instagramDiagnosticReply = `{"schema":"aku.instagram-reader-diagnostic.v1","primaryCause":"http_rejected","status":400,"mime":"json","bodyClass":"json_object","errorCategory":"unknown","documentBound":true,"wireMatched":true,"cleanup":"verified"}`

type instagramDiagnosticHTTPProcess struct {
	done chan error
	once sync.Once
}

func (p *instagramDiagnosticHTTPProcess) Done() <-chan error { return p.done }
func (p *instagramDiagnosticHTTPProcess) Terminate()         { p.once.Do(func() { close(p.done) }) }
func (*instagramDiagnosticHTTPProcess) CloseForRetry(context.Context) error {
	return errors.New("unexpected replacement")
}
func (*instagramDiagnosticHTTPProcess) PID() int { return 1234 }
func (*instagramDiagnosticHTTPProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("unexpected foreground")
}
func (*instagramDiagnosticHTTPProcess) ReplacementReadiness(context.Context) error { return nil }

func instagramDiagnosticServer(t *testing.T, held bool) *Server {
	t.Helper()
	s, _ := splitTestServer(t)
	s.config.Dev = true
	s.config.Deployment.Mode = "development"
	s.config.WindowsCaptureSplit = true
	s.config.Capture.XMode = config.XCaptureConcurrent
	s.config.Capture.XReaderSource = "main"
	s.config.Capture.InstagramTrial = true
	s.engine = engine.New(s.store, reasoning.Deterministic{}, s.config, s.logger)
	process := &instagramDiagnosticHTTPProcess{done: make(chan error)}
	owner, err := captureruntime.New(process)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Terminate)
	if err = owner.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	if err = s.engine.AttachCaptureRuntime(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	s.engine.AttachCollectionCoordinator(collection.NewCoordinator(owner, nil, func() error { return nil }))
	if held {
		path := filepath.Join(t.TempDir(), "diagnostic-hold.json")
		if err = os.WriteFile(path, []byte(`{"schema":"aku.native-derivation-hold.v1","activationId":"`+strings.Repeat("a", 32)+`","sources":["x","instagram"]}`), 0600); err != nil {
			t.Fatal(err)
		}
		guard, err := activationguard.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.engine.BindNativeDerivationHold(guard); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func instagramDiagnosticRequest(s *Server, body, epoch string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/diagnostics/instagram-reader", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Split-Epoch", epoch)
	w := httptest.NewRecorder()
	s.security(s.api()).ServeHTTP(w, r)
	return w
}

func TestInstagramDiagnosticOperatorStrictConsentAndEpoch(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("positive operator path requires supported development build")
	}
	calls := 0
	s.SetInstagramReaderDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		calls++
		return json.RawMessage(instagramDiagnosticReply), nil
	})
	for _, body := range []string{`{}`, `null`, strings.Replace(instagramDiagnosticConsent, `true`, `false`, 1), strings.TrimSuffix(instagramDiagnosticConsent, "}") + `,"nonce":"PRIVATE"}`, strings.TrimSuffix(instagramDiagnosticConsent, "}") + `,"allowAuthenticatedReaderDiagnostic":true}`, instagramDiagnosticConsent + ` {}`, `{"allowAuthenticatedReaderDiagnostic":null,"acknowledgeBoundedErrorBody":true}`} {
		if w := instagramDiagnosticRequest(s, body, s.engine.Epoch()); w.Code != 400 {
			t.Fatalf("malformed body accepted: %d", w.Code)
		}
	}
	if w := instagramDiagnosticRequest(s, instagramDiagnosticConsent, "stale"); w.Code != 409 {
		t.Fatal("stale epoch accepted")
	}
	if calls != 0 {
		t.Fatal("rejected request reached browser")
	}
}

func TestInstagramDiagnosticOperatorOneShotSafeProjectionAndCleanup(t *testing.T) {
	for _, cleanup := range []string{"verified", "uncertain"} {
		t.Run(cleanup, func(t *testing.T) {
			s := instagramDiagnosticServer(t, true)
			if !s.engine.InstagramReaderDiagnosticConfigured() {
				t.Skip("supported development build only")
			}
			before, _ := s.store.GetSettings(context.Background())
			calls := 0
			s.SetInstagramReaderDiagnosticAction(func(_ context.Context, nonce string) (json.RawMessage, error) {
				calls++
				if len(nonce) != 48 || strings.Trim(nonce, "0123456789abcdef") != "" {
					t.Fatal("server nonce invalid")
				}
				return json.RawMessage(strings.Replace(instagramDiagnosticReply, "verified", cleanup, 1)), nil
			})
			w := instagramDiagnosticRequest(s, instagramDiagnosticConsent, s.engine.Epoch())
			if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || !strings.Contains(w.Body.String(), `"cleanup":"`+cleanup+`"`) {
				t.Fatalf("safe response %d %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "nonce") || w.Body.Len() > 1024 {
				t.Fatal("operator response escaped closed projection")
			}
			if again := instagramDiagnosticRequest(s, instagramDiagnosticConsent, s.engine.Epoch()); again.Code != 409 || calls != 1 {
				t.Fatal("one-shot replay accepted")
			}
			after, _ := s.store.GetSettings(context.Background())
			if !reflect.DeepEqual(before, after) {
				t.Fatal("operator changed settings")
			}
		})
	}
}

func TestInstagramDiagnosticOperatorFailsClosedAndSuppressesPrivateErrors(t *testing.T) {
	for _, mode := range []string{"off_build_or_config", "no_hold", "no_action", "private_result", "private_error"} {
		t.Run(mode, func(t *testing.T) {
			s := instagramDiagnosticServer(t, mode != "no_hold")
			if mode == "off_build_or_config" {
				cfg := s.config
				cfg.Dev = false
				s.engine = engine.New(s.store, reasoning.Deterministic{}, cfg, s.logger)
			} else if !s.engine.InstagramReaderDiagnosticConfigured() {
				t.Skip("supported development build only")
			}
			calls := 0
			if mode != "no_action" {
				s.SetInstagramReaderDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
					calls++
					if mode == "private_result" {
						return json.RawMessage(`{"account":"PRIVATE_ACCOUNT"}`), nil
					}
					return nil, errors.New("PRIVATE_TOKEN")
				})
			}
			w := instagramDiagnosticRequest(s, instagramDiagnosticConsent, s.engine.Epoch())
			want := 409
			if mode == "off_build_or_config" {
				want = 404
			}
			if w.Code != want || strings.Contains(w.Body.String(), "PRIVATE") {
				t.Fatalf("unsafe failure %d %s", w.Code, w.Body)
			}
			if (mode == "off_build_or_config" || mode == "no_hold" || mode == "no_action") && calls != 0 {
				t.Fatal("rejected admission dispatched")
			}
		})
	}
}

func TestInstagramDiagnosticOperatorRejectsConcurrentWorkAndForeignOrigin(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("supported development build only")
	}
	calls := 0
	s.SetInstagramReaderDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		calls++
		return json.RawMessage(instagramDiagnosticReply), nil
	})
	for _, variant := range []string{"origin", "query", "method"} {
		url, method := "http://127.0.0.1:11122/api/diagnostics/instagram-reader", http.MethodPost
		if variant == "query" {
			url += "?url=https://outside.invalid"
		}
		if variant == "method" {
			method = http.MethodGet
		}
		r := httptest.NewRequest(method, url, strings.NewReader(instagramDiagnosticConsent))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
		if variant == "origin" {
			r.Header.Set("Origin", "https://outside.invalid")
		}
		w := httptest.NewRecorder()
		s.security(s.api()).ServeHTTP(w, r)
		if w.Code < 400 || calls != 0 {
			t.Fatal("foreign or nonfixed request dispatched")
		}
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := s.engine.RunInstagramReaderDiagnostic(context.Background(), func(context.Context) (json.RawMessage, error) {
			close(entered)
			<-release
			return json.RawMessage(instagramDiagnosticReply), nil
		})
		done <- err
	}()
	<-entered
	w := instagramDiagnosticRequest(s, instagramDiagnosticConsent, s.engine.Epoch())
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusConflict || calls != 0 || !strings.Contains(w.Body.String(), "instagram_diagnostic_busy") {
		t.Fatal("concurrent diagnostic bypassed operation admission")
	}
}

func TestInstagramDiagnosticOperatorUnavailableWithoutBuildSupport(t *testing.T) {
	s := instagramDiagnosticServer(t, true)
	if s.engine.InstagramReaderDiagnosticConfigured() {
		t.Skip("off-build assertion")
	}
	s.SetInstagramReaderDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		t.Fatal("off-build action dispatched")
		return nil, nil
	})
	if w := instagramDiagnosticRequest(s, instagramDiagnosticConsent, s.engine.Epoch()); w.Code != http.StatusNotFound {
		t.Fatal("unsupported build exposed operator route")
	}
}
