package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

const (
	policyStructuredReason = "The structured Instagram fallback produced bounded candidates but no navigable continuation frontier; repeating the same fallback would not extend coverage."
	policyTargetReason     = "The complete first capture round already produced the configured bounded candidate target; older-feed coverage is deferred to the next finite check."
	policyExhaustedReason  = "The complete bounded source frontier produced no new candidate after scrolling."
	policyXReason          = "The complete X capture exhausted its bounded scroll budget while reporting a navigable older-feed frontier; queue one adjacent follow-up round."
)

func acquisitionPolicyObservation(source domain.Source) domain.Observation {
	return domain.Observation{Source: source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{ScrollY: 100, Blocks: []domain.Block{{EvidenceKey: "bounded-evidence", PlatformID: "bounded-native-id", Author: "Fixture", Text: "Independent bounded source evidence for acquisition policy characterization."}}}}, Coverage: map[string]any{
		"performedScrolls": float64(1), "scrollStopReason": "budget_exhausted", "captureQuality": map[string]any{"verdict": "complete"},
		"frontier": map[string]any{"scrollY": float64(100), "newCandidateCount": float64(1), "hasMoreCandidateSignal": true, "anchorKeys": []any{"bounded-native-id"}},
	}}
}

func acquisitionPolicyJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEngineAcquisitionPolicyCharacterizationCompletion(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceLinkedIn, domain.SourceFacebook, domain.SourceInstagram, domain.SourceX, "unknown"} {
		for _, mode := range []string{"target", "eligible-zero", "eligible-negative", "above-target", "no-new", "negative-new", "more-without-new", "zero-scroll", "negative-scroll", "fractional-scroll", "int-scroll", "int64-scroll", "wrong-scroll", "nil-coverage", "missing-frontier", "wrong-frontier", "typed-nil-frontier", "missing-quality", "wrong-quality", "degraded", "deadline", "trimmed", "missing-new", "wrong-new", "int64-new", "wrong-more"} {
			t.Run(string(source)+"/"+mode, func(t *testing.T) {
				value := acquisitionPolicyObservation(source)
				frontier := value.Coverage["frontier"].(map[string]any)
				eligible, want := 1, ""
				local := source == domain.SourceLinkedIn || source == domain.SourceFacebook || source == domain.SourceInstagram
				if source == domain.SourceLinkedIn {
					want = policyTargetReason
				}
				switch mode {
				case "eligible-zero":
					eligible, want = 0, ""
				case "eligible-negative":
					eligible, want = -1, ""
				case "above-target":
					eligible = 2
					frontier["newCandidateCount"] = float64(2)
				case "no-new", "missing-new", "wrong-new", "wrong-more":
					frontier["newCandidateCount"], frontier["hasMoreCandidateSignal"] = float64(0), false
					if local {
						want = policyExhaustedReason
					}
					if mode == "missing-new" {
						delete(frontier, "newCandidateCount")
					}
					if mode == "wrong-new" {
						frontier["newCandidateCount"] = "1"
					}
					if mode == "wrong-more" {
						frontier["hasMoreCandidateSignal"] = "true"
					}
				case "negative-new":
					frontier["newCandidateCount"], frontier["hasMoreCandidateSignal"], want = -1, false, ""
				case "more-without-new":
					frontier["newCandidateCount"], want = 0, ""
				case "zero-scroll", "negative-scroll", "fractional-scroll", "wrong-scroll":
					want = ""
					value.Coverage["performedScrolls"] = map[string]any{"zero-scroll": 0, "negative-scroll": -1, "fractional-scroll": 0.9, "wrong-scroll": "1"}[mode]
				case "int-scroll":
					value.Coverage["performedScrolls"] = 1
				case "int64-scroll":
					value.Coverage["performedScrolls"] = int64(1)
				case "int64-new":
					frontier["newCandidateCount"] = int64(1)
				case "nil-coverage":
					value.Coverage, want = nil, ""
				case "missing-frontier":
					delete(value.Coverage, "frontier")
					want = ""
				case "wrong-frontier":
					value.Coverage["frontier"], want = []any{}, ""
				case "typed-nil-frontier":
					value.Coverage["frontier"], want = map[string]any(nil), ""
					if local {
						want = policyExhaustedReason
					}
				case "missing-quality":
					delete(value.Coverage, "captureQuality")
					want = ""
				case "wrong-quality":
					value.Coverage["captureQuality"], want = "complete", ""
				case "degraded":
					value.Coverage["captureQuality"] = map[string]any{"verdict": "usable_degraded"}
					want = ""
				case "deadline":
					value.Coverage["scrollStopReason"], want = "deadline", ""
				case "trimmed":
					value.Coverage["captureQuality"] = map[string]any{"verdict": " complete "}
				}
				before := acquisitionPolicyJSON(t, value)
				reason, ok := localAcquisitionCompletionReason(source, value, eligible)
				if reason != want || ok != (want != "") {
					t.Fatalf("reason=%q ok=%t want=%q", reason, ok, want)
				}
				if acquisitionPolicyJSON(t, value) != before {
					t.Fatal("completion policy mutated input")
				}
			})
		}
	}
	// Facebook and Instagram retain completion without the LinkedIn quality/deadline requirement.
	for _, source := range []domain.Source{domain.SourceFacebook, domain.SourceInstagram} {
		value := acquisitionPolicyObservation(source)
		value.Coverage["frontier"] = map[string]any{}
		value.Coverage["captureQuality"] = nil
		value.Coverage["scrollStopReason"] = "deadline"
		if reason, ok := localAcquisitionCompletionReason(source, value, 0); !ok || reason != policyExhaustedReason {
			t.Fatalf("source=%s reason=%q ok=%t", source, reason, ok)
		}
	}
}

