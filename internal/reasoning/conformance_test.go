package reasoning

import (
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/conformance"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
	sdkcodex "github.com/abangkis/ai4u-inference-sdk-go/providers/codexappserver"
	"github.com/abangkis/ai4u-inference-sdk-go/providers/ollama"
)

func TestConformanceOllamaBindingContract(t *testing.T) {
	adapter, err := ollama.New(ollama.Config{BaseURL: "http://127.0.0.1:9", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	model := config.ModelConfig{Model: "nemotron-3.5-lightning", MinReasoningTier: "high"}
	profile, err := newExecutionProfile(ExecutionProfileEvaluation, model)
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.Run(conformance.Contract{
		Adapter: adapter,
		Binding: conformance.BindingCase{
			Profile: profile,
			Binding: inference.Binding{AdapterID: "ollama", ModelID: "nemotron-3.5-lightning"},
		},
	})
	if err != nil {
		t.Fatalf("ollama conformance: %v", err)
	}
	if !report.BindingValidated {
		t.Fatalf("binding was not validated: %+v", report)
	}
}

func TestConformanceCodexBindingContract(t *testing.T) {
	adapter, err := sdkcodex.New(sdkcodex.Config{
		WorkingDir:    t.TempDir(),
		Timeout:       time.Second,
		ClientName:    "AkuSidecar",
		ClientVersion: domain.ApplicationVersion,
		Start: func() (sdkcodex.Session, error) {
			return sdkcodex.Session{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	model := config.ModelConfig{Model: "codex-luna", MinReasoningTier: "high"}
	profile, err := newExecutionProfile(ExecutionProfileEvaluation, model)
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.Run(conformance.Contract{
		Adapter: adapter,
		Binding: conformance.BindingCase{
			Profile: profile,
			Binding: inference.Binding{AdapterID: "codex-app-server", ModelID: "codex-luna"},
		},
	})
	if err != nil {
		t.Fatalf("codex conformance: %v", err)
	}
	if !report.BindingValidated {
		t.Fatalf("binding was not validated: %+v", report)
	}
}

func TestConformanceOllamaRejectsUnknownModel(t *testing.T) {
	adapter, err := ollama.New(ollama.Config{BaseURL: "http://127.0.0.1:9", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	model := config.ModelConfig{Model: "nemotron-3.5-lightning", MinReasoningTier: "high"}
	profile, err := newExecutionProfile(ExecutionProfileEvaluation, model)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := conformance.ValidateBindingContract(adapter, profile, inference.Binding{AdapterID: "ollama", ModelID: "not-a-curated-model"}); err == nil {
		t.Fatal("unknown model must fail closed")
	}
}

func TestConformanceCodexEvaluationProviderStrict(t *testing.T) {
	adapter, err := sdkcodex.New(sdkcodex.Config{
		WorkingDir:    t.TempDir(),
		Timeout:       time.Second,
		ClientName:    "AkuSidecar",
		ClientVersion: domain.ApplicationVersion,
		Start: func() (sdkcodex.Session, error) {
			return sdkcodex.Session{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	model := config.ModelConfig{Model: "codex-luna", MinReasoningTier: "high", Assurance: "provider_strict"}
	profile, err := newExecutionProfile(ExecutionProfileEvaluation, model)
	if err != nil {
		t.Fatal(err)
	}
	resolved, _, err := conformance.ValidateBindingContract(adapter, profile, inference.Binding{AdapterID: "codex-app-server", ModelID: "codex-luna", AssurancePolicy: inference.AssurancePolicyProviderStrict})
	if err != nil {
		t.Fatalf("provider-strict evaluation binding: %v", err)
	}
	if resolved.OutputPlan.AssurancePolicy != inference.AssurancePolicyProviderStrict {
		t.Fatalf("resolved assurance=%q", resolved.OutputPlan.AssurancePolicy)
	}
	if _, _, err := conformance.ValidateBindingContract(adapter, profile, inference.Binding{AdapterID: "codex-app-server", ModelID: "codex-luna", AssurancePolicy: "bogus_policy"}); err == nil {
		t.Fatal("unknown assurance policy must fail closed")
	}
}
