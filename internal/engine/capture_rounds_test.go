package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func captureRoundsJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestEngineCaptureRoundsCharacterizationMerge(t *testing.T) {
	for _, mode := range []string{"complete", "missing-latest", "nil-latest", "single", "three-rounds"} {
		t.Run(mode, func(t *testing.T) {
			first := domain.Observation{Source: domain.SourceX, CapturedAt: "first-time", PageURL: "https://x.com/first", Snapshots: []domain.Snapshot{{ScrollY: 10, Blocks: []domain.Block{{EvidenceKey: "a", Text: "first distinct evidence"}}}}, Coverage: map[string]any{"frontier": map[string]any{"scrollY": float64(10)}, "oldOnly": "capture", "captureQuality": map[string]any{"verdict": "complete"}}}
			last := domain.Observation{Source: domain.SourceX, CapturedAt: "last-time", PageURL: "https://x.com/last", Snapshots: []domain.Snapshot{{ScrollY: 20, Blocks: []domain.Block{{EvidenceKey: "b", Text: "second distinct evidence"}}}}, Coverage: map[string]any{"frontier": map[string]any{"scrollY": float64(20)}, "latestOnly": "capture"}}
			values := []domain.Observation{first, last}
			if mode == "missing-latest" {
				last.Coverage = map[string]any{}
				values[1] = last
			}
			if mode == "nil-latest" {
				last.Coverage = nil
				values[1] = last
			}
			if mode == "single" {
				values = values[1:]
			}
			if mode == "three-rounds" {
				values = []domain.Observation{first, first, last}
			}
			before := captureRoundsJSON(t, values)
			merged := mergeObservations(values)
			if merged.Source != last.Source || merged.CapturedAt != last.CapturedAt || merged.PageURL != last.PageURL || len(merged.Snapshots) != len(values) {
				t.Fatalf("merged=%+v", merged)
			}
			for i, value := range values {
				if merged.Snapshots[i].ScrollY != value.Snapshots[0].ScrollY || merged.Snapshots[i].Blocks[0].EvidenceKey != value.Snapshots[0].Blocks[0].EvidenceKey {
					t.Fatal("snapshot/evidence order changed")
				}
			}
			rounds, ok := merged.Coverage["rounds"].([]map[string]any)
			if !ok || len(rounds) != len(values) || merged.Coverage["acquisitionRounds"] != len(values) {
				t.Fatalf("coverage=%+v", merged.Coverage)
			}
			if _, ok := merged.Coverage["captureQuality"]; ok {
				t.Fatal("prior quality resurrected")
			}
			if _, ok := merged.Coverage["oldOnly"]; ok {
				t.Fatal("prior telemetry resurrected")
			}
			if !reflect.DeepEqual(merged.Coverage["frontier"], last.Coverage["frontier"]) {
				t.Fatal("latest frontier changed")
			}
			if captureRoundsJSON(t, values) != before {
				t.Fatal("merge mutated inputs")
			}
			merged.Coverage["top-level"] = "new"
			if _, ok := last.Coverage["top-level"]; ok {
				t.Fatal("top-level coverage aliases input")
			}
			if last.Coverage != nil {
				rounds[len(rounds)-1]["round-alias"] = "retained"
				if last.Coverage["round-alias"] != "retained" {
					t.Fatal("round coverage alias changed")
				}
			}
			if frontier, ok := merged.Coverage["frontier"].(map[string]any); ok {
				frontier["nested-alias"] = "retained"
				if last.Coverage["frontier"].(map[string]any)["nested-alias"] != "retained" {
					t.Fatal("nested shallow-copy alias changed")
				}
			}
		})
	}
	t.Run("empty-precondition", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("empty observation precondition changed")
			}
		}()
		mergeObservations(nil)
	})
}