func TestEngineAcquisitionPolicyCharacterizationStructuredFallback(t *testing.T) {
	for _, mode := range []string{"complete", "wrong-source", "no-candidates", "blank-key", "duplicate-key", "trimmed", "scroll", "negative-scroll", "fractional-scroll", "missing-scroll", "wrong-scroll", "wrong-method", "wrong-stop", "missing-frontier", "null-frontier", "wrong-frontier", "typed-nil-frontier", "has-more", "wrong-more", "anchor", "blank-anchors", "wrong-anchors", "nil-coverage"} {
		t.Run(mode, func(t *testing.T) {
			value := acquisitionPolicyObservation(domain.SourceInstagram)
			value.Coverage["captureMethod"], value.Coverage["scrollStopReason"], value.Coverage["performedScrolls"] = "instagram_structured_feed_json", "structured_fallback", float64(0)
			frontier := map[string]any{"hasMoreCandidateSignal": false, "anchorKeys": []any{}}
			value.Coverage["frontier"] = frontier
			want := true
			switch mode {
			case "wrong-source":
				value.Source, want = domain.SourceFacebook, false
			case "no-candidates":
				value.Snapshots, want = nil, false
			case "blank-key":
				value.Snapshots[0].Blocks[0].EvidenceKey, want = " \t", false
			case "duplicate-key":
				value.Snapshots[0].Blocks = append(value.Snapshots[0].Blocks, value.Snapshots[0].Blocks[0])
			case "trimmed":
				value.Coverage["captureMethod"], value.Coverage["scrollStopReason"] = " instagram_structured_feed_json ", " structured_fallback "
			case "scroll":
				value.Coverage["performedScrolls"], want = 1, false
			case "negative-scroll":
				value.Coverage["performedScrolls"], want = -1, false
			case "fractional-scroll":
				value.Coverage["performedScrolls"] = 0.9
			case "missing-scroll":
				delete(value.Coverage, "performedScrolls")
			case "wrong-scroll":
				value.Coverage["performedScrolls"] = "1"
			case "wrong-method":
				value.Coverage["captureMethod"], want = true, false
			case "wrong-stop":
				value.Coverage["scrollStopReason"], want = "deadline", false
			case "missing-frontier":
				delete(value.Coverage, "frontier")
				want = false
			case "null-frontier":
				value.Coverage["frontier"], want = nil, false
			case "wrong-frontier":
				value.Coverage["frontier"], want = map[string]string{}, false
			case "typed-nil-frontier":
				value.Coverage["frontier"] = map[string]any(nil)
			case "has-more":
				frontier["hasMoreCandidateSignal"], want = true, false
			case "wrong-more":
				frontier["hasMoreCandidateSignal"] = "true"
			case "anchor":
				frontier["anchorKeys"], want = []string{" navigable "}, false
			case "blank-anchors":
				frontier["anchorKeys"] = []any{nil, 1, " "}
			case "wrong-anchors":
				frontier["anchorKeys"] = "navigable"
			case "nil-coverage":
				value.Coverage, want = nil, false
			}
			before := acquisitionPolicyJSON(t, value)
			reason, ok := localStructuredFallbackCompletionReason(value.Source, value)
			wantReason := ""
			if want {
				wantReason = policyStructuredReason
			}
			if ok != want || reason != wantReason {
				t.Fatalf("reason=%q ok=%t want=%t", reason, ok, want)
			}
			if acquisitionPolicyJSON(t, value) != before {
				t.Fatal("structured policy mutated input")
			}
		})
	}
}

