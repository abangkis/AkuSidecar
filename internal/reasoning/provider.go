package reasoning

import (
	"context"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type AcquisitionPlan struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type Provider interface {
	Name() string
	Plan(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error)
	Analyze(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error)
}

// StructuredInvoker is the provider-neutral boundary used by schema-bound
// domain adapters such as Semantic Event resolution and AI Deep Detection.
// A replacement backend can implement this contract without exposing its
// transport details to either domain package.
type StructuredInvoker interface {
	InvokeStructured(context.Context, string, any, config.ModelConfig) (string, domain.ModelUsage, time.Duration, error)
}
