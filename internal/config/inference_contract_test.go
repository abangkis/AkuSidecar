package config

import "testing"

func TestProviderSummaryReportsOutputTokenLimitCapability(t *testing.T) {
	r := ReasoningConfig{Providers: map[string]ProviderConfig{
		"codex-app-server": {}, "gemini-flash-lite": {}, "groq": {}, "ollama-qwen": {}, "deterministic": {},
	}}
	for _, summary := range r.ProviderSummary() {
		want := summary.Name != "codex-app-server" && summary.Name != "deterministic"
		if summary.OutputTokenLimitEnforced != want {
			t.Errorf("%s token limit enforced=%t, want %t", summary.Name, summary.OutputTokenLimitEnforced, want)
		}
	}
}
