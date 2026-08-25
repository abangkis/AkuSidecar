package credentials

import (
	"fmt"
	"os"
	"strings"
)

// Resolver materializes a credential only at the provider composition
// boundary. Implementations must not persist or log the returned value.
type Resolver interface {
	Resolve(reference string) (string, error)
}

// Environment resolves the deliberately small development-only reference
// vocabulary. It is an adapter boundary, not a durable secret store.
type Environment struct{}

func (Environment) Resolve(reference string) (string, error) {
	name, ok := strings.CutPrefix(strings.TrimSpace(reference), "env:")
	if !ok || (name != "GROQ_API_KEY" && name != "GEMINI_API_KEY") {
		return "", fmt.Errorf("unsupported credential reference")
	}
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("credential %q is unavailable in the process environment", reference)
	}
	return value, nil
}
