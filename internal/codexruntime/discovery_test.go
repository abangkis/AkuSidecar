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

func TestManagedCandidatesSelectHighestSemanticVersion(t *testing.T) {
	older := filepath.Join(t.TempDir(), "older")
	newer := filepath.Join(t.TempDir(), "newer")
	for _, path := range []string{older, newer} {
		if err := osWriteTestFile(path); err != nil {
			t.Fatal(err)
		}
	}
	result, err := discover(context.Background(), []Candidate{
		{Path: newer, Source: "managed-newer-date", SelectionGroup: "codex-app"},
		{Path: older, Source: "managed-root", SelectionGroup: "codex-app"},
	}, false, func(_ context.Context, candidate Candidate) (string, error) {
		if candidate.Source == "managed-root" {
			return "codex-cli 0.145.0-alpha.18", nil
		}
		return "codex-cli 0.130.0-alpha.5", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "managed-root" || result.Version != "codex-cli 0.145.0-alpha.18" {
		t.Fatalf("result=%+v", result)
	}
}

func TestCompareCodexVersionsUsesSemverPrereleaseRules(t *testing.T) {
	for _, test := range []struct {
		left  string
		right string
		want  int
	}{
		{"codex-cli 0.145.0-alpha.18", "codex-cli 0.130.0-alpha.5", 1},
		{"codex-cli 0.145.0", "codex-cli 0.145.0-alpha.18", 1},
		{"codex-cli 0.145.0-alpha.5", "codex-cli 0.145.0-alpha.18", -1},
	} {
		if got := compareCodexVersions(test.left, test.right); got != test.want {
			t.Fatalf("compare(%q, %q)=%d want=%d", test.left, test.right, got, test.want)
		}
	}
}

func osWriteTestFile(path string) error {
	return os.WriteFile(path, []byte("test"), 0o700)
}
