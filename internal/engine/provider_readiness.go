package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/codexruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	sdkollama "github.com/abangkis/ai4u-inference-sdk-go/providers/ollama"
)

const (
	providerReadinessTimeout = 6 * time.Second
	ollamaReadinessBodyLimit = 1 << 20
)

type ollamaReadinessSnapshot struct {
	models  []string
	status  string
	message string
}

// ReasoningProviderReadiness performs cost-free local readiness checks for
// providers whose runtime must exist before activation. Remote API providers
// remain governed by their credential status and are never called by this
// endpoint.
func (e *Engine) ReasoningProviderReadiness(ctx context.Context) ([]config.ProviderSummary, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	summaries := e.ReasoningProviders()
	ollamaSnapshots := map[string]ollamaReadinessSnapshot{}
	for index := range summaries {
		summaries[index] = e.probeReasoningProvider(ctx, summaries[index], settings, ollamaSnapshots)
	}
	return summaries, nil
}

func (e *Engine) ensureReasoningProviderReady(ctx context.Context, name string) error {
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	provider, found := e.reasoningProviderConfiguration(name)
	if !found {
		return fmt.Errorf("reasoning provider %q is not selectable", name)
	}
	if !provider.Configured {
		return fmt.Errorf("%s is not configured: set %s before selecting it", provider.Label, provider.CredentialName)
	}
	if !provider.AvailabilityRequired {
		return nil
	}
	provider = e.probeReasoningProvider(ctx, provider, settings, map[string]ollamaReadinessSnapshot{})
	if provider.Available {
		return nil
	}
	message := strings.TrimSpace(provider.AvailabilityMessage)
	if message == "" {
		message = "the required local runtime is unavailable"
	}
	return fmt.Errorf("%s is not available: %s", provider.Label, message)
}

func (e *Engine) probeReasoningProvider(ctx context.Context, summary config.ProviderSummary, settings domain.Settings, ollamaSnapshots map[string]ollamaReadinessSnapshot) config.ProviderSummary {
	if !summary.AvailabilityRequired || !summary.Configured {
		return summary
	}
	provider := e.config.Reasoning.Providers[summary.Name]
	switch summary.RuntimeKind {
	case "executable":
		return e.probeCodexReadiness(ctx, summary, provider, settings)
	case "local_endpoint":
		endpoint := strings.TrimSpace(provider.Endpoint)
		snapshot, cached := ollamaSnapshots[endpoint]
		if !cached {
			snapshot = probeOllamaEndpoint(ctx, endpoint)
			ollamaSnapshots[endpoint] = snapshot
		}
		summary.AvailabilityChecked = true
		if snapshot.status != "endpoint_ready" {
			summary.AvailabilityStatus = snapshot.status
			summary.AvailabilityMessage = snapshot.message
			return summary
		}
		expectedModel := provider.Planning.StableModelID()
		if descriptor, ok := sdkollama.AllModel(sdkollama.ModelID(expectedModel)); ok {
			expectedModel = descriptor.ProviderModel
		}
		if !ollamaModelAvailable(snapshot.models, expectedModel) {
			summary.AvailabilityStatus = "model_missing"
			summary.AvailabilityMessage = fmt.Sprintf("Ollama is running, but model %s is not installed.", expectedModel)
			return summary
		}
		summary.Available = true
		summary.AvailabilityStatus = "model_ready"
		summary.AvailabilityMessage = fmt.Sprintf("Ollama is running and model %s is installed.", expectedModel)
	}
	return summary
}

