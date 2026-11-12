package reasoning

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func TestCodexLegacyProfileMigrationIsBounded(t *testing.T) {
	provider := &CodexAppServer{}
	for _, tc := range []struct{ old, canonical string }{
		{"luna_high", "gpt6_luna_high"}, {"luna_xhigh", "gpt6_luna_xhigh"}, {"luna_max", "gpt6_luna_max"},
		{"terra_high", "gpt61_terra_high"}, {"terra_xhigh", "gpt61_terra_xhigh"}, {"sol_medium", "gpt61_sol_medium"},
	} {
		t.Run(tc.old, func(t *testing.T) {
			got := EnsureResolvableProfile(provider, tc.old)
			if got != tc.canonical {
				t.Fatalf("migration=%q, want %q", got, tc.canonical)
			}
			model, ok := provider.ResolveProfile(got)
			if !ok || model.ProfileID != got || model.Effort != tc.old[strings.LastIndex(tc.old, "_")+1:] {
				t.Fatalf("migration lost declared identity or effort: model=%+v ok=%v", model, ok)
			}
			if again := EnsureResolvableProfile(provider, got); again != got {
				t.Fatalf("migration was not idempotent: %q", again)
			}
		})
	}
	for _, id := range []string{"", "unknown", "sol_xhigh", "terra_max", "luna_medium", "astra_max", "gpt6_sol_max"} {
		if _, migrated := provider.MigrateProfileID(id); migrated {
			t.Fatalf("unadvertised legacy profile %q was migrated", id)
		}
		if got := EnsureResolvableProfile(provider, id); got != "gpt6_luna_high" {
			t.Fatalf("unknown profile %q fallback=%q, want high", id, got)
		}
	}
}

func TestCodexDispatchRejectsRemovedAndConflictingModels(t *testing.T) {
	provider := &CodexAppServer{}
	for _, id := range []string{"codex-luna", "codex-terra", "codex-sol", "gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol", "codex-gpt-6-sol", "gpt-6-sol", "gpt-6-astra", "gpt-6.1-astra", "unknown"} {
		for _, model := range []config.ModelConfig{{Model: id}, {ModelID: id, Model: "gpt-6-luna"}, {ModelID: "codex-gpt-6-luna", Model: id}} {
			_, _, _, err := provider.invoke(context.Background(), ExecutionProfilePlanning, "prompt", nil, model)
			if err == nil || (!strings.Contains(err.Error(), "unknown Codex") && !strings.Contains(err.Error(), "conflicting or unsupported")) {
				t.Fatalf("removed model reached transport: model=%+v err=%v", model, err)
			}
		}
	}
	_, _, _, err := provider.invoke(context.Background(), ExecutionProfilePlanning, "prompt", nil, config.ModelConfig{ModelID: "codex-gpt-6.1-sol", Model: "gpt-6.1-terra"})
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflicting supported models reached transport: %v", err)
	}
}

// Exercise the product profile resolver, binding, SDK transport, and native
// protocol with a local synthetic server. No account or real inference is used.
func TestCodexProfilesResolveBindAndSerializeNativeWire(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	capture := filepath.Join(t.TempDir(), "wire.jsonl")
	t.Setenv("AKU_FAKE_CODEX_WIRE_CAPTURE", capture)
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	options := provider.ProfileOptions()
	for _, option := range options {
		model, ok := provider.ResolveProfile(option.ID)
		if !ok {
			t.Fatalf("profile %q did not resolve", option.ID)
		}
		plan, telemetry, err := provider.PlanWithModel(context.Background(), run, observation, nil, string(ExecutionProfilePlanning), model)
		if err != nil || plan.Decision != "finish" || telemetry.Model != option.Model || telemetry.Effort != option.Effort {
			t.Fatalf("profile %q: plan=%+v telemetry=%+v err=%v", option.ID, plan, telemetry, err)
		}
	}
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	requestIndex := 0
	for scanner.Scan() {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		if requestIndex >= len(options)*2 {
			t.Fatalf("unexpected extra wire request: %s", scanner.Bytes())
		}
		option := options[requestIndex/2]
		wantMethod := "thread/start"
		if requestIndex%2 == 1 {
			wantMethod = "turn/start"
			if request.Params["effort"] != option.Effort {
				t.Fatalf("profile %q native effort=%v", option.ID, request.Params["effort"])
			}
		}
		if request.Method != wantMethod || request.Params["model"] != option.Model {
			t.Fatalf("profile %q native request=%+v", option.ID, request)
		}
		if wantMethod == "turn/start" {
			t.Logf("%s -> thread/start model=%s; turn/start model=%s effort=%s", option.ID, option.Model, option.Model, option.Effort)
		}
		requestIndex++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if requestIndex != len(options)*2 {
		t.Fatalf("wire requests=%d, want %d", requestIndex, len(options)*2)
	}
}
