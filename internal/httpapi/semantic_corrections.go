package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"
)

func (s *Server) semanticEventSuggestions(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/event-suggestions"))
	limit := boundedInt(r.URL.Query().Get("limit"), 3, 1, 3)
	suggestions, err := s.engine.SemanticEventSuggestions(ctx, id, limit)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

func (s *Server) correctSemanticEvent(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/event-correction"))
	var body struct {
		Action        string `json:"action"`
		TargetEventID string `json:"targetEventId"`
		Relation      string `json:"relation"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	correction, err := s.engine.CorrectSemanticEvent(ctx, id, body.Action, body.TargetEventID, body.Relation)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("semantic event report")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"correction": correction})
}

func (s *Server) undoSemanticCorrection(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/undo"))
	correction, err := s.engine.UndoSemanticCorrection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("event correction")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"correction": correction})
}
