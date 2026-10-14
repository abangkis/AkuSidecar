//go:build windows && auth_session_fixture && auth_session_dev

package engine

import "testing"

// Uses actual cmd/akusidecar and its explicit CLI dispatch, with fresh grants,
// real Chrome sessions, physical capture, durable SQLite and process restart.
func TestDevelopmentSessionMainFullOSRestart(t *testing.T) {
	runProcessFixtureFullOSRestart(t, "aku-sidecar-dev.exe")
}
