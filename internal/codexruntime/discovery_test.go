package codexruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverValidatorSelectsFirstCapableCandidate(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, path := range []string{first, second} {
		if err := osWriteTestFile(path); err != nil {
			t.Fatal(err)
		}
	}
	result, err := discover(context.Background(), []Candidate{{Path: first, Source: "path"}, {Path: second, Source: "managed"}}, false, func(_ context.Context, candidate Candidate) (string, error) {
		if candidate.Source == "path" {
			return "", errors.New("missing app-server")
		}
		return "codex-cli 1.2.3", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "managed" || result.Version != "codex-cli 1.2.3" || len(result.Attempts) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestStrictCandidateDoesNotSilentlyFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex")
	if err := osWriteTestFile(path); err != nil {
		t.Fatal(err)
	}
	result, err := discover(context.Background(), []Candidate{{Path: path, Source: "explicit"}, {Path: path + "-other", Source: "path"}}, true, func(context.Context, Candidate) (string, error) {
		return "", errors.New("wrong binary")
	})
	if err == nil || len(result.Attempts) != 1 || result.Attempts[0].Source != "explicit" {
		t.Fatalf("strict discovery did not fail closed: result=%+v err=%v", result, err)
	}
}

func TestExtractVersionIgnoresRuntimeWarnings(t *testing.T) {
	output := "WARNING: failed to clean up stale temp dirs\r\ncodex-cli 0.145.0-alpha.18\r\n"
	if got := extractVersion(output); got != "codex-cli 0.145.0-alpha.18" {
		t.Fatalf("version=%q", got)
	}
}

func osWriteTestFile(path string) error {
	return os.WriteFile(path, []byte("test"), 0o700)
}
