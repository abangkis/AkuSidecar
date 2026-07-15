package engine

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func testEngine(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 2}}
	runtime := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(ExpectedHeartbeat())
	return runtime, state
}

func TestUnifiedSessionCompletesBothSources(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:000000000000000000000001")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 2 && value.Runs[1].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceLinkedIn, "linkedin:000000000000000000000002")
	completed := waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(completed.Items) != 2 {
		t.Fatalf("items=%d session=%+v", len(completed.Items), completed)
	}
	timeline, err := runtime.Timeline(ctx, 10, 0)
	if err != nil || len(timeline) != 2 {
		t.Fatalf("timeline=%d err=%v", len(timeline), err)
	}
}

func TestFirstRunCalibrationFollowsTheInitialUnifiedSession(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:000000000000000000000011")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 2 && value.Runs[1].Status == "waiting_for_bridge"
	})
	completeActiveRun(t, runtime, state, session.ID, domain.SourceLinkedIn, "linkedin:000000000000000000000012")
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	calibration := waitActiveCalibration(t, runtime)
	if calibration.Status != "reviewing" || calibration.SampleCount != 2 || calibration.Samples[0].Source != domain.SourceX || calibration.Samples[1].Source != domain.SourceLinkedIn {
		t.Fatalf("calibration=%+v", calibration)
	}
	if _, err := runtime.StartSession(ctx, "must be blocked"); err == nil {
		t.Fatal("an active forced calibration must block another update")
	}
	more := "more_like_this"
	calibration, err = runtime.DecideCalibration(ctx, calibration.ID, 0, domain.CalibrationDecision{Label: &more})
	if err != nil || calibration.Status != "reviewing" || calibration.ResolvedCount != 1 {
		t.Fatalf("first decision calibration=%+v err=%v", calibration, err)
	}
	neutral := "neutral"
	calibration, err = runtime.DecideCalibration(ctx, calibration.ID, 1, domain.CalibrationDecision{Label: &neutral})
	if err != nil {
		t.Fatal(err)
	}
	if calibration.Status != "completed" || calibration.Snapshot == nil || calibration.Snapshot.Labels["moreLikeThis"] != 1 || calibration.Snapshot.Labels["neutral"] != 1 || calibration.Snapshot.ActivationState != "feeds_local_fit" {
		t.Fatalf("completed calibration=%+v", calibration)
	}
	status, err := state.CalibrationFirstRunStatus(ctx)
	if err != nil || status != "completed" {
		t.Fatalf("first-run status=%q err=%v", status, err)
	}
	if err := runtime.ResetLearning(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = state.CalibrationFirstRunStatus(ctx)
	if err != nil || status != "not_started" {
		t.Fatalf("reset first-run status=%q err=%v", status, err)
	}
	if active, err := state.ActiveCalibration(ctx); err != nil || active != nil {
		t.Fatalf("active calibration after reset=%+v err=%v", active, err)
	}
	if onboarding, err := state.Onboarding(ctx); err != nil || onboarding.Status != "completed" {
		t.Fatalf("onboarding after learning reset=%+v err=%v", onboarding, err)
	}
}

func TestPartialFirstUpdateStillSuppliesCalibration(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	session, err := runtime.StartSession(ctx, "What changed?")
	if err != nil {
		t.Fatal(err)
	}
	completeActiveRun(t, runtime, state, session.ID, domain.SourceX, "x:000000000000000000000021")
	current := waitSession(t, runtime, session.ID, func(value domain.Session) bool {
		return len(value.Runs) == 2 && value.Runs[1].Status == "waiting_for_bridge"
	})
	linkedin := current.Runs[1]
	command, err := runtime.ClaimCommand(ctx, linkedin.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	if _, err := runtime.FailCommand(ctx, command.ID, linkedin.ID, domain.Failure{Code: "capture_failed", Stage: "capture", Message: "test failure"}); err != nil {
		t.Fatal(err)
	}
	waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "partial" })
	calibration := waitActiveCalibration(t, runtime)
	if calibration.SampleCount != 1 || calibration.Samples[0].Source != domain.SourceX {
		t.Fatalf("partial calibration=%+v", calibration)
	}
}

