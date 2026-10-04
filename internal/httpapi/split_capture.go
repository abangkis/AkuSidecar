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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// This transport is instantiated only by the Windows app-shell feature gate.
// It is deliberately ephemeral: Sidecar epoch + random capture-instance key
// fence old workers, and at-most-once claims never replay an interactive action.
const splitActionLimit = 32
const splitActionTimeout = 115 * time.Second

// split-action audit records only the metadata needed to correlate explicit
// user actions with native foreground samples. Never pass action payloads,
// URLs, source content, bridge credentials, or error strings to this store.
func (s *Server) splitActionAuditRecord(action splitCaptureAction, phase, outcome string) (store.SplitActionAudit, bool) {
	if s.engine == nil || (action.Type != "open_source" && action.Type != "open_native_post") || !splitID(action.ID) {
		return store.SplitActionAudit{}, false
	}
	switch phase {
	case "queued", "claimed", "reader_broker_attached", "reader_prepare", "reader_foreground", "result":
	default:
		return store.SplitActionAudit{}, false
	}
	switch outcome {
	case "", "pending", "accepted", "rejected":
	default:
		return store.SplitActionAudit{}, false
	}
	return store.SplitActionAudit{
		ActionID: action.ID, ActionType: action.Type, Phase: phase, Outcome: outcome,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, true
}

func (s *Server) persistSplitActionAudit(ctx context.Context, value store.SplitActionAudit) {
	err := s.engine.RecordSplitActionAudit(ctx, value)
	if err != nil && s.logger != nil {
		s.logger.Printf("split action audit unavailable")
	}
}

func (s *Server) auditSplitAction(ctx context.Context, action splitCaptureAction, phase, outcome string) {
	if value, ok := s.splitActionAuditRecord(action, phase, outcome); ok {
		s.persistSplitActionAudit(ctx, value)
	}
}

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
	HostOnly     bool     `json:"hostOnly,omitempty"`
}
type splitActionResult struct {
	OK      bool            `json:"ok"`
	Message string          `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}
type pendingSplitAction struct {
	collectionCleanupGeneration uint64
	browserRecaptureGeneration  uint64
	action                      splitCaptureAction
	queuedAt                    time.Time
	claimed                     bool
	result                      chan splitActionResult
	readerPreparing             bool
	sourcePreparing             bool
	sourcePrepared              bool
	interactionRelease          func()
	readerForeground            func(context.Context) error
	readerForegroundVerified    bool
	brokerReady                 chan struct{}
	brokerDone                  chan error
	brokerAttached              bool
	directReader                bool
	brokerTarget                readerbroker.Target
	runtimeLease                *captureruntime.Lease
	detached                    bool
	completed                   bool
	completionResult            *splitActionResult
}
type splitCaptureTransport struct {
	mu                      sync.Mutex
	key                     string
	closed                  bool
	actions                 []*pendingSplitAction
	wake                    chan struct{}
	hostWake                chan struct{}
	hostWakeStreams         int
	done                    chan struct{}
	prepareReader           func(context.Context, string) (func(context.Context) error, error)
	prepareBrokerReader     func(context.Context, string) (readerbroker.Target, func(context.Context) error, error)
	prepareSourceWindow     func(context.Context, string) error
	directReader            func(context.Context, string, string, string) (readerbroker.Target, func(context.Context) error, error)
	directReaderReadiness   func(context.Context) error
	runtime                 *captureruntime.Manager
	actionTimeout           time.Duration
	sourceTrackingSupported bool
	hostCloseSupported      bool
	hostOnlyRequired        bool
	hostOnlySupported       bool
	untrackedSourceOutcome  bool
}

// Capability declarations cannot clear earlier unverified source outcomes.
// Only a fresh transport/owner boundary may clear that uncertainty.
func (s *Server) SplitCaptureReplacementReadiness(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := s.splitCapture
	if t == nil {
		return errors.New("split capture transport unavailable")
	}
	t.mu.Lock()
	if t.directReaderReadiness != nil && !t.closed {
		check := t.directReaderReadiness
		t.mu.Unlock()
		return check(ctx)
	}
	defer t.mu.Unlock()
	if t.hostOnlyRequired && !t.hostOnlySupported {
		return errors.New("Bridge host-only retirement is not negotiated; update or reload AkuBridge before switching collection mode")
	}
	if t.closed || t.prepareSourceWindow == nil || !t.sourceTrackingSupported {
		return errors.New("Bridge source-window tracking is not negotiated")
	}
	if t.untrackedSourceOutcome {
		return errors.New("source window ownership is unverified")
	}
	return nil
}

// SetSplitCaptureRuntime adopts pending action ownership before exposing the
// manager to split admission. It does not rotate transport credentials or launch
// a browser; those belong to the eventual verified handoff coordinator.
func (s *Server) SetSplitCaptureRuntime(owner *captureruntime.Manager) error {
	t := s.splitCapture
	if t == nil {
		return errors.New("split capture transport unavailable")
	}
	if owner == nil {
		return errors.New("capture runtime unavailable")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.runtime != nil {
		return errors.New("split capture runtime already attached")
	}
	var adopted []*pendingSplitAction
	for _, entry := range t.actions {
		if entry == nil || entry.completed {
			continue
		}
		lease, err := owner.Acquire()
		if err != nil {
			for _, old := range adopted {
				old.runtimeLease.Release()
				old.runtimeLease = nil
			}
			return err
		}
		entry.runtimeLease = lease
		adopted = append(adopted, entry)
	}
	t.runtime = owner
	return nil
}

// Caller holds t.mu. A detached claimed action keeps both queue ownership and
// lease until its late result arrives; it cannot be claimed or replayed again.
func (t *splitCaptureTransport) removeAction(entry *pendingSplitAction) {
	for i, value := range t.actions {
		if value == entry {
			t.actions = append(t.actions[:i], t.actions[i+1:]...)
			break
		}
	}
	entry.runtimeLease.Release()
	entry.runtimeLease = nil
	if entry.interactionRelease != nil {
		entry.interactionRelease()
		entry.interactionRelease = nil
	}
}

func (s *Server) SetSplitReaderBroker(prepare func(context.Context, string) (readerbroker.Target, func(context.Context) error, error)) {
	if s.splitCapture == nil {
		return
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
	s.splitCapture.prepareBrokerReader = prepare
}

// Installed only for a headless native-reader process, never for a collector.
func (s *Server) SetSplitDirectNativeReader(prepare func(context.Context, string, string, string) (readerbroker.Target, func(context.Context) error, error), readiness func(context.Context) error) {
	t := s.splitCapture
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.directReader, t.directReaderReadiness = prepare, readiness
}

// HandleReaderBroker is called only by the OS-authenticated native pipe server.
// The collector has no route to this entry point and never receives a ticket.
func (s *Server) HandleReaderBroker(ctx context.Context, req readerbroker.Request, activate func(readerbroker.Target) (readerbroker.Reply, error)) (retErr error) {
	if err := req.Validate(); err != nil {
		return err
	}
	t := s.splitCapture
	if t == nil {
		return errors.New("reader broker disabled")
	}
	startedAt := time.Now()
	if s.logger != nil {
		s.logger.Printf("reader_broker request_id=%s phase=received outcome=active elapsed_ms=0", req.RequestID)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var entry *pendingSplitAction
	var attachedAt time.Time
	var attachedAudit store.SplitActionAudit
	var attachedAuditReady bool
	terminalOutcome := "unmatched"
	defer func() {
		if s.logger == nil {
			return
		}
		elapsedMS := time.Since(startedAt).Milliseconds()
		if entry == nil {
			if terminalOutcome == "unmatched" && ctx.Err() != nil {
				terminalOutcome = "cancelled"
			}
			s.logger.Printf("reader_broker request_id=%s phase=terminal outcome=%s elapsed_ms=%d", req.RequestID, terminalOutcome, elapsedMS)
			return
		}
		outcome := "rejected"
		switch {
		case retErr == nil:
			outcome = "accepted"
		case errors.Is(retErr, context.Canceled):
			outcome = "cancelled"
		case errors.Is(retErr, context.DeadlineExceeded):
			outcome = "timed_out"
		}
		queueWaitMS := splitQueueWaitMS(entry, attachedAt)
		s.logger.Printf("reader_broker request_id=%s action=%s phase=complete outcome=%s elapsed_ms=%d queue_wait_ms=%s", req.RequestID, entry.action.ID, outcome, elapsedMS, queueWaitMS)
	}()
	for entry == nil {
		t.mu.Lock()
		for _, a := range t.actions {
			if !a.completed && a.action.RequestID == req.RequestID && a.action.Type == "open_native_post" && a.action.Source == req.Source && a.action.URL == req.URL && a.brokerReady != nil && !a.brokerAttached {
				a.brokerAttached = true
				entry = a
				attachedAt = time.Now()
				attachedAudit, attachedAuditReady = s.splitActionAuditRecord(entry.action, "reader_broker_attached", "accepted")
				break
			}
		}
		closed := t.closed
		t.mu.Unlock()
		if attachedAuditReady {
			s.persistSplitActionAudit(ctx, attachedAudit)
			attachedAuditReady = false
		}
		if closed {
			terminalOutcome = "stopped"
			return errors.New("reader broker stopped")
		}
		if entry != nil {
			if s.logger != nil {
				s.logger.Printf("reader_broker request_id=%s action=%s phase=attached outcome=accepted elapsed_ms=%d queue_wait_ms=%s", req.RequestID, entry.action.ID, attachedAt.Sub(startedAt).Milliseconds(), splitQueueWaitMS(entry, attachedAt))
			}
			t.notifyCapture()
			break
		}
		select {
		case <-ctx.Done():
			terminalOutcome = "cancelled"
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
		s.auditSplitAction(ctx, entry.action, "reader_foreground", "rejected")
		return outcome
	}
	s.auditSplitAction(ctx, entry.action, "reader_foreground", "pending")
	activationStarted := time.Now()
	result, err := activate(target)
	if s.logger != nil {
		s.logger.Printf("native_reader_timing action=%s stage=helper_activation elapsed_ms=%d reason=%s", entry.action.ID, time.Since(activationStarted).Milliseconds(), readerbroker.ActivationReason(result, err))
	}
	if err != nil {
		outcome = err
	} else if !result.OK || !result.Readback {
		outcome = errors.New("reader helper activation rejected")
	} else {
		outcome = verify(ctx)
	}
	if s.logger != nil {
		activationOutcome := "rejected"
		activationReason := readerbroker.ActivationReason(result, err)
		if outcome == nil {
			activationOutcome = "accepted"
		} else if err == nil && result.OK && result.Readback {
			activationReason = "post_activation_completion_failed"
		}
		s.logger.Printf("reader_broker request_id=%s action=%s phase=activation outcome=%s reason=%s applied=%t readback=%t verified=%t", req.RequestID, entry.action.ID, activationOutcome, activationReason, result.Applied, result.Readback, outcome == nil)
	}
	if outcome == nil {
		s.auditSplitAction(ctx, entry.action, "reader_foreground", "accepted")
	} else {
		s.auditSplitAction(ctx, entry.action, "reader_foreground", "rejected")
	}
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

// Source preparation records native lifetime only; it grants no foreground intent.
func (s *Server) SetSplitSourceWindowPreparation(prepare func(context.Context, string) error) {
	if s.splitCapture == nil {
		return
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
	s.splitCapture.prepareSourceWindow = prepare
}

func newSplitCaptureTransport() *splitCaptureTransport {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		panic(err)
	}
	return &splitCaptureTransport{key: hex.EncodeToString(secret[:]), wake: make(chan struct{}, 1), hostWake: make(chan struct{}), done: make(chan struct{}), actionTimeout: splitActionTimeout}
}

// Wake hints never claim, carry or extend an action. The host's network callback
// can wake a suspended extension worker without a throttled page timer.
func (t *splitCaptureTransport) notifyCapture() {
	t.mu.Lock()
	close(t.hostWake)
	t.hostWake = make(chan struct{})
	t.mu.Unlock()
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

func (t *splitCaptureTransport) serveHostWake(w http.ResponseWriter, r *http.Request) error {
	if !t.authorized(r) {
		return apiError{Status: 403, Code: "capture_instance_mismatch", Message: "Capture wake rejected."}
	}
	t.mu.Lock()
	if t.closed || t.hostWakeStreams >= 4 {
		t.mu.Unlock()
		return apiError{Status: 503, Code: "capture_unavailable", Message: "Capture wake unavailable."}
	}
	t.hostWakeStreams++
	wake := t.hostWake
	t.mu.Unlock()
	defer func() { t.mu.Lock(); t.hostWakeStreams--; t.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	controller := http.NewResponseController(w)
	send := func() error {
		if _, err := io.WriteString(w, "wake\n"); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := send(); err != nil {
		return nil
	}
	// Reconnect on network completion, before the server's 130s write deadline.
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-wake:
			t.mu.Lock()
			if t.key != r.Header.Get("X-Aku-Capture-Instance") {
				t.mu.Unlock()
				return nil
			}
			wake = t.hostWake
			t.mu.Unlock()
			if err := send(); err != nil {
				return nil
			}
		case <-timer.C:
			return nil
		case <-t.done:
			return nil
		case <-r.Context().Done():
			return nil
		}
	}
}
func (t *splitCaptureTransport) authorized(r *http.Request) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return subtle.ConstantTimeCompare([]byte(t.key), []byte(r.Header.Get("X-Aku-Capture-Instance"))) == 1
}

// Invalidate the old process capability only after its owner has drained.
// Keep the transport stable for in-flight handlers; do not carry callbacks.
func (s *Server) RotateSplitCapture() error {
	t := s.splitCapture
	if t == nil {
		return errors.New("split capture unavailable")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errors.New("split capture closed")
	}
	for _, entry := range t.actions {
		if !entry.completed {
			t.mu.Unlock()
			return captureruntime.ErrBusy
		}
	}
	t.actions = nil
	t.key = hex.EncodeToString(secret[:])
	t.prepareReader, t.prepareBrokerReader, t.prepareSourceWindow = nil, nil, nil
	t.directReader, t.directReaderReadiness = nil, nil
	t.sourceTrackingSupported, t.untrackedSourceOutcome, t.hostCloseSupported, t.hostOnlySupported = false, false, false, false
	s.engine.ResetCaptureHeartbeat()
	t.mu.Unlock()
	t.notifyCapture()
	return nil
}
func (t *splitCaptureTransport) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		for _, entry := range t.actions {
			if entry != nil {
				entry.runtimeLease.Release()
				entry.runtimeLease = nil
				if entry.interactionRelease != nil {
					entry.interactionRelease()
					entry.interactionRelease = nil
				}
			}
		}
		t.actions = nil
		close(t.done)
	}
}

// SplitCaptureLaunchURL is never logged; its fragment carries only an ephemeral
// bootstrap capability, not the persistent Bridge token or profile credentials.
func (s *Server) SplitCaptureLaunchURL(origin string) (string, error) {
	if s.splitCapture == nil {
		return "", errors.New("Windows capture split is disabled")
	}
	s.splitCapture.mu.Lock()
	defer s.splitCapture.mu.Unlock()
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

// splitBrokerRequestID returns only the fixed-format opaque broker ID. This
// keeps log fields safe even for native-post actions created without the broker
// transport enabled.
func splitBrokerRequestID(action splitCaptureAction) string {
	const prefix = "broker_"
	if action.Type != "open_native_post" || len(action.RequestID) != len(prefix)+32 || !strings.HasPrefix(action.RequestID, prefix) {
		return "-"
	}
	for _, c := range action.RequestID[len(prefix):] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "-"
		}
	}
	return action.RequestID
}

type splitActionQueueSnapshot struct {
	total               int
	unclaimed           int
	claimed             int
	nativeUnclaimed     int
	nativeWaitingBroker int
	nativeReady         int
}

func snapshotSplitActionQueue(actions []*pendingSplitAction) splitActionQueueSnapshot {
	var snapshot splitActionQueueSnapshot
	snapshot.total = len(actions)
	for _, entry := range actions {
		if entry.claimed {
			snapshot.claimed++
			continue
		}
		snapshot.unclaimed++
		if entry.action.Type != "open_native_post" {
			continue
		}
		snapshot.nativeUnclaimed++
		if entry.brokerReady != nil && !entry.brokerAttached {
			snapshot.nativeWaitingBroker++
		} else {
			snapshot.nativeReady++
		}
	}
	return snapshot
}

func splitQueueWaitMS(entry *pendingSplitAction, attachedAt time.Time) string {
	if entry.queuedAt.IsZero() || attachedAt.IsZero() {
		return "unavailable"
	}
	return strconv.FormatInt(attachedAt.Sub(entry.queuedAt).Milliseconds(), 10)
}

func (s *Server) routeSplitCapture(w http.ResponseWriter, r *http.Request, p string) error {
	t := s.splitCapture
	if t == nil {
		return notFound("Windows capture split")
	}
	w.Header().Set("Cache-Control", "no-store")
	if p == "/api/bridge/split-capture/wake" && r.Method == http.MethodGet {
		return t.serveHostWake(w, r)
	}
	if p == "/api/bridge/split-capture/bootstrap" && r.Method == http.MethodPost {
		if !t.authorized(r) {
			return apiError{Status: 403, Code: "capture_instance_mismatch", Message: "Capture bootstrap rejected."}
		}
		var capability struct {
			SourceWindowLifetime      int `json:"sourceWindowLifetime"`
			CaptureHostClose          int `json:"captureHostClose"`
			CaptureHostOnlyRetirement int `json:"captureHostOnlyRetirement"`
		}
		if r.ContentLength != 0 {
			if err := readJSON(r, &capability); err != nil {
				return err
			}
		}
		token, err := s.store.BridgeToken(r.Context())
		if err != nil {
			return err
		}
		t.mu.Lock()
		if r.Header.Get("X-Aku-Capture-Instance") != t.key || t.closed {
			t.mu.Unlock()
			return apiError{Status: 403, Code: "capture_instance_mismatch", Message: "Capture bootstrap rejected."}
		}
		t.sourceTrackingSupported = capability.SourceWindowLifetime == 1
		t.hostCloseSupported = capability.CaptureHostClose == 1
		t.hostOnlySupported = capability.CaptureHostOnlyRetirement == 1 && capability.CaptureHostClose == 1
		hostOnlySupported := t.hostOnlySupported
		t.mu.Unlock()
		return writeJSON(w, 200, map[string]any{"token": token, "instanceEpoch": s.engine.Epoch(), "protocolMajor": 2, "captureHostClose": capability.CaptureHostClose == 1, "captureHostOnlyRetirement": hostOnlySupported})
	}
	if err := s.requireBridge(r); err != nil {
		return err
	}
	if strings.HasPrefix(p, "/api/bridge/split-capture/source/prepare/") && r.Method == http.MethodPost {
		id := strings.TrimPrefix(p, "/api/bridge/split-capture/source/prepare/")
		t.mu.Lock()
		var entry *pendingSplitAction
		for _, candidate := range t.actions {
			if candidate.action.ID == id && candidate.claimed && !candidate.completed && candidate.action.Type == "open_source" {
				entry = candidate
				break
			}
		}
		if t.closed || entry == nil || t.prepareSourceWindow == nil {
			t.mu.Unlock()
			return notFound("active source intent")
		}
		if entry.sourcePreparing {
			t.mu.Unlock()
			return apiError{Status: 409, Code: "source_intent_consumed", Message: "Source preparation was already claimed."}
		}
		entry.sourcePreparing = true
		prepare := t.prepareSourceWindow
		t.mu.Unlock()
		if err := prepare(r.Context(), "AkuBrowser source "+id); err != nil {
			return apiError{Status: 409, Code: "source_binding_rejected", Message: err.Error()}
		}
		t.mu.Lock()
		entry.sourcePrepared = true
		t.mu.Unlock()
		return writeJSON(w, 200, map[string]bool{"prepared": true})
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
			s.auditSplitAction(r.Context(), entry.action, "reader_foreground", "pending")
			if foreground == nil {
				s.auditSplitAction(r.Context(), entry.action, "reader_foreground", "rejected")
				s.logger.Printf("split_reader action=%s phase=foreground outcome=missing_intent", entry.action.ID)
				return apiError{Status: 409, Code: "reader_intent_consumed", Message: "Reader foreground intent is not available."}
			}
			if err := foreground(r.Context()); err != nil {
				s.auditSplitAction(r.Context(), entry.action, "reader_foreground", "rejected")
				s.logger.Printf("split_reader action=%s phase=foreground outcome=rejected", entry.action.ID)
				return apiError{Status: 409, Code: "reader_foreground_rejected", Message: err.Error()}
			}
			t.mu.Lock()
			entry.readerForegroundVerified = true
			t.mu.Unlock()
			s.auditSplitAction(r.Context(), entry.action, "reader_foreground", "accepted")
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
		s.auditSplitAction(r.Context(), entry.action, "reader_prepare", "pending")
		var foreground func(context.Context) error
		var target readerbroker.Target
		var err error
		if prepareBroker != nil {
			target, foreground, err = prepareBroker(r.Context(), "AkuBrowser reader "+entry.action.ID)
		} else {
			foreground, err = prepare(r.Context(), "AkuBrowser reader "+entry.action.ID)
		}
		if err != nil {
			s.auditSplitAction(r.Context(), entry.action, "reader_prepare", "rejected")
			s.logger.Printf("split_reader action=%s phase=prepare outcome=rejected", entry.action.ID)
			return apiError{Status: 409, Code: "reader_binding_rejected", Message: err.Error()}
		}
		t.mu.Lock()
		entry.readerForeground = foreground
		entry.brokerTarget = target
		t.mu.Unlock()
		s.auditSplitAction(r.Context(), entry.action, "reader_prepare", "accepted")
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
		// A browser-owned Quiet command is consumed by the internal pump. The
		// Bridge still owns heartbeat, permissions and other browser sources.
		if a.Type == "dispatch" {
			route, err := s.engine.RunCaptureCollector(r.Context(), a.RunID)
			if err != nil {
				return err
			}
			if route == collection.BackendQuiet {
				return writeJSON(w, 200, splitActionResult{OK: true, Result: json.RawMessage(`{}`)})
			}
		}
		if a.Type == "media_recapture" {
			job, err := s.store.MediaRecapture(r.Context(), a.RecaptureID)
			if err != nil {
				return err
			}
			route, err := store.CaptureCollector(job.Payload)
			if err != nil {
				return err
			}
			if route == collection.BackendQuiet || sidecarOwnsBrowserRecapture(job) {
				raw, err := json.Marshal(map[string]any{"recapture": job})
				if err != nil {
					return err
				}
				return writeJSON(w, 200, splitActionResult{OK: true, Result: raw})
			}
		}
		if s.engine.CollectionRuntime().Effective == "headless" {
			switch a.Type {
			case "dispatch", "configure_background", "release":
				return writeJSON(w, 200, splitActionResult{OK: true, Result: json.RawMessage(`{}`)})
			case "media_recapture":
				job, err := s.store.MediaRecapture(r.Context(), a.RecaptureID)
				if err != nil {
					return err
				}
				raw, err := json.Marshal(map[string]any{"recapture": job})
				if err != nil {
					return err
				}
				return writeJSON(w, 200, splitActionResult{OK: true, Result: raw})
			case "media_evidence":
				return writeJSON(w, 200, splitActionResult{OK: true, Result: json.RawMessage(`{"evidence":[]}`)})
			case "ping", "probe_source_sessions", "reload_self":
				return apiError{Status: 409, Code: "browser_handoff_required", Message: "This operation requires browser mode; open the source or select Browser in Settings."}
			}
		}
		a.ID = domain.NewID("split")
		readerStarted := time.Now()
		if a.Type == "open_native_post" && s.logger != nil {
			defer func() {
				s.logger.Printf("native_reader_timing action=%s stage=request_total elapsed_ms=%d", a.ID, time.Since(readerStarted).Milliseconds())
			}()
		}
		if a.Type == "open_native_post" {
			// One bounded conversation includes both profile handoff and queue wait.
			readerCtx, cancel := context.WithTimeout(r.Context(), readerbroker.PreparationLifetime)
			defer cancel()
			r = r.WithContext(readerCtx)
		}
		entry := &pendingSplitAction{action: a, result: make(chan splitActionResult, 1)}
		queued := false
		if a.Type == "open_source" || a.Type == "open_native_post" || a.Type == "revoke_source_access" {
			borrow := s.engine.BorrowInteractiveCapture
			if a.Type == "open_native_post" {
				borrow = s.engine.BorrowNativeReader
			}
			borrowStarted := time.Now()
			lease, release, err := borrow(r.Context())
			if a.Type == "open_native_post" && s.logger != nil {
				s.logger.Printf("native_reader_timing action=%s stage=profile_handoff elapsed_ms=%d ok=%t", a.ID, time.Since(borrowStarted).Milliseconds(), err == nil)
			}
			if err != nil {
				return apiError{Status: 409, Code: "interactive_handoff_unavailable", Message: err.Error()}
			}
			entry.runtimeLease, entry.interactionRelease = lease, release
			// Before queue ownership begins, every validation failure must return
			// the borrowed profile intent. Once queued, normal action drain owns it.
			defer func() {
				if !queued && entry.interactionRelease != nil {
					entry.runtimeLease.Release()
					entry.interactionRelease()
					entry.interactionRelease = nil
				}
			}()
		}
		t.mu.Lock()
		if a.Type == "open_native_post" && (t.prepareBrokerReader != nil || t.directReader != nil) {
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
			entry.directReader = t.directReader != nil
		}
		if t.closed || len(t.actions) >= splitActionLimit {
			t.mu.Unlock()
			return apiError{Status: 503, Code: "capture_unavailable", Message: "Capture transport is unavailable or busy."}
		}
		if t.runtime != nil && entry.runtimeLease == nil {
			lease, err := t.runtime.Acquire()
			if err != nil {
				t.mu.Unlock()
				return apiError{Status: 503, Code: "capture_runtime_unavailable", Message: "Capture runtime is not accepting actions."}
			}
			entry.runtimeLease = lease
		}
		entry.queuedAt = time.Now()
		t.actions = append(t.actions, entry)
		queued = true
		queueSnapshot := snapshotSplitActionQueue(t.actions)
		actionTimeout := t.actionTimeout
		queuedAudit, queuedAuditReady := s.splitActionAuditRecord(entry.action, "queued", "accepted")
		directPrepare := t.directReader
		t.mu.Unlock()
		if entry.action.Type == "open_native_post" && s.logger != nil {
			s.logger.Printf("split_action action=%s request_id=%s phase=queued outcome=accepted queue_total=%d unclaimed=%d claimed=%d native_unclaimed=%d native_waiting_broker=%d native_ready=%d", entry.action.ID, splitBrokerRequestID(entry.action), queueSnapshot.total, queueSnapshot.unclaimed, queueSnapshot.claimed, queueSnapshot.nativeUnclaimed, queueSnapshot.nativeWaitingBroker, queueSnapshot.nativeReady)
		}
		if queuedAuditReady {
			s.persistSplitActionAudit(r.Context(), queuedAudit)
		}
		if entry.directReader {
			go s.runDirectNativeReader(r.Context(), t, entry, directPrepare)
		}
		defer func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.runtime != nil && entry.claimed && !entry.completed && !t.closed {
				entry.detached = true
				return
			}
			t.removeAction(entry)
		}()
		t.notifyCapture()
		timeout := actionTimeout
		if timeout <= 0 {
			timeout = splitActionTimeout
		}
		if entry.brokerReady != nil {
			timeout = readerbroker.PreparationLifetime
		}
		finishAction := func(outcome string) {
			if entry.action.Type == "open_native_post" && s.logger != nil {
				s.logger.Printf("split_action action=%s request_id=%s phase=terminal outcome=%s elapsed_ms=%d", entry.action.ID, splitBrokerRequestID(entry.action), outcome, time.Since(entry.queuedAt).Milliseconds())
			}
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case result := <-entry.result:
			outcome := "rejected"
			if result.OK {
				outcome = "accepted"
			}
			finishAction(outcome)
			return writeJSON(w, 200, result)
		case <-t.done:
			finishAction("transport_stopped")
			return apiError{Status: 503, Code: "capture_stopped", Message: "Capture process stopped."}
		case <-r.Context().Done():
			finishAction("client_cancelled")
			return r.Context().Err()
		case <-timer.C:
			finishAction("timed_out")
			return apiError{Status: 504, Code: "capture_timeout", Message: "Capture action timed out; an already claimed action may still finish. It was not replayed."}
		}
	}
	if p == "/api/bridge/split-capture/next" && r.Method == http.MethodGet {
		pollID := domain.NewID("splitpoll")
		pollStartedAt := time.Now()
		if s.logger != nil {
			s.logger.Printf("split_capture_poll poll=%s phase=start outcome=active", pollID)
		}
		finishPoll := func(outcome string, action *splitCaptureAction) {
			if s.logger == nil {
				return
			}
			t.mu.Lock()
			snapshot := snapshotSplitActionQueue(t.actions)
			t.mu.Unlock()
			actionID, requestID := "-", "-"
			if action != nil {
				actionID = action.ID
				requestID = splitBrokerRequestID(*action)
			}
			s.logger.Printf("split_capture_poll poll=%s phase=end outcome=%s elapsed_ms=%d queue_total=%d unclaimed=%d claimed=%d native_unclaimed=%d native_waiting_broker=%d native_ready=%d action=%s request_id=%s", pollID, outcome, time.Since(pollStartedAt).Milliseconds(), snapshot.total, snapshot.unclaimed, snapshot.claimed, snapshot.nativeUnclaimed, snapshot.nativeWaitingBroker, snapshot.nativeReady, actionID, requestID)
		}
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		for {
			t.mu.Lock()
			if t.key != r.Header.Get("X-Aku-Capture-Instance") {
				t.mu.Unlock()
				return apiError{Status: 409, Code: "capture_instance_mismatch", Message: "Capture owner changed."}
			}
			for _, entry := range t.actions {
				if !entry.directReader && !entry.claimed && (entry.brokerReady == nil || entry.brokerAttached) {
					entry.claimed = true
					action := entry.action
					sourceLifetime := action.Type == "open_source" && t.prepareSourceWindow != nil
					hostClose := t.hostCloseSupported
					hostOnly := t.hostOnlySupported
					claimAudit, claimAuditReady := s.splitActionAuditRecord(action, "claimed", "accepted")
					t.mu.Unlock()
					if claimAuditReady {
						s.persistSplitActionAudit(r.Context(), claimAudit)
					}
					finishPoll("claimed", &action)
					return writeJSON(w, 200, map[string]any{"instanceEpoch": s.engine.Epoch(), "action": action, "sourceWindowLifetime": sourceLifetime, "captureHostClose": hostClose, "captureHostOnlyRetirement": hostOnly})
				}
			}
			t.mu.Unlock()
			select {
			case <-t.wake:
			case <-timer.C:
				finishPoll("idle_timeout", nil)
				w.WriteHeader(204)
				return nil
			case <-t.done:
				finishPoll("transport_stopped", nil)
				w.WriteHeader(410)
				return nil
			case <-r.Context().Done():
				finishPoll("client_cancelled", nil)
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
			if t.key != r.Header.Get("X-Aku-Capture-Instance") {
				return apiError{Status: 409, Code: "capture_instance_mismatch", Message: "Capture owner changed."}
			}
			if entry.action.ID == id && entry.claimed {
				if entry.directReader {
					return notFound("Bridge capture action")
				}
				if entry.completed {
					return apiError{Status: 409, Code: "capture_result_duplicate", Message: "Capture result was already received."}
				}
				if entry.action.Type == "open_source" && result.OK && !entry.sourcePrepared {
					t.untrackedSourceOutcome = true
				}
				if entry.action.Type == "open_native_post" {
					if result.OK && !entry.readerForegroundVerified {
						// Deliver a terminal failure to the waiting UI, rather than
						// rejecting this POST and leaving it waiting for a timeout.
						result = splitActionResult{OK: false, Message: "Native reader foreground was not verified. Reload AkuBridge to load the current runtime, then retry Open native post."}
					}
					if result.OK {
						s.logger.Printf("split_reader action=%s phase=result outcome=accepted", entry.action.ID)
					} else {
						s.logger.Printf("split_reader action=%s phase=result outcome=rejected", entry.action.ID)
					}
				}
				if entry.action.Type == "open_source" || entry.action.Type == "open_native_post" {
					outcome := "rejected"
					if result.OK {
						outcome = "accepted"
					}
					s.auditSplitAction(r.Context(), entry.action, "result", outcome)
				}
				select {
				case entry.result <- result:
					entry.completed = true
					entry.completionResult = &result
					entry.runtimeLease.Release()
					entry.runtimeLease = nil
					if entry.interactionRelease != nil {
						entry.interactionRelease()
						entry.interactionRelease = nil
					}
					if entry.detached {
						t.removeAction(entry)
					}
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
	case "/split-source-intent":
		id := r.URL.Query().Get("id")
		s.splitCapture.mu.Lock()
		allowed := false
		for _, entry := range s.splitCapture.actions {
			if entry.claimed && !entry.completed && entry.action.ID == id && entry.action.Type == "open_source" {
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
		_, _ = io.WriteString(w, "<!doctype html><html><head><title>"+html.EscapeString("AkuBrowser source "+id)+"</title></head><body>Opening source…</body></html>")
		return true
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
	case "/native-reader-idle":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><title>AkuBrowser native reader</title><p>Opening native post…</p>")
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
