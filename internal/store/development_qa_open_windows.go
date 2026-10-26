//go:build windows && auth_session_dev

package store

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// OpenDevelopmentQADatabase retains ordinary schema/foreign-key/version checks.
// The isolated caller holds the exact current-version marker against rename for
// the database lifetime; avoid replacing that already published marker. Ordinary
// Open/OpenWithClock still publish their version through the existing path.
func OpenDevelopmentQADatabase(path string, defaults domain.Settings, clock Clock, marker *os.File) (*Store, error) {
	if marker == nil || marker.Name() != filepath.Join(filepath.Dir(path), RuntimeVersionMarkerName) {
		return nil, errors.New("pinned QA runtime marker required")
	}
	held, err := marker.Stat()
	if err != nil {
		return nil, errors.New("QA marker unavailable")
	}
	current, err := os.Stat(marker.Name())
	if err != nil || !os.SameFile(held, current) {
		return nil, errors.New("QA marker changed")
	}
	var raw [129]byte
	n, _ := marker.ReadAt(raw[:], 0)
	if string(raw[:n]) != domain.ApplicationVersion+"\n" {
		return nil, errors.New("QA marker must match the current runtime")
	}
	return openWithClock(path, defaults, clock, false)
}
