package engine

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type routedProfileProvider struct {
	reasoning.Deterministic
	planningModel   config.ModelConfig
	evaluationModel config.ModelConfig
}

func (p *routedProfileProvider) Name() string { return "test-structured" }

func (p *routedProfileProvider) ProfileOptions() []reasoning.ProfileOption {
	return []reasoning.ProfileOption{
		{ID: "luna_xhigh", Label: "Luna XHigh", Model: "gpt-5.6-luna", Effort: "xhigh"},
		{ID: "sol_medium", Label: "Sol Medium", Model: "gpt-5.6-sol", Effort: "medium"},
	}
}

func (p *routedProfileProvider) ResolveProfile(id string) (config.ModelConfig, bool) {
	for _, option := range p.ProfileOptions() {
		if option.ID == id {
			return config.ModelConfig{Model: option.Model, Effort: option.Effort}, true
		}
	}
	return config.ModelConfig{}, false
}

func (p *routedProfileProvider) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, model config.ModelConfig) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	p.planningModel = model
	return p.Deterministic.Plan(ctx, run, observation, knowledge)
}

func (p *routedProfileProvider) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.evaluationModel = model
	return p.Deterministic.Analyze(ctx, run, observation, knowledge)
}

func TestReasoningProfilesUseBackendCatalogPerInvocation(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	provider := &routedProfileProvider{}
	runtime := New(state, provider, config.Config{}, log.New(io.Discard, "", 0))
	settings.ReasoningAcquisitionProfile = "sol_medium"
	settings.ReasoningEvaluationProfile = "luna_xhigh"
	profiles := runtime.ReasoningProcesses(settings)
	if profiles[0].Model != "gpt-5.6-sol" || profiles[0].Effort != "medium" || len(profiles[0].Options) != 2 {
		t.Fatalf("profiles=%+v", profiles)
	}
	run := domain.Run{ID: "run", SessionID: "session", Source: domain.SourceX}
	observation := domain.Observation{Source: run.Source}
	if _, _, err := runtime.planWithProfile(context.Background(), run, observation, nil, settings.ReasoningAcquisitionProfile); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.analyzeWithProfile(context.Background(), run, observation, nil, settings.ReasoningEvaluationProfile); err != nil {
		t.Fatal(err)
	}
	if provider.planningModel.Model != "gpt-5.6-sol" || provider.planningModel.Effort != "medium" || provider.evaluationModel.Model != "gpt-5.6-luna" || provider.evaluationModel.Effort != "xhigh" {
		t.Fatalf("planning=%+v evaluation=%+v", provider.planningModel, provider.evaluationModel)
	}
	settings.ReasoningAIDeepProfile = "unknown"
	if _, err := runtime.SaveSettings(context.Background(), settings); err == nil {
		t.Fatal("provider must reject a profile outside its catalog")
	}
}
