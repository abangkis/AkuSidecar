package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type xDevelopmentFixture struct{ s *Server }

func newXDevelopmentFixture(t *testing.T, concurrent bool) xDevelopmentFixture {
	t.Helper()
	root := t.TempDir()
	state, err := store.Open(filepath.Join(root, "x-development.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Dev: true, Root: root, Server: config.ServerConfig{Host: "127.0.0.1", Port: 11122}}
	if concurrent {
		cfg.Capture.XMode = config.XCaptureConcurrent
	}
	logger := log.New(io.Discard, "", 0)
	e := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	s, err := New(cfg, state, e, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s.splitCapture != nil {
			s.splitCapture.close()
		}
		e.Shutdown()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("X development fixture did not drain")
		}
	})
	return xDevelopmentFixture{s}
}

func (f xDevelopmentFixture) request(action, body string) *http.Request {
	r := sessionUpdatesRequest(http.MethodPost, "/api/development/x-"+action, body)
	r.Header.Set("X-Aku-Split-Epoch", f.s.engine.Epoch())
	return r
}

func (f xDevelopmentFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f xDevelopmentFixture) error(t *testing.T, w *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	(sessionUpdatesFixture{s: f.s}).error(t, w, status, code, message)
	if message != "" {
		var payload struct{ Message string }
		if json.Unmarshal(w.Body.Bytes(), &payload) != nil || payload.Message != message {
			t.Fatalf("error message changed: %s", w.Body)
		}
	}
}

func (f xDevelopmentFixture) success(t *testing.T, w *httptest.ResponseRecorder, daily bool) {
	t.Helper()
	payload := (sessionUpdatesFixture{s: f.s}).json(t, w, 200)
	key, value := "xManual", any(f.s.engine.XManualStatus())
	if daily {
		key, value = "xCapture", f.s.engine.XCaptureStatus()
	}
	want, err := json.Marshal(value)
	if err != nil || len(payload) != 1 || string(payload[key]) != string(want) {
		t.Fatalf("status envelope changed: %s", w.Body)
	}
}

type xDevelopmentBody struct {
	io.Reader
	reads, closes int
}

func (b *xDevelopmentBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *xDevelopmentBody) Close() error               { b.closes++; return nil }

func TestXDevelopmentCharacterizationAdmissionAndRouting(t *testing.T) {
	for _, action := range []string{"daily/start", "daily/stop", "manual/start", "manual/stop"} {
		t.Run(action, func(t *testing.T) {
			f := newXDevelopmentFixture(t, true)
			for _, scenario := range []string{"foreign_host", "foreign_origin", "extension_origin", "nondev", "missing_epoch", "stale_epoch", "wrong_method", "unknown_path", "preflight"} {
				t.Run(scenario, func(t *testing.T) {
					f.s.config.Dev = true
					r := f.request(action, `{}`)
					body := &xDevelopmentBody{Reader: strings.NewReader(`{}`)}
					r.Body = body
					status, code := 409, "capture_epoch_mismatch"
					switch scenario {
					case "foreign_host":
						r.Host, status = "example.com", 403
					case "foreign_origin":
						r.Header.Set("Origin", "https://example.com")
						status = 403
					case "extension_origin":
						r.Header.Set("Origin", "chrome-extension://trusted")
						f.s.config.Bridge.TrustedExtensionOrigins = []string{"chrome-extension://trusted"}
						status = 403
					case "nondev":
						f.s.config.Dev, status, code = false, 404, "not_found"
						r.Header.Del("X-Aku-Split-Epoch")
					case "missing_epoch":
						r.Header.Del("X-Aku-Split-Epoch")
					case "stale_epoch":
						r.Header.Set("X-Aku-Split-Epoch", "old-process")
					case "wrong_method":
						r.Method, status, code = http.MethodGet, 404, "not_found"
					case "unknown_path":
						r.URL.Path += "/extra"
						status, code = 404, "not_found"
					case "preflight":
						r.Method, status = http.MethodOptions, 204
						f.s.config.Dev = false
						r.Header.Del("X-Aku-Split-Epoch")
					}
					before := f.s.engine.XCaptureStatus()
					w := f.serve(r)
					if w.Code != status || body.reads != 0 || body.closes != 0 || !reflect.DeepEqual(before, f.s.engine.XCaptureStatus()) {
						t.Fatalf("admission read/mutated before rejection: status=%d body=%s reads=%d closes=%d", w.Code, w.Body, body.reads, body.closes)
					}
					if status == 403 {
						if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
							t.Fatal("security wrapper order changed")
						}
					} else if status == 204 {
						if w.Body.Len() != 0 || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
							t.Fatal("preflight envelope changed")
						}
					} else {
						f.error(t, w, status, code, "")
					}
				})
			}
		})
	}
}

