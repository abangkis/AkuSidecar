package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"
)

func (s *Server) startUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Intent string `json:"intent"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	session, err := s.engine.StartVisibleUpdate(ctx, body.Intent)
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
