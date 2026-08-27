package appshell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildArgsIncludesCoreSwitches(t *testing.T) {
	options := LaunchOptions{
		Executable:    "chrome.exe",
		URL:           "http://127.0.0.1:11122/",
		UserDataDir:   "profile",
		ExtensionPath: "C:\\bridge",
		ExtraArgs:     []string{"--window-size=1200,800"},
	}
	args := buildArgs(options)
	expected := []string{
		"--app=http://127.0.0.1:11122/",
		"--user-data-dir=profile",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode",
		"--disable-component-update",
		"--disable-session-crashed-bubble",
		"--load-extension=C:\\bridge",
		"--window-size=1200,800",
	}
	if len(args) != len(expected) {
		t.Fatalf("unexpected arg count %d: %v", len(args), args)
	}
	for index, value := range expected {
		if args[index] != value {
			t.Fatalf("arg[%d]=%q expected %q", index, args[index], value)
		}
	}
}

func TestBuildArgsOmitsExtensionWhenUnset(t *testing.T) {
	args := buildArgs(LaunchOptions{URL: "http://127.0.0.1:1/", UserDataDir: "p"})
	for _, value := range args {
		if len(value) > 16 && value[:16] == "--load-extension" {
			t.Fatalf("extension switch must be absent: %v", args)
		}
	}
}

func TestBuildInternalPageArgsUsesSameProfileAndSeparateWindow(t *testing.T) {
	args := buildInternalPageArgs(`C:\AkuBrowser\profile`, "chrome://extensions")
	expected := []string{
		`--user-data-dir=C:\AkuBrowser\profile`,
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		"chrome://extensions",
	}
	if len(args) != len(expected) {
		t.Fatalf("unexpected arg count %d: %v", len(args), args)
	}
	for index, value := range expected {
		if args[index] != value {
			t.Fatalf("arg[%d]=%q expected %q", index, args[index], value)
		}
	}
}

func TestLaunchOptionsCarryIndependentWindowIcon(t *testing.T) {
	options := LaunchOptions{ExtensionPath: "C:\\bridge", IconPath: "C:\\bridge\\icons\\icon-128.png"}
	if options.IconPath == "" || options.IconPath == options.ExtensionPath {
		t.Fatalf("window icon path must be an explicit asset: %+v", options)
	}
}

func TestApplicationIdentityRequiresCompleteRelaunchTuple(t *testing.T) {
	if err := (ApplicationIdentity{ID: "AI4U.AkuBrowser.Development"}).validate(); err == nil {
		t.Fatal("partial application identity must fail")
	}
	if err := (ApplicationIdentity{
		ID:              "AI4U.AkuBrowser.Development",
		RelaunchCommand: `AkuBrowserLauncher.exe --development-workspace C:\workspace`,
		DisplayName:     "AkuBrowser Development",
	}).validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverAcceptsExistingExplicitCandidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, executableName())
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var validated Candidate
	result, err := discover(context.Background(), []Candidate{{Path: path, Source: "explicit"}}, false, func(_ context.Context, c Candidate) (string, error) {
		validated = c
		return "140.0.7000.0", nil
	})
	if err != nil {
		t.Fatalf("discover returned error: %v", err)
	}
	if result.Status != "ok" || result.Version != "140.0.7000.0" || result.Source != "explicit" {
		t.Fatalf("unexpected result %+v", result)
	}
	if validated.Path != path {
		t.Fatalf("validator received %q", validated.Path)
	}
}

func TestDiscoverStrictModeSurfacesAttempts(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, executableName())
	_, err := discover(context.Background(), []Candidate{{Path: missing, Source: "explicit"}}, true, func(context.Context, Candidate) (string, error) {
		return "", errors.New("must not be reached")
	})
	var discoveryErr *DiscoveryError
	if !errors.As(err, &discoveryErr) {
		t.Fatalf("expected DiscoveryError, got %v", err)
	}
	if len(discoveryErr.Result.Attempts) != 1 {
		t.Fatalf("expected one attempt, got %+v", discoveryErr.Result.Attempts)
	}
	if discoveryErr.Result.Attempts[0].Source != "explicit" {
		t.Fatalf("attempt source mismatch: %+v", discoveryErr.Result.Attempts[0])
	}
}

func TestDiscoverFallsBackToLaterCandidate(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "missing", executableName())
	second := filepath.Join(dir, executableName())
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(second, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(second, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	candidates := []Candidate{{Path: first, Source: "environment"}, {Path: second, Source: "workspace"}}
	calls := 0
	result, err := discover(context.Background(), candidates, false, func(context.Context, Candidate) (string, error) {
		calls++
		return "141.0.0.0", nil
	})
	if err != nil {
		t.Fatalf("discover returned error: %v", err)
	}
	if result.Executable != second || result.Source != "workspace" {
		t.Fatalf("unexpected selection %+v", result)
	}
	if calls != 1 {
		t.Fatalf("validator must run once for resolvable candidate, ran %d", calls)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].Source != "environment" {
		t.Fatalf("successful result must retain earlier attempts for diagnostics: %+v", result)
	}
}

func TestResolveDirectoryCandidateUsesPlatformExecutable(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveCandidate(Candidate{Path: dir}); err == nil {
		t.Fatal("empty directory must fail resolution")
	}
	named := filepath.Join(dir, executableName())
	if err := os.WriteFile(named, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveCandidate(Candidate{Path: dir, Source: "workspace"})
	if err != nil {
		t.Fatalf("directory with platform executable must resolve: %v", err)
	}
	if resolved.Path != named {
		t.Fatalf("resolved %q expected %q", resolved.Path, named)
	}
}

func TestValidateCandidateRejectsOutputWithoutVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake --version probe requires a shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-chrome")
	body := "#!/bin/sh\necho not-a-browser\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCandidate(context.Background(), Candidate{Path: script}); err == nil {
		t.Fatal("output without a version must be rejected")
	}
}
