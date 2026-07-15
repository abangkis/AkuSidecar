package reasoning

import (
	"context"

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
