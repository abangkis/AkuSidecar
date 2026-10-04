//go:build windows

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestAuthenticatedHybridFacebookRecaptureWindowsSmoke(t *testing.T) {
	if os.Getenv("AKU_AUTH_JOURNEY_ACK_FACEBOOK_RECAPTURE") != "1" {
		t.Skip("fresh Facebook Update and automatic Recapture require explicit operator acknowledgement")
	}
	runAuthenticatedSourceFixture(t, false, true)
}

// runAuthenticatedFacebookRecapture first acquires fresh Facebook evidence
// through one saved-headless Update. It never manufactures missing-media
// state: if no newly-acquired supported target qualifies, the API is not used.
func runAuthenticatedFacebookRecapture(t *testing.T, ctx context.Context, state *store.Store, e *engine.Engine, coordinator *collection.Coordinator, manager *captureruntime.Manager) {
	t.Helper()
	e.StartHeadlessCollection(ctx)
	settings, err := state.GetSettings(ctx)
	if err != nil || settings.CollectionMode != "headless" || len(settings.ActiveSources) != 1 || settings.ActiveSources[0] != domain.SourceFacebook {
		t.Fatal("Facebook Recapture fixture did not retain the saved single-source headless selection")
	}
	if _, err := e.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal("isolated Facebook onboarding could not be completed after existing grants were verified")
	}
	settingsBefore, err := state.GetSettings(ctx)
	if err != nil || settingsBefore.CollectionMode != "headless" || len(settingsBefore.ActiveSources) != 1 || settingsBefore.ActiveSources[0] != domain.SourceFacebook {
		t.Fatal("Facebook onboarding changed its isolated saved collection selection")
	}
	acquisitionCtx, acquisitionCancel := context.WithTimeout(ctx, 6*time.Minute)
	defer acquisitionCancel()
	session, err := e.StartVisibleUpdate(acquisitionCtx, "bounded fresh Facebook evidence for automatic Recapture qualification")
	if err != nil {
		t.Fatalf("fresh Facebook Update admission was rejected: %v", err)
	}
	if len(session.Runs) != 1 || session.Runs[0].Source != domain.SourceFacebook {
		t.Fatal("fresh Facebook Update did not admit exactly one Facebook run")
	}
	runID := session.Runs[0].ID
	browserConfigured := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		owner := manager.Snapshot()
		if !browserConfigured && owner.State == captureruntime.Ready && owner.Driver == "browser" {
			if owner.Generation != 3 {
				t.Fatal("fresh Facebook Update did not enter its expected Browser fallback generation")
			}
			configureAuthenticatedHybridBridge(t, acquisitionCtx, state, e)
			browserConfigured = true
		}
		session, err = state.GetSession(acquisitionCtx, session.ID)
		if err != nil {
			t.Fatal("fresh Facebook Update state is unavailable")
		}
		if session.Status == "completed" || session.Status == "partial" || session.Status == "failed" || session.Status == "cancelled" {
			break
		}
		select {
		case <-acquisitionCtx.Done():
			t.Fatal("fresh Facebook Update exceeded its six-minute acquisition bound")
		case <-ticker.C:
		}
	}
	if session.Status != "completed" || !browserConfigured || len(session.Runs) != 1 || session.Runs[0].Status != "completed" {
		t.Fatal("fresh Facebook Browser fallback Update did not complete")
	}
	driver, hasDriver, driverErr := state.FirstCommandCaptureDriver(ctx, runID)
	collector, hasCollector, collectorErr := state.FirstCommandCollector(ctx, runID)
	observations, observationErr := state.Observations(ctx, runID)
	if driverErr != nil || collectorErr != nil || observationErr != nil || !hasDriver || !hasCollector || driver != "browser" || collector != collection.BackendBridge || len(observations) == 0 {
		t.Fatal("fresh Facebook Update did not persist its Browser and Bridge acquisition evidence")
	}
	if err := waitForAuthJourneyRuntime(ctx, coordinator, "headless", 4); err != nil {
		t.Fatal("fresh Facebook Update did not clean up and return to the saved headless mode")
	}
	items, err := state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal("fresh Facebook Update timeline is unavailable")
	}
	target, targetURL, targetNativeID, found := selectFreshFacebookRecaptureTarget(items, observations, session.ID, runID)
	if !found {
		t.Fatal("fresh Facebook Update produced no supported recent unavailable-media target; Recapture was not requested")
	}

	requestBody, _ := json.Marshal(map[string]string{"captureMode": "background", "reason": "missing_media"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:11122/api/timeline/"+url.PathEscape(target.ID)+"/recapture", strings.NewReader(string(requestBody)))
	if err != nil {
		t.Fatal("Facebook Recapture request could not be prepared")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(request)
	if err != nil {
		t.Fatal("public Facebook Recapture API did not respond")
	}
	var accepted struct {
		Recapture domain.MediaRecapture `json:"recapture"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 128*1024)).Decode(&accepted)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted || decodeErr != nil || accepted.Recapture.ID == "" || accepted.Recapture.TimelineID != target.ID || accepted.Recapture.Source != domain.SourceFacebook || accepted.Recapture.TargetURL != targetURL || accepted.Recapture.EvidenceKey != target.EvidenceKey {
		t.Fatal("public Recapture API did not preserve the exact fresh Facebook target identity")
	}

	deadline := time.Now().Add(75 * time.Second)
	browserGeneration := uint64(0)
	bridgeConfiguredForRecapture := false
	publicStatus := ""
	for time.Now().Before(deadline) {
		owner := manager.Snapshot()
		if owner.State == captureruntime.Ready && owner.Driver == "browser" {
			if owner.Generation != 5 {
				t.Fatal("Facebook Recapture borrowed an unexpected Browser generation")
			}
			browserGeneration = owner.Generation
			if !bridgeConfiguredForRecapture {
				configureAuthenticatedHybridBridge(t, ctx, state, e)
				bridgeConfiguredForRecapture = true
			}
		}
		statusRequest, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:11122/api/media-recaptures/"+url.PathEscape(accepted.Recapture.ID), nil)
		if requestErr != nil {
			t.Fatal("public Recapture status request could not be prepared")
		}
		statusResponse, requestErr := (&http.Client{Timeout: 5 * time.Second}).Do(statusRequest)
		if requestErr == nil {
			var public struct {
				Recapture struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"recapture"`
			}
			requestErr = json.NewDecoder(io.LimitReader(statusResponse.Body, 64*1024)).Decode(&public)
			_ = statusResponse.Body.Close()
			if requestErr == nil && statusResponse.StatusCode == http.StatusOK && public.Recapture.ID == accepted.Recapture.ID {
				publicStatus = public.Recapture.Status
				if publicStatus == "completed" || publicStatus == "failed" || publicStatus == "cancelled" {
					break
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("Facebook Recapture status wait was interrupted")
		case <-ticker.C:
		}
	}
	if publicStatus != "completed" || browserGeneration != 5 || !bridgeConfiguredForRecapture {
		t.Fatalf("public Facebook Recapture did not complete through its Browser borrow (terminal=%s)", publicStatus)
	}
	job, err := state.MediaRecapture(ctx, accepted.Recapture.ID)
	if err != nil || job.ID != accepted.Recapture.ID || job.TimelineID != target.ID || job.Source != domain.SourceFacebook || job.TargetURL != targetURL || job.EvidenceKey != target.EvidenceKey || job.Status != "completed" || job.Outcome != "recovered" {
		t.Fatal("durable Recapture job did not retain its exact recovered target identity")
	}
	admission, hasAdmission, admissionErr := store.MediaRecaptureAdmission(job.Payload)
	collectorRoute, routeErr := store.CaptureCollector(job.Payload)
	captureRuntime, hasRuntime := job.Payload["captureRuntime"].(map[string]any)
	if admissionErr != nil || !hasAdmission || admission.Policy != domain.MediaRecaptureAdmissionHybridHeadlessV1 || admission.Driver != "browser" || admission.Phase != "admitted" || routeErr != nil || collectorRoute != collection.BackendBridge || !hasRuntime || captureRuntime["driver"] != "browser" || captureRuntime["epoch"] != e.Epoch() || captureGeneration(captureRuntime["generation"]) != browserGeneration {
		t.Fatal("durable Recapture admission lacks the matching Browser/Bridge runtime stamp")
	}
	if job.Payload["reason"] != string(domain.MediaRecaptureMissingMedia) || job.Payload["foregroundAuthorized"] != false {
		t.Fatal("durable Recapture job changed its background missing-media policy")
	}
	updated, err := state.TimelineItem(ctx, target.ID)
	if err != nil || updated.Evidence == nil || !sameFreshFacebookRecaptureIdentity(target, updated, targetURL, targetNativeID) || !hasUsableFacebookMedia(updated.Evidence.Media) {
		t.Fatal("completed Recapture did not attach usable trusted media to the exact fresh native post")
	}
	if err := waitForAuthJourneyRuntime(ctx, coordinator, "headless", 6); err != nil {
		t.Fatal("Facebook Recapture did not release Browser ownership and return to headless")
	}
	settingsAfter, err := state.GetSettings(ctx)
	status := coordinator.Status()
	if err != nil || !reflect.DeepEqual(settingsBefore, settingsAfter) || settingsAfter.CollectionMode != "headless" || len(settingsAfter.ActiveSources) != 1 || settingsAfter.ActiveSources[0] != domain.SourceFacebook {
		t.Fatal("Facebook Recapture changed the saved settings selection")
	}
	if status.Pending || status.Effective != "headless" || status.ActiveLeases != 0 || status.CollectionBorrowSource != "" || manager.Snapshot().ActiveLeases != 0 {
		t.Fatal("Facebook Recapture retained a Browser hold after returning to headless")
	}
	t.Log("fresh_facebook_update=true automatic_recapture=true browser_bridge_stamp=true exact_native_target=true usable_trusted_media=true saved_settings_preserved=true final_holds=0")
}

func selectFreshFacebookRecaptureTarget(items []domain.TimelineItem, observations []domain.Observation, sessionID, runID string) (domain.TimelineItem, string, string, bool) {
	for _, item := range items {
		if item.ID == "" || item.SessionID != sessionID || item.RunID != runID || item.Source != domain.SourceFacebook || item.EvidenceKey == "" || item.Evidence == nil || item.Evidence.EvidenceKey != item.EvidenceKey || len(item.Evidence.Media) != 0 || item.Evidence.MediaRecovery["outcome"] != "unavailable" {
			continue
		}
		canonical, ok := domain.CanonicalSourceURL(domain.SourceFacebook, item.Evidence.Permalink)
		if !ok || !supportedFacebookRecaptureURL(canonical) {
			continue
		}
		// Facebook's native URL is not reversible to its opaque platform ID.
		// Preserve the exact ID captured by this fresh run and compare it again
		// with the post-Recapture timeline evidence.
		nativeID := strings.TrimSpace(item.Evidence.PlatformID)
		if nativeID == "" {
			continue
		}
		observed := false
		for _, observation := range observations {
			if observation.Source != domain.SourceFacebook {
				continue
			}
			for _, snapshot := range observation.Snapshots {
				for _, block := range snapshot.Blocks {
					observedURL, observedOK := domain.CanonicalSourceURL(domain.SourceFacebook, block.Permalink)
					if observedOK && block.EvidenceKey == item.EvidenceKey && observedURL == canonical && block.PlatformID == nativeID {
						observed = true
					}
				}
			}
		}
		if !observed {
			continue
		}
		return item, canonical, nativeID, true
	}
	return domain.TimelineItem{}, "", "", false
}

func sameFreshFacebookRecaptureIdentity(before, after domain.TimelineItem, targetURL, nativeID string) bool {
	return before.ID != "" && before.ID == after.ID && before.SessionID == after.SessionID && before.RunID == after.RunID &&
		before.Source == domain.SourceFacebook && after.Source == domain.SourceFacebook && before.EvidenceKey != "" && before.EvidenceKey == after.EvidenceKey &&
		after.Evidence != nil && after.Evidence.EvidenceKey == before.EvidenceKey && after.Evidence.Permalink == targetURL &&
		strings.TrimSpace(after.Evidence.PlatformID) == nativeID
}

func supportedFacebookRecaptureURL(raw string) bool {
	canonical, ok := domain.CanonicalSourceURL(domain.SourceFacebook, raw)
	if !ok || canonical != raw {
		return false
	}
	parsed, err := url.Parse(canonical)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "facebook.com") && !strings.EqualFold(parsed.Hostname(), "www.facebook.com") && !strings.EqualFold(parsed.Hostname(), "m.facebook.com") {
		return false
	}
	path := strings.ToLower(strings.TrimSuffix(parsed.EscapedPath(), "/"))
	if strings.Contains(path, "/watch/") || path == "/watch" {
		return false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index, part := range parts {
		if part != "reel" && part != "videos" && part != "posts" && part != "permalink" {
			continue
		}
		if index+1 < len(parts) && parts[index+1] != "" {
			return true
		}
	}
	return false
}

func hasUsableFacebookMedia(media []map[string]any) bool {
	for _, candidate := range media {
		kind, _ := candidate["kind"].(string)
		switch kind {
		case "video":
			playbackURL, _ := candidate["playbackUrl"].(string)
			if _, ok := domain.CanonicalInlinePlaybackURL(domain.SourceFacebook, playbackURL); ok {
				return true
			}
		case "image":
			mediaURL, _ := candidate["url"].(string)
			if strings.TrimSpace(mediaURL) != "" && domain.ValidatedMediaURL(domain.SourceFacebook, candidate) {
				return true
			}
		}
	}
	return false
}

func captureGeneration(value any) uint64 {
	switch generation := value.(type) {
	case float64:
		if generation > 0 && generation == float64(uint64(generation)) {
			return uint64(generation)
		}
	case int:
		if generation > 0 {
			return uint64(generation)
		}
	case int64:
		if generation > 0 {
			return uint64(generation)
		}
	}
	return 0
}

func TestAuthenticatedFacebookRecaptureTargetGuard(t *testing.T) {
	for _, raw := range []string{
		"https://www.facebook.com/reel/123456789/",
		"https://www.facebook.com/videos/123456789/",
		"https://www.facebook.com/example/posts/pfbid02ABC123/",
		"https://www.facebook.com/example/videos/123456789/",
	} {
		canonical, ok := domain.CanonicalSourceURL(domain.SourceFacebook, raw)
		if !ok || !supportedFacebookRecaptureURL(canonical) {
			t.Fatalf("supported fresh native target rejected: %s", raw)
		}
	}
	for _, raw := range []string{
		"https://www.facebook.com/watch/?v=123456789",
		"https://www.facebook.com/photo/?fbid=123456789",
		"https://www.facebook.com/example/posts/",
		"https://example.com/reel/123456789/",
	} {
		canonical, ok := domain.CanonicalSourceURL(domain.SourceFacebook, raw)
		if ok && supportedFacebookRecaptureURL(canonical) {
			t.Fatalf("unsupported or media-only target was admitted: %s", raw)
		}
	}
	valid := domain.TimelineItem{ID: "fresh-item", SessionID: "fresh-session", RunID: "fresh-run", Source: domain.SourceFacebook, EvidenceKey: "evidence", Evidence: &domain.Block{
		EvidenceKey: "evidence", PlatformID: "opaque-native-id", Permalink: "https://www.facebook.com/example/posts/pfbid02ABC123/", MediaRecovery: map[string]any{"outcome": "unavailable"},
	}}
	observations := []domain.Observation{{Source: domain.SourceFacebook, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{*valid.Evidence}}}}}
	selected, targetURL, nativeID, ok := selectFreshFacebookRecaptureTarget([]domain.TimelineItem{valid}, observations, "fresh-session", "fresh-run")
	if !ok || nativeID != "opaque-native-id" {
		t.Fatal("fresh Facebook session/run/native identity fence rejected its owned target")
	}
	recaptured := valid
	if !sameFreshFacebookRecaptureIdentity(selected, recaptured, targetURL, nativeID) {
		t.Fatal("exact Facebook native identity failed a matching Recapture projection")
	}
	recaptured.Evidence = &domain.Block{EvidenceKey: "evidence", PlatformID: "different-native-id", Permalink: targetURL}
	if sameFreshFacebookRecaptureIdentity(selected, recaptured, targetURL, nativeID) {
		t.Fatal("Recapture projection with a different native platform ID was accepted")
	}
	for _, stale := range []domain.TimelineItem{
		func() domain.TimelineItem { value := valid; value.SessionID = "old-session"; return value }(),
		func() domain.TimelineItem { value := valid; value.RunID = "old-run"; return value }(),
		func() domain.TimelineItem {
			value := valid
			value.Evidence = &domain.Block{Permalink: valid.Evidence.Permalink, MediaRecovery: map[string]any{"outcome": "unavailable"}}
			return value
		}(),
	} {
		if _, _, _, ok := selectFreshFacebookRecaptureTarget([]domain.TimelineItem{stale}, observations, "fresh-session", "fresh-run"); ok {
			t.Fatal("fresh Facebook target selector admitted stale session/run or an unidentified native post")
		}
	}
	if hasUsableFacebookMedia([]map[string]any{{"kind": "video", "url": "https://scontent.fbcdn.net/poster.jpg", "posterUrl": "https://scontent.fbcdn.net/poster.jpg"}}) {
		t.Fatal("a video poster without a direct playback URL was treated as usable Recapture media")
	}
}
