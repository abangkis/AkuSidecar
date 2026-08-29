package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/credentialstore"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/credentials"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestSessionProgressProjectionKeepsOnlyPollingState(t *testing.T) {
	session := &domain.Session{
		ID:       "session_projection",
		Status:   "running",
		Items:    []domain.TimelineItem{{}, {}},
		Coverage: map[string]any{"pipelineStage": "candidate_evaluation", "pipelineStageUpdatedAt": "now", "largeEvidence": []any{"unused"}},
		Runs:     []domain.Run{{ID: "run_projection", Status: "reasoning", Coverage: map[string]any{"largeCaptureTelemetry": []any{"unused"}}}},
	}

	projected := sessionProgressProjection(session)
	if projected == nil {
		t.Fatal("projection unexpectedly returned nil")
	}
	if projected.ItemCount != 2 || projected.Items != nil {
		t.Fatalf("projection must replace full items with itemCount, got count=%d items=%v", projected.ItemCount, projected.Items)
	}
	if len(projected.Coverage) != 2 || projected.Coverage["pipelineStage"] != "candidate_evaluation" {
		t.Fatalf("projection retained unexpected session coverage: %#v", projected.Coverage)
	}
	if len(projected.Runs) != 1 || projected.Runs[0].Coverage != nil {
		t.Fatalf("projection retained run coverage: %#v", projected.Runs)
	}
	if len(session.Items) != 2 || session.Runs[0].Coverage == nil || session.Coverage["largeEvidence"] == nil {
		t.Fatal("projection mutated the authoritative session")
	}
}

func TestSettingsSourcesExposePerSourceAccessFlow(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"onboarding-source-options", "Grant access or Sign in action below", "source-settings-group"},
		"web/app.js":     {"function openSourceAccessSettings", "function onboardingReadinessGate", "sourcePermissionReadyForOnboarding", "sourceAccessReadinessState", "Access: permission required", "Access: capture registration missing", "AKU_BROWSER_OPEN_SOURCE", "data-onboarding-source-readiness", "function configureNativePostLink", "AKU_BROWSER_OPEN_NATIVE_POST", "AKU_BROWSER_NATIVE_POST_OPENED"},
		"web/styles.css": {".source-settings-group .settings-group-heading-actions { margin-bottom: 12px; }", ".onboarding-source-access", "white-space: nowrap"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing setup CTA contract %q", asset, marker)
			}
		}
	}
	for _, asset := range []string{"web/index.html", "web/app.js"} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), "AKU_BROWSER_OPEN_BRIDGE_SETUP") || strings.Contains(string(contents), "Open AkuBrowser setup") {
			t.Fatalf("%s must not expose the legacy global Bridge setup action", asset)
		}
	}
}

func TestEmbeddedLibrarySurfaceContract(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {
			"library-view-button",
			"id=\"library-panel\"",
			"library-search-form",
			"library-search-toolbar",
			"library-filters-toggle",
			"aria-controls=\"library-filter-fields\"",
			"library-filter-fields",
			"role=\"search\"",
			"library-source",
			"library-tier",
			"library-published-from",
			"library-published-to",
			"library-load-more",
			"library-detail",
			"aria-live=\"polite\"",
		},
		"web/app.js": {
			"library-state.js",
			"timeline-memory-state.js",
			"function loadLibrary",
			"function submitLibrarySearch",
			"function toggleLibraryFilters",
			"function syncLibraryFilterToggle",
			"function loadMoreLibrary",
			"buildLibraryRequestPath",
			"buildLibraryRemovePath",
			"buildLibraryForgetPath",
			"buildLibraryReleasePath",
			"libraryRemoveConfirmation",
			"libraryForgetConfirmation",
			"libraryReleaseConfirmation",
			"buildTimelineKeepPath",
			"timelineKeepConfirmation",
			"/api/library/items/",
			"method: \"DELETE\"",
			"method: \"POST\"",
			"Remove from Library",
			"Forget permanently",
			"Release full copy",
			"Keep full copy",
			"Library reads never call a provider",
			"sidecar_unavailable",
		},
		"web/library-state.js": {
			"LIBRARY_MAX_QUERY = 200",
			"buildLibraryRequestPath",
			"buildLibraryRemovePath",
			"buildLibraryForgetPath",
			"buildLibraryReleasePath",
			"/forget-permanently",
			"/release-full-copy",
			"mergeLibraryPage",
		},
		"web/timeline-memory-state.js": {
			"buildTimelineKeepPath",
			"/keep-full-copy",
		},
		"web/styles.css": {
			".library-panel",
			".library-search-form",
			".library-search-toolbar",
			".library-layout.has-selection",
			".timeline-primary-actions",
			".library-layout",
			".library-detail",
			"@media (max-width: 760px)",
		},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing read-only Library UI contract %q", asset, marker)
			}
		}
	}
	appContents, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(appContents), "buildLibraryDeletePath") || strings.Contains(string(appContents), "libraryDeleteConfirmation") || strings.Contains(string(appContents), "deleteLibraryItem") {
		t.Fatal("Library UI must distinguish local Remove from permanent Forget")
	}
	indexContents, err := embeddedAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexContents)
	start := strings.Index(index, `<section id="library-panel"`)
	if start < 0 {
		t.Fatal("embedded Library panel is missing")
	}
	end := strings.Index(index[start:], `</section>`)
	if end < 0 {
		t.Fatal("embedded Library panel is incomplete")
	}
	libraryPanel := index[start : start+end]
	if strings.Contains(libraryPanel, `method="POST"`) || strings.Contains(libraryPanel, `method="PUT"`) {
		t.Fatal("Library panel must not embed broad form mutations")
	}
}

func TestEmbeddedFailureStateMatrixStaysSidecarOwned(t *testing.T) {
	appPayload, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	indexPayload, err := embeddedAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"sourceAccessReadinessState",
		"permission_not_granted",
		"registration_missing",
		"Access: capture registration missing",
		"login_required: \"Session: sign-in required\"",
		"availabilityMessage",
		"Unavailable",
		"function renderBrowserConnection",
		"Waiting for AkuBridge to reconnect",
		"AkuSidecar offline",
		"RUN STOPPED",
		"failure-manual-bundle-download",
		"Start-AkuBrowser.ps1",
	} {
		if !strings.Contains(string(appPayload), marker) && !strings.Contains(string(indexPayload), marker) {
			t.Fatalf("Sidecar failure-state contract is missing %q", marker)
		}
	}
	for _, asset := range []struct {
		name string
		data []byte
	}{
		{name: "web/app.js", data: appPayload},
		{name: "web/index.html", data: indexPayload},
	} {
		for _, forbidden := range []string{"setup.html", "AKU_BROWSER_OPEN_BRIDGE_SETUP", "AKU_BRIDGE_OPEN_SETUP"} {
			if strings.Contains(string(asset.data), forbidden) {
				t.Fatalf("%s must not invoke legacy setup surface %q", asset.name, forbidden)
			}
		}
	}
}

func TestOnboardingSessionObservationIsAdvisory(t *testing.T) {
	contents, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(contents)
	for _, forbidden := range []string{
		`session?.state === "ready"`,
		`session?.state === "login_required"`,
		`sourceSessionProbeInFlight) {`,
	} {
		if strings.Contains(app, forbidden) {
			t.Fatalf("onboarding must not gate on live source tabs: found %q", forbidden)
		}
	}
}

func TestOnboardingGatesSourceSetupOnDevelopmentBridgeConnection(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {
			"onboarding-browser-connection",
			"Connect AkuBridge first",
			"open-chrome-extensions",
			"Open Chrome Extensions",
			"onboarding-source-setup",
		},
		"web/app.js": {
			"function renderBrowserConnection",
			"function openChromeExtensions",
			"/api/app-shell/open-extensions",
			"This installed AkuBrowser runtime needs repair",
		},
		"web/styles.css": {".browser-connection-actions"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing browser connection contract %q", asset, marker)
			}
		}
		if strings.Contains(string(contents), `C:\WorkspaceCodex\AkuWorkspace\AkuBridge`) || strings.Contains(string(contents), "clipboard.writeText") {
			t.Fatalf("%s must not hard-code or copy the development AkuBridge path", asset)
		}
	}
}

