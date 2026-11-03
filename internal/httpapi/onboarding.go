package httpapi

import (
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) onboarding(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	onboarding, err := s.engine.Onboarding(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"onboarding": onboarding})
}

func (s *Server) completeOnboarding(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		ActiveSources []domain.Source `json:"activeSources"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	onboarding, err := s.engine.CompleteOnboarding(ctx, body.ActiveSources)
	if err != nil {
		return badRequest(err.Error())
	}
	settings, err := s.engine.Settings(ctx)
	if err != nil {
		return err
	}
	calibration, err := s.engine.CalibrationOverview(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"onboarding": onboarding, "settings": settings, "collectionRuntime": s.engine.CollectionRuntime(), "calibration": calibration})
}
