package httpapi

import (
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) bridgeHeartbeat(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireBridge(r); err != nil {
		return err
	}
	// The UI's legacy handshake may read capture readiness but cannot
	// manufacture/refresh a capture-process heartbeat in split mode.
	if s.splitCapture != nil && !s.splitCapture.authorized(r) {
		return writeJSON(w, http.StatusAccepted, map[string]any{"instanceEpoch": s.engine.Epoch(), "bridge": s.engine.BridgeStatus()})
	}
	var body struct {
		Capabilities domain.BridgeHeartbeat `json:"capabilities"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if origin := canonicalExtensionOrigin(r.Header.Get("Origin")); origin != "" {
		body.Capabilities.ExtensionOrigin = origin
	} else {
		body.Capabilities.ExtensionOrigin = canonicalExtensionOrigin(body.Capabilities.ExtensionOrigin)
	}
	if s.splitCapture != nil {
		s.splitCapture.mu.Lock()
		defer s.splitCapture.mu.Unlock()
		if s.splitCapture.key != r.Header.Get("X-Aku-Capture-Instance") {
			return apiError{Status: 409, Code: "capture_instance_mismatch", Message: "Capture owner changed."}
		}
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"instanceEpoch": s.engine.Epoch(), "bridge": s.engine.RecordHeartbeat(body.Capabilities)})
}

func (s *Server) bridgeHealth(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, http.StatusOK, map[string]any{"bridge": s.engine.BridgeStatus()})
}
