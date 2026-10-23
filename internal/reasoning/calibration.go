package reasoning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/ai4u-inference-sdk-go/calibration"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
	"github.com/abangkis/ai4u-inference-sdk-go/providers/ollama"
)

// ErrCalibrationUnsupported marks provider contracts that cannot honor the
// bounded request required by calibration.
var ErrCalibrationUnsupported = errors.New("calibration unsupported")

// RunCalibration performs one bounded live capability check against supported
// configured providers. Codex cannot honor calibration's bounded request, and
// Ollama must have a maintained stable model before any adapter is constructed.
func RunCalibration(ctx context.Context, cfg config.Config) (calibration.Report, error) {
	return runCalibrationWithAdapterFactory(ctx, cfg, newCalibrationAdapter)
}

type calibrationAdapterFactory func(provider string, cfg config.Config) (inference.Adapter, func(), error)

func runCalibrationWithAdapterFactory(ctx context.Context, cfg config.Config, adapterFactory calibrationAdapterFactory) (calibration.Report, error) {
	assurancePolicy, err := normalizeCalibrationAssurancePolicy(cfg.Reasoning.Evaluation.Assurance)
	if err != nil {
		return calibration.Report{}, err
	}

	providerName := strings.TrimSpace(cfg.Reasoning.Provider)
	if providerName == "codex-app-server" {
		return calibration.Report{}, fmt.Errorf("%w: Codex App Server rejects bounded MaxOutputTokens requests", ErrCalibrationUnsupported)
	}
	model := cfg.Reasoning.Evaluation
	modelID := strings.TrimSpace(model.StableModelID())
	if modelID == "" {
		return calibration.Report{}, fmt.Errorf("calibration requires a configured evaluation model")
	}

	adapterID := ""
	maxOutputTokens := 0
	switch provider := providerName; {
	case strings.HasPrefix(provider, "ollama"):
		if err := ValidateOllamaModelSupport(modelID); err != nil {
			return calibration.Report{}, err
		}
		adapterID = "ollama"
	case provider == "groq":
		adapterID = "groq"
	case config.IsGeminiProvider(provider):
		adapterID = "gemini"
		maxOutputTokens = modelMaxOutputTokens(model)
	default:
		return calibration.Report{}, fmt.Errorf("calibration is unavailable for provider %q", provider)
	}

	adapter, cleanup, err := adapterFactory(providerName, cfg)
	if err != nil {
		return calibration.Report{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return calibration.Calibrate(ctx, calibration.Config{
		Adapter: adapter, AdapterID: adapterID, ModelID: modelID,
		ReasoningOptionID: strings.TrimSpace(model.ExactReasoningOption()),
		AssurancePolicy:   assurancePolicy,
		MaxOutputTokens:   maxOutputTokens,
	})
}

func normalizeCalibrationAssurancePolicy(value string) (inference.AssurancePolicy, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return inference.AssurancePolicySDKValidated, nil
	}
	switch policy := inference.AssurancePolicy(normalized); policy {
	case inference.AssurancePolicySDKValidated, inference.AssurancePolicyProviderStrict:
		return policy, nil
	case inference.AssurancePolicyPromptFallback:
		return "", fmt.Errorf("calibration assurance policy %q is not enabled", normalized)
	default:
		return "", fmt.Errorf("calibration assurance policy %q is unsupported; use sdk_validated or provider_strict", normalized)
	}
}

func newCalibrationAdapter(providerName string, cfg config.Config) (inference.Adapter, func(), error) {
	switch provider := providerName; {
	case strings.HasPrefix(provider, "ollama"):
		endpoint := strings.TrimSpace(cfg.Reasoning.Endpoint)
		if endpoint == "" {
			endpoint = ollamaDefaultEndpoint
		}
		adapter, err := ollama.New(ollama.Config{
			BaseURL: endpoint,
			Timeout: time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		})
		if err != nil {
			return nil, nil, err
		}
		return adapter, func() { closeCalibrationAdapter(adapter) }, nil
	case provider == "groq":
		live, err := NewGroq(cfg)
		if err != nil {
			return nil, nil, err
		}
		return live.transport, func() { _ = live.Close() }, nil
	case config.IsGeminiProvider(provider):
		live, err := NewGemini(cfg)
		if err != nil {
			return nil, nil, err
		}
		return live.transport, func() { _ = live.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("calibration is unavailable for provider %q", provider)
	}
}

func closeCalibrationAdapter(adapter inference.Adapter) {
	if closer, ok := adapter.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