func TestCalibrationSamplerPreservesSourceOrderAndCapsEachSource(t *testing.T) {
	var candidates []domain.CalibrationCandidate
	for index := 0; index < 6; index++ {
		candidates = append(candidates,
			domain.CalibrationCandidate{RunID: "run-x", EvidenceKey: fmt.Sprintf("x:%d", index), Source: domain.SourceX, FeedPosition: index},
			domain.CalibrationCandidate{RunID: "run-li", EvidenceKey: fmt.Sprintf("linkedin:%d", index), Source: domain.SourceLinkedIn, FeedPosition: index},
		)
	}
	runs := []domain.Run{{Source: domain.SourceX, Ordinal: 0}, {Source: domain.SourceLinkedIn, Ordinal: 1}}
	samples := sampleCalibrationCandidates(candidates, runs, 10)
	if len(samples) != 10 {
		t.Fatalf("samples=%d", len(samples))
	}
	for index, sample := range samples {
		expected := domain.SourceX
		if index%2 == 1 {
			expected = domain.SourceLinkedIn
		}
		if sample.Source != expected || sample.Candidate.FeedPosition != index/2 {
			t.Fatalf("sample[%d]=%+v", index, sample)
		}
	}
}

func TestBridgeV2RequiresExactCapabilities(t *testing.T) {
	runtime, _ := testEngine(t)
	if status := runtime.BridgeStatus(); !status.Compatible || status.State != "healthy" {
		t.Fatalf("expected exact heartbeat to pass: %+v", status)
	}
	value := ExpectedHeartbeat()
	value.Actions = value.Actions[:len(value.Actions)-1]
	status := runtime.RecordHeartbeat(value)
	if status.Compatible || status.State != "incompatible" {
		t.Fatalf("missing reload_self action must fail closed: %+v", status)
	}
}

func TestObservationGetsStableGoEvidenceIdentity(t *testing.T) {
	value := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: "1890000000000000000", Text: "Material source update"}}}}}
	normalizeObservation(&value)
	first := value.Snapshots[0].Blocks[0].EvidenceKey
	if !regexp.MustCompile(`^x:[a-f0-9]{24}$`).MatchString(first) {
		t.Fatalf("evidenceKey=%q", first)
	}
	normalizeObservation(&value)
	if value.Snapshots[0].Blocks[0].EvidenceKey != first {
		t.Fatal("normalization must be idempotent")
	}
}

func TestLinkedInPermalinkRecoveryReconcilesDuplicateCaptureBeforeReasoning(t *testing.T) {
	text := "A sufficiently long LinkedIn post body is repeated across bounded snapshots and later exposes an exact native post permalink for the same author and content."
	fallback := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:fallback", Author: "Example Company", Text: text,
			Presentation: map[string]any{"permalinkSource": "unavailable"}, FeedPosition: 3,
		}}}},
		Coverage: map[string]any{"round": 1},
	}
	native := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:native", Author: "Example Company", Text: text,
			Permalink:    "https://www.linkedin.com/feed/update/urn:li:share:7412345678901234567/",
			PlatformID:   "linkedin:share:7412345678901234567",
			Presentation: map[string]any{"permalinkSource": "embed_urn"}, FeedPosition: 5,
		}}}},
		Coverage: map[string]any{"round": 2},
	}

	for _, observations := range [][]domain.Observation{{fallback, native}, {native, fallback}} {
		merged := mergeObservations(observations)
		blocks := []domain.Block{merged.Snapshots[0].Blocks[0], merged.Snapshots[1].Blocks[0]}
		for index, block := range blocks {
			if block.EvidenceKey != "linkedin:native" || block.PlatformID != "linkedin:share:7412345678901234567" {
				t.Fatalf("block[%d] identity=%+v", index, block)
			}
			if block.Permalink != native.Snapshots[0].Blocks[0].Permalink || block.Presentation["permalinkSource"] != "embed_urn" {
				t.Fatalf("block[%d] permalink recovery=%+v", index, block)
			}
		}
		if blocks[0].FeedPosition != observations[0].Snapshots[0].Blocks[0].FeedPosition || blocks[1].FeedPosition != observations[1].Snapshots[0].Blocks[0].FeedPosition {
			t.Fatalf("feed positions changed: %+v", blocks)
		}
	}
}

