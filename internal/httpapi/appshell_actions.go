package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Server) openExtensionsHTTP(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !s.config.Dev || strings.TrimSpace(s.config.Deployment.Mode) != "development" {
		return notFound("app-shell development action")
	}
	openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.openExtensionsPage(openCtx); err != nil {
		return apiError{Status: http.StatusConflict, Code: "app_shell_action_unavailable", Message: "AkuBrowser could not open Chrome Extensions in its development profile.", Details: map[string]any{"reason": err.Error()}}
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}
