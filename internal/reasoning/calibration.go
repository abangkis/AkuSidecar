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
	providerName := strings.TrimSpace(cfg.Reasoning.Provider)
	if providerName == "codex-app-server" {
		return calibration.Report{}, fmt.Errorf("%w: Codex App Server rejects bounded MaxOutputTokens requests", ErrCalibrationUnsupported)
	}
	model := cfg.Reasoning.Evaluation
	modelID := strings.TrimSpace(model.StableModelID())
	if modelID == "" {
		return calibration.Report{}, fmt.Errorf("calibration requires a configured evaluation model")
	}
	switch provider := providerName; {
	case strings.HasPrefix(provider, "ollama"):
		if err := ValidateOllamaModelSupport(modelID); err != nil {
			return calibration.Report{}, err
		}
		endpoint := strings.TrimSpace(cfg.Reasoning.Endpoint)
		if endpoint == "" {
			endpoint = ollamaDefaultEndpoint
		}
		adapter, err := ollama.New(ollama.Config{
			BaseURL: endpoint,
			Timeout: time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		})
		if err != nil {
			return calibration.Report{}, err
		}
		defer closeCalibrationAdapter(adapter)
		return calibration.Calibrate(ctx, calibration.Config{
			Adapter: adapter, AdapterID: "ollama", ModelID: modelID,
			ReasoningOptionID: strings.TrimSpace(model.ExactReasoningOption()),
		})
	case provider == "groq":
		live, err := NewGroq(cfg)
		if err != nil {
			return calibration.Report{}, err
		}
		defer live.Close()
		return calibration.Calibrate(ctx, calibration.Config{
			Adapter: live.transport, AdapterID: "groq", ModelID: modelID,
			ReasoningOptionID: strings.TrimSpace(model.ExactReasoningOption()),
		})
	case config.IsGeminiProvider(provider):
		live, err := NewGemini(cfg)
		if err != nil {
			return calibration.Report{}, err
		}
		defer live.Close()
		return calibration.Calibrate(ctx, calibration.Config{
			Adapter: live.transport, AdapterID: "gemini", ModelID: modelID,
			ReasoningOptionID: strings.TrimSpace(model.ExactReasoningOption()),
			MaxOutputTokens:   modelMaxOutputTokens(model),
		})
	default:
		return calibration.Report{}, fmt.Errorf("calibration is unavailable for provider %q", provider)
	}
}

func closeCalibrationAdapter(adapter inference.Adapter) {
	if closer, ok := adapter.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
