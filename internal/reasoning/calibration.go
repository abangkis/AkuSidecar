package reasoning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/calibration"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
	sdkcodex "github.com/abangkis/ai4u-inference-sdk-go/providers/codexappserver"
	"github.com/abangkis/ai4u-inference-sdk-go/providers/ollama"
)

// RunCalibration performs one bounded live capability check against the
// configured reasoning provider's evaluation model. It spawns an independent
// adapter process for Codex and never touches the managed runtime transport.
func RunCalibration(ctx context.Context, cfg config.Config) (calibration.Report, error) {
	model := cfg.Reasoning.Evaluation
	modelID := strings.TrimSpace(model.StableModelID())
	if modelID == "" {
		return calibration.Report{}, fmt.Errorf("calibration requires a configured evaluation model")
	}
	switch provider := strings.TrimSpace(cfg.Reasoning.Provider); {
	case provider == "codex-app-server":
		live, err := NewCodexAppServer(cfg)
		if err != nil {
			return calibration.Report{}, err
		}
		defer live.Close()
		adapter, err := sdkcodex.New(sdkcodex.Config{
			WorkingDir:    cfg.Root,
			Timeout:       time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
			ClientName:    "AkuSidecar",
			ClientVersion: domain.ApplicationVersion,
			Start:         live.startSession,
		})
		if err != nil {
			return calibration.Report{}, err
		}
		defer closeCalibrationAdapter(adapter)
		return calibration.Calibrate(ctx, calibration.Config{
			Adapter: adapter, AdapterID: "codex-app-server", ModelID: modelID,
			ReasoningOptionID: strings.TrimSpace(model.ExactReasoningOption()),
		})
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
	default:
		return calibration.Report{}, fmt.Errorf("calibration is unavailable for provider %q", provider)
	}
}

func closeCalibrationAdapter(adapter inference.Adapter) {
	if closer, ok := adapter.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
