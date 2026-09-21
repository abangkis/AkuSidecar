package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// This transport is instantiated only by the Windows app-shell feature gate.
// It is deliberately ephemeral: Sidecar epoch + random capture-instance key
// fence old workers, and at-most-once claims never replay an interactive action.
const splitActionLimit = 32
const splitActionTimeout = 115 * time.Second

//go:embed split_ui_bridge.js
var splitUIBridge []byte

type splitCaptureAction struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Source       string   `json:"source,omitempty"`
	URL          string   `json:"url,omitempty"`
	RunID        string   `json:"runId,omitempty"`
	LeaseID      string   `json:"leaseId,omitempty"`
	RecaptureID  string   `json:"recaptureId,omitempty"`
	ActionID     string   `json:"actionId,omitempty"`
	RequestID    string   `json:"requestId,omitempty"`
	CandidateIDs []string `json:"candidateIds,omitempty"`
}
type splitActionResult struct {
	OK      bool            `json:"ok"`
	Message string          `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}
type pendingSplitAction struct {
	action                   splitCaptureAction
	claimed                  bool
	result                   chan splitActionResult
	readerPreparing          bool
	readerForeground         func(context.Context) error
	readerForegroundVerified bool
	brokerReady              chan struct{}
	brokerDone               chan error
	brokerAttached           bool
	brokerTarget             readerbroker.Target
}
type splitCaptureTransport struct {
	mu                  sync.Mutex
	key                 string
	closed              bool
	actions             []*pendingSplitAction
	wake                chan struct{}
	done                chan struct{}
	prepareReader       func(context.Context, string) (func(context.Context) error, error)
	prepareBrokerReader func(context.Context, string) (readerbroker.Target, func(context.Context) error, error)
}

func (s *Server) SetSplitReaderBroker(prepare func(context.Context, string) (readerbroker.Target, func(context.Context) error, error)) {
	if s.splitCapture == nil {
		return
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
	s.splitCapture.prepareBrokerReader = prepare
}

// HandleReaderBroker is called only by the OS-authenticated native pipe server.
// The collector has no route to this entry point and never receives a ticket.
func (s *Server) HandleReaderBroker(ctx context.Context, req readerbroker.Request, activate func(readerbroker.Target) (readerbroker.Reply, error)) error {
	if err := req.Validate(); err != nil {
		return err
	}
	t := s.splitCapture
	if t == nil {
		return errors.New("reader broker disabled")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var entry *pendingSplitAction
	for entry == nil {
		t.mu.Lock()
		for _, a := range t.actions {
			if a.action.RequestID == req.RequestID && a.action.Type == "open_native_post" && a.action.Source == req.Source && a.action.URL == req.URL && a.brokerReady != nil && !a.brokerAttached {
				a.brokerAttached = true
				entry = a
				break
			}
		}
		closed := t.closed
		t.mu.Unlock()
		if closed {
			return errors.New("reader broker stopped")
		}
		if entry != nil {
			select {
			case t.wake <- struct{}{}:
			default:
			}
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	var outcome error
	defer func() {
		select {
		case entry.brokerDone <- outcome:
		default:
		}
	}()
	select {
	case <-ctx.Done():
		outcome = ctx.Err()
		return outcome
	case <-entry.brokerReady:
	}
	t.mu.Lock()
	target, verify := entry.brokerTarget, entry.readerForeground
	entry.readerForeground = nil
	t.mu.Unlock()
	if target.HWND == 0 || verify == nil || time.Now().After(target.Expires) {
		outcome = errors.New("reader binding unavailable or expired")
		return outcome
	}
	result, err := activate(target)
	if err != nil {
		outcome = err
	} else if !result.OK || !result.Readback {
		outcome = errors.New("reader helper activation rejected")
	} else {
		outcome = verify(ctx)
	}
	s.logger.Printf("reader_broker action=%s applied=%t readback=%t verified=%t", entry.action.ID, result.Applied, result.Readback, outcome == nil)
	if outcome == nil {
		t.mu.Lock()
		entry.readerForegroundVerified = true
		t.mu.Unlock()
	}
	return outcome
}

// Called only after the separately owned Windows capture host is launched.
func (s *Server) SetSplitReaderPreparation(prepare func(context.Context, string) (func(context.Context) error, error)) {
	if s.splitCapture == nil {
		return
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
	s.splitCapture.prepareReader = prepare
}

func newSplitCaptureTransport() *splitCaptureTransport {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		panic(err)
	}
	return &splitCaptureTransport{key: hex.EncodeToString(secret[:]), wake: make(chan struct{}, 1), done: make(chan struct{})}
}
func (t *splitCaptureTransport) authorized(r *http.Request) bool {
	return subtle.ConstantTimeCompare([]byte(t.key), []byte(r.Header.Get("X-Aku-Capture-Instance"))) == 1
}
func (t *splitCaptureTransport) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		close(t.done)
	}
}

// SplitCaptureLaunchURL is never logged; its fragment carries only an ephemeral
// bootstrap capability, not the persistent Bridge token or profile credentials.
func (s *Server) SplitCaptureLaunchURL(origin string) (string, error) {
	if s.splitCapture == nil {
		return "", errors.New("Windows capture split is disabled")
	}
	return strings.TrimSuffix(origin, "/") + "/split-capture-host#" + s.splitCapture.key, nil
}
func splitCaptureOwnedRoute(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/bridge/commands/") || strings.HasPrefix(p, "/api/bridge/media-recaptures/") || p == "/api/bridge/capture-surfaces/events" || strings.HasPrefix(p, "/api/bridge/split-capture/") || (strings.HasPrefix(p, "/api/operations/bridge/actions/") && strings.HasSuffix(p, "/accept"))
}
func validateSplitAction(a splitCaptureAction) error {
	switch a.Type {
	case "ping", "probe_source_sessions", "revoke_source_access", "configure_background":
	case "open_source":
		if !splitSource(a.Source) {
			return errors.New("unsupported source")
		}
	case "open_native_post":
		if !splitSource(a.Source) || len(a.URL) == 0 || len(a.URL) > 4096 {
			return errors.New("invalid native post request")
		}
	case "dispatch":
		if !splitID(a.RunID) {
			return errors.New("invalid run ID")
		}
	case "release":
		if !splitID(a.LeaseID) || (a.Source != "" && !splitSource(a.Source)) {
			return errors.New("invalid capture lease")
		}
	case "media_recapture":
		if !splitID(a.RecaptureID) {
			return errors.New("invalid recapture ID")
		}
	case "reload_self":
		if !splitID(a.ActionID) {
			return errors.New("invalid reload action ID")
		}
	case "media_evidence":
		if len(a.CandidateIDs) > 100 {
			return errors.New("too many candidate IDs")
		}
		for _, id := range a.CandidateIDs {
			if !splitID(id) {
				return errors.New("invalid candidate ID")
			}
		}
	default:
		return errors.New("unsupported capture action")
	}
	return nil
}
func splitID(v string) bool {
	return len(v) > 0 && len(v) <= 256 && !strings.ContainsAny(v, "\r\n\x00")
}
func splitSource(v string) bool {
	return v == "x" || v == "linkedin" || v == "facebook" || v == "instagram"
}

func (s *Server) routeSplitCapture(w http.ResponseWriter, r *http.Request, p string) error {
	t := s.splitCapture
	if t == nil {
		return notFound("Windows capture split")
	}
	w.Header().Set("Cache-Control", "no-store")
	if p == "/api/bridge/split-capture/bootstrap" && r.Method == http.MethodPost {
		if !t.authorized(r) {
			return apiError{Status: 403, Code: "capture_instance_mismatch", Message: "Capture bootstrap rejected."}
		}
		token, err := s.store.BridgeToken(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, 200, map[string]any{"token": token, "instanceEpoch": s.engine.Epoch(), "protocolMajor": 2})
	}
	if err := s.requireBridge(r); err != nil {
		return err
	}
	if strings.HasPrefix(p, "/api/bridge/split-capture/reader/") && r.Method == http.MethodPost {
		parts := strings.Split(strings.TrimPrefix(p, "/api/bridge/split-capture/reader/"), "/")
		if len(parts) != 2 || (parts[0] != "prepare" && parts[0] != "foreground") {
			return notFound("reader intent")
		}
		t.mu.Lock()
		var entry *pendingSplitAction
		for _, candidate := range t.actions {
			if candidate.action.ID == parts[1] && candidate.claimed && candidate.action.Type == "open_native_post" {
				entry = candidate
				break
			}
		}
		if t.closed || entry == nil || (t.prepareReader == nil && t.prepareBrokerReader == nil) {
			t.mu.Unlock()
			return notFound("active reader intent")
		}
		if parts[0] == "foreground" {
			if entry.brokerReady != nil {
				select {
				case <-entry.brokerReady:
					t.mu.Unlock()
					return apiError{Status: 409, Code: "reader_intent_consumed", Message: "Reader intent was already requested."}
				default:
					close(entry.brokerReady)
				}
				t.mu.Unlock()
				select {
				case err := <-entry.brokerDone:
					if err != nil {
						return apiError{Status: 409, Code: "reader_broker_rejected", Message: err.Error()}
					}
					return writeJSON(w, 200, map[string]bool{"foreground": true})
				case <-r.Context().Done():
					return r.Context().Err()
				case <-time.After(readerbroker.Lifetime):
					return apiError{Status: 409, Code: "reader_broker_expired", Message: "The UI reader broker did not complete this click."}
				}
			}
			foreground := entry.readerForeground
			entry.readerForeground = nil // Consume before the native call; never replay.
			t.mu.Unlock()
			if foreground == nil {
				s.logger.Printf("split_reader action=%s phase=foreground outcome=missing_intent", entry.action.ID)
				return apiError{Status: 409, Code: "reader_intent_consumed", Message: "Reader foreground intent is not available."}
			}
			if err := foreground(r.Context()); err != nil {
				s.logger.Printf("split_reader action=%s phase=foreground outcome=rejected error=%q", entry.action.ID, err.Error())
				return apiError{Status: 409, Code: "reader_foreground_rejected", Message: err.Error()}
			}
			t.mu.Lock()
			entry.readerForegroundVerified = true
			t.mu.Unlock()
			s.logger.Printf("split_reader action=%s phase=foreground outcome=accepted", entry.action.ID)
			return writeJSON(w, 200, map[string]bool{"foreground": true})
		}
		if entry.readerPreparing {
			t.mu.Unlock()
			return apiError{Status: 409, Code: "reader_intent_consumed", Message: "Reader preparation was already claimed."}
		}
		entry.readerPreparing = true
		prepare := t.prepareReader
		prepareBroker := t.prepareBrokerReader
		t.mu.Unlock()
		var foreground func(context.Context) error
		var target readerbroker.Target
		var err error
		if prepareBroker != nil {
			target, foreground, err = prepareBroker(r.Context(), "AkuBrowser reader "+entry.action.ID)
		} else {
			foreground, err = prepare(r.Context(), "AkuBrowser reader "+entry.action.ID)
		}
		if err != nil {
			s.logger.Printf("split_reader action=%s phase=prepare outcome=rejected error=%q", entry.action.ID, err.Error())
			return apiError{Status: 409, Code: "reader_binding_rejected", Message: err.Error()}
		}
		t.mu.Lock()
		entry.readerForeground = foreground
		entry.brokerTarget = target
		t.mu.Unlock()
		s.logger.Printf("split_reader action=%s phase=prepare outcome=accepted", entry.action.ID)
		return writeJSON(w, 200, map[string]bool{"prepared": true})
	}
	if p == "/api/split-capture/actions" && r.Method == http.MethodPost {
		if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
			return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
		}
		var a splitCaptureAction
		if err := readJSON(r, &a); err != nil {
			return err
		}
		if err := validateSplitAction(a); err != nil {
			return apiError{Status: 400, Code: "invalid_capture_action", Message: err.Error()}
		}
		a.ID = domain.NewID("split")
		entry := &pendingSplitAction{action: a, result: make(chan splitActionResult, 1)}
		t.mu.Lock()
		if a.Type == "open_native_post" && t.prepareBrokerReader != nil {
			if err := (readerbroker.Request{RequestID: a.RequestID, Source: a.Source, URL: a.URL}).Validate(); err != nil {
				t.mu.Unlock()
				return badRequest("Native reader requires an explicit UI broker click.")
			}
			for _, old := range t.actions {
				if old.action.RequestID == a.RequestID {
					t.mu.Unlock()
					return badRequest("Reader click was already queued.")
				}
			}
			entry.brokerReady = make(chan struct{})
			entry.brokerDone = make(chan error, 1)
		}
		if t.closed || len(t.actions) >= splitActionLimit {
			t.mu.Unlock()
			return apiError{Status: 503, Code: "capture_unavailable", Message: "Capture transport is unavailable or busy."}
		}
		t.actions = append(t.actions, entry)
		t.mu.Unlock()
		defer func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			for i, v := range t.actions {
				if v == entry {
					t.actions = append(t.actions[:i], t.actions[i+1:]...)
					break
				}
			}
		}()
		select {
		case t.wake <- struct{}{}:
		default:
		}
		timeout := splitActionTimeout
		if entry.brokerReady != nil {
			timeout = readerbroker.Lifetime
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case result := <-entry.result:
			return writeJSON(w, 200, result)
		case <-t.done:
			return apiError{Status: 503, Code: "capture_stopped", Message: "Capture process stopped."}
		case <-r.Context().Done():
			return r.Context().Err()
		case <-timer.C:
			return apiError{Status: 504, Code: "capture_timeout", Message: "Capture action timed out; an already claimed action may still finish. It was not replayed."}
		}
	}
	if p == "/api/bridge/split-capture/next" && r.Method == http.MethodGet {
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		for {
			t.mu.Lock()
			for _, entry := range t.actions {
				if !entry.claimed && (entry.brokerReady == nil || entry.brokerAttached) {
					entry.claimed = true
					action := entry.action
					t.mu.Unlock()
					return writeJSON(w, 200, map[string]any{"instanceEpoch": s.engine.Epoch(), "action": action})
				}
			}
			t.mu.Unlock()
			select {
			case <-t.wake:
			case <-timer.C:
				w.WriteHeader(204)
				return nil
			case <-t.done:
				w.WriteHeader(410)
				return nil
			case <-r.Context().Done():
				return r.Context().Err()
			}
		}
	}
	if strings.HasPrefix(p, "/api/bridge/split-capture/results/") && r.Method == http.MethodPost {
		id := strings.TrimPrefix(p, "/api/bridge/split-capture/results/")
		var result splitActionResult
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if err := readJSON(r, &result); err != nil {
			return err
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		for _, entry := range t.actions {
			if entry.action.ID == id && entry.claimed {
				if entry.action.Type == "open_native_post" {
					if result.OK && !entry.readerForegroundVerified {
						// Deliver a terminal failure to the waiting UI, rather than
						// rejecting this POST and leaving it waiting for a timeout.
						result = splitActionResult{OK: false, Message: "Native reader foreground was not verified. Reload AkuBridge to load the current runtime, then retry Open native post."}
					}
					if result.OK {
						s.logger.Printf("split_reader action=%s phase=result outcome=accepted", entry.action.ID)
					} else {
						s.logger.Printf("split_reader action=%s phase=result outcome=rejected error=%q", entry.action.ID, result.Message)
					}
				}
				select {
				case entry.result <- result:
					w.WriteHeader(204)
					return nil
				default:
					return apiError{Status: 409, Code: "capture_result_duplicate", Message: "Capture result was already received."}
				}
			}
		}
		return notFound("capture action")
	}
	return notFound("capture route")
}

func (s *Server) serveSplitCaptureAsset(w http.ResponseWriter, r *http.Request) bool {
	if s.splitCapture == nil {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/split-reader-intent":
		id := r.URL.Query().Get("id")
		s.splitCapture.mu.Lock()
		allowed := false
		for _, entry := range s.splitCapture.actions {
			if entry.claimed && entry.action.ID == id && entry.action.Type == "open_native_post" {
				allowed = true
				break
			}
		}
		s.splitCapture.mu.Unlock()
		if !allowed {
			http.NotFound(w, r)
			return true
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><head><title>"+html.EscapeString("AkuBrowser reader "+id)+"</title></head><body>Opening native post…</body></html>")
		return true
	case "/split-ui-bridge.js":
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write(splitUIBridge)
		return true
	case "/split-capture-host":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head><title>AkuBrowser capture host</title></head><body><h1>Experimental capture host</h1><p id="split-capture-status" role="status">Waiting for the capture extension. If this message remains, update or reload AkuBridge in this capture profile, then restart AkuBrowser.</p><p>This separate Windows browser owns your source sign-ins. Keep this minimized window open while AkuBrowser runs. Closing its last window stops the app. Source permission and sign-in windows open here only when requested.</p></body></html>`)
		return true
	case "/", "/index.html":
		data, err := embeddedAssets.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "UI unavailable", 500)
			return true
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, strings.Replace(string(data), "</head>", `<script src="/split-ui-bridge.js"></script></head>`, 1))
		return true
	}
	return false
}
