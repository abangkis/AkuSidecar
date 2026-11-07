package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSettingsSwitchesGeminiModelsWithoutRestart(t *testing.T) {
	ctx := context.Background()
	runtime, state := swapTestEngine(t)
	cfg, err := config.Load(config.Options{
		ConfigPath:            filepath.Join("..", "..", "config", "sidecar.json"),
		BridgeExtensionOrigin: "chrome-extension://mfeebfabkhmoaepbcdbbeefpobkedfmp",
	})
	if err != nil {
		t.Fatal(err)
	}
	requestedModels := make(chan string, 4)
	var flash36Unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("Gemini variant did not reuse the configured credential")
			http.Error(w, "wrong credential", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/models/") {
			if r.URL.Path == "/models/gemini-3.8-flash" {
				t.Error("unsupported model must not reach the provider")
			}
			if r.URL.Path == "/models/gemini-3.6-flash" && flash36Unavailable.Load() {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprintf(w, `{"name":%q}`, r.URL.Path[1:])
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/interactions" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "wrong route", http.StatusBadRequest)
			return
		}
		var request struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requestedModels <- request.Model
		actual := request.Model
		if actual == "gemini-3.7-flash" {
			actual = "gemini-3.8-flash"
		}
		_, _ = fmt.Fprintf(w, `{"id":%q,"model":%q,"status":"completed","output_text":"{\"decision\":\"finish\",\"reason\":\"continuation_blocked\"}"}`, request.Model, actual)
	}))
	defer server.Close()
	for name, provider := range cfg.Reasoning.Providers {
		if config.IsGeminiProvider(name) {
			provider.Endpoint, provider.CredentialRef = server.URL, "gemini.test"
			runtime.config.Reasoning.Providers[name] = provider
		}
	}
	readiness, err := runtime.ReasoningProviderReadiness(ctx)
	if err != nil {
		t.Fatal(err)
	}
	available := map[string]config.ProviderSummary{}
	for _, summary := range readiness {
		available[summary.Name] = summary
	}
	current, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, label, model, actual string }{
		{"gemini-flash-lite", "Gemini · 3.1 Flash-Lite", "gemini-3.1-flash-lite", "gemini-3.1-flash-lite"},
		{"gemini-3-5-flash-lite", "Gemini · 3.5 Flash-Lite", "gemini-3.5-flash-lite", "gemini-3.5-flash-lite"},
		{"gemini-3-6-flash", "Gemini · 3.6 Flash", "gemini-3.6-flash", "gemini-3.6-flash"},
		{"gemini-3-7-flash", "Gemini · 3.7 Flash", "gemini-3.7-flash", "gemini-3.8-flash"},
	} {
		if summary := available[test.name]; !summary.Available || summary.Label != test.label {
			t.Fatalf("%s readiness = %+v", test.name, summary)
		}
		current.ReasoningProvider = test.name
		current, err = runtime.SaveSettings(ctx, current)
		if err != nil || current.ReasoningProvider != test.name || runtime.ProviderName() != test.name {
			t.Fatalf("could not activate %s: %v", test.name, err)
		}
		for _, process := range runtime.ReasoningProcesses(current) {
			if process.Model != test.model {
				t.Errorf("%s model = %q, want %q", process.ID, process.Model, test.model)
			}
		}
		plan, receipt, err := runtime.planWithProfile(ctx, domain.Run{Source: domain.SourceX}, domain.Observation{}, nil, current.ReasoningAcquisitionProfile)
		if err != nil || plan.Decision != "finish" || receipt.Model != test.actual {
			t.Fatalf("%s planning: decision=%q receipt=%+v error=%v", test.name, plan.Decision, receipt, err)
		}
		select {
		case requested := <-requestedModels:
			if requested != test.model {
				t.Fatalf("%s requested %q instead of %q", test.name, requested, test.model)
			}
		default:
			t.Fatalf("%s did not send a model request", test.name)
		}
	}
	if len(available) != 5 { // The four Gemini choices plus the deterministic test provider.
		t.Fatalf("unexpected provider choices: %+v", available)
	}
	flash36Unavailable.Store(true)
	current.ReasoningProvider = "gemini-3-6-flash"
	if _, err := runtime.SaveSettings(ctx, current); err == nil || runtime.ProviderName() != "gemini-3-7-flash" {
		t.Fatalf("unavailable model changed the active provider: %v", err)
	}
	current.ReasoningProvider = "gemini-flash-lite"
	if _, err := runtime.SaveSettings(ctx, current); err != nil {
		t.Fatalf("could not switch back to 3.1 while 3.6 is unavailable: %v", err)
	}
	persisted, err := state.GetSettings(ctx)
	if err != nil || persisted.ReasoningProvider != "gemini-flash-lite" || runtime.ProviderName() != persisted.ReasoningProvider {
		t.Fatalf("switch did not persist the selected provider: %q, %v", persisted.ReasoningProvider, err)
	}
}
