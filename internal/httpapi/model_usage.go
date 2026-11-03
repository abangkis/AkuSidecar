package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) sessionModelUsage(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	sessionID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/sessions/"), "/model-usage")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		return notFound("session")
	}
	usage, err := s.engine.SessionModelUsage(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("session")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"usage": usage})
}

func (s *Server) aggregateModelUsage(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	windowDays := boundedInt(r.URL.Query().Get("windowDays"), 30, 7, 90)
	if windowDays != 7 && windowDays != 30 && windowDays != 90 {
		return badRequest("windowDays must be 7, 30, or 90")
	}
	usage, err := s.engine.AggregateModelUsage(ctx, windowDays)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"usage": usage})
}