func TestXDevelopmentCharacterizationStartConsent(t *testing.T) {
	for _, kind := range []string{"daily", "manual"} {
		t.Run(kind, func(t *testing.T) {
			f := newXDevelopmentFixture(t, true)
			permission, other := "allowProcessSessionDerivation", "allowOneSessionDerivation"
			if kind == "manual" {
				permission, other = other, permission
			}
			valid := `{"acknowledgeLimitations":true,"` + permission + `":true}`
			for _, tc := range []struct {
				name, body, contentType string
				status                  int
			}{
				{"valid_unavailable", valid, "application/json", 409},
				{"false", strings.Replace(valid, "true", "false", 1), "application/json", 409},
				{"missing", `{}`, "application/json", 400},
				{"wrong_scope", strings.Replace(valid, permission, other, 1), "application/json", 400},
				{"unknown", strings.TrimSuffix(valid, "}") + `,"profile":"outside"}`, "application/json", 400},
				{"duplicate", strings.TrimSuffix(valid, "}") + `,"` + permission + `":false}`, "application/json", 400},
				{"trailing", valid + `{}`, "application/json", 400},
				{"malformed", `{`, "application/json", 400},
				{"wrong_type", `{"acknowledgeLimitations":"true","` + permission + `":true}`, "application/json", 400},
				{"null_field", `{"acknowledgeLimitations":null,"` + permission + `":true}`, "application/json", 409},
				{"exact_limit", strings.Repeat(" ", 1024-len(valid)) + valid, "application/json", 409},
				{"oversized", strings.Repeat(" ", 1025-len(valid)) + valid, "application/json", 400},
				{"media_type", valid, "text/plain", 415},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := f.request(kind+"/start", tc.body)
					r.Header.Set("Content-Type", tc.contentType)
					body := &xDevelopmentBody{Reader: strings.NewReader(tc.body)}
					r.Body = body
					before := f.s.engine.XCaptureStatus()
					w := f.serve(r)
					code, message := "invalid_request", ""
					if tc.status == 409 {
						code, message = "daily_start_unavailable", "Daily X startup failed or consent was consumed; inspect runtime status."
						if kind == "manual" {
							code, message = "manual_start_unavailable", "Manual X start was rejected or failed; inspect runtime status and cleanup."
						}
					} else if tc.status == 415 {
						code, message = "unsupported_media_type", "Content-Type must be application/json"
					}
					f.error(t, w, tc.status, code, message)
					if body.closes != 1 || !reflect.DeepEqual(before, f.s.engine.XCaptureStatus()) || f.s.engine.XManualStatus().Available {
						t.Fatal("failed consent changed authority or body lifetime")
					}
				})
			}
		})
	}
}

func TestXDevelopmentCharacterizationStopParsingAndProjection(t *testing.T) {
	for _, kind := range []string{"daily", "manual"} {
		for _, suffix := range []string{"", "/", "%2F"} {
			t.Run(kind+map[string]string{"": "/canonical", "/": "/slash", "%2F": "/encoded_slash"}[suffix], func(t *testing.T) {
				f := newXDevelopmentFixture(t, false)
				for _, tc := range []struct {
					name, body, contentType string
					status                  int
				}{
					{"empty_object", `{}`, "application/json", 200},
					{"null", `null`, "application/json", 200},
					{"trailing_existing_behavior", `{} {}`, "application/json", 200},
					{"malformed", `{`, "application/json", 400},
					{"unknown", `{"extra":true}`, "application/json", 400},
					{"media_type", `{}`, "text/plain", 415},
				} {
					t.Run(tc.name, func(t *testing.T) {
						r := f.request(kind+"/stop"+suffix, tc.body)
						r.Header.Set("Content-Type", tc.contentType)
						w := f.serve(r)
						if tc.status == 200 {
							f.success(t, w, kind == "daily")
						} else {
							code := "invalid_request"
							if tc.status == 415 {
								code = "unsupported_media_type"
							}
							f.error(t, w, tc.status, code, "")
						}
					})
				}
			})
		}
	}
}