func (e *Engine) probeCodexReadiness(ctx context.Context, summary config.ProviderSummary, provider config.ProviderConfig, settings domain.Settings) config.ProviderSummary {
	summary.AvailabilityChecked = true
	var candidate reasoning.Provider
	var closeCandidate bool
	version := ""
	if e.ProviderName() == summary.Name {
		candidate = e.provider
	} else {
		requested := strings.TrimSpace(provider.Executable)
		if strings.TrimSpace(settings.ReasoningExecutablePath) != "" {
			requested = strings.TrimSpace(settings.ReasoningExecutablePath)
		}
		probeCtx, cancel := context.WithTimeout(ctx, providerReadinessTimeout)
		result, err := codexruntime.Discover(probeCtx, requested)
		cancel()
		if err != nil {
			summary.AvailabilityStatus = "runtime_missing"
			summary.AvailabilityMessage = "Codex App Server runtime was not found. Install Codex or choose a valid executable in Settings."
			return summary
		}
		version = strings.TrimSpace(result.Version)
		cfgCopy := e.config
		if err := cfgCopy.Reasoning.Select(summary.Name); err != nil {
			summary.AvailabilityStatus = "runtime_missing"
			summary.AvailabilityMessage = "Codex App Server configuration is invalid."
			return summary
		}
		cfgCopy.Reasoning.Executable = result.Executable
		candidate, err = reasoning.NewProvider(cfgCopy)
		if err != nil {
			summary.AvailabilityStatus = "runtime_missing"
			summary.AvailabilityMessage = "Codex App Server runtime could not be started."
			return summary
		}
		closeCandidate = true
	}
	if closeCandidate {
		defer func() {
			if closer, ok := candidate.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}()
	}
	readiness, ok := candidate.(reasoning.ReadinessProvider)
	if !ok {
		summary.AvailabilityStatus = "preflight_unsupported"
		summary.AvailabilityMessage = "Codex App Server runtime does not expose the required readiness check."
		return summary
	}
	probeCtx, cancel := context.WithTimeout(ctx, providerReadinessTimeout)
	err := readiness.Preflight(probeCtx)
	cancel()
	if err != nil {
		summary.AvailabilityStatus = "session_unavailable"
		summary.AvailabilityMessage = "Codex App Server started, but its signed-in session is not ready. Sign in to Codex, then check again."
		return summary
	}
	summary.Available = true
	summary.AvailabilityStatus = "session_ready"
	summary.AvailabilityMessage = "Codex App Server and its signed-in session are ready."
	if version != "" && version != "unknown" {
		summary.AvailabilityMessage = "Codex App Server " + version + " and its signed-in session are ready."
	}
	return summary
}

func probeOllamaEndpoint(ctx context.Context, endpoint string) ollamaReadinessSnapshot {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ollamaReadinessSnapshot{status: "endpoint_invalid", message: "The configured Ollama endpoint is invalid."}
	}
	tagsURL := *base
	tagsURL.Path = strings.TrimRight(tagsURL.Path, "/") + "/api/tags"
	tagsURL.RawQuery = ""
	tagsURL.Fragment = ""
	probeCtx, cancel := context.WithTimeout(ctx, providerReadinessTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, tagsURL.String(), nil)
	if err != nil {
		return ollamaReadinessSnapshot{status: "endpoint_invalid", message: "The configured Ollama endpoint is invalid."}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return ollamaReadinessSnapshot{status: "endpoint_unreachable", message: "Ollama is not reachable. Start Ollama, then check again."}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ollamaReadinessSnapshot{status: "endpoint_unreachable", message: fmt.Sprintf("Ollama returned HTTP %d while checking installed models.", response.StatusCode)}
	}
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, ollamaReadinessBodyLimit)).Decode(&payload); err != nil {
		return ollamaReadinessSnapshot{status: "endpoint_invalid_response", message: "Ollama returned an invalid model list."}
	}
	models := make([]string, 0, len(payload.Models)*2)
	for _, model := range payload.Models {
		models = append(models, model.Name, model.Model)
	}
	return ollamaReadinessSnapshot{models: models, status: "endpoint_ready", message: "Ollama is reachable."}
}

func ollamaModelAvailable(models []string, expected string) bool {
	expected = normalizeOllamaModelName(expected)
	for _, model := range models {
		if normalizeOllamaModelName(model) == expected {
			return true
		}
	}
	return false
}

func normalizeOllamaModelName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.TrimSuffix(value, ":latest")
}
