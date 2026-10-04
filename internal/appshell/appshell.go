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
	"sync"
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

var errCleanupUnverified = errors.New("app shell cleanup could not be verified")

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
	Executable     string
	ExtensionPath  string
	IconPath       string
	Identity       ApplicationIdentity
	UserDataDir    string
	URL            string
	StartupLogPath string
	ExtraArgs      []string
	Startup        *Startup
	// The readiness handshake still runs when the native status is hidden.
	SuppressStartupWindow bool
	OnStartupReady        func()
	// Windows capture-host experiment only; Chromium still owns window showing.
	StartMinimized bool
	// PrivateCDP opts this managed minimized capture window into a private
	// inherited-pipe DevTools connection. It never opens a debugging port.
	PrivateCDP bool
	// Explicit native reader windows need normal Chrome navigation controls.
	// The product UI and legacy capture host retain app mode by default.
	NormalWindow bool
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
	ownershipMu          sync.Mutex
	closed               chan struct{}
	cleanupErr           error
	command              *exec.Cmd
	owner                processOwnership
	capturePipe          captureProtocolLaunch
	captureProtocol      CaptureProtocol
	icon                 windowIcon
	done                 chan error
	executable           string
	userDataDir          string
	startup              *Startup
	captureHost          bool
	containment          CaptureContainment
	captureHandoff       func(context.Context) error
	retiring             bool
	terminationRequested bool
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

// CaptureProtocol returns the private pipe client only for a Window launched
// with LaunchOptions.PrivateCDP. It returns nil for ordinary app windows.
func (w *Window) CaptureProtocol() CaptureProtocol {
	if w == nil {
		return nil
	}
	return w.captureProtocol
}

func (w *Window) Terminate() {
	if w == nil {
		return
	}
	w.startup.Stop()
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	select {
	case <-w.closed:
		return
	default:
	}
	w.terminationRequested = true
	var root *os.Process
	if w.command != nil {
		root = w.command.Process
	}
	w.owner.terminate(root)
}