func TestEngineCaptureRoundsCharacterizationCoverage(t *testing.T) {
	for _, mode := range []string{"typed-rounds", "json-rounds", "no-rounds"} {
		t.Run(mode, func(t *testing.T) {
			round := map[string]any{"captureField": "old", "missingLatest": nil}
			target := map[string]any{"captureField": "latest", "orchestration": "old", "acquisitionRounds": 2}
			if mode == "typed-rounds" {
				target["rounds"] = []map[string]any{round}
			}
			if mode == "json-rounds" {
				target["rounds"] = []any{round}
			}
			durable := map[string]any{"rounds": "cannot-overwrite", "acquisitionRounds": 99, "captureField": "durable", "missingLatest": "stale", "orchestration": "current", "newReceipt": map[string]any{"proof": "saved"}}
			before := captureRoundsJSON(t, durable)
			mergeDurableRunCoverage(target, durable)
			if target["acquisitionRounds"] != 2 || target["orchestration"] != "current" || target["rounds"] == "cannot-overwrite" {
				t.Fatalf("target=%+v", target)
			}
			if mode == "typed-rounds" {
				if target["captureField"] != "latest" {
					t.Fatal("capture overwritten")
				}
				if _, ok := target["missingLatest"]; ok {
					t.Fatal("absent latest field resurrected")
				}
			} else if target["captureField"] != "durable" || target["missingLatest"] != "stale" {
				t.Fatal("existing rounds type contract changed")
			}
			if captureRoundsJSON(t, durable) != before {
				t.Fatal("durable input mutated")
			}
			target["newReceipt"].(map[string]any)["alias"] = true
			if durable["newReceipt"].(map[string]any)["alias"] != true {
				t.Fatal("durable nested alias changed")
			}
		})
	}
	t.Run("clone-nil", func(t *testing.T) {
		if got := cloneCoverageMap(nil); got == nil || len(got) != 0 {
			t.Fatalf("clone=%v", got)
		}
	})
	t.Run("clone-shallow", func(t *testing.T) {
		input := map[string]any{"nested": map[string]any{"value": 1}}
		clone := cloneCoverageMap(input)
		clone["new"] = true
		if input["new"] != nil {
			t.Fatal("top-level alias")
		}
		clone["nested"].(map[string]any)["value"] = 2
		if input["nested"].(map[string]any)["value"] != 2 {
			t.Fatal("nested alias changed")
		}
	})
}

func TestEngineCaptureRoundsCharacterizationYield(t *testing.T) {
	for _, mode := range []string{"none", "one", "overlap", "new", "three-rounds", "blank-key", "native-recovery", "ambiguous-native"} {
		t.Run(mode, func(t *testing.T) {
			body := strings.Repeat("A captured post with enough stable author and content to reconcile native identity. ", 2)
			first := domain.Observation{Source: domain.SourceLinkedIn, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "a", Author: "Author", Text: body}}}}}
			second := first
			second.Snapshots = []domain.Snapshot{{Blocks: []domain.Block{first.Snapshots[0].Blocks[0]}}}
			values := []domain.Observation{first, second}
			want := 0
			switch mode {
			case "none":
				values = nil
			case "one":
				values = values[:1]
			case "new":
				values[1].Snapshots[0].Blocks = append(values[1].Snapshots[0].Blocks, domain.Block{EvidenceKey: "b", Text: "distinct new evidence"})
				want = 1
			case "three-rounds":
				values = append(values, domain.Observation{Source: domain.SourceLinkedIn, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "b"}, {EvidenceKey: "b"}, {EvidenceKey: "c"}}}}})
				want = 2
			case "blank-key":
				values[1].Snapshots[0].Blocks = append(values[1].Snapshots[0].Blocks, domain.Block{EvidenceKey: " \t"})
				want = 0
			case "native-recovery":
				values[1].Snapshots[0].Blocks[0].EvidenceKey = "native"
				values[1].Snapshots[0].Blocks[0].PlatformID = "urn:li:activity:7411111111111111111"
				values[1].Snapshots[0].Blocks[0].Permalink = "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111"
				want = 1
			case "ambiguous-native":
				values[0].Snapshots[0].Blocks[0].PlatformID = "urn:li:activity:7411111111111111111"
				values[1].Snapshots[0].Blocks[0].EvidenceKey = "b"
				values[1].Snapshots[0].Blocks[0].PlatformID = "urn:li:activity:7422222222222222222"
				want = 1
			}
			before := captureRoundsJSON(t, values)
			if got := followUpNewCandidateCount(values); got != want {
				t.Fatalf("yield=%d want=%d", got, want)
			}
			if captureRoundsJSON(t, values) != before {
				t.Fatal("yield changed observations")
			}
			if mode == "native-recovery" {
				all := append(append([]domain.Snapshot{}, values[0].Snapshots...), values[1].Snapshots...)
				if got := observationEvidenceKeys(domain.SourceLinkedIn, all); !reflect.DeepEqual(got, map[string]bool{"native": true}) {
					t.Fatalf("reconciled keys=%v", got)
				}
			}
		})
	}
	t.Run("key-trimming", func(t *testing.T) {
		got := observationEvidenceKeys(domain.SourceX, []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: " a "}, {EvidenceKey: "a"}, {EvidenceKey: " "}}}})
		if !reflect.DeepEqual(got, map[string]bool{"a": true}) {
			t.Fatalf("keys=%v", got)
		}
	})
	t.Run("empty-keys", func(t *testing.T) {
		if got := observationEvidenceKeys(domain.SourceX, nil); got == nil || len(got) != 0 {
			t.Fatalf("keys=%v", got)
		}
	})
}