func TestOpenExtensionsActionIsDevelopmentOnlyAndAppShellOwned(t *testing.T) {
	server := &Server{config: config.Config{
		Dev:        true,
		Deployment: config.DeploymentConfig{Mode: "development"},
	}}
	called := false
	server.SetOpenExtensionsAction(func(context.Context) error {
		called = true
		return nil
	})

	request := httptest.NewRequest(http.MethodPost, "/api/app-shell/open-extensions", nil)
	response := httptest.NewRecorder()
	if err := server.route(response, request); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusAccepted || !called {
		t.Fatalf("development action status=%d called=%v body=%s", response.Code, called, response.Body.String())
	}

	server.config.Deployment.Mode = "production-installed-app"
	request = httptest.NewRequest(http.MethodPost, "/api/app-shell/open-extensions", nil)
	response = httptest.NewRecorder()
	err := server.route(response, request)
	var apiErr apiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("production action error=%v", err)
	}

	server.config.Deployment.Mode = "development"
	server.SetOpenExtensionsAction(nil)
	request = httptest.NewRequest(http.MethodPost, "/api/app-shell/open-extensions", nil)
	response = httptest.NewRecorder()
	err = server.route(response, request)
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != "app_shell_action_unavailable" {
		t.Fatalf("missing app-shell action error=%v", err)
	}
}

func TestOnboardingExposesProviderSelectionDialog(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {
			"onboarding-provider-dialog",
			"Choose how AkuBrowser reasons",
			"provider-selection-workspace",
			"onboarding-provider-options",
			"<aside id=\"onboarding-provider-setup\"",
			"onboarding-provider-panel-label",
			"onboarding-provider-recheck",
			"onboarding-provider-check-status",
			"onboarding-provider-secret",
			"Save key",
			"secure credential store",
			"Keep Codex App Server",
		},
		"web/app.js": {
			"ONBOARDING_PROVIDER_COPY",
			"function openOnboardingProviderDialog",
			"function confirmOnboardingProvider",
			"function skipOnboardingProvider",
			"https://aistudio.google.com/apikey",
			"/api/reasoning/credentials",
			"function saveOnboardingProviderCredential",
			"Google may use that data to improve its products",
			"has-provider-context",
			"CODEX APP CONNECTION",
			"LOCAL MODEL SETUP",
			"ollama pull nemotron-3.5-lightning",
			"ollama pull qwen3.8:27b",
			"/api/reasoning/providers/readiness",
			"providerReadinessCheckedAt",
			"Ready",
			"Unavailable",
		},
		"web/styles.css": {
			".provider-selection-dialog",
			".provider-selection-dialog.has-provider-context .provider-selection-workspace",
			"grid-template-columns: minmax(0, 1.08fr) minmax(300px, 0.92fr)",
			".onboarding-provider-option",
			"max-height: min(calc(100dvh - 40px), 900px)",
			"@media (max-height: 820px)",
		},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing onboarding provider contract %q", asset, marker)
			}
		}
	}
	contents, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "credentials.local.json") {
		t.Fatal("product onboarding must not instruct users to edit the development credential file")
	}
}

type httpCredentialStore struct {
	values map[credentialstore.Reference]string
}

func (store *httpCredentialStore) Get(reference credentialstore.Reference) (string, error) {
	value, ok := store.values[reference]
	if !ok {
		return "", credentialstore.ErrNotFound
	}
	return value, nil
}

func (store *httpCredentialStore) Put(reference credentialstore.Reference, value string) error {
	store.values[reference] = value
	return nil
}

func (store *httpCredentialStore) Delete(reference credentialstore.Reference) error {
	delete(store.values, reference)
	return nil
}