func TestEngineAcquisitionPolicyCharacterizationX(t *testing.T) {
	for _, mode := range []string{"complete", "unknown", "linkedin", "zero-scroll", "wrong-scroll", "int64-scroll", "fractional-scroll", "missing-stop", "deadline", "trimmed", "missing-quality", "wrong-quality", "wrong-verdict", "degraded", "missing-frontier", "wrong-frontier", "nil-frontier", "no-more", "wrong-more", "zero-new", "wrong-new", "fractional-new", "int64-new", "missing-anchors", "wrong-anchors", "blank-anchors", "string-anchors", "not-ready", "wrong-ready", "null-ready", "no-snapshots", "nil-coverage"} {
		t.Run(mode, func(t *testing.T) {
			value := acquisitionPolicyObservation(domain.SourceX)
			frontier := value.Coverage["frontier"].(map[string]any)
			want := mode == "complete" || mode == "trimmed" || mode == "int64-scroll" || mode == "int64-new" || mode == "string-anchors"
			switch mode {
			case "unknown":
				value.Source = "unknown"
			case "linkedin":
				value.Source = domain.SourceLinkedIn
			case "zero-scroll":
				value.Coverage["performedScrolls"] = 0
			case "wrong-scroll":
				value.Coverage["performedScrolls"] = "1"
			case "int64-scroll":
				value.Coverage["performedScrolls"] = int64(1)
			case "fractional-scroll":
				value.Coverage["performedScrolls"] = 0.9
			case "missing-stop":
				delete(value.Coverage, "scrollStopReason")
			case "deadline":
				value.Coverage["scrollStopReason"] = "deadline"
			case "trimmed":
				value.Coverage["scrollStopReason"], value.Coverage["captureQuality"] = " budget_exhausted ", map[string]any{"verdict": " complete "}
			case "missing-quality":
				delete(value.Coverage, "captureQuality")
			case "wrong-quality":
				value.Coverage["captureQuality"] = "complete"
			case "wrong-verdict":
				value.Coverage["captureQuality"] = map[string]any{"verdict": true}
			case "degraded":
				value.Coverage["captureQuality"] = map[string]any{"verdict": "usable_degraded"}
			case "missing-frontier":
				delete(value.Coverage, "frontier")
			case "wrong-frontier":
				value.Coverage["frontier"] = []any{}
			case "nil-frontier":
				value.Coverage["frontier"] = map[string]any(nil)
			case "no-more":
				frontier["hasMoreCandidateSignal"] = false
			case "wrong-more":
				frontier["hasMoreCandidateSignal"] = "true"
			case "zero-new":
				frontier["newCandidateCount"] = 0
			case "wrong-new":
				frontier["newCandidateCount"] = "1"
			case "fractional-new":
				frontier["newCandidateCount"] = 0.9
			case "int64-new":
				frontier["newCandidateCount"] = int64(1)
			case "missing-anchors":
				delete(frontier, "anchorKeys")
			case "wrong-anchors":
				frontier["anchorKeys"] = "navigable"
			case "blank-anchors":
				frontier["anchorKeys"] = []any{nil, 1, " "}
			case "string-anchors":
				frontier["anchorKeys"] = []string{" native-id "}
			case "not-ready":
				frontier["continuationReady"] = false
			case "wrong-ready":
				frontier["continuationReady"] = "true"
			case "null-ready":
				frontier["continuationReady"] = nil
			case "no-snapshots":
				value.Snapshots = nil
			case "nil-coverage":
				value.Coverage = nil
			}
			before := acquisitionPolicyJSON(t, value)
			reason, ok := localXFollowUpReason(value.Source, value)
			wantReason := ""
			if want {
				wantReason = policyXReason
			}
			if ok != want || reason != wantReason {
				t.Fatalf("reason=%q ok=%t want=%t", reason, ok, want)
			}
			if acquisitionPolicyJSON(t, value) != before {
				t.Fatal("X policy mutated input")
			}
		})
	}
}

