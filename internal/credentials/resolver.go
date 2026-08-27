package credentials

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/abangkis/AkuSidecar/credentialstore"
)

const LocalStoreRelativePath = "runtime/config/credentials.local.json"

// Resolver materializes a credential only at the provider composition
// boundary. Implementations must not persist or log the returned value.
type Resolver interface {
	Resolve(reference string) (string, error)
}

type storeDocument struct {
	SchemaVersion   int             `json:"schemaVersion"`
	CredentialStore credentialStore `json:"credentialStore"`
}

type credentialStore struct {
	Type   string            `json:"type"`
	Values map[string]string `json:"values"`
}

// LocalStore is AkuSidecar's development-only, project-local fallback. The
// ignored file is read on demand without retaining or exposing the whole store.
type LocalStore struct {
	path string
}

func ForRoot(root string) LocalStore {
	return LocalStore{path: filepath.Join(root, filepath.FromSlash(LocalStoreRelativePath))}
}

func AtPath(path string) LocalStore {
	return LocalStore{path: path}
}

func (store LocalStore) Resolve(reference string) (string, error) {
	parsed, err := credentialstore.ParseReference(reference)
	if err != nil {
		return "", err
	}
	reference = parsed.String()
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("credential ref %q is unavailable: local credential store is missing", reference)
		}
		return "", fmt.Errorf("read local credential store for ref %q: %w", reference, err)
	}
	var document storeDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("decode local credential store for ref %q: %w", reference, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return "", fmt.Errorf("decode local credential store for ref %q: %w", reference, err)
	}
	if document.SchemaVersion != 1 {
		return "", fmt.Errorf("credential ref %q is unavailable: unsupported local credential store schemaVersion %d", reference, document.SchemaVersion)
	}
	if strings.ToLower(strings.TrimSpace(document.CredentialStore.Type)) != "inline" {
		return "", fmt.Errorf("credential ref %q is unavailable: local credential store type must be %q", reference, "inline")
	}
	if document.CredentialStore.Values == nil {
		return "", fmt.Errorf("credential ref %q is unavailable: local credential store values are missing", reference)
	}
	for configuredRef := range document.CredentialStore.Values {
		if _, err := credentialstore.ParseReference(configuredRef); err != nil {
			return "", fmt.Errorf("local credential store contains malformed ref %q", configuredRef)
		}
	}
	value, ok := document.CredentialStore.Values[reference]
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("credential ref %q is missing", reference)
	}
	return strings.TrimSpace(value), nil
}

// ValidateReference ensures that a provider can only select a credential from
// its own namespace, for example gemini.primary or groq.primary.
func ValidateReference(provider, reference string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return fmt.Errorf("credential ref is required for provider %q", provider)
	}
	parsed, err := credentialstore.ParseReference(reference)
	if err != nil {
		return fmt.Errorf("credential ref %q is malformed for provider %q", reference, provider)
	}
	prefix := strings.SplitN(parsed.String(), ".", 2)[0]
	if prefix != provider {
		return fmt.Errorf("credential ref %q has the wrong provider prefix for %q", reference, provider)
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("local credential store contains multiple JSON values")
}
