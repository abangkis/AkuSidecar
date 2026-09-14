package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestBrowserProfilePathPrefersExplicitInstalledAppPath(t *testing.T) {
	cfg := config.Config{Root: `C:\Program Files\AkuBrowser\runtime\versions\1.2.3`}
	want := `C:\Users\tester\AppData\Local\AkuBrowser\browser-profile`
	if got := browserProfilePath(config.Options{BrowserProfilePath: "  " + want + "  "}, cfg); got != want {
		t.Fatalf("browserProfilePath=%q want=%q", got, want)
	}
}

func TestChromiumStartupLogIsRestrictedToIsolatedTestNamespace(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "browser-profile")
	t.Setenv("AKUBROWSER_CHROMIUM_STARTUP_DIAGNOSTICS", "1")
	t.Setenv("AKUBROWSER_ISOLATED_TEST_CREDENTIAL_NAMESPACE", "")
	if got := isolatedChromiumStartupLogPath(profile); got != "" {
		t.Fatalf("production profile unexpectedly enabled diagnostics: %q", got)
	}
	t.Setenv("AKUBROWSER_ISOLATED_TEST_CREDENTIAL_NAMESPACE", "AkuBrowserTest-f842094d1f035547")
	if want, got := filepath.Join(profile, "chrome-startup.log"), isolatedChromiumStartupLogPath(profile); got != want {
		t.Fatalf("isolated diagnostic path=%q, want %q", got, want)
	}
	t.Setenv("AKUBROWSER_CHROMIUM_STARTUP_DIAGNOSTICS", "")
	if got := isolatedChromiumStartupLogPath(profile); got != "" {
		t.Fatalf("diagnostics remained active without opt-in: %q", got)
	}
}

func TestDatabaseDecisionRequiredOnlyForInstalledApplication(t *testing.T) {
	for _, mode := range []string{"", "development", "development-supervised", "production-runtime", "production-installed-app"} {
		if got := requiresDatabaseDecision(config.DeploymentConfig{Mode: mode}); got != (mode == "production-installed-app") {
			t.Fatalf("mode %q guard=%v", mode, got)
		}
	}
}

func TestBrowserProfilePathKeepsLegacyFallback(t *testing.T) {
	cfg := config.Config{Root: t.TempDir()}
	want := filepath.Join(cfg.Root, "runtime", "app-profile")
	if got := browserProfilePath(config.Options{}, cfg); got != want {
		t.Fatalf("browserProfilePath=%q want=%q", got, want)
	}
}

func TestStartupStatusOnlyForFreshOrUpdatedInstalledTuple(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "browser-profile")
	installed := config.DeploymentConfig{Mode: "production-installed-app", ReleaseVersion: "0.9.0", SourceFreeze: "browser-a:sidecar-a:bridge-a"}
	show, mark, err := startupStatusPolicy(config.DeploymentConfig{Mode: "development"}, profile)
	if err != nil || show || mark != nil {
		t.Fatalf("development should not show native status: show=%v mark=%v err=%v", show, mark != nil, err)
	}
	show, mark, err = startupStatusPolicy(installed, profile)
	if err != nil || !show || mark == nil {
		t.Fatalf("fresh profile must show recovery: show=%v mark=%v err=%v", show, mark != nil, err)
	}
	if _, err := os.Stat(filepath.Join(profile, startupReadyMarker)); !os.IsNotExist(err) {
		t.Fatalf("marker created before interface acknowledgement: %v", err)
	}
	if err := mark(); err != nil {
		t.Fatal(err)
	}
	show, mark, err = startupStatusPolicy(installed, profile)
	if err != nil || show || mark != nil {
		t.Fatalf("daily launch should stay quiet: show=%v mark=%v err=%v", show, mark != nil, err)
	}
	installed.SourceFreeze = "browser-b:sidecar-a:bridge-a"
	show, mark, err = startupStatusPolicy(installed, profile)
	if err != nil || !show || mark == nil {
		t.Fatalf("updated tuple must show recovery once: show=%v mark=%v err=%v", show, mark != nil, err)
	}
	if err := mark(); err != nil {
		t.Fatal(err)
	}
	show, _, err = startupStatusPolicy(installed, profile)
	if err != nil || show {
		t.Fatalf("updated tuple should stay quiet after acknowledgement: show=%v err=%v", show, err)
	}
	installed.ReleaseVersion = "0.9.1"
	show, _, err = startupStatusPolicy(installed, profile)
	if err != nil || !show {
		t.Fatalf("new release version must show recovery: show=%v err=%v", show, err)
	}
}

