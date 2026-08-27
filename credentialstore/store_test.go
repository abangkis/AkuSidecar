package credentialstore

import "testing"

func TestParseReferenceAcceptsNamespacedOpaqueIdentifiers(t *testing.T) {
	reference, err := ParseReference(" gemini.primary ")
	if err != nil || reference.String() != "gemini.primary" {
		t.Fatalf("ParseReference()=%q, %v", reference, err)
	}
}

func TestParseReferenceRejectsTransportAndEnvironmentSyntax(t *testing.T) {
	for _, value := range []string{"", "GEMINI_API_KEY", "env:GEMINI_API_KEY", "gemini", "gemini/primary"} {
		if _, err := ParseReference(value); err == nil {
			t.Fatalf("reference %q unexpectedly accepted", value)
		}
	}
}
