package reasoning

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

func calibrationTestConfig(t *testing.T, serverURL string) config.Config {
	t.Helper()
	return config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider:   "ollama-nemotron",
			Endpoint:   serverURL,
			TimeoutMS:  5000,
			Evaluation: config.ModelConfig{Model: "nemotron-3.5-lightning", Effort: "high"},
		},
	}
}

func TestRunCalibrationRejectsRetiredOllamaModelBeforeEndpoint(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := calibrationTestConfig(t, server.URL)
	report, err := RunCalibration(context.Background(), cfg)
	var unsupported *UnsupportedModelError
	if !errors.As(err, &unsupported) || unsupported.Provider != "ollama" || unsupported.ModelID != "nemotron-3.5-lightning" {
		t.Fatalf("error=%T %v, want typed retired-model error", err, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unsupported calibration contacted endpoint %d times", requests.Load())
	}
	if report.ModelID != "" || cfg.Reasoning.Evaluation.Model != "nemotron-3.5-lightning" {
		t.Fatalf("failed calibration report=%+v config model=%q", report, cfg.Reasoning.Evaluation.Model)
	}
	if !strings.Contains(err.Error(), "no model substitution was made") {
		t.Fatalf("unsupported error is not actionable: %v", err)
	}
}

func TestRunCalibrationRejectsCodexBeforeExecutableResolution(t *testing.T) {
	cfg := config.Config{Reasoning: config.ReasoningConfig{Provider: "codex-app-server"}}
	report, err := RunCalibration(context.Background(), cfg)
	if !errors.Is(err, ErrCalibrationUnsupported) {
		t.Fatalf("error=%v, want ErrCalibrationUnsupported", err)
	}
	if report.ModelID != "" {
		t.Fatalf("unsupported calibration emitted report: %+v", report)
	}
}

func TestRunCalibrationUnsupportedProviderFailsClosed(t *testing.T) {
	cfg := calibrationTestConfig(t, "http://127.0.0.1:9")
	cfg.Reasoning.Provider = "deterministic"
	report, err := RunCalibration(context.Background(), cfg)
	if err == nil {
		t.Fatal("deterministic provider must not calibrate")
	}
	if !strings.Contains(err.Error(), "unavailable for provider") {
		t.Fatalf("unexpected error=%v", err)
	}
	if report.ModelID != "" {
		t.Fatalf("failed configuration must not emit a report: %+v", report)
	}
}

func TestRunCalibrationRejectsUnsupportedAssuranceBeforeAdapterConstruction(t *testing.T) {
	for _, assurance := range []string{"prompt_fallback", "future_policy"} {
		t.Run(assurance, func(t *testing.T) {
			cfg := supportedCalibrationTestConfig(assurance)
			var factoryCalls atomic.Int32
			factory := func(string, config.Config) (inference.Adapter, func(), error) {
				factoryCalls.Add(1)
				return nil, nil, nil
			}

			report, err := runCalibrationWithAdapterFactory(context.Background(), cfg, factory)
			if err == nil || !strings.Contains(err.Error(), "calibration assurance policy") {
				t.Fatalf("error=%v, want unsupported calibration assurance policy", err)
			}
			if factoryCalls.Load() != 0 {
				t.Fatalf("adapter factory was called %d times before policy rejection", factoryCalls.Load())
			}
			if report.ModelID != "" {
				t.Fatalf("rejected calibration emitted a report: %+v", report)
			}
		})
	}
}

func TestRunCalibrationPassesNormalizedAssuranceToSDKAndReportsSuccess(t *testing.T) {
	tests := []struct {
		name      string
		assurance string
		want      inference.AssurancePolicy
	}{
		{name: "default", want: inference.AssurancePolicySDKValidated},
		{name: "explicit sdk validated", assurance: "sdk_validated", want: inference.AssurancePolicySDKValidated},
		{name: "provider strict", assurance: " provider_strict ", want: inference.AssurancePolicyProviderStrict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := supportedCalibrationTestConfig(test.assurance)
			adapter := newCalibrationSuccessAdapter(`{"ok":true,"label":"calibration"}`)
			var factoryCalls atomic.Int32
			factory := func(provider string, _ config.Config) (inference.Adapter, func(), error) {
				factoryCalls.Add(1)
				if provider != "groq" {
					t.Fatalf("adapter factory provider=%q, want groq", provider)
				}
				return adapter, func() { _ = adapter.Close() }, nil
			}

			report, err := runCalibrationWithAdapterFactory(context.Background(), cfg, factory)
			if err != nil {
				t.Fatalf("calibrate: %v", err)
			}
			if factoryCalls.Load() != 1 {
				t.Fatalf("adapter factory calls=%d, want one", factoryCalls.Load())
			}
			if report.AdapterID != "groq" || report.ModelID != "test-model" {
				t.Fatalf("calibration report route=%+v", report)
			}
			if report.RequestedAssurancePolicy != test.want {
				t.Fatalf("reported assurance=%q, want %q", report.RequestedAssurancePolicy, test.want)
			}
			if !report.Result.Success || report.Capability.ValidationStatus != "passed" {
				t.Fatalf("SDK calibration did not validate its synthetic response: %+v", report)
			}
			if report.Capability.Fallback || (report.Result.FallbackUsed != nil && *report.Result.FallbackUsed) {
				t.Fatalf("calibration unexpectedly used prompt fallback: %+v", report)
			}
		})
	}
}

