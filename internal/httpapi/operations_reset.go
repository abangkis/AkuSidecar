package httpapi

import (
	"net/http"
	"strings"
)

func (s *Server) resetLearning(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Confirmation string `json:"confirmation"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Confirmation != "RESET LEARNING" {
		return badRequest("learning reset requires the exact confirmation RESET LEARNING")
	}
	if err := s.engine.ResetLearning(ctx); err != nil {
		if strings.Contains(err.Error(), "update is running") {
			return conflict(err.Error())
		}
		return err
	}
	calibration, err := s.engine.CalibrationOverview(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"status": "reset", "operation": "reset_learning", "calibration": calibration})
}

func (s *Server) fullReset(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Confirmation string `json:"confirmation"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.Confirmation != "RESET AKUBROWSER" {
		return badRequest("full reset requires the exact confirmation RESET AKUBROWSER")
	}
	reset, err := s.engine.FullReset(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "update is running") {
			return conflict(err.Error())
		}
		return err
	}
	onboarding, err := s.engine.Onboarding(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"status": "reset", "operation": "full_reset", "reset": reset, "onboarding": onboarding})
}
