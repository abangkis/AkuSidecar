package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) addAIFeedback(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/ai-feedback"))
	var body domain.AIFeedbackInput
	if err := readJSON(r, &body); err != nil {
		return err
	}
	feedback, err := s.engine.AddAIFeedback(ctx, id, body)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"feedback": feedback})
}

func (s *Server) aiFeedbackHistory(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/ai-feedback"))
	feedback, err := s.engine.AIFeedbackHistory(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
}

func (s *Server) undoAIFeedback(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/undo"))
	feedback, err := s.engine.UndoAIFeedback(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("AI feedback")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
}
