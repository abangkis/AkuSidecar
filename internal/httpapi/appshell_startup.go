package httpapi

import (
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

func (s *Server) SetAppShellStartup(startup *appshell.Startup) {
	s.appShellActionsMu.Lock()
	defer s.appShellActionsMu.Unlock()
	if s.appShellStartup != nil {
		s.appShellStartup.Stop()
	}
	s.appShellStartup = startup
}

func (s *Server) acknowledgeAppShellStartup(w http.ResponseWriter, r *http.Request) error {
	s.appShellActionsMu.RLock()
	defer s.appShellActionsMu.RUnlock()
	w.Header().Set("Cache-Control", "no-store")
	if !s.appShellStartup.Acknowledge(r) {
		return apiError{Status: http.StatusForbidden, Code: "startup_acknowledgement_denied", Message: "Startup acknowledgement was not accepted."}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
