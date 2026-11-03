package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"
)

func (s *Server) correctSelection(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/selection-corrections")
	if runID == "" || strings.Contains(runID, "/") {
		return notFound("run")
	}
	var body struct {
		CandidateRef string `json:"candidateRef"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if strings.TrimSpace(body.CandidateRef) == "" {
		return badRequest("candidateRef is required")
	}
	correction, item, err := s.engine.CorrectSelection(ctx, runID, body.CandidateRef)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("candidate")
	}
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"correction": correction, "item": item})
}

func (s *Server) reevaluateFailedRun(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/re-evaluate")
	if runID == "" || strings.Contains(runID, "/") {
		return notFound("run")
	}
	run, err := s.engine.ReevaluateFailedRun(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("run")
	}
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
}

func (s *Server) undoSelectionCorrection(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/undo"))
	correction, err := s.engine.UndoSelectionCorrection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("selection correction")
	}
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"correction": correction})
}
