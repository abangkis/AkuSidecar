package httpapi

import (
	"net/http"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func (s *Server) reasoningProviderReadiness(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	providers, err := s.engine.ReasoningProviderReadiness(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"reasoningProviders": providers})
}

func (s *Server) saveReasoningCredential(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Provider string `json:"provider"`
		Secret   string `json:"secret"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	providerName := strings.TrimSpace(body.Provider)
	provider, ok := s.config.Reasoning.Providers[providerName]
	if !ok || provider.HideFromSettings {
		return apiError{Status: http.StatusBadRequest, Code: "unknown_provider", Message: "The selected reasoning provider is unavailable."}
	}
	credentialRef := strings.TrimSpace(provider.CredentialRef)
	if credentialRef == "" {
		return apiError{Status: http.StatusBadRequest, Code: "credential_not_supported", Message: "The selected reasoning provider does not use an API credential."}
	}
	secret := strings.TrimSpace(body.Secret)
	if secret == "" {
		return badRequest("credential value is required")
	}
	var validation reasoning.GeminiCredentialValidation
	if config.IsGeminiProvider(providerName) {
		validation = reasoning.ValidateGeminiCredential(ctx, provider.Endpoint, provider.Planning.StableModelID(), secret)
		if validation.Status != reasoning.GeminiCredentialValid {
			return apiError{Status: validation.HTTPStatus, Code: "gemini_" + validation.Status, Message: validation.Message, Details: map[string]any{"provider": providerName, "model": validation.Model, "validationStatus": validation.Status}}
		}
	}
	if err := s.credentials.Put(credentialRef, secret); err != nil {
		s.logger.Printf("secure credential write failed for ref %q: %v", credentialRef, err)
		return apiError{Status: http.StatusInternalServerError, Code: "credential_store_failed", Message: "AkuBrowser could not save this credential securely."}
	}
	body.Secret = ""
	providerSummaries := s.engine.ReasoningProviders()
	for index := range providerSummaries {
		if providerSummaries[index].Name == providerName {
			providerSummaries[index].Configured = true
			providerSummaries[index].ConfigurationStatus = "ready"
			if config.IsGeminiProvider(providerName) {
				providerSummaries[index].AvailabilityChecked = true
				providerSummaries[index].Available = true
				providerSummaries[index].AvailabilityStatus = reasoning.GeminiCredentialValid
				providerSummaries[index].AvailabilityMessage = validation.Message
			}
		}
	}
	response := map[string]any{
		"credential":         map[string]any{"provider": providerName, "reference": credentialRef, "configured": true},
		"reasoningProviders": providerSummaries,
	}
	if config.IsGeminiProvider(providerName) {
		response["validation"] = map[string]any{"status": validation.Status, "model": validation.Model, "message": validation.Message}
	}
	return writeJSON(w, http.StatusOK, response)
}

func (s *Server) discoverReasoningRuntime(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	runtime, err := s.engine.DiscoverReasoningExecutable(ctx)
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"reasoningRuntime": runtime})
}
