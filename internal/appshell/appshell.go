// Package appshell owns the optional pinned-Chromium application window.
// It discovers a local Chromium-family executable, validates its capability,
// and launches it in app mode pointed at AkuSidecar's loopback UI with the
// AkuBridge sensor extension loaded. AkuSidecar remains the process owner.
package appshell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const probeTimeout = 5 * time.Second

// gracefulTerminateTimeout bounds how long terminate() waits for the app
// shell to exit on its own after a close request before the hard kill
// becomes the fallback. A clean Chromium exit writes exit_type=Normal, which
// keeps the next launch from restoring the previous session as if after a
// crash.
const gracefulTerminateTimeout = 8 * time.Second

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(\.\d+)?`)

type Candidate struct {
	Path   string `json:"path"`
	Source string `json:"source"`
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

type DiscoveryError struct {
	Result Result
}

func (e *DiscoveryError) Error() string {
	if e == nil || e.Result.Message == "" {
		return "no usable pinned-Chromium executable was found"
	}
	return e.Result.Message
}

type LaunchOptions struct {
	Executable    string
	ExtensionPath string
	IconPath      string
	Identity      ApplicationIdentity
	UserDataDir   string
	URL           string
	ExtraArgs     []string
}

type ApplicationIdentity struct {
	ID              string
	RelaunchCommand string
	DisplayName     string
}

func (identity ApplicationIdentity) validate() error {
	values := []string{identity.ID, identity.RelaunchCommand, identity.DisplayName}
	present := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			present++
		}
	}
	if present != 0 && present != len(values) {
		return errors.New("app-shell application identity requires ID, relaunch command, and display name together")
	}
	if len(strings.TrimSpace(identity.ID)) > 128 {
		return errors.New("app-shell application identity exceeds 128 characters")
	}
	return nil
}

type Window struct {
	command *exec.Cmd
	owner   processOwnership
	icon    windowIcon
	done    chan error
}

func (w *Window) PID() int {
	if w == nil || w.command == nil || w.command.Process == nil {
		return 0
	}
	return w.command.Process.Pid
}

func (w *Window) Done() <-chan error {
	if w == nil {
		return nil
	}
	return w.done
}

func (w *Window) Terminate() {
	if w == nil {
		return
	}
	var root *os.Process
	if w.command != nil {
		root = w.command.Process
	}
	w.owner.terminate(root)
}

func (w *Window) release() {
	w.icon.close()
	w.owner.close()
}

func Discover(ctx context.Context, explicit string) (Result, error) {
	candidates, strict := discoveryCandidates(explicit)
	return discover(ctx, candidates, strict, validateCandidate)
}

func discoveryCandidates(explicit string) ([]Candidate, bool) {
	if value := strings.TrimSpace(explicit); value != "" {
		return []Candidate{{Path: value, Source: "explicit"}}, true
	}
	if value := strings.TrimSpace(os.Getenv("AKU_CHROMIUM_PATH")); value != "" {
		return []Candidate{{Path: value, Source: "environment"}}, true
	}
	candidates := make([]Candidate, 0, 4)
	for _, name := range []string{"chrome", "chromium"} {
		if found, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, Candidate{Path: found, Source: "path"})
		}
	}
	candidates = append(candidates, platformCandidates()...)
	return candidates, false
}

func discover(ctx context.Context, candidates []Candidate, strict bool, validate func(context.Context, Candidate) (string, error)) (Result, error) {
	result := Result{Status: "not_found", Message: "no usable pinned-Chromium executable was found"}
	for _, candidate := range candidates {
		resolved, err := resolveCandidate(candidate)
		if err != nil {
			result.Attempts = append(result.Attempts, Attempt{Path: candidate.Path, Source: candidate.Source, Reason: boundedReason(err)})
			if strict {
				return result, &DiscoveryError{Result: result}
			}
			continue
		}
		version, err := validate(ctx, resolved)
		if err != nil {
			result.Attempts = append(result.Attempts, Attempt{Path: resolved.Path, Source: resolved.Source, Reason: boundedReason(err)})
			if strict {
				return result, &DiscoveryError{Result: result}
			}
			continue
		}
		result.Status = "ok"
		result.Executable = resolved.Path
		result.Source = resolved.Source
		result.Version = version
		result.Message = "pinned-Chromium executable is available."
		return result, nil
	}
	if len(result.Attempts) == 0 {
		result.Attempts = []Attempt{{Source: "discovery", Reason: "no candidate executable was exposed by PATH, environment, or known platform locations"}}
	}
	return result, &DiscoveryError{Result: result}
}

func resolveCandidate(candidate Candidate) (Candidate, error) {
	value := strings.TrimSpace(candidate.Path)
	if value == "" {
		return Candidate{}, errors.New("candidate path is empty")
	}
	info, err := os.Stat(value)
	if err != nil {
		return Candidate{}, fmt.Errorf("stat candidate: %w", err)
	}
	if info.IsDir() {
		named := filepath.Join(value, executableName())
		if _, err := os.Stat(named); err != nil {
			return Candidate{}, fmt.Errorf("directory candidate does not contain %s: %w", executableName(), err)
		}
		value = named
	} else if runtime.GOOS != "windows" {
		if info.Mode().Perm()&0o111 == 0 {
			return Candidate{}, errors.New("candidate is not executable")
		}
	}
	return Candidate{Path: value, Source: candidate.Source}, nil
}

func validateCandidate(ctx context.Context, candidate Candidate) (string, error) {
	if runtime.GOOS == "windows" {
		return platformVersion(candidate.Path)
	}
	probe, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	output, err := exec.CommandContext(probe, candidate.Path, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("--version probe failed: %w", err)
	}
	version := versionPattern.FindString(strings.TrimSpace(string(output)))
	if version == "" {
		return "", fmt.Errorf("--version output %q carries no recognizable browser version", boundedText(string(output)))
	}
	return version, nil
}

func Launch(ctx context.Context, options LaunchOptions) (*Window, error) {
	if strings.TrimSpace(options.Executable) == "" {
		return nil, errors.New("app shell executable is required")
	}
	if strings.TrimSpace(options.URL) == "" {
		return nil, errors.New("app shell URL is required")
	}
	if err := options.Identity.validate(); err != nil {
		return nil, err
	}
	args := buildArgs(options)
	command := exec.Command(options.Executable, args...)
	command.Stdin = nil
	prepareCommand(command)
	owner, err := newProcessOwnership()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		owner.close()
		return nil, fmt.Errorf("start app shell executable: %w", err)
	}
	if err := owner.attach(command); err != nil {
		_ = command.Process.Kill()
		owner.close()
		return nil, err
	}
	icon, err := applyWindowIcon(command.Process.Pid, options.IconPath, options.UserDataDir, options.Identity)
	if err != nil {
		owner.terminate(command.Process)
		_ = command.Wait()
		owner.close()
		return nil, err
	}
	window := &Window{command: command, owner: owner, icon: icon, done: make(chan error, 1)}
	go func() {
		err := command.Wait()
		window.release()
		window.done <- err
	}()
	return window, nil
}

func buildArgs(options LaunchOptions) []string {
	args := []string{
		"--app=" + strings.TrimSpace(options.URL),
		"--user-data-dir=" + options.UserDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
		"--disable-component-update",
		"--disable-session-crashed-bubble",
	}
	if value := strings.TrimSpace(options.ExtensionPath); value != "" {
		args = append(args, "--load-extension="+value)
	}
	args = append(args, options.ExtraArgs...)
	return args
}

func boundedReason(err error) string {
	return boundedText(err.Error())
}

func boundedText(value string) string {
	const limit = 240
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