func TestEngineCaptureRoundsCharacterizationAnchors(t *testing.T) {
	for _, test := range []struct {
		name     string
		coverage map[string]any
		want     []string
	}{
		{"nil", nil, nil},
		{"wrong-frontier", map[string]any{"frontier": "invalid"}, nil},
		{"strings", map[string]any{"frontier": map[string]any{"anchorKeys": []string{" ", " a ", "a", "b", "c"}}}, []string{"a", "a", "b"}},
		{"mixed", map[string]any{"frontier": map[string]any{"anchorKeys": []any{nil, 5, " ", " a ", "b", "c", "d"}}}, []string{"a", "b", "c"}},
		{"wrong-type", map[string]any{"frontier": map[string]any{"anchorKeys": []int{1, 2}}}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := captureRoundsJSON(t, test.coverage)
			if got := continuationAnchors(test.coverage); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("anchors=%v want=%v", got, test.want)
			}
			if captureRoundsJSON(t, test.coverage) != before {
				t.Fatal("anchor extraction changed input")
			}
		})
	}
	t.Run("snapshot-priority-and-cap", func(t *testing.T) {
		snapshot := domain.Snapshot{Blocks: []domain.Block{{EvidenceKey: "go-key-only"}, {PlatformID: " Platform-ID ", Permalink: "https://example.test/ignored", Text: "ignored"}, {Permalink: " https://example.test/post ", Text: "ignored"}, {Text: " Mixed\n CASE\t text "}, {PlatformID: "fourth"}}}
		before := captureRoundsJSON(t, snapshot)
		if got := snapshotContinuationAnchors(snapshot); !reflect.DeepEqual(got, []string{"Platform-ID", "https://example.test/post", "mixed case text"}) {
			t.Fatalf("anchors=%v", got)
		}
		if captureRoundsJSON(t, snapshot) != before {
			t.Fatal("snapshot changed")
		}
	})
	t.Run("snapshot-byte-limit", func(t *testing.T) {
		got := snapshotContinuationAnchors(domain.Snapshot{Blocks: []domain.Block{{Text: strings.Repeat("a", 299) + "é"}}})
		if len(got) != 1 || len(got[0]) != 300 || got[0] != strings.Repeat("a", 299)+"\xc3" || utf8.ValidString(got[0]) {
			t.Fatalf("byte bound=%q", got)
		}
	})
	t.Run("snapshot-empty", func(t *testing.T) {
		if got := snapshotContinuationAnchors(domain.Snapshot{}); got == nil || len(got) != 0 {
			t.Fatalf("empty anchors=%v", got)
		}
	})
}

