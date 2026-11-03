package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func (s *Server) activeCalibration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	calibration, err := s.engine.CalibrationOverview(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"calibration": calibration.Active})
}

func (s *Server) startCalibration(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		UnifiedSessionID string `json:"unifiedSessionId"`
		TriggerKind      string `json:"triggerKind"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	calibration, err := s.engine.StartCalibration(ctx, body.UnifiedSessionID, body.TriggerKind)
	if err != nil {
		if errors.Is(err, engine.ErrCalibrationRequiresValidatedCandidate) {
			return apiError{Status: http.StatusConflict, Code: "calibration_sample_unavailable", Message: "The latest check produced no validated calibration entry. Choose Update now again."}
		}
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"calibration": calibration})
}

func (s *Server) calibrationSession(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(p)
	calibration, err := s.engine.Calibration(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("calibration")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"calibration": calibration})
}

func (s *Server) decideCalibrationSample(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	parts := strings.Split(strings.TrimPrefix(p, "/api/calibration/sessions/"), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "samples" {
		return notFound("calibration route")
	}
	ordinal, err := strconv.Atoi(parts[2])
	if err != nil || ordinal < 0 || ordinal > 9 {
		return badRequest("calibration sample ordinal must be between 0 and 9")
	}
	var decision domain.CalibrationDecision
	if err := readJSON(r, &decision); err != nil {
		return err
	}
	calibration, err := s.engine.DecideCalibration(ctx, parts[0], ordinal, decision)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"calibration": calibration})
}
