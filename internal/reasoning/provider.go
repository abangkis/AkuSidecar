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

type ProfileOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// ProfileProvider lets a backend publish and resolve its own bounded model
// choices. Settings stores only opaque profile IDs, so an alternate backend can
// replace this catalog without changing domain settings or web rendering.
type ProfileProvider interface {
	ProfileOptions() []ProfileOption
	ResolveProfile(string) (config.ModelConfig, bool)
}

type RoutedProvider interface {
	PlanWithModel(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem, config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error)
	AnalyzeWithModel(context.Context, domain.Run, domain.Observation, []domain.ReasonedItem, config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error)
}
