package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/config"
)

const startupReadyMarker = ".aku-startup-ready-build"

// The recovery window is needed for a fresh installed profile or a changed
// source tuple. The marker is UI state only; it grants no runtime authority.
// A missing or unreadable marker fails open to the manual recovery window.
func startupStatusPolicy(deployment config.DeploymentConfig, profilePath string) (show bool, markReady func() error, err error) {
	if deployment.Mode != "production-installed-app" {
		return false, nil, nil
	}
	freeze := strings.TrimSpace(deployment.SourceFreeze)
	if freeze == "" {
		return true, nil, fmt.Errorf("installed-app source freeze is missing; startup status will remain available")
	}
	identity := deployment.ReleaseVersion + "\n" + freeze
	sum := sha256.Sum256([]byte(identity))
	fingerprint := hex.EncodeToString(sum[:])
	markerPath := filepath.Join(profilePath, startupReadyMarker)
	previous, readErr := os.ReadFile(markerPath)
	if readErr == nil && strings.TrimSpace(string(previous)) == fingerprint {
		return false, nil, nil
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		err = fmt.Errorf("read startup status marker: %w", readErr)
	}
	return true, func() error {
		if mkdirErr := os.MkdirAll(profilePath, 0o700); mkdirErr != nil {
			return fmt.Errorf("prepare startup status marker directory: %w", mkdirErr)
		}
		if writeErr := os.WriteFile(markerPath, []byte(fingerprint+"\n"), 0o600); writeErr != nil {
			return fmt.Errorf("write startup status marker: %w", writeErr)
		}
		return nil
	}, err
}
