// Package credentialstore defines the portable secret-storage contract used by
// AkuBrowser. It intentionally has no dependency on AkuSidecar configuration,
// providers, HTTP, or UI so it can be extracted into a standalone module later.
package credentialstore

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrNotFound    = errors.New("credential not found")
	ErrUnavailable = errors.New("credential store unavailable")
)

var referencePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9._-]*$`)

// Reference is an opaque, namespaced credential identifier such as
// gemini.primary. It is safe to persist and expose; its secret value is not.
type Reference string

func ParseReference(value string) (Reference, error) {
	value = strings.TrimSpace(value)
	if !referencePattern.MatchString(value) {
		return "", fmt.Errorf("credential ref %q is malformed", value)
	}
	return Reference(value), nil
}

func (reference Reference) String() string { return string(reference) }

// Store is the provider-neutral credential lifecycle. Implementations must not
// log values or include them in returned errors.
type Store interface {
	Get(reference Reference) (string, error)
	Put(reference Reference, value string) error
	Delete(reference Reference) error
}