func TestReasoningCredentialWriteUsesConfiguredReferenceAndNeverEchoesSecret(t *testing.T) {
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Reasoning: config.ReasoningConfig{
		ActiveProvider: "deterministic",
		Providers: map[string]config.ProviderConfig{
			"deterministic":     {},
			"gemini-flash-lite": {CredentialRef: "gemini.primary"},
		},
	}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	secureStore := &httpCredentialStore{values: map[credentialstore.Reference]string{}}
	server.credentials = credentials.NewManager(secureStore, nil)

	const secret = "test-only-secret-that-must-not-be-returned"
	request := httptest.NewRequest(http.MethodPut, "/api/reasoning/credentials", strings.NewReader(`{"provider":"gemini-flash-lite","secret":"`+secret+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	if err := server.route(response, request); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := secureStore.values["gemini.primary"]; got != secret {
		t.Fatalf("stored value=%q", got)
	}
	if strings.Contains(response.Body.String(), secret) {
		t.Fatalf("response leaked credential: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"reference":"gemini.primary"`) || !strings.Contains(response.Body.String(), `"configured":true`) {
		t.Fatalf("response=%s", response.Body.String())
	}
}

func TestFullResetReportsBridgeRevocationOutcomeHonestly(t *testing.T) {
	contents, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	asset := string(contents)
	for _, marker := range []string{
		"sourceAccessRevoked = await revokeSourceAccessViaBridge()",
		"Full reset stopped because AkuBridge did not confirm source-access revocation",
		"Source access revoked; browser profile preserved.",
		"The browser profile, AkuBridge installation, and existing website sign-ins are preserved.",
	} {
		if !strings.Contains(asset, marker) {
			t.Fatalf("web/app.js is missing full-reset revocation outcome %q", marker)
		}
	}
	for _, retired := range []string{"staged browser-profile reset", "browser-profile wipe will remove it"} {
		if strings.Contains(asset, retired) {
			t.Fatalf("web/app.js retains retired profile-wipe copy %q", retired)
		}
	}
}

func TestSettingsSourcesExposeSessionReadinessAndSignInCTA(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/app.js": {
			"requestSourceSessionReadiness",
			"AKU_BROWSER_PROBE_SOURCE_SESSIONS",
			"AKU_BROWSER_SOURCE_SESSIONS_RESULT",
			"AKU_BROWSER_OPEN_SOURCE",
			"data-source-session-readiness",
			"Session: feed available",
		},
		"web/styles.css": {
			".source-session-status",
			".source-session-warning",
			".source-open-button",
		},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing session readiness contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedAppDoesNotEscapeTemplateLiteralDelimiters(t *testing.T) {
	contents, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"\\`", "\\${"} {
		if strings.Contains(string(contents), invalid) {
			t.Fatalf("web/app.js contains invalid escaped template syntax %q", invalid)
		}
	}
}

func TestInstagramPostUsesFamiliarLocalSourceIcon(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/app.js":     {`source === "instagram"`, `glyph.className = "instagram-source-glyph"`, `aria-hidden`},
		"web/styles.css": {".timeline-source-icon-instagram", ".instagram-source-glyph::before", ".instagram-source-glyph::after", "radial-gradient"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing Instagram post icon contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedMediaViewerStartsFittedSupportsZoomAndClosesCleanly(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"method=\"dialog\"", "media-viewer-zoom", "aria-pressed=\"false\"", "media-viewer-canvas", "type=\"submit\""},
		"web/app.js":     {"function setMediaZoom", "setMediaZoom(false)", "mediaZoomed", "Fit image to container", "Zoom image to full size"},
		"web/styles.css": {".media-viewer[open]", ".media-viewer:not([open])", ".media-viewer-canvas", ".media-viewer.is-zoomed", "overflow: hidden", "max-width: none"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing media viewer zoom contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedSingleImageFitSettingDefaultsToCoverAndSupportsContain(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"data-single-image-fit=\"cover\"", "id=\"single-image-fit\"", "value=\"cover\"", "value=\"contain\""},
		"web/app.js":     {"function applySingleImageFit", "settings.singleImageFit || \"cover\"", "singleImageFit: $(\"#single-image-fit\").value"},
		"web/styles.css": {"[data-single-image-fit=\"contain\"] .source-layout-media.media-count-1 > .source-layout-media-item img", "[data-single-image-fit=\"contain\"] .source-layout-media-carousel-viewport .source-layout-media-item img", "object-fit: contain"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing single-image fit setting contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedPostFreshnessCueSupportsConfigurablePresentation(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"data-post-freshness-style=\"header_shade\"", "id=\"post-freshness-style\"", "value=\"header_shade\"", "value=\"border_shade\"", "value=\"off\""},
		"web/app.js":     {"post-freshness.js", "function applyPostFreshness", "function applyPostFreshnessStyle", "settings.postFreshnessStyle || \"header_shade\"", "postFreshnessStyle: $(\"#post-freshness-style\").value"},
		"web/styles.css": {"data-freshness=\"current\"", "data-post-freshness-style=\"header_shade\"", "data-post-freshness-style=\"border_shade\"", "freshness-header-opacity"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing post freshness contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedTimelineMediaCarouselStartsAtFiveAndPreservesLightboxAccess(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/app.js":                     {"timeline-media-carousel.js", "function buildTimelineMediaCarousel", "timelineCarouselIndexes", "Previous post image", "Next post image", "openMedia(imageMedia.map"},
		"web/timeline-media-carousel.js": {"MAX_TIMELINE_MEDIA = 20", "TIMELINE_CAROUSEL_THRESHOLD = 5", "moveTimelineCarouselIndex", "timelineCarouselDotIndexes"},
		"web/styles.css":                 {".source-layout-media-carousel-stage", ".source-layout-media-carousel-navigation", ".source-layout-media-carousel-dot.is-current", "touch-action: pan-y"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing timeline carousel contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedTimelineCalibrationBlockerExposesProgress(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {
			"timeline-calibration-progress",
			"timeline-calibration-progress-label",
			"Calibration completion",
			"timeline-calibration-continue",
		},
		"web/app.js": {
			"function renderTimelineCalibrationProgress",
			"calibration samples reviewed",
			"showCalibrationProgress",
		},
		"web/styles.css": {
			".timeline-calibration-progress",
			".timeline-calibration-progress-track",
			"calibration-preparing",
		},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing Timeline calibration progress contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedPrimaryViewsShareClearNavigationAndHeaders(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {
			"aria-current=\"page\"",
			"AKUBROWSER SETTINGS",
			"Personalization, source access, automation, and local reasoning controls",
			"CHECK HISTORY",
		},
		"web/app.js": {
			"setAttribute(\"aria-current\", timeline ? \"page\" : \"false\")",
			"setAttribute(\"aria-current\", inbox ? \"page\" : \"false\")",
			"setAttribute(\"aria-current\", settings ? \"page\" : \"false\")",
		},
		"web/styles.css": {
			".timeline-heading-row, .section-heading",
			".settings-panel > .section-heading",
			".inbox-session:hover",
			".settings-row:focus-within",
		},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing primary-view redesign contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedReasoningProviderSelectionContract(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"reasoning-provider", "name=\"reasoningProvider\"", "reasoning-provider-status", "Reasoning provider", "Choose the inference backend used by every reasoning process", "the switch applies when no update is running"},
		"web/app.js":     {"state.bootstrap?.reasoningProviders", "function beginSettingsProviderSelection", "function openSettingsProviderDialog", "function activateSettingsReasoningProvider", "AkuBrowser checks any stored credential before asking for a new key", "credential missing", "Provider changed", "$(\"#reasoning-provider\")?.value", "Ollama · Qwen 3.8 27B", "availabilityRequired"},
		"web/styles.css": {".reasoning-executable-row[hidden]", ".provider-selection-dialog.provider-switch-mode", ".reasoning-provider-control { display: grid; justify-self: end; width: min(100%, 260px); min-width: 0;"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s is missing reasoning provider selection contract %q", asset, marker)
			}
		}
	}
	indexContents, err := embeddedAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(indexContents), "reasoning-provider-readiness-refresh") {
		t.Fatal("Settings must not expose provider readiness controls until the user chooses a different provider")
	}
}

func TestRuntimeControlTokenIsRequiredAndComparedExactly(t *testing.T) {
	token := strings.Repeat("a", 64)
	if !validRuntimeControlToken(token, token) {
		t.Fatal("exact runtime control token was rejected")
	}
	for _, supplied := range []string{"", strings.Repeat("a", 63), strings.Repeat("b", 64)} {
		if validRuntimeControlToken(token, supplied) {
			t.Fatalf("invalid runtime control token was accepted: %q", supplied)
		}
	}
}

func TestHealthAndBootstrapExposeGoBoundary(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Capture: config.CaptureConfig{Profile: "expanded", Visibility: "quiet", OpenMissingSource: true, MaxAcquisitionRounds: 2}, Preference: config.PreferenceConfig{Mode: "promote_unused_budget"}}
	logger := log.New(io.Discard, "", 0)
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	address, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + address.String() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var health map[string]any
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["runtime"] != "go" || health["bridgeContractVersion"] != domain.BridgeContractVersion {
		t.Fatalf("health=%+v", health)
	}
	softwareUpdate := health["softwareUpdate"].(map[string]any)
	if softwareUpdate["component"] != "AkuSidecar" || softwareUpdate["currentVersion"] != domain.ApplicationVersion || softwareUpdate["protocolVersion"] != domain.SidecarUpdateProtocolVersion || softwareUpdate["databaseSchemaVersion"] != float64(store.SchemaVersion) {
		t.Fatalf("software update metadata=%+v", softwareUpdate)
	}
	bridgeProtocol := softwareUpdate["bridgeProtocol"].(map[string]any)
	if bridgeProtocol["name"] != "aku-browser.bridge" || bridgeProtocol["minVersion"] != float64(engine.BridgeProtocolMajor) || bridgeProtocol["maxVersion"] != float64(engine.BridgeProtocolMajor) {
		t.Fatalf("software update Bridge protocol=%+v", bridgeProtocol)
	}
	updateCapabilities := softwareUpdate["updateCapabilities"].([]any)
	if len(updateCapabilities) != 3 || updateCapabilities[0] != "runtime_update_readiness" || updateCapabilities[1] != "authorized_idle_shutdown" || updateCapabilities[2] != "instance_epoch_health" {
		t.Fatalf("software update capabilities=%+v", updateCapabilities)
	}
	database := health["database"].(map[string]any)
	if database["status"] != "healthy" {
		t.Fatalf("database health=%+v", database)
	}
	if _, exposed := database["path"]; exposed {
		t.Fatalf("health must not expose the absolute database path: %+v", database)
	}
	if response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
	csp := response.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "https://*.fbcdn.net") || !strings.Contains(csp, "https://*.fbsbx.com") {
		t.Fatalf("Facebook media hosts missing from CSP: %s", csp)
	}
	if !strings.Contains(csp, "https://cdninstagram.com") || !strings.Contains(csp, "https://*.cdninstagram.com") {
		t.Fatalf("Instagram media hosts missing from CSP: %s", csp)
	}
	if !strings.Contains(csp, "media-src 'self' https://video.twimg.com https://dms.licdn.com https://fbcdn.net https://*.fbcdn.net https://fbsbx.com https://*.fbsbx.com https://cdninstagram.com https://*.cdninstagram.com") {
		t.Fatalf("video playback hosts missing from CSP: %s", csp)
	}
	response, err = client.Get("http://" + address.String() + "/api/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("bootstrap must not be cached: %q", response.Header.Get("Cache-Control"))
	}
	var bootstrap map[string]any
	if err := json.NewDecoder(response.Body).Decode(&bootstrap); err != nil {
		t.Fatal(err)
	}
	if bootstrap["bridgeToken"] == "" || bootstrap["provider"] != "deterministic" {
		t.Fatalf("bootstrap=%+v", bootstrap)
	}
	bootstrapSoftwareUpdate := bootstrap["softwareUpdate"].(map[string]any)
	healthSoftwareUpdateJSON, _ := json.Marshal(softwareUpdate)
	bootstrapSoftwareUpdateJSON, _ := json.Marshal(bootstrapSoftwareUpdate)
	if !bytes.Equal(bootstrapSoftwareUpdateJSON, healthSoftwareUpdateJSON) {
		t.Fatalf("bootstrap software update metadata=%+v", bootstrapSoftwareUpdate)
	}
	sources := bootstrap["sources"].([]any)
	if len(sources) != 4 || sources[2].(map[string]any)["id"] != "facebook" || sources[2].(map[string]any)["defaultActive"] != true || sources[3].(map[string]any)["id"] != "instagram" || sources[3].(map[string]any)["defaultActive"] != true {
		t.Fatalf("source descriptors=%+v", sources)
	}
	reasoningProcesses := bootstrap["reasoningProcesses"].([]any)
	if len(reasoningProcesses) != 4 || reasoningProcesses[3].(map[string]any)["id"] != "ai_deep_detection" || reasoningProcesses[3].(map[string]any)["model"] != "local-deterministic" {
		t.Fatalf("reasoning processes=%+v", reasoningProcesses)
	}
	reasoningRuntime := bootstrap["reasoningRuntime"].(map[string]any)
	if reasoningRuntime["provider"] != "deterministic" || reasoningRuntime["editable"] != false {
		t.Fatalf("reasoning runtime=%+v", reasoningRuntime)
	}
	if reasoningProviders, ok := bootstrap["reasoningProviders"].([]any); !ok || len(reasoningProviders) != 0 {
		t.Fatalf("reasoning providers=%+v", bootstrap["reasoningProviders"])
	}
	bootstrapSettings := bootstrap["settings"].(map[string]any)
	if bootstrapSettings["timelineBoundaryCueMode"] != "follow" || bootstrapSettings["timelineBoundaryReturnMs"] != float64(350) || bootstrapSettings["showLearningPanel"] != true || bootstrapSettings["semanticEventMergeThreshold"] != .92 || bootstrapSettings["aiDetectionPresentation"] != "drawer" || bootstrapSettings["aiDetectionEnabled"] != true || bootstrapSettings["resurfaceMode"] != "smart" || bootstrapSettings["resurfaceCooldownDays"] != float64(7) {
		t.Fatalf("timeline boundary cue settings=%+v", bootstrapSettings)
	}
	if bootstrapSettings["reasoningAcquisitionProfile"] != "luna_high" || bootstrapSettings["reasoningEvaluationProfile"] != "luna_high" || bootstrapSettings["reasoningSemanticProfile"] != "luna_high" || bootstrapSettings["reasoningAiDeepProfile"] != "luna_high" {
		t.Fatalf("reasoning defaults=%+v", bootstrapSettings)
	}
	if bootstrapSettings["autoUpdateEnabled"] != true || bootstrapSettings["autoUpdateMode"] != "adaptive" || bootstrapSettings["autoUpdateRefillMinutes"] != float64(15) || bootstrapSettings["preparedBatchLimit"] != float64(2) || bootstrapSettings["autoUpdateDailyTokenBudget"] != float64(2000000) || bootstrapSettings["autoUpdateManualReservePct"] != float64(25) || bootstrapSettings["preparedBatchMaxAgeHours"] != float64(24) || bootstrapSettings["nextBatchBehavior"] != "require_action" {
		t.Fatalf("auto update defaults=%+v", bootstrapSettings)
	}
	autoUpdate := bootstrap["autoUpdate"].(map[string]any)
	if autoUpdate["enabled"] != true || autoUpdate["mode"] != "adaptive" || autoUpdate["state"] != "idle" {
		t.Fatalf("auto update status=%+v", autoUpdate)
	}
	if autoUpdate["preparedBatchLimit"] != float64(2) || autoUpdate["availablePreparedSlots"] != float64(2) || autoUpdate["refillIntervalMinutes"] != float64(15) {
		t.Fatalf("auto update queue telemetry=%+v", autoUpdate)
	}
	if _, exposed := autoUpdate["lastUserActivityAt"]; exposed || autoUpdate["recentUserActivity"] != false || autoUpdate["activityWindowMinutes"] != float64(30) || autoUpdate["cadenceTier"] != "standby" || autoUpdate["cadenceMinutes"] != float64(15) || autoUpdate["nextCheckAt"] == "" {
		t.Fatalf("auto update activity telemetry=%+v", autoUpdate)
	}
	if autoUpdate["adaptiveTargetBatches"] != float64(1) || autoUpdate["adaptiveBaseTargetBatches"] != float64(1) || autoUpdate["adaptiveReadyItems"] != float64(0) || autoUpdate["adaptiveReadyItemTarget"] != float64(3) || autoUpdate["consumptionSamples"] != float64(0) || autoUpdate["preparationLeadMinutes"] != float64(8) || autoUpdate["generationWindowMinutes"] != float64(30) || autoUpdate["generationAllowanceUsed"] != float64(0) || autoUpdate["generationAllowanceLimit"] != float64(2) || autoUpdate["replenishmentPressure"] != float64(0) || autoUpdate["pressureWindowMinutes"] != float64(60) || autoUpdate["pressureHalfLifeMinutes"] != float64(30) {
		t.Fatalf("adaptive demand telemetry=%+v", autoUpdate)
	}
	if receipts, ok := autoUpdate["schedulerReceipts"].([]any); !ok || len(receipts) != 0 {
		t.Fatalf("fresh scheduler receipts=%+v", autoUpdate["schedulerReceipts"])
	}
	if autoUpdate["dailyTokenBudget"] != float64(2000000) || autoUpdate["dailyTokensUsed"] != float64(0) || autoUpdate["quotaTokensUsed"] != float64(0) || autoUpdate["dailyTokensRemaining"] != float64(2000000) || autoUpdate["manualReserveTokens"] != float64(500000) || autoUpdate["automaticTokenLimit"] != float64(1500000) || autoUpdate["automaticTokensRemaining"] != float64(1500000) || autoUpdate["budgetResetAt"] == "" {
		t.Fatalf("auto update budget telemetry=%+v", autoUpdate)
	}
	activityBeforeStatus, err := state.AutoUpdateScheduleState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if activityBeforeStatus.LastUIAccessAt != "" {
		t.Fatal("read-only bootstrap fetch must not impersonate visible user activity")
	}
	statusResponse, err := client.Get("http://" + address.String() + "/api/auto-update/status")
	if err != nil {
		t.Fatal(err)
	}
	statusResponse.Body.Close()
	activityAfterStatus, err := state.AutoUpdateScheduleState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if activityAfterStatus.LastUIAccessAt != activityBeforeStatus.LastUIAccessAt {
		t.Fatal("background status polling must not renew human activity")
	}
	time.Sleep(time.Millisecond)
	activityRequest, err := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/ui/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	activityResponse, err := client.Do(activityRequest)
	if err != nil {
		t.Fatal(err)
	}
	var activityPayload struct {
		Recorded   bool                    `json:"recorded"`
		AutoUpdate domain.AutoUpdateStatus `json:"autoUpdate"`
	}
	if err := json.NewDecoder(activityResponse.Body).Decode(&activityPayload); err != nil {
		activityResponse.Body.Close()
		t.Fatal(err)
	}
	activityResponse.Body.Close()
	if !activityPayload.Recorded || !activityPayload.AutoUpdate.RecentUserActivity || activityPayload.AutoUpdate.LastUserActivityAt == "" || activityPayload.AutoUpdate.State != "idle" || activityPayload.AutoUpdate.CadenceTier != "demand" || activityPayload.AutoUpdate.CadenceMinutes != 5 || activityPayload.AutoUpdate.AdaptiveTargetBatches != 1 {
		t.Fatalf("explicit UI activity status=%+v", activityPayload)
	}
	activityAfterEvent, err := state.AutoUpdateScheduleState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if activityAfterEvent.LastUIAccessAt == activityAfterStatus.LastUIAccessAt {
		t.Fatal("explicit UI activity did not renew adaptive scheduling")
	}
	resetRequest, err := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/auto-update/budget/reset", nil)
	if err != nil {
		t.Fatal(err)
	}
	resetResponse, err := client.Do(resetRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer resetResponse.Body.Close()
	if resetResponse.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resetResponse.Body)
		t.Fatalf("quota reset status=%d body=%s", resetResponse.StatusCode, payload)
	}
	if err := state.PauseAutoUpdateForUsageLimit(context.Background(), domain.AutoUpdateUsageLimitPause{Message: "You've hit your usage limit"}); err != nil {
		t.Fatal(err)
	}
	usageStatusResponse, err := client.Get("http://" + address.String() + "/api/auto-update/status")
	if err != nil {
		t.Fatal(err)
	}
	var usageStatusPayload struct {
		AutoUpdate domain.AutoUpdateStatus `json:"autoUpdate"`
	}
	if err := json.NewDecoder(usageStatusResponse.Body).Decode(&usageStatusPayload); err != nil {
		usageStatusResponse.Body.Close()
		t.Fatal(err)
	}
	usageStatusResponse.Body.Close()
	if usageStatusPayload.AutoUpdate.State != "usage_limit_paused" || usageStatusPayload.AutoUpdate.UsageLimitMessage == "" {
		t.Fatalf("usage-limit status=%+v", usageStatusPayload.AutoUpdate)
	}
	restoreRequest, err := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/auto-update/usage-limit/restore", nil)
	if err != nil {
		t.Fatal(err)
	}
	restoreResponse, err := client.Do(restoreRequest)
	if err != nil {
		t.Fatal(err)
	}
	var restorePayload struct {
		AutoUpdate domain.AutoUpdateStatus `json:"autoUpdate"`
	}
	if err := json.NewDecoder(restoreResponse.Body).Decode(&restorePayload); err != nil {
		restoreResponse.Body.Close()
		t.Fatal(err)
	}
	restoreResponse.Body.Close()
	if restoreResponse.StatusCode != http.StatusOK || restorePayload.AutoUpdate.State == "usage_limit_paused" {
		t.Fatalf("usage restore status=%d payload=%+v", restoreResponse.StatusCode, restorePayload.AutoUpdate)
	}
	onboarding := bootstrap["onboarding"].(map[string]any)
	if onboarding["status"] != "not_started" {
		t.Fatalf("fresh onboarding=%+v", onboarding)
	}
	for path, markers := range map[string][]string{
		"/":                           {"Semantic event engine", "AI Detector", "AI Detection", "ai-detection-enabled", "Resurfaced content", "resurface-mode", "resurface-cooldown-days", "Auto Update", "auto-update-enabled", "auto-update-budget-status", "auto-update-budget-detail", "Confirm Codex usage restored", "Prepare batch now", "Update now", "timeline-prepared-button", "Load next batch", "Reasoning processes", "reasoning-processes", "reasoning-executable-path", "detect-reasoning-executable", "ai-detection-presentation", "timeline-side-pane", "semantic-event-shortlist", "semantic-event-merge-threshold", "reset-semantic-event-merge-threshold", "knowledge-retention-days", "knowledge-storage-limit", "timeline-boundary-follow", "timeline-boundary-return-ms", "capture-visibility-policy", "quiet_multi_window", "Quiet capture — single window — recommended", "Quiet capture — multiple windows (trial)", "show-learning-panel", "Learning panel", "timeline-runner-status", "onboarding-learning-panel", "MEET AKUBROWSER", "WHAT THIS BUILD ADDS", "INSIDE AN UPDATE", "First, each source is read and evaluated", "Then, one finite Timeline is composed", "data-onboarding-slide=\"5\"", "onboarding-check-stages", "Restoring your Timeline and active check", "finish-line hidden", "view-switch hidden"},
		"/app.js":                     {"SOURCE_TEXT_COLLAPSE_CHARACTERS = 420", "function buildExpandableText", "function buildAttachments", "source-layout-attachments", "function buildMedia(values, source, contentKind", "source-layout-video-cue", "Video preview", "notice notice-complete", "function createNoticeDismissButton", "Dismiss notification", "timeline-history-boundary", "timeline-older-batch-marker", "syncBackToTopBoundaryPosition", "timelineBoundaryCueMode", "DEFAULT_TIMELINE_BOUNDARY_RETURN_MS = 350", "DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD = 0.92", "semanticEventMergeThreshold", "resetSemanticEventMergeThreshold", "is-following-boundary", "duplicate report", "function buildCollapsedDuplicate", "function showCorrectionNotice", "function buildMediaRecaptureButton", "function buildForegroundRecaptureOffer", "Try in foreground", "body: { captureMode }", "document.querySelectorAll(\".recapture-button\")", "AKU_BROWSER_MEDIA_RECAPTURE", "AKU_BROWSER_X_MEDIA_EVIDENCE_LOOKUP", "AKU_BROWSER_DISPATCH_FAILED", "recoverInvalidatedBridgeContext", "BRIDGE_CONTEXT_RECOVERY_WINDOW_MS = 30000", "authoritative Sidecar run outcome", "function enrichPassiveXMedia", "passive_x_cache", "/media-evidence", "\"not_interested\"", "Local fast path", "strongest overlap", "DEFAULT_TIMELINE_BATCH_GAP_PX = 36", "function buildInboxPreferenceDecisions", "section = document.createElement(\"details\")", "The latest More or Less decision is authoritative.", "function buildCaptureSurfaceTelemetry", "Capture surface", "function buildMediaAcquisitionTelemetry", "Media evidence", "function buildAcquisitionIdentityTelemetry", "Acquisition & identity", "does not invoke a model", "function buildVisionEvaluationTelemetry", "Instagram media-only review", "Labels are generated locally", "function buildInboxFlowInspector", "/api/inbox/runs/", "function buildInboxFlowItem", "Open source", "Should have selected", "/selection-corrections", "Re-evaluate run", "Selected by you", "Already captured", "Semantic duplicate", "source_unavailable", "SOURCE UNAVAILABLE", "function routeAIDetectedItems", "explicitAIOverride", "function buildAIDetectionControls", "function buildSourceIcon", "timeline-source-icon-", "expandedAIDetails", "expandedAIFeedbackOptions", "AI signal · Neutral", "Mark as not AI-generated", "Mark as AI-generated", "Unsure · Review more deeply", "Change scope or add a reason", "Optional AI feedback reason", "/ai-feedback", "function loadAIFeedbackHistory", "HIDE STRONG AI SIGNALS", "function timelineInteractionActive", "function queueBackgroundTimelineRefresh", "function flushBackgroundTimelineRefresh", "function timelineSidePaneAvailable", "function syncTimelineSidePaneVisibility", "state.timelineItems.length > 0", "function scheduleTimelineSidePanePosition", "--timeline-side-pane-top", "--timeline-side-pane-toggle-top", "ResizeObserver", "borderTopLeftRadius", "#result-items > [data-timeline-id]", "function detectReasoningExecutable", "/api/reasoning/runtime/discover", "reasoningExecutablePath", "function enforceCurrentSidecarEpoch", "sidecar_epoch_changed", "AkuSidecar is offline or unreachable", "AkuSidecar offline", "sidecar_unavailable", "pollInFlight", "function describeSessionProgress", "AI Fast Detection", "AI Deep Detection continues asynchronously", "ONBOARDING_LEARNING_INTERVAL_MS = 7000", "function shouldShowOnboardingLearning", "function toggleOnboardingLearningPlayback", "bootstrapLoading", "Restoring your Timeline and active check", "function onboardingRequiresSetup", "state.bootstrapLoading || !state.bootstrap", "status === \"not_started\""},
		"/styles.css":                 {".notice-complete", ".notice-dismiss", ".expandable-text-copy.is-collapsed", ".content-expander", ".timeline-batch-marker", ".timeline-older-batch-marker", "--timeline-batch-gap", "--back-to-top-return-duration", ".semantic-duplicate-item", ".paired-setting-control", ".recapture-button", ".foreground-recapture-offer", ".inbox-preference-decision", ".inbox-flow-inspector", ".inbox-flow-filters", ".inbox-flow-item-actions", ".inbox-selection-correction-button", ".inbox-flow-outcome-user_selected", ".inbox-flow-outcome-collapsed_duplicate", ".acquisition-identity-telemetry", ".acquisition-identity-body", ".vision-evaluation-telemetry", ".ai-origin-badge", ".ai-origin-neutral", ".timeline-side-pane", ".timeline-source-icon-x { background: #050505; color: #fff; }", ".onboarding-learning-panel", ".onboarding-learning-track", ".onboarding-learning-dots", ".onboarding-check-stages", ".source-layout-media-carousel-stage"},
		"/timeline-media-carousel.js": {"MAX_TIMELINE_MEDIA = 20", "TIMELINE_CAROUSEL_THRESHOLD = 5"},
	} {
		response, err = client.Get("http://" + address.String() + path)
		if err != nil {
			t.Fatal(err)
		}
		payload, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("asset %s status=%d err=%v", path, response.StatusCode, readErr)
		}
		for _, marker := range markers {
			if !strings.Contains(string(payload), marker) {
				t.Fatalf("asset %s missing %q", path, marker)
			}
		}
	}
	response, err = client.Get("http://" + address.String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	indexPayload, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !strings.Contains(string(indexPayload), `rel="icon" type="image/svg+xml" href="/favicon.svg?runtime=release-0.9.0"`) {
		t.Fatal("AkuBrowser page does not declare its branded favicon")
	}
	response, err = client.Get("http://" + address.String() + "/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	faviconPayload, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(faviconPayload), `aria-label="AkuBrowser"`) {
		t.Fatal("AkuBrowser favicon is not served as an embedded asset")
	}
	response, err = client.Get("http://" + address.String() + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	appPayload, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"function buildVideoMedia",
		"function buildVideoPosterControl",
		"function activateInlineVideo",
		"function observeInlineVideoVisibility",
		"function safePlaybackUrl",
		"source === \"linkedin\"",
		"host !== \"dms.licdn.com\"",
		"source === \"facebook\"",
		"source === \"instagram\"",
		"video.preload = \"none\"",
		"video.src = playbackUrl",
		"video.addEventListener(\"timeupdate\"",
		"if (!video.paused) recordUIActivity(false)",
		"video.addEventListener(\"error\", useNativeFallback",
		"!entry.isIntersecting && !observedVideo.paused",
		"Play on native post",
	} {
		if !strings.Contains(string(appPayload), marker) {
			t.Fatalf("app.js missing inline playback contract %q", marker)
		}
	}
	if strings.Contains(string(appPayload), "Play video") {
		t.Fatal("inline playback should rely on the centered play cue without a redundant text label")
	}
	if strings.Contains(string(appPayload), "source-layout-video-native-link") {
		t.Fatal("inline X video poster must not render a simultaneous native-post overlay")
	}
	if !strings.Contains(string(appPayload), "entry.feedback?.direction") {
		t.Fatal("timeline feedback state is not restored after rendering")
	}
	if !strings.Contains(string(appPayload), "state.expandedTimelineText.has(expansionKey)") {
		t.Fatal("expanded Timeline text state is not restored after rendering")
	}
	pollStart := strings.Index(string(appPayload), "async function pollAutoUpdate()")
	if pollStart < 0 {
		t.Fatal("Auto Update polling boundary is missing")
	}
	pollEnd := strings.Index(string(appPayload[pollStart:]), "\nfunction setView(")
	if pollEnd < 0 {
		t.Fatal("Auto Update polling boundary is missing")
	}
	if strings.Contains(string(appPayload[pollStart:pollStart+pollEnd]), "renderTimeline(") {
		t.Fatal("Auto Update telemetry polling must not rebuild interactive Timeline content")
	}
	if strings.Contains(string(appPayload), "already_knew") || strings.Contains(string(appPayload), "old_info") {
		t.Fatal("retired feedback reasons remain in the active UI")
	}
	if strings.Contains(string(appPayload), "Legacy run") || strings.Contains(string(appPayload), "trigger diagnostics unavailable") {
		t.Fatal("retired pre-trigger diagnostic UI remains in the active bundle")
	}
	if strings.Contains(string(appPayload), "function buildItemActionsMenu") || strings.Contains(string(appPayload), "AI origin correction") {
		t.Fatal("AI correction must stay with the toolbar badge instead of the footer actions menu")
	}
	for _, marker := range []string{"processing-inbox-button", "processing-settings-button"} {
		if !strings.Contains(string(appPayload), marker) {
			t.Fatalf("active-check navigation guidance missing %q", marker)
		}
	}
	for _, marker := range []string{"function reasoningProfileValue", "function syncTimelineSidePanePosition", "reasoningAcquisitionProfile", "reasoningAiDeepProfile", "function recoverInvalidBridgeToken", "invalid_bridge_token", "function runDisabledReason", "function sourceAccessNeedsAttention", "function onboardingReadinessGate", "AKU_BROWSER_SOURCE_PERMISSION_REQUIRED", "Access: permission required", "Grant access", "source-access-setup-button", "function preparedBatchDisabledReason", "No prepared batch is available.", "Finishing capture cleanup", "BOOTSTRAP_TIMEOUT_MS", "function retryBootstrap", "Retry connection", "state.lastUIActivitySentAt = 0", "recordUIActivity(true)", "session.itemCount", "presentation === \"latest\" ? \"prepend\" : \"append\"", "body: { presentation: revealPlacement }", "placement === \"prepend\""} {
		if !strings.Contains(string(appPayload), marker) {
			t.Fatalf("app.js missing runtime reasoning or drawer contract %q", marker)
		}
	}
	for _, marker := range []string{"function bridgeUnavailableReason", "AkuBridge update or reload required", "Waiting for AkuBridge to reconnect"} {
		if !strings.Contains(string(appPayload), marker) {
			t.Fatalf("app.js missing distinct Bridge compatibility guidance %q", marker)
		}
	}
	response, err = client.Get("http://" + address.String() + "/api/inbox?limit=5&offset=0")
	if err != nil {
		t.Fatal(err)
	}
	var inbox struct {
		Sessions []domain.InboxSession `json:"sessions"`
		Total    int                   `json:"total"`
		Limit    int                   `json:"limit"`
	}
	if err := json.NewDecoder(response.Body).Decode(&inbox); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || inbox.Total != 0 || inbox.Limit != 5 || len(inbox.Sessions) != 0 {
		t.Fatalf("inbox=%+v status=%d", inbox, response.StatusCode)
	}
	response, err = client.Get("http://" + address.String() + "/api/inbox/runs/missing/trace?stage=captured")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing run trace status=%d", response.StatusCode)
	}
	response, err = client.Get("http://" + address.String() + "/api/inbox/runs/missing/trace?stage=raw")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid run trace stage status=%d", response.StatusCode)
	}
	correctionRequest, _ := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/inbox/runs/missing/selection-corrections", bytes.NewBufferString(`{"candidateRef":"candidate_missing"}`))
	correctionRequest.Header.Set("Content-Type", "application/json")
	response, err = client.Do(correctionRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing selection correction candidate status=%d", response.StatusCode)
	}
	retryRequest, _ := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/inbox/runs/missing/re-evaluate", nil)
	response, err = client.Do(retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing re-evaluation run status=%d", response.StatusCode)
	}
	response, err = client.Get("http://" + address.String() + "/api/inbox/runs/missing/vision-evaluations")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing vision-evaluation run status=%d", response.StatusCode)
	}
	visionRetryRequest, _ := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/vision-evaluations/missing/retry", nil)
	response, err = client.Do(visionRetryRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing vision-evaluation retry status=%d", response.StatusCode)
	}
	request, err := http.NewRequest(http.MethodPut, "http://"+address.String()+"/api/onboarding", bytes.NewBufferString(`{"activeSources":["x","linkedin"]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var onboardingResponse map[string]any
	if err := json.NewDecoder(response.Body).Decode(&onboardingResponse); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("onboarding status=%d", response.StatusCode)
	}
	calibration := onboardingResponse["calibration"].(map[string]any)
	if calibration["firstRunStatus"] != "pending" || calibration["enabled"] != true || calibration["batchSize"] != float64(10) {
		t.Fatalf("onboarding calibration=%+v", calibration)
	}
	postHeartbeat := func(capabilities domain.BridgeHeartbeat) (int, engine.BridgeStatus) {
		t.Helper()
		heartbeat, _ := json.Marshal(map[string]any{"capabilities": capabilities})
		heartbeatRequest, requestErr := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/bridge/heartbeat", bytes.NewReader(heartbeat))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		heartbeatRequest.Header.Set("Content-Type", "application/json")
		heartbeatRequest.Header.Set("X-Aku-Bridge-Token", bootstrap["bridgeToken"].(string))
		heartbeatRequest.Header.Set("X-Aku-Bridge-Id", "http-test")
		heartbeatRequest.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		heartbeatResponse, requestErr := client.Do(heartbeatRequest)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer heartbeatResponse.Body.Close()
		var heartbeatPayload struct {
			Bridge engine.BridgeStatus `json:"bridge"`
		}
		if err := json.NewDecoder(heartbeatResponse.Body).Decode(&heartbeatPayload); err != nil {
			t.Fatal(err)
		}
		return heartbeatResponse.StatusCode, heartbeatPayload.Bridge
	}
	legacyHeartbeat := engine.ExpectedHeartbeat()
	legacyHeartbeat.ProtocolMajor = 0
	legacyHeartbeat.ProtocolMinor = 0
	legacyHeartbeat.UpdateCapabilities = nil
	legacyJSON, _ := json.Marshal(legacyHeartbeat)
	if bytes.Contains(legacyJSON, []byte("protocolMajor")) || bytes.Contains(legacyJSON, []byte("protocolMinor")) || bytes.Contains(legacyJSON, []byte("updateCapabilities")) {
		t.Fatalf("legacy heartbeat fixture contains additive protocol fields: %s", legacyJSON)
	}
	statusCode, legacyStatus := postHeartbeat(legacyHeartbeat)
	if statusCode != http.StatusAccepted || !legacyStatus.Compatible || legacyStatus.State != "degraded" || legacyStatus.NegotiatedProtocol == nil || !legacyStatus.NegotiatedProtocol.LegacyHeartbeat {
		t.Fatalf("legacy heartbeat status=%d bridge=%+v", statusCode, legacyStatus)
	}
	statusCode, currentStatus := postHeartbeat(engine.ExpectedHeartbeat())
	if statusCode != http.StatusAccepted || !currentStatus.Compatible || currentStatus.State != "healthy" || currentStatus.NegotiatedProtocol == nil || currentStatus.NegotiatedProtocol.LegacyHeartbeat || currentStatus.Actual == nil || len(currentStatus.Actual.UpdateCapabilities) != 4 {
		t.Fatalf("protocol 2 heartbeat status=%d bridge=%+v", statusCode, currentStatus)
	}
	hideSettings := settings
	hideSettings.AIDetectionPresentation = "hide"
	badHidePayload, _ := json.Marshal(map[string]any{"settings": hideSettings, "confirmationPhrase": "wrong"})
	request, _ = http.NewRequest(http.MethodPut, "http://"+address.String()+"/api/settings", bytes.NewReader(badHidePayload))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong AI Hide confirmation status=%d", response.StatusCode)
	}
	goodHidePayload, _ := json.Marshal(map[string]any{"settings": hideSettings, "confirmationPhrase": domain.AIHideConfirmationPhrase})
	request, _ = http.NewRequest(http.MethodPut, "http://"+address.String()+"/api/settings", bytes.NewReader(goodHidePayload))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("confirmed AI Hide status=%d", response.StatusCode)
	}
	request, _ = http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/operations/full-reset", bytes.NewBufferString(`{"confirmation":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong reset confirmation status=%d", response.StatusCode)
	}
	request, _ = http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/operations/full-reset", bytes.NewBufferString(`{"confirmation":"RESET AKUBROWSER"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("full reset status=%d", response.StatusCode)
	}
	var reset map[string]any
	if err := json.NewDecoder(response.Body).Decode(&reset); err != nil {
		t.Fatal(err)
	}
	if reset["operation"] != "full_reset" || reset["onboarding"].(map[string]any)["status"] != "not_started" {
		t.Fatalf("reset=%+v", reset)
	}
	staged, err := state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if staged {
		t.Fatal("full reset must preserve the browser profile")
	}
}

func TestModelUsageEndpointsAndEmbeddedUI(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	logger := log.New(io.Discard, "", 0)
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	address, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	client := http.Client{Timeout: 2 * time.Second}

	response, err := client.Get("http://" + address.String() + "/api/model-usage?windowDays=30")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Usage domain.ModelUsageReport `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || payload.Usage.Scope != "aggregate" || payload.Usage.WindowDays != 30 || payload.Usage.SessionCount != 0 || len(payload.Usage.Categories) != 4 {
		t.Fatalf("usage=%+v status=%d", payload.Usage, response.StatusCode)
	}

	response, err = client.Get("http://" + address.String() + "/api/model-usage?windowDays=8")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid window status=%d", response.StatusCode)
	}
	response, err = client.Get("http://" + address.String() + "/api/inbox/sessions/missing/model-usage")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing session usage status=%d", response.StatusCode)
	}

	for asset, markers := range map[string][]string{
		"web/index.html": {"model-usage-view", "model-usage-window", "model-usage-back"},
		"web/app.js":     {"function buildSessionModelUsage", "function buildModelUsageHelp", "function modelUsageCategoryProfile", "Mixed models", "Configured model pending", "function loadAggregateModelUsage", "Input already includes cached input", "/api/model-usage?windowDays=", "function buildCaptureCandidateTelemetry", "Capture candidates", "Counts are per snapshot observations"},
		"web/styles.css": {".model-usage-help-button", ".model-usage-totals", ".model-usage-category", ".capture-candidate-telemetry", ".capture-candidate-snapshots"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedRelayRetriesAfterCaptureLaneContention(t *testing.T) {
	payload, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"dispatchRetryAfter", "expectedLaneWait", "No queued browser command was available"} {
		if !strings.Contains(string(payload), marker) {
			t.Fatalf("embedded relay is missing %q", marker)
		}
	}
}

func TestEmbeddedBridgePingAdvertisesBoundedProtocol(t *testing.T) {
	payload, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`type: "AKU_BROWSER_BRIDGE_PING"`, "protocolMajor: 2", "protocolMinor: 0"} {
		if !strings.Contains(string(payload), marker) {
			t.Fatalf("embedded Bridge ping is missing %q", marker)
		}
	}
}

func TestEmbeddedContinuousBackgroundSettingsExposeIntervalConditionally(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"Continuous background", "continuous-background-interval-row", "Continuous interval", "A skipped tick waits for the next interval.", "15 minutes — recommended", "1 hour", "Adaptive demand learns batch consumption pace"},
		"web/app.js":     {"function syncAutoUpdateModeSettings", `value !== "fixed"`, "status.cadenceTier", "settings.autoUpdateRefillMinutes || 15"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing continuous scheduler contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedSchedulerStatusUsesScannableSummary(t *testing.T) {
	indexHTML, err := embeddedAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(indexHTML)
	backgroundPolicy := strings.Index(indexText, `for="auto-update-mode"`)
	schedulerStatus := strings.Index(indexText, `class="settings-row auto-update-status-row"`)
	preparedQueue := strings.Index(indexText, `for="prepared-batch-limit"`)
	if backgroundPolicy < 0 || schedulerStatus < 0 || preparedQueue < 0 || !(backgroundPolicy < preparedQueue && preparedQueue < schedulerStatus) {
		t.Fatalf("scheduler status must appear below prepared batch queue: background=%d queue=%d scheduler=%d", backgroundPolicy, preparedQueue, schedulerStatus)
	}

	for asset, markers := range map[string][]string{
		"web/index.html": {"auto-update-mode-badge", "auto-update-mode-notice", "auto-update-state-badge", "auto-update-metrics", "auto-update-prepared-metric", "auto-update-runway-metric", "auto-update-result-metric", "auto-update-pressure-metric", "auto-update-allowance-metric", "auto-update-next-metric", "Technical details"},
		"web/styles.css": {".auto-update-status-row", ".auto-update-mode-pill", ".auto-update-mode-notice", ".auto-update-state-pill", ".auto-update-metrics", ".auto-update-diagnostics", "@media (max-width: 460px)"},
		"web/app.js":     {"function renderAutoUpdateStatus", "function autoUpdateModeLabel", "function autoUpdateModeDescription", "unsavedModeChange", "applyingModeChange", "function formatSchedulerMoment", "stateBadge.className", "preparedMetric.textContent", "runwayMetric.textContent", "resultMetric.textContent", "pressureMetric.textContent", "allowanceMetric.textContent", "nextMetric.textContent", `diagnostics.join(" · ")`},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing scheduler summary contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedSettingsDirtyStateContract(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html":              {"settings-dirty-indicator", "Unsaved changes", "save-runtime-settings"},
		"web/app.js":                  {`import { createDirtyStateTracker } from "./settings-dirty-state.js"`, "const settingsDirty = createDirtyStateTracker", "function readSettingsDraft", "settingsDirty.setBaseline", "beforeunload", "settingsForm.addEventListener(\"input\"", "settingsForm.addEventListener(\"change\""},
		"web/settings-dirty-state.js": {"export function createDirtyStateTracker", "setBaseline", "isDirty"},
		"web/styles.css":              {".settings-dirty-indicator"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing settings dirty-state contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedCaptureSurfaceReleaseBarrierContract(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/app.js":                             {`import { releaseCompletedSourceSurfaces } from "./capture-surface-release-barrier.js"`, "const captureSurfaceBarrier = await releaseCompletedSourceSurfaces", "captureSurfaceBarrier.ready"},
		"web/capture-surface-release-barrier.js": {"export async function releaseCompletedSourceSurfaces", "await releaseSource(session.id, run.source)", "ready: false", "releasedSources.delete(key)"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing capture-surface release barrier contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedUsageLimitPauseRequiresExplicitRestoreConfirmation(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"confirm-codex-usage-restored", "Confirm Codex usage restored"},
		"web/app.js":     {"usage_limit_paused", "Auto Update paused by Codex usage limit", "Automatic checks will not retry until you confirm", "/api/auto-update/usage-limit/restore", "function confirmCodexUsageRestored", "function handleAutoUpdateTimelineAction", "Confirm usage restored", "The Codex provider pause is unchanged"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing usage-limit pause contract %q", asset, marker)
			}
		}
	}
}

func TestEmbeddedInboxDistinguishesActiveEvaluationFromFinalZero(t *testing.T) {
	payload, err := embeddedAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"function inboxSessionFlowText", "function inboxRunMetricText", "Skipped unchanged", "inboxRunCaptureReliabilityNotice", `Evaluating\u2026`, "composition pending"} {
		if !strings.Contains(string(payload), marker) {
			t.Fatalf("embedded Inbox progress is missing %q", marker)
		}
	}
}

func TestEmbeddedWindowsSecurityFailureOffersManualBundleRecovery(t *testing.T) {
	for asset, markers := range map[string][]string{
		"web/index.html": {"failure-runtime-recovery", "Download manual Windows bundle", "Do not run installed and portable runtimes together"},
		"web/app.js":     {"function isWindowsSecurityRuntimeFailure", "function matchingWindowsPortableBundleURL", "WINDOWS SECURITY BLOCKED RUNTIME", "Repeating Update now without restarting AkuSidecar will fail again"},
		"web/styles.css": {".failure-runtime-recovery", ".failure-download-link"},
	} {
		contents, err := embeddedAssets.ReadFile(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(contents), marker) {
				t.Fatalf("%s missing %q", asset, marker)
			}
		}
	}
}

func TestLoopbackBoundaryRejectsForeignHostsAndOrigins(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Bridge: config.BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://abcdefghijklmnop/"}}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		method      string
		path        string
		host        string
		origin      string
		contentType string
		want        int
	}{
		{name: "foreign host", method: http.MethodGet, path: "/api/health", host: "attacker.example", want: http.StatusForbidden},
		{name: "foreign browser origin", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "https://attacker.example", contentType: "application/json", want: http.StatusForbidden},
		{name: "extension cannot call UI mutation", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "chrome-extension://abcdefghijklmnop", contentType: "application/json", want: http.StatusForbidden},
		{name: "same origin UI reaches route", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "http://127.0.0.1:11122", contentType: "application/json", want: http.StatusOK},
		{name: "localhost alias reaches route", method: http.MethodPut, path: "/api/onboarding", host: "localhost:11122", origin: "http://localhost:11122", contentType: "application/json", want: http.StatusOK},
		{name: "extension reaches bridge authentication", method: http.MethodPost, path: "/api/bridge/heartbeat", host: "localhost:11122", origin: "chrome-extension://abcdefghijklmnop", contentType: "application/json", want: http.StatusUnauthorized},
		{name: "different extension cannot reach bridge", method: http.MethodPost, path: "/api/bridge/heartbeat", host: "localhost:11122", origin: "chrome-extension://ponmlkjihgfedcba", contentType: "application/json", want: http.StatusForbidden},
		{name: "JSON mutation rejects text content", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "http://127.0.0.1:11122", contentType: "text/plain", want: http.StatusUnsupportedMediaType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := ""
			if test.method == http.MethodPut {
				body = `{"activeSources":["x","linkedin"]}`
			} else if test.method == http.MethodPost {
				body = `{}`
			}
			request := httptest.NewRequest(test.method, "http://"+test.host+test.path, strings.NewReader(body))
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			server.http.Handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestStopClosesActiveHTTPConnectionsAfterDrainDeadline(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fixture := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	fixture.Start()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		fixture.Close()
	})
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		_, _ = fixture.Client().Get(fixture.URL)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("fixture request did not become active")
	}
	server := &Server{http: fixture.Config, listener: fixture.Listener}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := server.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("HTTP stop exceeded bounded fallback: %s", elapsed)
	}
	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("active request did not leave after connection close")
	}
}

func TestBridgeV82ObservationShapeDecodesStrictly(t *testing.T) {
	raw := `{
		"source":"x","pageUrl":"https://x.com/home","pageTitle":"Home","capturedAt":"2026-07-15T00:00:00Z",
		"snapshots":[{
			"index":0,"adapterVersion":"x-dom-v21","selectorStrategy":"article","selectorCounts":{"article":1},
			"selectorCandidateCount":1,"structuralCandidateCount":1,"visibleContainerCount":1,"capturedAt":"2026-07-15T00:00:00Z",
			"candidateDiagnostics":{"structuralCandidates":3,"eligibleCandidates":2,"visibleEligibleCandidates":1,"actionAnchoredCandidates":1,"admittedReasons":{"post_action_cluster":2},"rejectedReasons":{"no_stable_post_identity":1}},
			"scrollY":0,"viewportHeight":900,"newCandidateCount":1,
			"blocks":[{
				"text":"Material source update","author":"author","avatarUrl":null,"publishedAt":null,
				"permalink":"https://x.com/author/status/1","platformId":"1","contentKind":"post",
				"relationshipType":"original","parentPermalink":null,"quotedPost":null,"engagement":{},
				"presentation":{},"media":[],"links":[],"mediaRecovery":{},"captureQuality":{},"feedPosition":1
			}],"qualityReports":[]
		}],"coverage":{"browserAdapter":"aku-bridge"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/bridge/commands/example/observation", bytes.NewBufferString(raw))
	request.Header.Set("Content-Type", "application/json")
	var observation domain.Observation
	if err := readJSON(request, &observation); err != nil {
		t.Fatalf("v82 observation must satisfy the strict Go shape: %v", err)
	}
	if observation.Snapshots[0].StructuralCandidateCount != 1 ||
		observation.Snapshots[0].CandidateDiagnostics == nil ||
		observation.Snapshots[0].CandidateDiagnostics.RejectedReasons["no_stable_post_identity"] != 1 ||
		observation.Snapshots[0].Blocks[0].PlatformID != "1" {
		t.Fatalf("observation=%+v", observation)
	}
}

func TestPassiveMediaEvidenceEndpointRequiresBridgeAuthentication(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	logger := log.New(io.Discard, "", 0)
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"candidateId":"x:status:12345","media":[{"kind":"image","url":"https://pbs.twimg.com/media/example.jpg"}],"provenance":"passive_x_cache"}`

	request := httptest.NewRequest(http.MethodPost, "/api/bridge/timeline/missing/media-evidence", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", response.Code, response.Body.String())
	}

	token, err := state.BridgeToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/bridge/timeline/missing/media-evidence", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Bridge-Token", token)
	request.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	request.Header.Set("X-Aku-Bridge-Id", "http-passive-test")
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("authenticated status=%d body=%s", response.Code, response.Body.String())
	}

	textBearingBody := `{"candidateId":"x:status:12345","media":[{"kind":"image","url":"https://pbs.twimg.com/media/example.jpg","alt":"post text must not cross this contract"}],"provenance":"passive_x_cache"}`
	request = httptest.NewRequest(http.MethodPost, "/api/bridge/timeline/missing/media-evidence", strings.NewReader(textBearingBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Bridge-Token", token)
	request.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("text-bearing media status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFirstRunHTTPFlowEndsInForcedCalibration(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Capture: config.CaptureConfig{Profile: "expanded", Visibility: "quiet", OpenMissingSource: true, MaxAcquisitionRounds: 1}, Preference: config.PreferenceConfig{Mode: "promote_unused_budget"}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(engine.ExpectedHeartbeat())
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	address, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	origin := "http://" + address.String()
	client := http.Client{Timeout: 2 * time.Second}

	var onboarding map[string]any
	requestJSON(t, client, http.MethodPut, origin+"/api/onboarding", `{"activeSources":["x","linkedin"]}`, &onboarding)
	if onboarding["calibration"].(map[string]any)["firstRunStatus"] != "pending" {
		t.Fatalf("onboarding=%+v", onboarding)
	}
	var started struct {
		Session domain.Session `json:"session"`
	}
	requestJSON(t, client, http.MethodPost, origin+"/api/updates", `{"intent":"What changed?"}`, &started)
	if started.Session.Trigger != domain.UpdateTriggerOnboarding || started.Session.Delivery != domain.UpdateDeliveryVisible || started.Session.BudgetAuthority != domain.BudgetAuthorityUser {
		t.Fatalf("onboarding update policy=%+v", started.Session)
	}
	completeHTTPTestRun(t, runtime, started.Session.ID, domain.SourceX, "x-http-1")
	completeHTTPTestRun(t, runtime, started.Session.ID, domain.SourceLinkedIn, "linkedin-http-1")
	waitHTTPTestSession(t, runtime, started.Session.ID, "completed")

	var bootstrapWithCalibration struct {
		Calibration domain.CalibrationOverview `json:"calibration"`
	}
	requestJSON(t, client, http.MethodGet, origin+"/api/bootstrap", "", &bootstrapWithCalibration)
	if bootstrapWithCalibration.Calibration.Active == nil || bootstrapWithCalibration.Calibration.Active.Status != "reviewing" || bootstrapWithCalibration.Calibration.Active.SampleCount != 2 {
		t.Fatalf("automatic calibration=%+v", bootstrapWithCalibration.Calibration)
	}
	created := *bootstrapWithCalibration.Calibration.Active
	var decided struct {
		Calibration domain.CalibrationSession `json:"calibration"`
	}
	requestJSON(t, client, http.MethodPut, origin+"/api/calibration/sessions/"+created.ID+"/samples/0", `{"label":"more_like_this"}`, &decided)
	requestJSON(t, client, http.MethodPut, origin+"/api/calibration/sessions/"+created.ID+"/samples/1", `{"label":"neutral"}`, &decided)
	if decided.Calibration.Status != "completed" || decided.Calibration.Snapshot == nil {
		t.Fatalf("completed calibration=%+v", decided.Calibration)
	}

	var bootstrap map[string]any
	requestJSON(t, client, http.MethodGet, origin+"/api/bootstrap", "", &bootstrap)
	calibration := bootstrap["calibration"].(map[string]any)
	if calibration["firstRunStatus"] != "completed" || calibration["active"] != nil {
		t.Fatalf("bootstrap calibration=%+v", calibration)
	}
}

func requestJSON(t *testing.T, client http.Client, method, url, body string, target any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s status=%d body=%s", method, url, response.StatusCode, payload)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func completeHTTPTestRun(t *testing.T, runtime *engine.Engine, sessionID string, source domain.Source, platformID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, err := runtime.Session(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range session.Runs {
			if run.Source != source || run.Status != "waiting_for_bridge" {
				continue
			}
			command, err := runtime.ClaimCommand(context.Background(), run.ID, "http-flow-test")
			if err != nil || command == nil {
				t.Fatalf("claim command=%+v err=%v", command, err)
			}
			permalink := "https://x.com/example/status/" + httpFixtureXStatusID(platformID)
			if source == domain.SourceLinkedIn {
				permalink = "https://www.linkedin.com/feed/update/urn:li:activity:" + platformID
			}
			observation := domain.Observation{Source: source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: platformID, Text: "Material source update", Author: "author", Permalink: permalink, FeedPosition: 1}}}}, Coverage: map[string]any{"quality": "complete"}}
			if _, err := runtime.AcceptObservation(context.Background(), command.ID, run.ID, observation); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s run did not become ready", source)
}

func httpFixtureXStatusID(value string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return strconv.FormatUint(hash.Sum64(), 10)
}

func waitHTTPTestSession(t *testing.T, runtime *engine.Engine, sessionID, status string) domain.Session {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, err := runtime.Session(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if session.Status == status {
			return session
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, _ := runtime.Session(context.Background(), sessionID)
	t.Fatalf("session status=%s, wanted %s", session.Status, status)
	return domain.Session{}
}
