//go:build !windows

package credentialstore

import "fmt"

// OpenOS fails closed until the platform-specific secure backend is supplied.
// The portable Store contract remains unchanged when macOS/Linux backends land.
func OpenOS(namespace string) (Store, error) {
	return nil, fmt.Errorf("%w: no secure backend for this platform", ErrUnavailable)
}
