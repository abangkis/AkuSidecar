//go:build windows

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	facebookReelBridgeBaselineURL = "https://www.facebook.com/reel/1639349817600268/"
	facebookReelBaselineOptIn     = "AKU_FACEBOOK_REEL_BRIDGE_BASELINE_SMOKE"
	facebookReelForegroundOptIn   = "AKU_FACEBOOK_REEL_BRIDGE_FOREGROUND_APPROVED"
)

// This opt-in baseline uses the registered authenticated profile and the
// normal AkuBrowser page-to-Bridge handshake. It queues one bounded native
// target media-recapture command only after live Bridge source readiness.
func TestFacebookReelBridgeForegroundMediaRecaptureBaselineWindows(t *testing.T) {
	if os.Getenv(facebookReelBaselineOptIn) != "1" {
		t.Skip("set the operator-only Facebook Reel baseline opt-in through its wrapper")
	}
	if os.Getenv(facebookReelForegroundOptIn) != "1" {
		t.Fatal("foreground capture requires the wrapper's explicit foreground-baseline approval")
	}
	profilePath := os.Getenv("AKU_FACEBOOK_REEL_BRIDGE_PROFILE")
	chromePath := os.Getenv("AKU_FACEBOOK_REEL_BRIDGE_CHROME")
	bridgePath := os.Getenv("AKU_FACEBOOK_REEL_BRIDGE_SOURCE")
	wantOrigin := os.Getenv("AKU_FACEBOOK_REEL_BRIDGE_ORIGIN")
	receiptDir := os.Getenv("AKU_FACEBOOK_REEL_BRIDGE_RECEIPT_DIR")
	if profilePath == "" || chromePath == "" || bridgePath == "" || wantOrigin == "" || receiptDir == "" {
		t.Fatal("registered profile, Chrome, source Bridge, derived origin, and private receipt directory are required")
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve project-local baseline directory")
	}
	sidecarRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	workspaceRoot := filepath.Dir(sidecarRoot)
	profilePath = mustRealPath(t, profilePath)
	chromePath = mustRealPath(t, chromePath)
	bridgePath = mustRealPath(t, bridgePath)
	profileRoot := mustRealPath(t, filepath.Join(sidecarRoot, "runtime"))
	profileChild, err := filepath.Rel(profileRoot, profilePath)
	if err != nil || profileChild == "." || profileChild == ".." || strings.HasPrefix(profileChild, ".."+string(filepath.Separator)) || filepath.IsAbs(profileChild) {
		t.Fatal("registered capture profile is outside AkuSidecar/runtime")
	}
	expectedBridgePath := mustRealPath(t, filepath.Join(workspaceRoot, "AkuBridge"))
	if !sameWindowsPath(bridgePath, expectedBridgePath) {
		t.Fatal("baseline must use the original source-tree AkuBridge")
	}
	origin, err := bridgeExtensionOrigin(bridgePath)
	if err != nil || !sameExtensionOrigin(origin, wantOrigin) {
		t.Fatal("configured Bridge origin does not match the source-tree manifest identity")
	}
	if strings.ToLower(filepath.Base(chromePath)) != "chrome.exe" {
		t.Fatal("registered Chromium executable is not chrome.exe")
	}
	profileDirectory, err := appshell.ResolveProfileDirectory(profilePath)
	if err != nil {
		t.Fatal("resolve the registered capture profile slot", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 210*time.Second)
	defer cancel()
	probe, err := net.Listen("tcp", "127.0.0.1:11122")
	if err != nil {
		t.Fatal("127.0.0.1:11122 is occupied; refusing to launch Chrome")
	}
	_ = probe.Close()

	artifactRoot := filepath.Join(sidecarRoot, "build", "facebook-reel-bridge-baseline")
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("create project-local baseline directory")
	}
	receiptDir = mustRealPath(t, receiptDir)
	receiptChild, err := filepath.Rel(mustRealPath(t, artifactRoot), receiptDir)
	if err != nil || receiptChild == "." || receiptChild == ".." || strings.HasPrefix(receiptChild, ".."+string(filepath.Separator)) || filepath.IsAbs(receiptChild) {
		t.Fatal("private baseline receipt directory must be a child of the project-local baseline directory")
	}
	writePrivateJSON(t, receiptDir, "target-metadata.json", map[string]any{
		"scope":                   "facebook_native_target_media_recapture_foreground_baseline",
		"targetUrl":               facebookReelBridgeBaselineURL,
		"source":                  string(domain.SourceFacebook),
		"mode":                    "recapture_media",
		"captureVisibilityPolicy": "adaptive_fidelity",
		"foregroundAuthorized":    true,
		"foregroundApprovalOptIn": true,
		"profileDirectory":        profileDirectory,
		"browserLifecycle":        "ordinary_owned_foreground_test_ui",
		"bounds": map[string]any{
			"scrolls": 0, "maxBlocksPerSnapshot": 1, "maxBlockCharacters": 4000,
			"qualityRetryBudget": 0, "maxAcquisitionRounds": 1,
		},
		"rawObservationContainsSignedMediaUrls": true,
	})
	workDir, err := os.MkdirTemp(artifactRoot, "run-")
	if err != nil {
		t.Fatal("create isolated baseline workspace")
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ActiveSources = []domain.Source{domain.SourceFacebook}
	settings.CaptureVisibility = "adaptive_fidelity"
	settings.AIDetectionEnabled = false
	state, err := store.Open(filepath.Join(workDir, "baseline.db"), settings)
	if err != nil {
		t.Fatal("open isolated baseline database")
	}
	if err := state.SaveSettings(ctx, settings); err != nil {
		_ = state.Close()
		t.Fatal("configure isolated baseline database")
	}
	if _, err := state.CompleteOnboarding(ctx, []domain.Source{domain.SourceFacebook}); err != nil {
		_ = state.Close()
		t.Fatal("complete onboarding in isolated baseline database")
	}

	logger := log.New(io.Discard, "", 0)
	cfg := config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 11122},
		Bridge: config.BridgeConfig{TrustedExtensionOrigins: []string{origin}},
	}
	e := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	s, err := New(cfg, state, e, logger)
	if err != nil {
		_ = state.Close()
		t.Fatal("create isolated baseline server")
	}
	serverStopped := false
	t.Cleanup(func() {
		if !serverStopped {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			if err := s.Stop(stopCtx); err != nil {
				t.Errorf("stop isolated baseline server: %v", err)
			}
		}
		if err := state.Close(); err != nil {
			t.Errorf("close isolated baseline database: %v", err)
		}
	})
	address, err := s.Start()
	if err != nil || address.String() != "127.0.0.1:11122" {
		t.Fatal("bind the isolated server to 127.0.0.1:11122 before Chrome launch")
	}

	chromium, err := appshell.Discover(ctx, chromePath)
	if err != nil || !strings.HasPrefix(chromium.Version, "154.") {
		writePrivateJSON(t, receiptDir, "version-failure.json", map[string]any{"status": chromium.Status, "version": chromium.Version})
		t.Fatal("registered Chrome executable must report version 154.x")
	}
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{
		Executable: chromePath, ExtensionPath: bridgePath, UserDataDir: profilePath,
		URL: "http://127.0.0.1:11122/", StartMinimized: false, PrivateCDP: false,
		ExtraArgs: []string{"--profile-directory=" + profileDirectory},
	})
	if err != nil {
		writePrivateJSON(t, receiptDir, "launch-failure.json", map[string]any{"error": err.Error()})
		t.Fatal("launch registered Chrome/profile failed; see private launch-failure.json")
	}
	browserClosed := false
	t.Cleanup(func() {
		if browserClosed {
			return
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer closeCancel()
		if err := window.CloseForRetry(closeCtx); err != nil {
			t.Errorf("Chrome ownership drain was not verified: %v", err)
		}
	})
	t.Logf("registered_chrome=Chrome/%s source=original_dev_bridge version_evidence=registered_executable", chromium.Version)

	if err := waitForFacebookBridgeSourceReady(ctx, e, origin, 90*time.Second); err != nil {
		t.Fatal(err)
	}
	if active, err := state.ActiveSession(ctx); err != nil || active != nil {
		t.Fatal("isolated baseline unexpectedly has an active session")
	}

	settings, err = state.GetSettings(ctx)
	if err != nil {
		t.Fatal("read isolated settings")
	}
	policy := domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible,
		BudgetAuthority: domain.BudgetAuthorityUser,
	}
	session, err := state.CreateUpdateSession(ctx, "Bounded Facebook native-target baseline", settings, policy)
	if err != nil {
		t.Fatal("create isolated target session")
	}
	run, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || run == nil || run.Source != domain.SourceFacebook {
		t.Fatal("create one Facebook target run")
	}
	payload := map[string]any{
		"mode": "recapture_media", "source": domain.SourceFacebook,
		"targetUrl":      facebookReelBridgeBaselineURL,
		"browserAdapter": "aku-bridge", "captureVisibilityPolicy": "adaptive_fidelity",
		"sourceHydrationTimeoutMs": 30000, "captureTimeoutMs": 45000,
		"pendingContentPolicy": "detect_only", "pendingContentTimeoutMs": 500,
		"pendingContentSettleMs": 100, "sourceFreshnessPolicy": "preserve_target",
		"scrolls": 0, "scrollFraction": 0.75, "scrollSettleMs": 100,
		"sameTabMutationAllowed": false, "openIfMissing": true,
		"captureLeaseId": session.ID, "maxBlocksPerSnapshot": 1,
		"maxBlockCharacters": 4000, "qualityReportRequired": true,
		"qualityRetryBudget": 0, "acquisitionRound": 1, "maxAcquisitionRounds": 1,
		"foregroundAuthorized": true,
		"tabLifecycle":         map[string]any{"ownership": "managed", "openedTabDisposition": "close_after_capture"},
		"restoreScroll":        true,
	}
	command, err := state.StartRun(ctx, run.ID, payload)
	if err != nil {
		t.Fatal("queue the bounded collect_visible target command")
	}
	storedPayload, err := state.CaptureCommandPayload(ctx, command.ID, run.ID)
	if err != nil || storedPayload["targetUrl"] != facebookReelBridgeBaselineURL ||
		storedPayload["source"] != string(domain.SourceFacebook) ||
		storedPayload["mode"] != "recapture_media" ||
		storedPayload["captureVisibilityPolicy"] != "adaptive_fidelity" ||
		storedPayload["foregroundAuthorized"] != true ||
		!payloadNumberEquals(storedPayload, "scrolls", 0) ||
		!payloadNumberEquals(storedPayload, "maxBlocksPerSnapshot", 1) ||
		!payloadNumberEquals(storedPayload, "maxBlockCharacters", 4000) ||
		!payloadNumberEquals(storedPayload, "qualityRetryBudget", 0) ||
		!payloadNumberEquals(storedPayload, "maxAcquisitionRounds", 1) {
		t.Fatal("queued command does not preserve the approved foreground Reel policy and exact capture bounds")
	}
	writePrivateJSON(t, receiptDir, "run-metadata.json", map[string]any{
		"sessionId":               session.ID,
		"runId":                   run.ID,
		"commandId":               command.ID,
		"commandType":             command.Type,
		"mode":                    storedPayload["mode"],
		"targetUrl":               storedPayload["targetUrl"],
		"source":                  storedPayload["source"],
		"captureVisibilityPolicy": storedPayload["captureVisibilityPolicy"],
		"foregroundAuthorized":    storedPayload["foregroundAuthorized"],
		"bounds": map[string]any{
			"scrolls":              storedPayload["scrolls"],
			"maxBlocksPerSnapshot": storedPayload["maxBlocksPerSnapshot"],
			"maxBlockCharacters":   storedPayload["maxBlockCharacters"],
			"qualityRetryBudget":   storedPayload["qualityRetryBudget"],
			"maxAcquisitionRounds": storedPayload["maxAcquisitionRounds"],
		},
	})

	deadline := time.NewTimer(105 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var observations []domain.Observation
	for {
		observations, err = state.Observations(ctx, run.ID)
		if err != nil {
			t.Fatal("read baseline observation")
		}
		current, getErr := state.GetRun(ctx, run.ID)
		if getErr != nil {
			t.Fatal("read target run status")
		}
		if len(observations) > 0 {
			break
		}
		if current.BridgeCommandStatus == "failed" {
			writePrivateJSON(t, receiptDir, "capture-failure.json", map[string]any{
				"runId": run.ID, "status": current.Status,
				"bridgeCommandStatus": current.BridgeCommandStatus, "error": current.Error,
			})
			code := "unknown"
			if current.Error != nil && current.Error.Code != "" {
				code = current.Error.Code
			}
			t.Fatalf("normal Bridge background dispatch failed before observation (code=%s)", code)
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for normal Bridge background command polling")
		case <-deadline.C:
			t.Fatal("no background poll claimed the queued target command within 105 seconds")
		case <-tick.C:
		}
	}
	if len(observations) != 1 {
		t.Fatalf("expected one target observation, got %d", len(observations))
	}
	observation := observations[0]
	writePrivateJSON(t, receiptDir, "raw-observation.json", observation)
	if observation.Source != domain.SourceFacebook {
		t.Fatal("observation source changed during Bridge transport")
	}
	if parsed, parseErr := url.Parse(observation.PageURL); parseErr != nil ||
		(parsed.Host != "www.facebook.com" && parsed.Host != "facebook.com") ||
		!strings.Contains(parsed.EscapedPath(), "/reel/1639349817600268") {
		t.Fatalf("observation did not remain on the requested Facebook Reel: %s", boundedURLPath(observation.PageURL))
	}
	blocks := 0
	selectorCandidates := 0
	structuralCandidates := 0
	visibleContainers := 0
	eligibleCandidates := 0
	mediaRefs := 0
	mediaKinds := map[string]int{}
	captionMatch := false
	qualityVerdicts := map[string]int{}
	qualityIssues := map[string]int{}
	qualityAttempts := 0
	for _, snapshot := range observation.Snapshots {
		blocks += len(snapshot.Blocks)
		selectorCandidates += snapshot.SelectorCandidateCount
		structuralCandidates += snapshot.StructuralCandidateCount
		visibleContainers += snapshot.VisibleContainerCount
		if snapshot.CandidateDiagnostics != nil {
			eligibleCandidates += snapshot.CandidateDiagnostics.EligibleCandidates
		}
		for _, report := range snapshot.QualityReports {
			if verdict, ok := report["verdict"].(string); ok && verdict != "" {
				qualityVerdicts[verdict]++
			}
			if attempt, ok := report["attempt"].(float64); ok {
				qualityAttempts += int(attempt)
			}
			if issues, ok := report["issues"].([]any); ok {
				for _, rawIssue := range issues {
					issue, ok := rawIssue.(map[string]any)
					if !ok {
						continue
					}
					field, _ := issue["field"].(string)
					code, _ := issue["code"].(string)
					if field != "" && code != "" {
						qualityIssues[field+":"+code]++
					}
				}
			}
		}
		for _, block := range snapshot.Blocks {
			mediaRefs += len(block.Media)
			for _, media := range block.Media {
				if kind, ok := media["kind"].(string); ok && kind != "" {
					mediaKinds[kind]++
				}
			}
			if strings.Contains(strings.ToLower(block.Text), "physics girl") {
				captionMatch = true
			}
		}
	}
	qualitySummary := map[string]any{
		"reportCount":   totalQualityReports(observation.Snapshots),
		"verdictCounts": qualityVerdicts,
		"issueCounts":   qualityIssues,
		"retryAttempts": qualityAttempts,
	}
	summary := map[string]any{
		"scope":                   "facebook_native_target_media_recapture_foreground_baseline",
		"source":                  string(observation.Source),
		"observedPagePath":        boundedURLPath(observation.PageURL),
		"pageMatchedTarget":       true,
		"snapshotCount":           len(observation.Snapshots),
		"selectorCandidates":      selectorCandidates,
		"structuralCandidates":    structuralCandidates,
		"eligibleCandidates":      eligibleCandidates,
		"visibleContainers":       visibleContainers,
		"blockCount":              blocks,
		"mediaReferenceCount":     mediaRefs,
		"mediaKindCounts":         mediaKinds,
		"captionMatchPhysicsGirl": captionMatch,
		"quality":                 qualitySummary,
		"rawObservationFile":      "raw-observation.json",
	}
	writePrivateJSON(t, receiptDir, "sanitized-summary.json", summary)
	t.Logf("foreground_target_capture source=facebook page_path=%s snapshots=%d selector_candidates=%d structural_candidates=%d eligible_candidates=%d visible_containers=%d blocks=%d media_refs=%d media_kinds=%v caption_match_physics_girl=%t quality_reports=%d quality_verdicts=%v quality_issues=%v quality_retry_attempts=%d",
		boundedURLPath(observation.PageURL), len(observation.Snapshots), selectorCandidates, structuralCandidates,
		eligibleCandidates, visibleContainers, blocks, mediaRefs, mediaKinds, captionMatch,
		qualitySummary["reportCount"], qualityVerdicts, qualityIssues, qualityAttempts)

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 20*time.Second)
	err = window.CloseForRetry(closeCtx)
	closeCancel()
	if err != nil {
		t.Fatal("Chrome ownership drain was not verified", err)
	}
	browserClosed = true
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal("stop isolated baseline server", err)
	}
	serverStopped = true
}

