package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) startUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Intent        string                `json:"intent"`
		CaptureTarget *domain.CaptureTarget `json:"captureTarget,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.CaptureTarget != nil {
		if err := body.CaptureTarget.Validate(); err != nil {
			return badRequest(err.Error())
		}
	}
	session, err := s.engine.StartVisibleTargetUpdate(ctx, body.Intent, body.CaptureTarget)
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func (s *Server) activeSession(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	session, err := s.engine.ActiveSession(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"session": sessionProgressProjection(session)})
}

func (s *Server) sessionDetail(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(p)
	session, err := s.engine.Session(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("session")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"session": sessionProgressProjection(&session)})
}

func (s *Server) cancelSession(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/cancel"))
	if err := s.engine.CancelSession(ctx, id); err != nil {
		return err
	}
	session, err := s.engine.Session(ctx, id)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"session": sessionProgressProjection(&session)})
}

func (s *Server) runDetail(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(p)
	run, err := s.engine.Run(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("run")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
