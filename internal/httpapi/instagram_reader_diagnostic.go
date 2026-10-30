package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/engine"
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

func (s *Server) runInstagramReaderDiagnostic(w http.ResponseWriter, r *http.Request) error {
	if !s.engine.InstagramReaderDiagnosticConfigured() {
		return notFound("Instagram reader diagnostic")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: http.StatusConflict, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	if r.URL.RawQuery != "" {
		return badRequest("Instagram reader diagnostic does not accept query parameters")
	}
	bootstrap := r.URL.Path == "/api/diagnostics/instagram-bootstrap"
	keys := []string{"allowAuthenticatedReaderDiagnostic", "acknowledgeBoundedErrorBody"}
	if bootstrap {
		keys = []string{"allowPassiveBootstrapInspection"}
	}
	consent, err := readConsentFields(r, keys)
	accepted := err == nil
	for _, key := range keys {
		accepted = accepted && consent[key]
	}
	if !accepted {
		return badRequest("Instagram reader diagnostic requires exact, explicit consent")
	}
	s.appShellActionsMu.RLock()
	action := s.instagramReaderDiagnostic
	if bootstrap {
		action = s.instagramBootstrapDiagnostic
	}
	s.appShellActionsMu.RUnlock()
	if action == nil {
		return apiError{Status: http.StatusConflict, Code: "instagram_diagnostic_unavailable", Message: "The existing main browser diagnostic is unavailable."}
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return apiError{Status: http.StatusServiceUnavailable, Code: "instagram_diagnostic_unavailable", Message: "Instagram reader diagnostic could not be admitted."}
	}
	run := s.engine.RunInstagramReaderDiagnostic
	if bootstrap {
		run = s.engine.RunInstagramBootstrapDiagnostic
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
		return apiError{Status: http.StatusConflict, Code: "instagram_diagnostic_" + reason, Message: "Instagram reader diagnostic was rejected; no automatic retry was scheduled."}
	}
	w.Header().Set("Cache-Control", "no-store")
	return writeJSON(w, http.StatusOK, raw)
}
