package codexruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const probeTimeout = 5 * time.Second

type Candidate struct {
	Path           string `json:"path"`
	Source         string `json:"source"`
	SelectionGroup string `json:"-"`
}

type Attempt struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type Result struct {
	Status     string    `json:"status"`
	Executable string    `json:"executable,omitempty"`
	Source     string    `json:"source,omitempty"`
	Version    string    `json:"version,omitempty"`
	Attempts   []Attempt `json:"attempts,omitempty"`
	Message    string    `json:"message"`
}

type validator func(context.Context, Candidate) (string, error)

func Discover(ctx context.Context, explicit string) (Result, error) {
	candidates, strict := discoveryCandidates(explicit)
	return discover(ctx, candidates, strict, validateCandidate)
}

func discoveryCandidates(explicit string) ([]Candidate, bool) {
	if value := strings.TrimSpace(explicit); value != "" {
		return []Candidate{{Path: value, Source: "explicit"}}, true
	}
	if value := strings.TrimSpace(os.Getenv("AKU_CODEX_PATH")); value != "" {
		return []Candidate{{Path: value, Source: "environment"}}, true
	}

	candidates := make([]Candidate, 0, 12)
	for _, name := range executableNames() {
		if found, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, Candidate{Path: found, Source: "path"})
		}
	}
	candidates = append(candidates, platformCandidates()...)
	return candidates, false
}

func discover(ctx context.Context, candidates []Candidate, strict bool, validate validator) (Result, error) {
	unique := deduplicate(candidates)
	result := Result{Status: "not_found", Message: notFoundMessage()}
	for index := 0; index < len(unique); {
		candidate := unique[index]
		if candidate.SelectionGroup != "" {
			end := index + 1
			for end < len(unique) && unique[end].SelectionGroup == candidate.SelectionGroup {
				end++
			}
			if selected, version, ok := selectHighestVersion(ctx, unique[index:end], validate, &result); ok {
				return successfulResult(result, selected, version), nil
			}
			index = end
			continue
		}
		resolved, err := resolveCandidate(candidate)
		if err != nil {
			result.Attempts = append(result.Attempts, Attempt{Path: candidate.Path, Source: candidate.Source, Reason: boundedReason(err)})
			if strict {
				return result, errors.New(result.Message)
			}
			index++
			continue
		}
		version, err := validate(ctx, resolved)
		if err != nil {
			result.Attempts = append(result.Attempts, Attempt{Path: resolved.Path, Source: resolved.Source, Reason: boundedReason(err)})
			if strict {
				return result, errors.New(result.Message)
			}
			index++
			continue
		}
		return successfulResult(result, resolved, version), nil
	}
	if len(result.Attempts) == 0 {
		result.Attempts = []Attempt{{Source: "discovery", Reason: "no candidate executable was exposed by PATH or known platform locations"}}
	}
	return result, errors.New(result.Message)
}

func selectHighestVersion(ctx context.Context, candidates []Candidate, validate validator, result *Result) (Candidate, string, bool) {
	var selected Candidate
	selectedVersion := ""
	selectedTime := time.Time{}
	found := false
	for _, candidate := range candidates {
		resolved, err := resolveCandidate(candidate)
		if err != nil {
			result.Attempts = append(result.Attempts, Attempt{Path: candidate.Path, Source: candidate.Source, Reason: boundedReason(err)})
			continue
		}
		version, err := validate(ctx, resolved)
		if err != nil {
			result.Attempts = append(result.Attempts, Attempt{Path: resolved.Path, Source: resolved.Source, Reason: boundedReason(err)})
			continue
		}
		modified := fileModified(resolved.Path)
		comparison := compareCodexVersions(version, selectedVersion)
		if !found || comparison > 0 || (comparison == 0 && modified.After(selectedTime)) {
			selected = resolved
			selectedVersion = version
			selectedTime = modified
			found = true
		}
	}
	return selected, selectedVersion, found
}

func successfulResult(result Result, candidate Candidate, version string) Result {
	result.Status = "ok"
	result.Executable = candidate.Path
	result.Source = candidate.Source
	result.Version = version
	result.Message = "Codex App Server runtime is available."
	return result
}

func fileModified(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func resolveCandidate(candidate Candidate) (Candidate, error) {
	value := strings.TrimSpace(candidate.Path)
	if value == "" {
		return candidate, errors.New("candidate path is empty")
	}
	if !strings.ContainsAny(value, `\/`) {
		found, err := exec.LookPath(value)
		if err != nil {
			return candidate, fmt.Errorf("not available on PATH: %w", err)
		}
		value = found
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return candidate, fmt.Errorf("resolve absolute path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return candidate, fmt.Errorf("file is unavailable: %w", err)
	}
	if info.IsDir() {
		return candidate, errors.New("path points to a directory")
	}
	candidate.Path = absolute
	return candidate, nil
}

func validateCandidate(ctx context.Context, candidate Candidate) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, candidate.Path, "app-server", "--help").CombinedOutput()
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		return "", errors.New("App Server capability probe timed out")
	}
	if err != nil {
		return "", fmt.Errorf("App Server capability probe failed: %w", err)
	}
	help := strings.ToLower(string(output))
	if !strings.Contains(help, "codex app-server") || !strings.Contains(help, "--listen") {
		return "", errors.New("executable does not expose the expected Codex App Server contract")
	}

	versionCtx, versionCancel := context.WithTimeout(ctx, probeTimeout)
	defer versionCancel()
	versionOutput, versionErr := exec.CommandContext(versionCtx, candidate.Path, "--version").CombinedOutput()
	if versionErr != nil {
		return "unknown", nil
	}
	version := extractVersion(string(versionOutput))
	if len(version) > 160 {
		version = version[:160]
	}
	if version == "" {
		version = "unknown"
	}
	return version, nil
}

func extractVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "codex-cli ") || strings.HasPrefix(lower, "codex ") {
			return line
		}
	}
	return ""
}

func deduplicate(candidates []Candidate) []Candidate {
	seen := map[string]bool{}
	result := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := filepath.Clean(strings.TrimSpace(candidate.Path))
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if key == "." || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, candidate)
	}
	return result
}

func executableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"codex.exe", "codex"}
	}
	return []string{"codex"}
}

func newestFirst(paths []string) []string {
	type pathTime struct {
		path string
		time time.Time
	}
	values := make([]pathTime, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			values = append(values, pathTime{path: path, time: info.ModTime()})
		}
	}
	sort.SliceStable(values, func(i, j int) bool { return values[i].time.After(values[j].time) })
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.path)
	}
	return result
}

func boundedReason(err error) string {
	value := strings.Join(strings.Fields(err.Error()), " ")
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func notFoundMessage() string {
	return "Codex App Server was not found. Install and sign in to Codex App or install a Codex CLI version that provides `codex app-server`; then retry or set AKU_CODEX_PATH."
}
