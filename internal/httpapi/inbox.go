package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) inboxSessions(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	limit := boundedInt(r.URL.Query().Get("limit"), 12, 1, 25)
	offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
	sessions, total, err := s.engine.Inbox(ctx, limit, offset)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) inboxRunTrace(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/trace")
	if runID == "" || strings.Contains(runID, "/") {
		return notFound("run")
	}
	stage := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("stage")))
	if stage == "" {
		stage = "captured"
	}
	if stage != "captured" && stage != "skipped" && stage != "evaluated" && stage != "selected" && stage != "added" {
		return badRequest("stage must be captured, skipped, evaluated, selected, or added")
	}
	limit := boundedInt(r.URL.Query().Get("limit"), 10, 1, 20)
	offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
	trace, err := s.engine.InboxRunTrace(ctx, runID, stage, limit, offset)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("run")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"trace": trace})
}