func TestEngineAcquisitionPolicyCharacterizationWrapper(t *testing.T) {
	value := acquisitionPolicyObservation(domain.SourceLinkedIn)
	value.Snapshots[0].Blocks = []domain.Block{{EvidenceKey: " one "}, {EvidenceKey: "one"}, {EvidenceKey: " "}}
	before := acquisitionPolicyJSON(t, value)
	if !localFrontierFinishesAcquisition(domain.SourceLinkedIn, value) {
		t.Fatal("wrapper lost raw candidate count")
	}
	if acquisitionPolicyJSON(t, value) != before {
		t.Fatal("wrapper mutated input")
	}
	if reason, ok := localAcquisitionCompletionReason(domain.SourceLinkedIn, value, 0); ok || reason != "" {
		t.Fatal("eligible count was replaced by raw count")
	}
	value.Snapshots[0].Blocks = []domain.Block{{EvidenceKey: " "}}
	if localFrontierFinishesAcquisition(domain.SourceLinkedIn, value) {
		t.Fatal("blank evidence counted toward target")
	}
}

type acquisitionPolicyProvider struct {
	reasoning.Deterministic
	planned, analyzed int
	analyzeFailure    error
}

func (p *acquisitionPolicyProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	p.planned++
	_, telemetry, err := p.Deterministic.Plan(ctx, run, observation, knowledge)
	return reasoning.AcquisitionPlan{Decision: "finish", Reason: "offline model decision"}, telemetry, err
}

func (p *acquisitionPolicyProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.analyzed++
	if p.analyzeFailure != nil {
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, p.analyzeFailure
	}
	return p.Deterministic.Analyze(ctx, run, observation, knowledge)
}