func TestRunCalibrationReportsSDKValidationFailureForInvalidSyntheticResponse(t *testing.T) {
	cfg := supportedCalibrationTestConfig("sdk_validated")
	adapter := newCalibrationSuccessAdapter(`{"ok":true}`)
	factory := func(string, config.Config) (inference.Adapter, func(), error) {
		return adapter, func() { _ = adapter.Close() }, nil
	}

	report, err := runCalibrationWithAdapterFactory(context.Background(), cfg, factory)
	if err == nil {
		t.Fatal("calibration unexpectedly accepted a response missing a required schema property")
	}
	if report.RequestedAssurancePolicy != inference.AssurancePolicySDKValidated {
		t.Fatalf("reported assurance=%q, want %q", report.RequestedAssurancePolicy, inference.AssurancePolicySDKValidated)
	}
	if report.Result.Success || report.Capability.ValidationStatus != "failed" || report.Error == nil {
		t.Fatalf("invalid synthetic response did not produce a failed validation report: %+v", report)
	}
}

func supportedCalibrationTestConfig(assurance string) config.Config {
	return config.Config{Reasoning: config.ReasoningConfig{
		Provider: "groq",
		Evaluation: config.ModelConfig{
			Model: "test-model", Effort: "high", ReasoningOptionID: "high", Assurance: assurance,
		},
	}}
}

type calibrationSuccessAdapter struct {
	*poolTestAdapter
	responseText string
}

func newCalibrationSuccessAdapter(responseText string) *calibrationSuccessAdapter {
	base := newPoolTestAdapter()
	base.adapterID = "groq"
	catalog, err := inference.NewModelCatalog(inference.ModelDescriptor{
		ModelID: "test-model", ProviderModel: "test-model", Version: "1",
		Source: inference.CapabilitySourceDeclared,
		Capabilities: inference.ModelCapabilities{
			OutputCapabilities: []inference.OutputCapability{{
				Format: inference.ResponseFormatJSONSchema, Mechanism: inference.OutputMechanismProviderParameter,
				Enforcement: inference.OutputEnforcementProviderStrict, Provenance: "synthetic calibration test fixture",
			}},
			Assurance: []inference.AssuranceMode{inference.AssuranceModeNative},
			Reasoning: []inference.ReasoningOption{{
				ID: "high", NativeValue: "high", Tier: inference.ReasoningEffortHigh,
				Source: inference.TierSourceAdapter, Provenance: string(inference.TierSourceAdapter),
			}},
		},
	})
	if err != nil {
		panic(err)
	}
	base.catalog = catalog
	close(base.release)
	return &calibrationSuccessAdapter{poolTestAdapter: base, responseText: responseText}
}

func (a *calibrationSuccessAdapter) BindResolved(inference.ResolvedBinding) (inference.Client, error) {
	a.poolTestAdapter.bindCalls.Add(1)
	return &calibrationSuccessClient{poolTestClient: a.poolTestAdapter.client, responseText: a.responseText}, nil
}

type calibrationSuccessClient struct {
	*poolTestClient
	responseText string
}

func (c *calibrationSuccessClient) Generate(context.Context, inference.Request) (*inference.Response, error) {
	return &inference.Response{Text: c.responseText}, nil
}
