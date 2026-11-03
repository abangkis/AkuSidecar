package httpapi

import (
	"net/http"
)

func (s *Server) confirmXCaptureRecovery(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		PauseID   string `json:"pauseId"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if !body.Confirmed {
		return badRequest("explicit manual recovery confirmation is required")
	}
	if err := s.engine.ConfirmXCaptureRecovery(ctx, body.PauseID); err != nil {
		return conflict(err.Error())
	}
	status, err := s.engine.AutoUpdateStatus(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
}