func TestEngineAcquisitionPolicyCharacterizationDailyGate(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		for _, mode := range []string{"issued", "coverage-only", "foreign-run"} {
			t.Run(string(source)+"/"+mode, func(t *testing.T) {
				f := newBridgeCommandFixture(t, source)
				ctx := context.Background()
				stop := errors.New("offline evaluation reached")
				provider := &acquisitionPolicyProvider{analyzeFailure: stop}
				f.e.provider = provider
				command := f.claim(t)
				value := acquisitionPolicyObservation(source)
				field, suffix := "dailyXAcquisitionPolicy", ""
				if source == domain.SourceInstagram {
					field, suffix = "dailyInstagramAcquisitionPolicy", ":instagram"
					value.Coverage["frontier"] = map[string]any{"newCandidateCount": float64(0), "hasMoreCandidateSignal": false}
				}
				value.Coverage[field] = "round_one_v1"
				if err := validateObservation(value); err != nil {
					t.Fatal(err)
				}
				if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, value); err != nil {
					t.Fatal(err)
				}
				// Only disposable fixture state supplies the private issuance record.
				if mode != "coverage-only" {
					if err := f.state.SetRunCoverageField(ctx, f.run.ID, field, "round_one_v1"); err != nil {
						t.Fatal(err)
					}
					issuedRun := f.run.ID
					if mode == "foreign-run" {
						issuedRun = "other-run"
					}
					f.exec(t, `INSERT INTO meta(key,value) VALUES(?,json_object('runScoped',json('true'),'runId',?))`, "manual_pilot_session_v1:"+f.session.ID+suffix, issuedRun)
				}
				selected, err := f.state.IsDailySourceCapture(ctx, f.run.ID)
				if err != nil || selected != (mode == "issued") {
					t.Fatalf("daily selected=%t err=%v", selected, err)
				}
				err = f.e.process(ctx, f.run.ID, true)
				run, loadErr := f.state.GetRun(ctx, f.run.ID)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				var commands int
				if queryErr := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&commands); queryErr != nil {
					t.Fatal(queryErr)
				}
				if mode == "issued" {
					if !errors.Is(err, stop) || commands != 1 || provider.planned != 0 || provider.analyzed != 1 || run.Coverage["acquisitionPlanning"] != nil {
						t.Fatalf("daily err=%v commands=%d run=%+v calls=%+v", err, commands, run, provider)
					}
				} else {
					receipt, _ := run.Coverage["acquisitionPlanning"].(map[string]any)
					if receipt["mode"] != "local_frontier" || provider.planned != 0 {
						t.Fatalf("ordinary policy lost: %v err=%v", receipt, err)
					}
					if source == domain.SourceX {
						if err != nil || commands != 2 || provider.analyzed != 0 {
							t.Fatalf("ordinary X err=%v commands=%d", err, commands)
						}
					} else if !errors.Is(err, stop) || commands != 1 || provider.analyzed != 1 {
						t.Fatalf("ordinary Instagram err=%v commands=%d", err, commands)
					}
				}
			})
		}
	}
}

