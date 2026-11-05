package reasoning

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/credentials"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/connectors/openrouter"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

const (
	GLMPlanningShadowCredential = "openrouter.planning-shadow"
	GLMPlanningShadowModel      = string(openrouter.ModelGLM53Flash)
	GLMPlanningShadowTimeout    = 10 * time.Second
	glmPlanningShadowTokens     = 2048
)

// GLMPlanningShadow is bound exclusively to the Acquisition Planning schema.
type GLMPlanningShadow struct {
	client inference.Client
	schema json.RawMessage
}

func NewGLMPlanningShadow(cfg config.Config) (*GLMPlanningShadow, error) {
	if !cfg.Dev {
		return nil, errors.New("GLM planning shadow requires development mode")
	}
	return newGLMPlanningShadow(cfg.Root, credentials.ForRuntime(cfg.Root, cfg.Dev), "")
}

func newGLMPlanningShadow(root string, resolver credentials.Resolver, baseURL string) (*GLMPlanningShadow, error) {
	key, err := resolver.Resolve(GLMPlanningShadowCredential)
	if err != nil {
		return nil, errors.New("OpenRouter planning shadow credential is unavailable")
	}
	schema, err := readSchema(filepath.Join(root, "schemas", "acquisition-plan.schema.json"))
	if err != nil {
		return nil, err
	}
	rawSchema, err := schemaJSON(schema)
	if err != nil {
		return nil, err
	}
	profile, err := newExecutionProfile(ExecutionProfilePlanning, config.ModelConfig{
		MinReasoningTier: "high", MaxOutputTokens: glmPlanningShadowTokens,
	})
	if err != nil {
		return nil, err
	}
	adapter, err := openrouter.New(openrouter.Config{
		APIKey: key, BaseURL: baseURL, Timeout: GLMPlanningShadowTimeout,
		PrivacyPolicy: inference.StrictZeroRetentionPolicy(),
		Bindings: map[inference.ProfileID]openrouter.Binding{
			ExecutionProfilePlanning: {
				ID: "akusidecar-planning-glm-shadow", Version: "1", ModelID: GLMPlanningShadowModel,
				ReasoningOptionID: "high", AssurancePolicy: inference.AssurancePolicySDKValidated,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	client, err := adapter.Bind(profile)
	if err != nil {
		return nil, err
	}
	return &GLMPlanningShadow{client: client, schema: rawSchema}, nil
}

type GLMPlanningShadowInput struct{ prompt string }

// NewGLMPlanningShadowInput projects only the bounded telemetry vocabulary onto
// the shared planner prompt. Unknown text is omitted before leaving Sidecar.
func NewGLMPlanningShadowInput(run domain.Run, observation domain.Observation) GLMPlanningShadowInput {
	input := planningPromptObservation(observation)
	source := run.Source
	if !source.Valid() {
		source = ""
	}
	input.Source = source
	switch input.ScrollStopReason {
	case "not_requested", "budget_exhausted", "deadline", "no_movement", "scroll_position_unavailable",
		"evidence_size_limit", "no_new_candidates", "structured_fallback":
	default:
		input.ScrollStopReason = ""
	}
	switch input.CaptureQuality.Verdict {
	case "complete", "usable_degraded", "retryable", "invalid":
	default:
		input.CaptureQuality.Verdict = ""
	}
	return GLMPlanningShadowInput{prompt: formatPlanningPrompt(source, input)}
}

func (p *GLMPlanningShadow) Plan(ctx context.Context, input GLMPlanningShadowInput) (string, domain.ModelUsage, error) {
	started := time.Now()
	response, err := p.client.Generate(ctx, structuredRequest(ExecutionProfilePlanning, input.prompt, p.schema, glmPlanningShadowTokens))
	elapsed := time.Since(started)
	if err != nil {
		return "", modelUsageFromInferenceFailure(err, elapsed), err
	}
	usage := modelUsageFromInferenceEvidence(response.Usage, &response.Receipt, response.Timing, response.DurationMillis, elapsed)
	var plan AcquisitionPlan
	if err := json.Unmarshal([]byte(response.Text), &plan); err != nil || (plan.Decision != "finish" && plan.Decision != "request_follow_up") {
		return "", usage, inference.MarkOutputEvidence(err, response.Text)
	}
	return plan.Decision, usage, nil
}
