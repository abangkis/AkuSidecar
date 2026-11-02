package httpapi

import (
	"net/http"
	"path"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) nextBridgeCommand(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	runID := r.URL.Query().Get("runId")
	if runID == "" {
		return badRequest("runId is required")
	}
	bridgeID := r.Header.Get("X-Aku-Bridge-Id")
	command, err := s.engine.ClaimCommand(ctx, runID, bridgeID)
	if err != nil {
		return err
	}
	if command == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	s.startNativeCaptureTrace(command)
	return writeJSON(w, http.StatusOK, map[string]any{"command": command})
}

func (s *Server) pendingBridgeCommand(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	runID, err := s.engine.PendingBridgeRunID(ctx)
	if err != nil {
		return err
	}
	if runID == "" {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return writeJSON(w, http.StatusOK, map[string]any{"runId": runID})
}

func (s *Server) acceptBridgeCommandObservation(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	commandID := path.Base(strings.TrimSuffix(p, "/observation"))
	var body struct {
		RunID       string             `json:"runId"`
		Observation domain.Observation `json:"observation"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	run, err := s.engine.AcceptObservation(ctx, commandID, body.RunID, body.Observation)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
}

func (s *Server) failBridgeCommand(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	commandID := path.Base(strings.TrimSuffix(p, "/failure"))
	var body struct {
		RunID string         `json:"runId"`
		Error domain.Failure `json:"error"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	run, err := s.engine.FailCommand(ctx, commandID, body.RunID, body.Error)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