func TestXDevelopmentCharacterizationStopSideEffects(t *testing.T) {
	for _, kind := range []string{"daily", "manual"} {
		t.Run(kind, func(t *testing.T) {
			f := newXDevelopmentFixture(t, true)
			before, err := f.s.store.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			// Invalid input cannot invoke cleanup. The current valid-input daily
			// failure still stops admission before reporting the missing scope.
			initial := f.s.engine.XCaptureStatus()
			f.error(t, f.serve(f.request(kind+"/stop", `{"extra":true}`)), 400, "invalid_request", "")
			if !reflect.DeepEqual(initial, f.s.engine.XCaptureStatus()) {
				t.Fatal("invalid stop body changed runtime")
			}
			for repeat := 0; repeat < 2; repeat++ {
				w := f.serve(f.request(kind+"/stop", `{}`))
				if kind == "daily" {
					f.error(t, w, 409, "daily_cleanup_uncertain", "Daily X cleanup has not been verified.")
				} else {
					f.success(t, w, false)
				}
				status := f.s.engine.XCaptureStatus()
				if status.Reason != "opted-out" || status.DerivationAuthorized {
					t.Fatal("stop lost its existing partial effect", status)
				}
			}
			after, err := f.s.store.GetSettings(context.Background())
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("stop changed ordinary settings or Auto Update")
			}
		})
	}
}

func TestXDevelopmentCharacterizationContextAndWriteFailure(t *testing.T) {
	for _, kind := range []string{"daily", "manual"} {
		t.Run(kind, func(t *testing.T) {
			for _, expired := range []bool{false, true} {
				t.Run(map[bool]string{false: "cancelled", true: "deadline"}[expired], func(t *testing.T) {
					f := newXDevelopmentFixture(t, false)
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					if expired {
						ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
						defer cancel()
					}
					permission := "allowProcessSessionDerivation"
					if kind == "manual" {
						permission = "allowOneSessionDerivation"
					}
					r := f.request(kind+"/start", `{"acknowledgeLimitations":true,"`+permission+`":true}`).WithContext(ctx)
					f.error(t, f.serve(r), 409, kind+"_start_unavailable", "")
					// An unconfigured stop is a successful no-op even with an ended
					// caller context; no new blanket cancellation gate is implied.
					f.success(t, f.serve(f.request(kind+"/stop", `{}`).WithContext(ctx)), kind == "daily")
				})
			}
			f := newXDevelopmentFixture(t, true)
			failure := errors.New("x-development-response-write-failed")
			w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
			if kind == "manual" {
				if err := f.s.route(w, f.request("manual/stop", `{}`)); !errors.Is(err, failure) || w.status != 200 || f.s.engine.XCaptureStatus().Reason != "opted-out" {
					t.Fatal("response failure lost the stop side effect", w.status, err)
				}
			} else {
				// The real concurrent daily failure maps to its existing closed
				// error before any response write, while preserving its stop effect.
				err := f.s.route(w, f.request("daily/stop", `{}`))
				var api apiError
				if !errors.As(err, &api) || api.Code != "daily_cleanup_uncertain" || w.status != 0 || f.s.engine.XCaptureStatus().Reason != "opted-out" {
					t.Fatal("daily cleanup error/response order changed", err)
				}
				plain := newXDevelopmentFixture(t, false)
				if err := plain.s.route(w, plain.request("daily/stop", `{}`)); !errors.Is(err, failure) || w.status != 200 {
					t.Fatal("daily response failure was swallowed", w.status, err)
				}
			}
		})
	}
}