func TestEngineCaptureRoundsCharacterizationContinuation(t *testing.T) {
	for _, mode := range []string{"frontier", "missing-frontier", "ready", "not-ready", "nil-ready", "wrong-ready", "no-snapshots", "no-identity", "integer-scroll", "negative-scroll", "zero-scroll", "fractional-scroll", "fallback-priority", "duplicates", "linkedin-overlap", "linkedin-empty-checkpoint", "linkedin-zero-checkpoint", "instagram-no-overlap"} {
		t.Run(mode, func(t *testing.T) {
			value := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{ScrollY: 99, Blocks: []domain.Block{{EvidenceKey: "go-key", PlatformID: " last-id ", Permalink: "https://example.test/ignored", Text: "fallback text"}}}}, Coverage: map[string]any{"frontier": map[string]any{"scrollY": float64(123), "anchorKeys": []any{" frontier-id "}}}}
			frontier := value.Coverage["frontier"].(map[string]any)
			wantScroll := 123
			wantAnchors := []string{"frontier-id"}
			wantNil := false
			switch mode {
			case "missing-frontier":
				value.Coverage = nil
				wantScroll = 99
				wantAnchors = []string{"last-id"}
			case "ready":
				frontier["continuationReady"] = true
			case "not-ready":
				frontier["continuationReady"] = false
				wantNil = true
			case "nil-ready":
				frontier["continuationReady"] = nil
				wantNil = true
			case "wrong-ready":
				frontier["continuationReady"] = "true"
				wantNil = true
			case "no-snapshots":
				value.Snapshots = nil
				wantNil = true
			case "no-identity":
				delete(frontier, "anchorKeys")
				value.Snapshots[0].Blocks = []domain.Block{{EvidenceKey: "go-key-only"}}
				wantNil = true
			case "integer-scroll":
				frontier["scrollY"] = 123
				wantScroll = 99
			case "negative-scroll":
				frontier["scrollY"] = float64(-1)
				wantScroll = 99
			case "zero-scroll":
				frontier["scrollY"] = float64(0)
				wantScroll = 99
			case "fractional-scroll":
				frontier["scrollY"] = float64(123.9)
			case "fallback-priority":
				delete(frontier, "anchorKeys")
				wantAnchors = []string{"last-id"}
			case "duplicates":
				frontier["anchorKeys"] = []string{"same", "same", "third", "fourth"}
				wantAnchors = []string{"same", "same", "third"}
			case "linkedin-overlap", "linkedin-empty-checkpoint", "linkedin-zero-checkpoint", "instagram-no-overlap":
				value.Source = domain.SourceLinkedIn
				value.Snapshots = []domain.Snapshot{{ScrollY: 12, Blocks: []domain.Block{{PlatformID: "older"}}}, {ScrollY: 55, Blocks: []domain.Block{{PlatformID: "checkpoint"}}}, value.Snapshots[0]}
				wantScroll = 55
				wantAnchors = []string{"checkpoint"}
				if mode == "linkedin-empty-checkpoint" {
					value.Snapshots[1].Blocks = nil
					wantScroll = 12
					wantAnchors = []string{"older"}
				}
				if mode == "linkedin-zero-checkpoint" {
					value.Snapshots[1].ScrollY = 0
					wantScroll = 99
				}
				if mode == "instagram-no-overlap" {
					value.Source = domain.SourceInstagram
					wantScroll = 123
					wantAnchors = []string{"frontier-id"}
				}
			}
			before := captureRoundsJSON(t, value)
			got := continuationFrom(value)
			if wantNil {
				if got != nil {
					t.Fatalf("continuation=%v", got)
				}
			} else if want := map[string]any{"startScrollY": wantScroll, "anchorKeys": wantAnchors, "settleMs": 900}; !reflect.DeepEqual(got, want) {
				t.Fatalf("continuation=%v want=%v", got, want)
			}
			if captureRoundsJSON(t, value) != before {
				t.Fatal("continuation changed observation")
			}
		})
	}
}

