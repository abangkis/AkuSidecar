package engine

import (
	"context"
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
