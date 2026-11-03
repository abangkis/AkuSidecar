package httpapi

import (
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) settings(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	settings, err := s.engine.Settings(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"provider": s.engine.ProviderName(), "settings": settings, "collectionRuntime": s.engine.CollectionRuntime(), "reasoningProviders": s.engine.ReasoningProviders(), "reasoningRuntime": s.engine.ReasoningRuntime(), "reasoningProcesses": s.engine.ReasoningProcesses(settings), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime()})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Settings           domain.Settings `json:"settings"`
		ConfirmationPhrase string          `json:"confirmationPhrase"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	current, err := s.engine.Settings(ctx)
	if err != nil {
		return err
	}
	if body.Settings.AIDetectionEnabled && body.Settings.AIDetectionPresentation == "hide" && current.AIDetectionPresentation != "hide" && body.ConfirmationPhrase != domain.AIHideConfirmationPhrase {
		return badRequest("activating Hide requires the exact confirmation phrase")
	}
	settings, err := s.engine.SaveSettings(ctx, body.Settings)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"provider": s.engine.ProviderName(), "settings": settings, "collectionRuntime": s.engine.CollectionRuntime(), "reasoningProviders": s.engine.ReasoningProviders(), "reasoningRuntime": s.engine.ReasoningRuntime(), "reasoningProcesses": s.engine.ReasoningProcesses(settings), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime()})
}