type roundsCharacterizationProvider struct {
	reasoning.Deterministic
	planned  []domain.Observation
	analyzed []domain.Observation
}

func (p *roundsCharacterizationProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	plan, telemetry, err := p.Deterministic.Plan(ctx, run, observation, knowledge)
	p.planned = append(p.planned, observation)
	plan.Decision = "request_follow_up"
	plan.Reason = "offline bounded adjacent round"
	return plan, telemetry, err
}
func (p *roundsCharacterizationProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.analyzed = append(p.analyzed, observation)
	return p.Deterministic.Analyze(ctx, run, observation, knowledge)
}

func TestEngineCaptureRoundsCharacterizationProcess(t *testing.T) {
	for _, mode := range []string{"two-rounds", "not-ready", "queue-error", "receipt-error", "yield-error", "no-observation"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t, domain.SourceLinkedIn)
			ctx := context.Background()
			settings, err := f.state.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.CalibrationEnabled = false
			settings.AIDetectionEnabled = false
			if err := f.state.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			provider := &roundsCharacterizationProvider{}
			f.e.provider = provider
			command := f.claim(t)
			first := domain.Observation{Source: domain.SourceLinkedIn, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{ScrollY: 105, Blocks: []domain.Block{{EvidenceKey: "linkedin:first", PlatformID: "linkedin:activity:7411111111111111111", Author: "First Author", Text: "The first bounded source post has independent captured evidence.", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111"}}}}, Coverage: map[string]any{"performedScrolls": float64(1), "scrollStopReason": "budget_exhausted", "captureQuality": map[string]any{"verdict": "usable_degraded"}, "frontier": map[string]any{"scrollY": float64(105), "anchorKeys": []any{"first-bridge-anchor"}, "continuationReady": true, "newCandidateCount": float64(1), "hasMoreCandidateSignal": true}}}
			if mode == "not-ready" {
				first.Coverage["frontier"].(map[string]any)["continuationReady"] = false
			}
			if mode != "no-observation" {
				if err := validateObservation(first); err != nil {
					t.Fatal(err)
				}
				if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, first); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "queue-error" {
				f.exec(t, `CREATE TRIGGER rounds_queue_abort BEFORE INSERT ON bridge_commands WHEN json_extract(NEW.payload_json,'$.acquisitionRound')=2 BEGIN SELECT RAISE(ABORT,'offline queue rejected'); END`)
			}
			if mode == "receipt-error" {
				f.exec(t, `CREATE TRIGGER rounds_receipt_abort BEFORE UPDATE OF coverage_json ON runs WHEN json_extract(NEW.coverage_json,'$.acquisitionPlanning.followUpQueued')=1 BEGIN SELECT RAISE(ABORT,'offline receipt rejected'); END`)
			}
			err = f.e.process(ctx, f.run.ID, true)
			if mode == "no-observation" {
				if err == nil || err.Error() != "no accepted observation" || len(provider.planned) != 0 || len(provider.analyzed) != 0 {
					t.Fatalf("missing observation=%v", err)
				}
				return
			}
			if len(provider.planned) != 1 {
				t.Fatalf("planner calls=%d", len(provider.planned))
			}
			var count int
			if queryErr := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&count); queryErr != nil {
				t.Fatal(queryErr)
			}
			if mode == "queue-error" || mode == "receipt-error" {
				want := "offline queue rejected"
				wantCount := 1
				wantStatus := "reasoning"
				if mode == "receipt-error" {
					want = "save model acquisition planning receipt: constraint failed: offline receipt rejected"
					wantCount = 2
					wantStatus = "waiting_for_bridge"
				}
				if err == nil || !strings.Contains(err.Error(), want) || count != wantCount || len(provider.analyzed) != 0 {
					t.Fatalf("error=%v commands=%d", err, count)
				}
				run, loadErr := f.state.GetRun(ctx, f.run.ID)
				if loadErr != nil || run.Status != wantStatus || run.Coverage["acquisitionPlanning"] != nil {
					t.Fatalf("run=%+v err=%v", run, loadErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "not-ready" {
				if count != 1 || len(provider.analyzed) != 1 {
					t.Fatalf("blocked continuation commands=%d analyses=%d", count, len(provider.analyzed))
				}
				return
			}
			if count != 2 || len(provider.analyzed) != 0 {
				t.Fatalf("first round commands=%d analyses=%d", count, len(provider.analyzed))
			}
			next := f.claim(t)
			continuation, ok := next.Payload["continuation"].(map[string]any)
			if !ok || continuation["startScrollY"] != float64(105) || continuation["settleMs"] != float64(900) || !reflect.DeepEqual(continuation["anchorKeys"], []any{"first-bridge-anchor"}) {
				t.Fatalf("wire continuation=%v", continuation)
			}
			if next.Payload["sourceFreshnessPolicy"] != "preserve_frontier" || next.Payload["pendingContentPolicy"] != "detect_only" || next.Payload["sameTabMutationAllowed"] != false || next.Payload["maxAcquisitionRounds"] != float64(2) {
				t.Fatalf("wire=%+v", next.Payload)
			}
			second := domain.Observation{Source: domain.SourceLinkedIn, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{ScrollY: 205, Blocks: []domain.Block{{EvidenceKey: "linkedin:second", PlatformID: "linkedin:activity:7422222222222222222", Author: "Second Author", Text: "A distinct second post supplies new evidence beyond the first bounded capture.", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:7422222222222222222"}}}}, Coverage: map[string]any{"performedScrolls": float64(0), "scrollStopReason": "no_movement", "frontier": map[string]any{"scrollY": float64(205), "anchorKeys": []any{"second-bridge-anchor"}, "hasMoreCandidateSignal": false, "newCandidateCount": float64(0)}}}
			if err := validateObservation(second); err != nil {
				t.Fatal(err)
			}
			if err := f.state.SaveObservation(ctx, next.ID, f.run.ID, second); err != nil {
				t.Fatal(err)
			}
			if mode == "yield-error" {
				f.exec(t, `CREATE TRIGGER rounds_yield_abort BEFORE UPDATE OF coverage_json ON runs WHEN json_extract(NEW.coverage_json,'$.acquisitionPlanning.followUpNewCandidates')=1 BEGIN SELECT RAISE(ABORT,'offline yield rejected'); END`)
			}
			err = f.e.process(ctx, f.run.ID, true)
			if mode == "yield-error" {
				if err == nil || !strings.HasPrefix(err.Error(), "save acquisition planning yield: ") || !strings.Contains(err.Error(), "offline yield rejected") || len(provider.analyzed) != 0 {
					t.Fatalf("yield error=%v", err)
				}
				run, loadErr := f.state.GetRun(ctx, f.run.ID)
				if loadErr != nil || run.Status != "reasoning" {
					t.Fatalf("run=%+v err=%v", run, loadErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.planned) != 1 || len(provider.analyzed) != 1 {
				t.Fatal("bounded planner/analysis count changed")
			}
			merged := provider.analyzed[0]
			if len(merged.Snapshots) != 2 || merged.Coverage["performedScrolls"] != float64(0) || merged.Coverage["scrollStopReason"] != "no_movement" || merged.Coverage["captureQuality"] != nil || merged.Coverage["acquisitionRounds"] != 2 {
				t.Fatalf("merged=%+v", merged)
			}
			run, loadErr := f.state.GetRun(ctx, f.run.ID)
			if loadErr != nil || run.Status != "completed" {
				t.Fatalf("run=%+v err=%v", run, loadErr)
			}
			receipt, ok := run.Coverage["acquisitionPlanning"].(map[string]any)
			if !ok || receipt["followUpNewCandidates"] != float64(1) || receipt["followUpQueued"] != true {
				t.Fatalf("receipt=%v", receipt)
			}
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatal("extra follow-up queued")
			}
		})
	}
}
