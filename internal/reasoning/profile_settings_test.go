package reasoning

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestActivateProviderProfileSetKeepsSelectionsIndependent(t *testing.T) {
	codex := &CodexAppServer{}
	settings := domain.DefaultSettings("standard", "quiet", "rank_only", false)
	settings.ReasoningProvider = "codex-app-server"
	settings.ApplyReasoningProfileSet(domain.ReasoningProfileSet{
		Acquisition: "luna_max",
		Evaluation:  "luna_max",
		Semantic:    "luna_max",
		AIDeep:      "luna_max",
	})
	if !settings.RememberReasoningProfileSet("codex-app-server") {
		t.Fatal("initial Codex profile snapshot should be saved")
	}
	codexSet := settings.ActiveReasoningProfileSet()
	if codexSet.Acquisition != "luna_max" {
		t.Fatalf("Codex active profiles=%+v", codexSet)
	}

	wantOllama := domain.ReasoningProfileSet{
		Acquisition: "deep_reasoning",
		Evaluation:  "deep_reasoning",
		Semantic:    "deep_reasoning",
		AIDeep:      "deep_reasoning",
	}
	settings.ApplyReasoningProfileSet(wantOllama)
	if !settings.RememberReasoningProfileSet("ollama") {
		t.Fatal("initial Ollama profile snapshot should be saved")
	}
	ollamaSet := settings.ActiveReasoningProfileSet()
	if ollamaSet != wantOllama {
		t.Fatalf("Ollama active profiles=%+v, want=%+v", ollamaSet, wantOllama)
	}
	if got := settings.ReasoningProfilesByProvider["codex-app-server"]; got != codexSet {
		t.Fatalf("Codex profiles were overwritten: got=%+v want=%+v", got, codexSet)
	}

	if restored, err := ActivateProviderProfileSet(&settings, "codex-app-server", codex); err != nil || !restored {
		t.Fatalf("restore Codex profiles changed=%v err=%v", restored, err)
	}
	if got := settings.ActiveReasoningProfileSet(); got != codexSet {
		t.Fatalf("restored Codex profiles=%+v, want=%+v", got, codexSet)
	}
}

func TestActivateProviderProfileSetLeavesStaleIDsForLegacyFallback(t *testing.T) {
	codex := &CodexAppServer{}
	settings := domain.DefaultSettings("standard", "quiet", "rank_only", false)
	settings.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
		"codex-app-server": {
			Acquisition: "unknown",
			Evaluation:  "unknown",
			Semantic:    "unknown",
			AIDeep:      "unknown",
		},
	}

	changed, err := ActivateProviderProfileSet(&settings, "codex-app-server", codex)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("persisted stale profiles should be restored for legacy validation")
	}
	want := settings.ReasoningProfilesByProvider["codex-app-server"]
	if got := settings.ActiveReasoningProfileSet(); got != want {
		t.Fatalf("active profiles=%+v, want persisted stale set=%+v", got, want)
	}
	if got := EnsureResolvableProfile(codex, "unknown"); got != "luna_high" {
		t.Fatalf("safe unknown-profile fallback=%q, want luna_high", got)
	}
}