func TestEngineAcquisitionPolicyCharacterizationProcess(t *testing.T) {
	for _, mode := range []string{"linkedin-target", "instagram-structured", "x-follow-up", "x-model", "planning-disabled", "single-round", "finish-receipt-error", "queue-error", "follow-up-receipt-error"} {
		t.Run(mode, func(t *testing.T) {
			source := domain.SourceLinkedIn
			if mode == "instagram-structured" {
				source = domain.SourceInstagram
			}
			if strings.HasPrefix(mode, "x-") || mode == "queue-error" || mode == "follow-up-receipt-error" {
				source = domain.SourceX
			}
			f := newBridgeCommandFixture(t, source)
			ctx := context.Background()
			settings, err := f.state.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.CalibrationEnabled, settings.AIDetectionEnabled = false, false
			if err := f.state.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			provider := &acquisitionPolicyProvider{}
			f.e.provider = provider
			if mode == "single-round" {
				f.e.config.Capture.MaxAcquisitionRounds = 1
			}
			command := f.claim(t)
			value := acquisitionPolicyObservation(source)
			if mode == "instagram-structured" {
				value.Coverage["performedScrolls"], value.Coverage["captureMethod"], value.Coverage["scrollStopReason"] = float64(0), "instagram_structured_feed_json", "structured_fallback"
				value.Coverage["frontier"] = map[string]any{"hasMoreCandidateSignal": false, "newCandidateCount": float64(1)}
			}
			if mode == "x-model" {
				value.Coverage["captureQuality"] = map[string]any{"verdict": "usable_degraded"}
			}
			if err := validateObservation(value); err != nil {
				t.Fatal(err)
			}
			if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, value); err != nil {
				t.Fatal(err)
			}
			if mode == "queue-error" {
				f.exec(t, `CREATE TRIGGER policy_queue_abort BEFORE INSERT ON bridge_commands WHEN json_extract(NEW.payload_json,'$.acquisitionRound')=2 BEGIN SELECT RAISE(ABORT,'offline queue rejected'); END`)
			}
			if mode == "finish-receipt-error" || mode == "follow-up-receipt-error" {
				f.exec(t, `CREATE TRIGGER policy_receipt_abort BEFORE UPDATE OF coverage_json ON runs WHEN json_type(NEW.coverage_json,'$.acquisitionPlanning')='object' BEGIN SELECT RAISE(ABORT,'offline receipt rejected'); END`)
			}
			err = f.e.process(ctx, f.run.ID, mode != "planning-disabled")
			var commands int
			if queryErr := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&commands); queryErr != nil {
				t.Fatal(queryErr)
			}
			run, loadErr := f.state.GetRun(ctx, f.run.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if strings.HasSuffix(mode, "error") {
				wantPrefix, wantCount, wantStatus := "save local acquisition planning receipt: ", 1, "reasoning"
				if mode == "queue-error" {
					wantPrefix = "constraint failed: offline queue rejected"
				}
				if mode == "follow-up-receipt-error" {
					wantPrefix, wantCount, wantStatus = "save local X acquisition planning receipt: ", 2, "waiting_for_bridge"
				}
				if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) || commands != wantCount || run.Status != wantStatus || run.Coverage["acquisitionPlanning"] != nil || provider.planned != 0 || provider.analyzed != 0 {
					t.Fatalf("err=%v commands=%d run=%+v calls=%+v", err, commands, run, provider)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			receipt, _ := run.Coverage["acquisitionPlanning"].(map[string]any)
			if mode == "planning-disabled" || mode == "single-round" {
				if receipt != nil || commands != 1 || provider.planned != 0 || provider.analyzed != 1 || run.Status != "completed" {
					t.Fatalf("gated run=%+v calls=%+v", run, provider)
				}
				return
			}
			wantMode, wantDecision, wantReason, wantQueued := "local_frontier", "finish", policyTargetReason, false
			wantPlans, wantAnalyses, wantCommands := 0, 1, 1
			if mode == "instagram-structured" {
				wantMode, wantReason = "structured_fallback", policyStructuredReason
			}
			if mode == "x-model" {
				wantMode, wantReason, wantPlans = "model", "offline model decision", 1
			}
			if mode == "x-follow-up" {
				wantDecision, wantReason, wantQueued, wantAnalyses, wantCommands = "request_follow_up", policyXReason, true, 0, 2
			}
			want := map[string]any{"mode": wantMode, "decision": wantDecision, "reason": wantReason, "followUpQueued": wantQueued, "followUpNewCandidates": float64(0)}
			if !reflect.DeepEqual(receipt, want) || provider.planned != wantPlans || provider.analyzed != wantAnalyses || commands != wantCommands {
				t.Fatalf("receipt=%v want=%v calls=%+v commands=%d", receipt, want, provider, commands)
			}
			if mode == "x-follow-up" {
				next := f.claim(t)
				wantContinuation := map[string]any{"startScrollY": float64(100), "anchorKeys": []any{"bounded-native-id"}, "settleMs": float64(900)}
				if next.Payload["acquisitionRound"] != float64(2) || !reflect.DeepEqual(next.Payload["continuation"], wantContinuation) || next.Payload["sameTabMutationAllowed"] != false || next.Payload["sourceFreshnessPolicy"] != "preserve_frontier" {
					t.Fatalf("follow-up payload=%v", next.Payload)
				}
			}
		})
	}
}
