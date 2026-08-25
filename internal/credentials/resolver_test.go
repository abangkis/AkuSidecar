package credentials

import (
	"strings"
	"testing"
)

func TestEnvironmentResolvesAllowlistedReference(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "test-secret-value")
	got, err := (Environment{}).Resolve("env:GROQ_API_KEY")
	if err != nil || got != "test-secret-value" {
		t.Fatalf("Resolve()=%q, %v", got, err)
	}
}

func TestEnvironmentResolvesGeminiReference(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-test-secret")
	got, err := (Environment{}).Resolve("env:GEMINI_API_KEY")
	if err != nil || got != "gemini-test-secret" {
		t.Fatalf("Resolve()=%q, %v", got, err)
	}
}

func TestEnvironmentRejectsUnknownAndDoesNotLeakValue(t *testing.T) {
	t.Setenv("OTHER_API_KEY", "must-not-appear")
	_, err := (Environment{}).Resolve("env:OTHER_API_KEY")
	if err == nil || strings.Contains(err.Error(), "must-not-appear") {
		t.Fatalf("unsafe error=%v", err)
	}
}

func TestEnvironmentReportsMissingByReferenceOnly(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "")
	_, err := (Environment{}).Resolve("env:GROQ_API_KEY")
	if err == nil || !strings.Contains(err.Error(), "env:GROQ_API_KEY") {
		t.Fatalf("missing error=%v", err)
	}
}
