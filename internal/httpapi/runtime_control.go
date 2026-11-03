package httpapi

import (
	"net/http"
	"time"
)

func (s *Server) runtimeUpdateReadiness(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	ready, reason, err := s.engine.RuntimeUpdateReadiness(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{
		"ready": ready, "reason": reason, "instanceEpoch": s.engine.Epoch(),
		"controlAvailable": s.config.RuntimeControlToken != "",
	})
}

func (s *Server) shutdownIfIdle(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !validRuntimeControlToken(s.config.RuntimeControlToken, r.Header.Get("X-Aku-Runtime-Control-Token")) {
		return apiError{Status: http.StatusForbidden, Code: "runtime_control_denied", Message: "Runtime control authorization failed."}
	}
	ready, reason, err := s.engine.RuntimeUpdateReadiness(ctx)
	if err != nil {
		return err
	}
	if !ready {
		return apiError{Status: http.StatusConflict, Code: "runtime_busy", Message: "Runtime update is blocked by active work.", Details: map[string]any{"reason": reason}}
	}
	if err := writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "instanceEpoch": s.engine.Epoch()}); err != nil {
		return err
	}
	go func() {
		time.Sleep(25 * time.Millisecond)
		s.requestShutdown()
	}()
	return nil
}