func TestStartupStatusMissingIdentityKeepsRecoveryAvailable(t *testing.T) {
	show, mark, err := startupStatusPolicy(config.DeploymentConfig{Mode: "production-installed-app", ReleaseVersion: "0.9.0"}, t.TempDir())
	if !show || mark != nil || err == nil {
		t.Fatalf("missing source identity must not silently suppress recovery: show=%v mark=%v err=%v", show, mark != nil, err)
	}
}

func TestAppShellIconPathUsesAkuBridgeAsset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AkuBridge")
	want := filepath.Join(root, "icons", "icon-128.png")
	if got := appShellIconPath("  " + root + "  "); got != want {
		t.Fatalf("appShellIconPath=%q want=%q", got, want)
	}
	if got := appShellIconPath("  "); got != "" {
		t.Fatalf("empty extension path produced icon path %q", got)
	}
}

func TestDevelopmentAppShellIdentityRelaunchesThroughAkuBrowserLauncher(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Config{Root: filepath.Join(workspace, "AkuSidecar")}
	identity, err := appShellIdentity(config.Options{Dev: true}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ID != "AI4U.AkuBrowser.Development" || identity.DisplayName != "AkuBrowser Development" {
		t.Fatalf("identity=%+v", identity)
	}
	launcher := filepath.Join(workspace, "AkuBrowser", "launcher", "AkuBrowserLauncher.exe")
	if !strings.Contains(identity.RelaunchCommand, launcher) || !strings.Contains(identity.RelaunchCommand, "--development-workspace") {
		t.Fatalf("relaunch command=%q", identity.RelaunchCommand)
	}
}

func TestExistingInstanceDetectsHealthySidecar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Fatalf("unexpected probe path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"0.8.0"}`))
	}))
	defer server.Close()
	version, running := existingInstance(server.URL+"/api/health", time.Second)
	if !running || version != "0.8.0" {
		t.Fatalf("existing instance not detected: running=%v version=%q", running, version)
	}
}

func TestExistingInstanceIgnoresUnhealthyResponses(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"not ok status": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"degraded","version":"0.8.0"}`))
		},
		"error payload": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
		"malformed json": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`not-json`))
		},
	} {
		server := httptest.NewServer(handler)
		if _, running := existingInstance(server.URL+"/api/health", time.Second); running {
			defer server.Close()
			t.Fatalf("%s must not count as an existing instance", name)
		}
		server.Close()
	}
}

func TestExistingInstanceTreatsUnreachableAsAbsent(t *testing.T) {
	if _, running := existingInstance("http://127.0.0.1:1/api/health", 250*time.Millisecond); running {
		t.Fatal("unreachable endpoint must not count as an existing instance")
	}
}

func TestRuntimeCandidateProbeMatchesPublishedUpdateMetadata(t *testing.T) {
	probe, err := runtimeCandidateProbe(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	metadata := domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion)
	if len(probe) != 6 {
		t.Fatalf("candidate probe must remain bounded for the strict native-host decoder: %+v", probe)
	}
	if probe["status"] != "ok" || probe["runtime"] != "go" || probe["configVersion"] != 1 {
		t.Fatalf("candidate probe identity=%+v", probe)
	}
	if probe["version"] != metadata.CurrentVersion || probe["databaseSchemaVersion"] != metadata.DatabaseSchemaVersion || probe["bridgeContractVersion"] != domain.BridgeContractVersion {
		t.Fatalf("candidate probe=%+v software update metadata=%+v", probe, metadata)
	}
	if metadata.BridgeProtocol.Name != "aku-browser.bridge" || metadata.BridgeProtocol.MinVersion != engine.BridgeProtocolMajor || metadata.BridgeProtocol.MaxVersion != engine.BridgeProtocolMajor {
		t.Fatalf("candidate probe Bridge contract=%v software update protocol=%+v", probe["bridgeContractVersion"], metadata.BridgeProtocol)
	}
	if _, exposed := probe["softwareUpdate"]; exposed {
		t.Fatal("candidate probe must not add fields rejected by the strict native-host decoder")
	}
}

func TestRuntimeCandidateProbeKeepsLegacyHostShapeExact(t *testing.T) {
	probe, err := runtimeCandidateProbe(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(probe) != 5 {
		t.Fatalf("legacy candidate probe shape changed: %+v", probe)
	}
	if _, exposed := probe["databaseSchemaVersion"]; exposed {
		t.Fatal("legacy strict native host must not receive databaseSchemaVersion")
	}
	if _, err := runtimeCandidateProbe(1, 3); err == nil {
		t.Fatal("unsupported candidate probe schema was accepted")
	}
}
