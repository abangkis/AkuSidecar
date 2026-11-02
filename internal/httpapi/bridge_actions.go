package httpapi

import (
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/engine"
)

func (s *Server) requestBridgeReload(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireBridge(r); err != nil {
		return err
	}
	var body struct {
		RequestID string `json:"requestId"`
		Actor     any    `json:"actor"`
		Reason    string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	action, err := s.engine.RequestBridgeReload(body.RequestID, body.Actor, body.Reason)
	if errors.Is(err, engine.ErrActionConflict) {
		return conflict(err.Error())
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"action": action})
}

func (s *Server) nextBridgeAction(w http.ResponseWriter, r *http.Request) error {
	if err := s.requireBridge(r); err != nil {
		return err
	}
	wait := boundedInt(r.URL.Query().Get("waitMs"), 0, 0, 30000)
	action, err := s.engine.NextBridgeAction(time.Duration(wait)*time.Millisecond, r.Context().Done())
	if err != nil {
		return err
	}
	if action == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return writeJSON(w, http.StatusOK, map[string]any{"action": action})
}

func (s *Server) acceptBridgeAction(w http.ResponseWriter, r *http.Request, p string) error {
	if err := s.requireBridge(r); err != nil {
		return err
	}
	id := path.Base(strings.TrimSuffix(p, "/accept"))
	action, err := s.engine.AcceptBridgeAction(id)
	if errors.Is(err, engine.ErrActionNotFound) {
		return notFound("bridge action")
	}
	if errors.Is(err, engine.ErrActionConflict) {
		return conflict(err.Error())
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"action": action})
}

func (s *Server) bridgeActionStatus(w http.ResponseWriter, r *http.Request, p string) error {
	if err := s.requireBridge(r); err != nil {
		return err
	}
	id := path.Base(p)
	action, err := s.engine.BridgeAction(id)
	if errors.Is(err, engine.ErrActionNotFound) {
		return notFound("bridge action")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"action": action})
}
