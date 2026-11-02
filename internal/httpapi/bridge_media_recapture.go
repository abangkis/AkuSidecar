package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) claimBridgeMediaRecapture(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	id := path.Base(strings.TrimSuffix(p, "/claim"))
	recapture, err := s.engine.ClaimMediaRecapture(ctx, id, r.Header.Get("X-Aku-Bridge-Id"))
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("media recapture")
	}
	if err != nil {
		return conflict(err.Error())
	}
	if recapture.ID == "" {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return writeJSON(w, http.StatusOK, map[string]any{"recapture": recapture})
}

func (s *Server) acceptBridgeMediaRecaptureObservation(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	id := path.Base(strings.TrimSuffix(p, "/observation"))
	var body struct {
		Observation domain.Observation `json:"observation"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	recapture, err := s.engine.AcceptMediaRecapture(ctx, id, body.Observation)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"recapture": recapture})
}

func (s *Server) failBridgeMediaRecapture(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	id := path.Base(strings.TrimSuffix(p, "/failure"))
	var body struct {
		Error domain.Failure `json:"error"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	recapture, err := s.engine.FailMediaRecapture(ctx, id, body.Error)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"recapture": recapture})
}