func waitForFacebookBridgeSourceReady(ctx context.Context, e *engine.Engine, expectedOrigin string, timeout time.Duration) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	last := "no compatible heartbeat from the expected source-tree Bridge origin"
	for {
		status := e.BridgeStatus()
		if status.Compatible && status.Actual != nil && sameExtensionOrigin(status.Actual.ExtensionOrigin, expectedOrigin) {
			last = "Facebook source readiness was omitted from the heartbeat"
			for _, source := range status.Actual.SourceAccess.Sources {
				if source.Source != string(domain.SourceFacebook) {
					continue
				}
				if source.PermissionGranted && source.ScriptRegistered && source.Ready {
					return nil
				}
				last = fmt.Sprintf("Facebook readiness: permissionGranted=%t scriptRegistered=%t ready=%t reason=%q",
					source.PermissionGranted, source.ScriptRegistered, source.Ready, source.Reason)
				break
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for the normal UI Bridge heartbeat and Facebook readiness; last observation: %s", last)
		case <-deadline.C:
			return fmt.Errorf("timed out after %s waiting for genuine Facebook Bridge readiness; last observation: %s", timeout, last)
		case <-tick.C:
		}
	}
}

func writePrivateJSON(t *testing.T, dir, name string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode private baseline artifact %s: %v", name, err)
	}
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatalf("create private baseline artifact %s: %v", name, err)
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		_ = file.Close()
		t.Fatalf("write private baseline artifact %s: %v", name, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close private baseline artifact %s: %v", name, err)
	}
}

func totalQualityReports(snapshots []domain.Snapshot) int {
	total := 0
	for _, snapshot := range snapshots {
		total += len(snapshot.QualityReports)
	}
	return total
}

func payloadNumberEquals(payload map[string]any, key string, expected int) bool {
	switch value := payload[key].(type) {
	case float64:
		return value == float64(expected)
	case int:
		return value == expected
	case int64:
		return value == int64(expected)
	default:
		return false
	}
}

func mustRealPath(t *testing.T, path string) string {
	t.Helper()
	value, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve required baseline path: %v", err)
	}
	return filepath.Clean(value)
}

func sameWindowsPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func sameExtensionOrigin(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(a), "/"), strings.TrimSuffix(strings.TrimSpace(b), "/"))
}

func boundedURLPath(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "www.facebook.com" && parsed.Host != "facebook.com") {
		return "unverified"
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if len(path) > 120 {
		return path[:120]
	}
	return path
}
