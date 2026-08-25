package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestStore(t *testing.T, data string) LocalStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.local.json")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return AtPath(path)
}

func TestLocalStoreResolvesCentralizedReference(t *testing.T) {
	store := writeTestStore(t, `{"schemaVersion":1,"credentialStore":{"type":"inline","values":{"groq.primary":" test-secret-value "}}}`)
	got, err := store.Resolve("groq.primary")
	if err != nil || got != "test-secret-value" {
		t.Fatalf("Resolve()=%q, %v", got, err)
	}
}

func TestLocalStoreRejectsMalformedDocumentWithoutLeakingSecret(t *testing.T) {
	store := writeTestStore(t, `{"schemaVersion":1,"credentialStore":{"type":"wrong","values":{"gemini.primary":"must-not-appear"}}}`)
	_, err := store.Resolve("gemini.primary")
	if err == nil || strings.Contains(err.Error(), "must-not-appear") {
		t.Fatalf("unsafe error=%v", err)
	}
}

func TestLocalStoreReportsMissingByReferenceOnly(t *testing.T) {
	store := writeTestStore(t, `{"schemaVersion":1,"credentialStore":{"type":"inline","values":{"gemini.primary":""}}}`)
	_, err := store.Resolve("gemini.primary")
	if err == nil || !strings.Contains(err.Error(), "gemini.primary") {
		t.Fatalf("missing error=%v", err)
	}
}

func TestValidateReferenceRequiresProviderNamespace(t *testing.T) {
	if err := ValidateReference("gemini", "gemini.primary"); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"", "env:GEMINI_API_KEY", "groq.primary"} {
		if err := ValidateReference("gemini", ref); err == nil {
			t.Fatalf("reference %q unexpectedly validated", ref)
		}
	}
}
