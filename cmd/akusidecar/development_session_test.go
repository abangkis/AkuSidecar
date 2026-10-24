package main

import (
	"github.com/abangkis/AkuSidecar/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentSessionDefaultDispatchDoesNothing(t *testing.T) {
	handled, err := dispatchDevelopmentSession(config.Options{}, strings.NewReader(""), nil)
	if handled || err != nil {
		t.Fatal("ordinary startup changed", err)
	}
}

// Three compiled actual-main variants must reject before a config read, SQLite
// mutation, provider startup or browser launch. The accepted synthetic variant
// also rejects missing private grant before it can create scoped fixture state.
func TestDevelopmentSessionCLIRejectionBeforeOrdinaryStartup(t *testing.T) {
	root := os.Getenv("AKU_DEV_SESSION_TEST_ROOT")
	if root == "" {
		t.Skip("requires explicit VerifyDevelopment helper")
	}
	for _, test := range []struct {
		image  string
		args   []string
		reason string
	}{
		{"aku-sidecar-default-check.exe", []string{"--dev", "--dev-session-reuse=synthetic-fixture"}, "requires an auth_session_dev build"},
		{"aku-sidecar-dev-unavailable.exe", []string{"--dev", "--dev-session-reuse=synthetic-fixture"}, "provider unavailable"},
		{"aku-sidecar-dev.exe", []string{"--dev-session-reuse=synthetic-fixture"}, "requires --dev"},
		{"aku-sidecar-dev.exe", []string{"--dev", "--dev-session-reuse=real-source"}, "supported synthetic-fixture provider"},
		{"aku-sidecar-dev.exe", []string{"--dev", "--dev-session-reuse=synthetic-fixture"}, "startup grant missing"},
		{"aku-sidecar-default-check.exe", []string{"--dev", "--dev-session-reuse=synthetic-policy-fixture"}, "requires an auth_session_dev build"},
		{"aku-sidecar-dev-unavailable.exe", []string{"--dev", "--dev-session-reuse=synthetic-policy-fixture"}, "provider unavailable"},
		{"aku-sidecar-dev.exe", []string{"--dev-session-reuse=synthetic-policy-fixture"}, "requires --dev"},
		{"aku-sidecar-dev.exe", []string{"--dev", "--dev-session-reuse=synthetic-policy-fixture"}, "startup grant missing"},
		{"aku-sidecar-default-check.exe", []string{"--dev", "--dev-session-reuse=synthetic-x-manual-fixture"}, "requires an auth_session_dev build"},
		{"aku-sidecar-dev-unavailable.exe", []string{"--dev", "--dev-session-reuse=synthetic-x-manual-fixture"}, "provider unavailable"},
		{"aku-sidecar-dev.exe", []string{"--dev-session-reuse=synthetic-x-manual-fixture"}, "requires --dev"},
	} {
		t.Run(test.image+test.reason, func(t *testing.T) {
			database := filepath.Join(root, "must-not-exist", test.image+".sqlite")
			configPath := filepath.Join(root, "missing-config.json")
			args := append(append([]string{}, test.args...), "--config="+configPath, "--database="+database)
			cmd := exec.Command(filepath.Join(root, test.image), args...)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.reason) || strings.Contains(string(output), "missing-config") {
				t.Fatalf("wrong early rejection: %s %v", output, err)
			}
			if _, err := os.Stat(filepath.Dir(database)); !os.IsNotExist(err) {
				t.Fatal("rejected startup touched ordinary state", err)
			}
			t.Logf("image=%s earlyRejection=%s ordinaryStateAbsent=true", test.image, test.reason)
		})
	}
}
