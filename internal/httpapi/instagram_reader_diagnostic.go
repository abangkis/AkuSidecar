package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

// SetInstagramReaderDiagnosticAction binds only an already-running browser.
// The callback receives a server-generated nonce, never caller-selected CDP.
func (s *Server) SetInstagramReaderDiagnosticAction(action func(context.Context, string) (json.RawMessage, error)) {
	s.appShellActionsMu.Lock()
	defer s.appShellActionsMu.Unlock()
	s.instagramReaderDiagnostic = action
}

func (s *Server) SetInstagramBootstrapDiagnosticAction(action func(context.Context, string) (json.RawMessage, error)) {
	s.appShellActionsMu.Lock()
	defer s.appShellActionsMu.Unlock()
	s.instagramBootstrapDiagnostic = action
}

func (s *Server) SetInstagramBootstrapCorroborationDiagnosticAction(action func(context.Context, string) (json.RawMessage, error)) {
	s.appShellActionsMu.Lock()
	defer s.appShellActionsMu.Unlock()
	s.instagramBootstrapCorroborationDiagnostic = action
}

func (s *Server) runInstagramReaderDiagnostic(w http.ResponseWriter, r *http.Request) error {
	path := strings.TrimSuffix(r.URL.Path, "/")
	bootstrap := path == "/api/diagnostics/instagram-bootstrap"
	corroboration := path == "/api/diagnostics/instagram-bootstrap-corroboration"
	operation := "reader"
	if bootstrap {
		operation = "bootstrap"
	} else if corroboration {
		operation = "corroboration"
	}
	ctx, attempt := igdiagnostic.Ensure(r.Context(), operation)
	if attempt == nil {
		return apiError{Status: http.StatusServiceUnavailable, Code: "instagram_diagnostic_unavailable", Message: "Instagram diagnostic correlation could not be created."}
	}
	r = r.WithContext(ctx)
	igdiagnostic.Note(ctx, "http")
	if attempt != nil {
		w.Header().Set("X-Aku-Diagnostic-ID", attempt.ID())
	}
	fail := func(status int, code, message, condition string) error {
		igdiagnostic.Fail(ctx, "http", condition)
		err := writeJSON(w, status, map[string]any{"error": code, "message": message, "details": map[string]any{"diagnostic": igdiagnostic.Projection(ctx)}})
		if err == nil {
			igdiagnostic.SetHTTPDelivery(ctx, "response_written", "none")
		} else {
			igdiagnostic.SetHTTPDelivery(ctx, "unavailable", "response_lost")
		}
		return err
	}
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		return notFound("Instagram reader diagnostic")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return fail(http.StatusConflict, "capture_epoch_mismatch", "AkuBrowser restarted; refresh the page.", "epoch_mismatch")
	}
	if r.URL.RawQuery != "" {
		return fail(http.StatusBadRequest, "invalid_request", "Instagram reader diagnostic does not accept query parameters", "query_rejected")
	}
	keys := []string{"allowAuthenticatedReaderDiagnostic", "acknowledgeBoundedErrorBody"}
	if bootstrap {
		keys = []string{"allowPassiveBootstrapInspection"}
	}
	if corroboration {
		keys = []string{"allowBootstrapCorroboration", "acknowledgeBoundedDocumentBody", "acknowledgeAccountCookieComparison"}
	}
	consent, err := readConsentFields(r, keys)
	accepted := err == nil
	for _, key := range keys {
		accepted = accepted && consent[key]
	}
	if !accepted {
		return fail(http.StatusBadRequest, "invalid_request", "Instagram reader diagnostic requires exact, explicit consent", "consent_required")
	}
	s.appShellActionsMu.RLock()
	action := s.instagramReaderDiagnostic
	if bootstrap {
		action = s.instagramBootstrapDiagnostic
	}
	if corroboration {
		action = s.instagramBootstrapCorroborationDiagnostic
	}
	s.appShellActionsMu.RUnlock()
	if action == nil {
		return fail(http.StatusConflict, "instagram_diagnostic_unavailable", "The existing main browser diagnostic is unavailable.", "action_missing")
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fail(http.StatusServiceUnavailable, "instagram_diagnostic_unavailable", "Instagram reader diagnostic could not be admitted.", "nonce_failed")
	}
	run := s.engine.RunInstagramReaderDiagnostic
	if bootstrap {
		run = s.engine.RunInstagramBootstrapDiagnostic
	}
	if corroboration {
		run = s.engine.RunInstagramBootstrapCorroborationDiagnostic
	}
	raw, err := run(r.Context(), func(ctx context.Context) (json.RawMessage, error) {
		return action(ctx, hex.EncodeToString(nonce[:]))
	})
	if err != nil {
		reason := "unavailable"
		var rejected *engine.InstagramReaderDiagnosticError
		if errors.As(err, &rejected) {
			switch rejected.Reason {
			case "unavailable", "canceled", "busy", "already_used", "maintenance_required", "source_busy", "owner_unavailable", "diagnostic_rejected", "projection_invalid":
				reason = rejected.Reason
			}
		}
		return fail(http.StatusConflict, "instagram_diagnostic_"+reason, "Instagram reader diagnostic was rejected; no automatic retry was scheduled.", "diagnostic_rejected")
	}
	igdiagnostic.Note(ctx, "http")
	w.Header().Set("Cache-Control", "no-store")
	err = writeJSON(w, http.StatusOK, raw)
	if err == nil {
		igdiagnostic.SetHTTPDelivery(ctx, "response_written", "none")
	} else {
		igdiagnostic.SetHTTPDelivery(ctx, "unavailable", "response_lost")
	}
	return err
}

// Observation only: the retained snapshot never dispatches browser work and
// remains readable while an admitted operation settles after client timeout.
func (s *Server) instagramDiagnosticStatus(w http.ResponseWriter, r *http.Request) error {
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		return notFound("Instagram reader diagnostic")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: http.StatusConflict, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	if r.URL.RawQuery != "" {
		return badRequest("Instagram diagnostic status does not accept query parameters")
	}
	raw := s.engine.InstagramDiagnosticSnapshot()
	if len(raw) == 0 {
		return writeJSON(w, http.StatusOK, map[string]any{"diagnostic": nil})
	}
	validated, ok := igdiagnostic.ValidateJSON(raw)
	if !ok {
		return apiError{Status: http.StatusConflict, Code: "instagram_diagnostic_projection_invalid", Message: "The retained diagnostic is unavailable."}
	}
	return writeJSON(w, http.StatusOK, map[string]any{"diagnostic": validated})
}