// OpenExtensionsPage opens Chrome's extension management page in a separate
// browser window that uses the same isolated AkuBrowser profile. This is a
// development recovery action; Chrome Stable still requires the developer to
// choose Load unpacked and select the extension directory themselves.
func (w *Window) OpenExtensionsPage(ctx context.Context) error {
	if w == nil || strings.TrimSpace(w.executable) == "" || strings.TrimSpace(w.userDataDir) == "" {
		return errors.New("app shell browser is unavailable")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	command := exec.Command(w.executable, buildInternalPageArgs(w.userDataDir, "chrome://extensions")...)
	command.Stdin = nil
	prepareCommand(command)
	if err := command.Start(); err != nil {
		return fmt.Errorf("open Chrome Extensions: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

func (w *Window) release() {
	w.ownershipMu.Lock()
	w.startup.Stop()
	if w.containment != nil {
		w.containment.Stop()
	}
	w.icon.close()
	for {
		if w.capturePipe != nil && w.terminationRequested {
			if err := w.owner.drain(); err == nil {
				w.cleanupErr = w.capturePipe.closeAfterOwnerExit()
				w.owner.close()
				close(w.closed)
				w.ownershipMu.Unlock()
				return
			} else {
				w.cleanupErr = err
				w.ownershipMu.Unlock()
				time.Sleep(100 * time.Millisecond)
				w.ownershipMu.Lock()
				continue
			}
		}
		if !w.captureHost || w.terminationRequested {
			w.cleanupErr = w.owner.drain()
			w.owner.close()
			close(w.closed)
			w.ownershipMu.Unlock()
			return
		}
		// Natural root exit is not permission to kill a remaining helper or
		// close a KILL_ON_JOB_CLOSE handle. Retain ownership until zero is
		// positively verified; explicit whole-app shutdown still uses drain.
		complete, err := w.owner.naturallyDrained()
		w.cleanupErr = err
		if err == nil && complete {
			if w.capturePipe != nil {
				w.cleanupErr = w.capturePipe.closeAfterOwnerExit()
			}
			w.owner.close()
			close(w.closed)
			w.ownershipMu.Unlock()
			return
		}
		w.ownershipMu.Unlock()
		time.Sleep(25 * time.Millisecond)
		w.ownershipMu.Lock()
	}
}

// CloseForRetry only permits profile reuse after root wait and owned-tree
// cleanup have both completed. A timeout/error must never trigger relaunch.
func (w *Window) CloseForRetry(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.ownershipMu.Lock()
	select {
	case <-w.closed:
		err := w.cleanupErr
		w.ownershipMu.Unlock()
		return err
	default:
	}
	if w.captureHost {
		closeHost := w.captureHandoff
		if closeHost == nil {
			w.ownershipMu.Unlock()
			return ErrCaptureHandoffUnsafe
		}
		w.retiring = true
		w.ownershipMu.Unlock()
		if err := closeHost(ctx); err != nil {
			return fmt.Errorf("%w: %v", ErrCaptureHandoffPending, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrCaptureHandoffPending, ctx.Err())
		case <-w.closed:
			return w.cleanupErr
		}
	}
	w.ownershipMu.Unlock()
	w.Terminate()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.closed:
		return w.cleanupErr
	}
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.Executable) == "" {
		return nil, errors.New("app shell executable is required")
	}
	if strings.TrimSpace(options.URL) == "" {
		return nil, errors.New("app shell URL is required")
	}
	if err := options.Identity.validate(); err != nil {
		return nil, err
	}
	if options.PrivateCDP {
		if !options.StartMinimized && !options.NormalWindow {
			return nil, errors.New("private CDP requires a managed minimized capture window or normal reader")
		}
		for _, arg := range options.ExtraArgs {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(arg)), "--remote-debugging-") {
				return nil, errors.New("private CDP cannot be combined with caller-supplied DevTools switches")
			}
		}
	}
	var capturePipe captureProtocolLaunch
	if options.PrivateCDP {
		var err error
		capturePipe, err = newCaptureProtocolLaunch()
		if err != nil {
			return nil, err
		}
	}
	args := buildArgs(options)
	if capturePipe != nil {
		args = append(args, capturePipe.args()...)
	}
	command := exec.Command(options.Executable, args...)
	command.Stdin = nil
	prepareCommand(command)
	if capturePipe != nil {
		if err := capturePipe.configure(command); err != nil {
			_ = capturePipe.closeAfterOwnerExit()
			return nil, err
		}
	}
	owner, err := newProcessOwnership()
	if err != nil {
		if capturePipe != nil {
			_ = capturePipe.closeAfterOwnerExit()
		}
		return nil, err
	}
	if err := command.Start(); err != nil {
		owner.close()
		if capturePipe != nil {
			_ = capturePipe.closeAfterOwnerExit()
		}
		return nil, fmt.Errorf("start app shell executable: %w", err)
	}
	if err := owner.attach(command); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		if capturePipe != nil {
			// The process was not admitted to the Job, so retain the private
			// endpoints rather than claiming that its descendants have exited.
			quarantineCaptureProtocol(owner, capturePipe)
			return nil, fmt.Errorf("%w: private capture process could not be assigned to its Job: %v", errCleanupUnverified, err)
		}
		owner.close()
		return nil, fmt.Errorf("%w: %v", errCleanupUnverified, err)
	}
	if capturePipe != nil {
		if err := capturePipe.childStarted(); err != nil {
			owner.terminate(command.Process)
			_ = command.Wait()
			cleanupErr := owner.drain()
			if cleanupErr == nil {
				_ = capturePipe.closeAfterOwnerExit()
				owner.close()
				return nil, err
			}
			// Keep the pipes and Job alive if cleanup cannot be verified.
			quarantineCaptureProtocol(owner, capturePipe)
			return nil, fmt.Errorf("%w: private pipe setup: %v; cleanup: %v", errCleanupUnverified, err, cleanupErr)
		}
	}
	if options.StartMinimized && runtime.GOOS == "windows" {
		if err := owner.minimizeInitialWindow(ctx, command.Process.Pid); err != nil {
			owner.terminate(command.Process)
			_ = command.Wait()
			cleanupErr := owner.drain()
			if cleanupErr == nil {
				if capturePipe != nil {
					_ = capturePipe.closeAfterOwnerExit()
				}
			}
			if cleanupErr == nil || capturePipe == nil {
				owner.close()
			}
			if cleanupErr != nil {
				if capturePipe != nil {
					quarantineCaptureProtocol(owner, capturePipe)
				}
				return nil, fmt.Errorf("%w: minimize: %v; cleanup: %v", errCleanupUnverified, err, cleanupErr)
			}
			return nil, err
		}
	}
	icon, err := applyWindowIcon(command.Process.Pid, options.IconPath, options.UserDataDir, options.Identity)
	if err != nil {
		owner.terminate(command.Process)
		_ = command.Wait()
		cleanupErr := owner.drain()
		if cleanupErr == nil {
			if capturePipe != nil {
				_ = capturePipe.closeAfterOwnerExit()
			}
		}
		if cleanupErr == nil || capturePipe == nil {
			owner.close()
		}
		if cleanupErr != nil {
			if capturePipe != nil {
				quarantineCaptureProtocol(owner, capturePipe)
			}
			return nil, fmt.Errorf("%w: initialization: %v; cleanup: %v", errCleanupUnverified, err, cleanupErr)
		}
		return nil, err
	}
	window := &Window{
		closed:  make(chan struct{}),
		command: command, owner: owner, icon: icon, done: make(chan error, 1),
		capturePipe: capturePipe,
		startup:     options.Startup,
		captureHost: options.StartMinimized || (options.NormalWindow && options.PrivateCDP),
		executable:  options.Executable, userDataDir: options.UserDataDir,
	}
	if capturePipe != nil {
		window.captureProtocol = capturePipe.protocol()
	}
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
		// Chrome for Testing supports suppressing informational infobars;
		// interactive permission prompts remain available. See Chromium's
		// docs/chrome_for_testing/README.md, User Interface & Infobars.
		"--disable-infobars",
	}
	if options.NormalWindow {
		args[0] = "--new-window"
		args = append(args, strings.TrimSpace(options.URL))
	}
	if value := strings.TrimSpace(options.ExtensionPath); value != "" {
		args = append(args, "--load-extension="+value)
	}
	if value := strings.TrimSpace(options.StartupLogPath); value != "" {
		args = append(args, "--enable-logging", "--log-file="+value)
	}
	args = append(args, options.ExtraArgs...)
	return args
}

func buildInternalPageArgs(userDataDir, page string) []string {
	return []string{
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-infobars",
		"--new-window",
		page,
	}
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