func TestCapturedContentSignatureDoesNotCollapseShortGenericEntries(t *testing.T) {
	first := domain.Block{Author: "Example", Text: "Short repeated status", EvidenceKey: "linkedin:first"}
	second := domain.Block{Author: "Example", Text: "Short repeated status", EvidenceKey: "linkedin:second", PlatformID: "linkedin:share:2"}
	merged := reconcileCapturedSnapshots(domain.SourceLinkedIn, []domain.Snapshot{{Blocks: []domain.Block{first, second}}})
	if merged[0].Blocks[0].EvidenceKey == merged[0].Blocks[1].EvidenceKey {
		t.Fatal("short generic entries must retain distinct identities")
	}
}

func TestContinuationPreservesBridgeFrontierIdentity(t *testing.T) {
	value := domain.Observation{
		Source: domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{
			ScrollY: 1600,
			Blocks: []domain.Block{{
				EvidenceKey: "linkedin:0123456789abcdef01234567",
				PlatformID:  "linkedin:activity:7420000000000000000",
			}},
		}},
		Coverage: map[string]any{
			"frontier": map[string]any{
				"scrollY":    float64(1600),
				"anchorKeys": []any{"linkedin:activity:7420000000000000000"},
			},
		},
	}

	continuation := continuationFrom(value)
	anchors, ok := continuation["anchorKeys"].([]string)
	if !ok || len(anchors) != 1 || anchors[0] != "linkedin:activity:7420000000000000000" {
		t.Fatalf("continuation anchors=%#v", continuation["anchorKeys"])
	}
	if anchors[0] == value.Snapshots[0].Blocks[0].EvidenceKey {
		t.Fatal("Bridge continuation must not use the Go-derived evidence key")
	}
	if continuation["startScrollY"] != 1600 {
		t.Fatalf("startScrollY=%#v", continuation["startScrollY"])
	}
}

func completeActiveRun(t *testing.T, runtime *Engine, state *store.Store, sessionID string, source domain.Source, evidenceKey string) {
	t.Helper()
	session := waitSession(t, runtime, sessionID, func(value domain.Session) bool {
		for _, run := range value.Runs {
			if run.Source == source && run.Status == "waiting_for_bridge" {
				return true
			}
		}
		return false
	})
	var run domain.Run
	for _, value := range session.Runs {
		if value.Source == source {
			run = value
			break
		}
	}
	command, err := runtime.ClaimCommand(context.Background(), run.ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim: %+v %v", command, err)
	}
	value := domain.Observation{Source: source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: evidenceKey, Text: "Material source update", Author: "author", Permalink: "https://example.test/post"}}}}, Coverage: map[string]any{"quality": "complete"}}
	if _, err := runtime.AcceptObservation(context.Background(), command.ID, run.ID, value); err != nil {
		t.Fatal(err)
	}
}

func waitSession(t *testing.T, runtime *Engine, id string, predicate func(domain.Session) bool) domain.Session {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		value, err := runtime.Session(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if predicate(value) {
			return value
		}
		time.Sleep(20 * time.Millisecond)
	}
	value, _ := runtime.Session(context.Background(), id)
	t.Fatalf("session did not reach expected state: %+v", value)
	return domain.Session{}
}

func waitActiveCalibration(t *testing.T, runtime *Engine) domain.CalibrationSession {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		overview, err := runtime.CalibrationOverview(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if overview.Active != nil {
			return *overview.Active
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("first-run calibration did not start automatically")
	return domain.CalibrationSession{}
}
