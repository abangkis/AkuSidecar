package credentials

import (
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/credentialstore"
)

const osStoreNamespace = "AkuBrowser"

// Manager applies AkuSidecar's credential resolution policy to the portable
// store contract. The OS store is authoritative; the ignored JSON store is a
// development-only fallback for existing developer environments.
type Manager struct {
	primary      credentialstore.Store
	fallback     Resolver
	primaryError error
}

func ForRuntime(root string, development bool) Manager {
	primary, err := credentialstore.OpenOS(osStoreNamespace)
	manager := Manager{primary: primary, primaryError: err}
	if development {
		local := ForRoot(root)
		manager.fallback = local
	}
	return manager
}

// NewManager exposes dependency injection without binding callers to a
// platform implementation. It is primarily useful to adapters and tests.
func NewManager(primary credentialstore.Store, fallback Resolver) Manager {
	return Manager{primary: primary, fallback: fallback}
}

func (manager Manager) Resolve(reference string) (string, error) {
	parsed, err := credentialstore.ParseReference(reference)
	if err != nil {
		return "", err
	}
	if manager.primary != nil {
		value, getErr := manager.primary.Get(parsed)
		if getErr == nil {
			value = strings.TrimSpace(value)
			if value == "" {
				return "", fmt.Errorf("credential ref %q is empty", parsed)
			}
			return value, nil
		}
		if !errors.Is(getErr, credentialstore.ErrNotFound) {
			return "", fmt.Errorf("resolve credential ref %q from secure store: %w", parsed, getErr)
		}
	}
	if manager.fallback != nil {
		return manager.fallback.Resolve(parsed.String())
	}
	if manager.primaryError != nil {
		return "", fmt.Errorf("credential ref %q is unavailable: %w", parsed, manager.primaryError)
	}
	return "", fmt.Errorf("credential ref %q is missing", parsed)
}

func (manager Manager) Configured(reference string) bool {
	value, err := manager.Resolve(reference)
	return err == nil && value != ""
}

func (manager Manager) Put(reference, value string) error {
	parsed, err := credentialstore.ParseReference(reference)
	if err != nil {
		return err
	}
	if manager.primary == nil {
		if manager.primaryError != nil {
			return fmt.Errorf("store credential ref %q: %w", parsed, manager.primaryError)
		}
		return fmt.Errorf("store credential ref %q: %w", parsed, credentialstore.ErrUnavailable)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("credential value is required")
	}
	if err := manager.primary.Put(parsed, value); err != nil {
		return fmt.Errorf("store credential ref %q: %w", parsed, err)
	}
	return nil
}

func (manager Manager) Delete(reference string) error {
	parsed, err := credentialstore.ParseReference(reference)
	if err != nil {
		return err
	}
	if manager.primary == nil {
		return fmt.Errorf("delete credential ref %q: %w", parsed, credentialstore.ErrUnavailable)
	}
	if err := manager.primary.Delete(parsed); err != nil {
		return fmt.Errorf("delete credential ref %q: %w", parsed, err)
	}
	return nil
}
