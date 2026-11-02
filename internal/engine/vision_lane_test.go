package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func visionLaneJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func visionLaneBlock(id, text string) domain.Block {
	return domain.Block{EvidenceKey: "instagram:p:" + id, PlatformID: "instagram:p:" + id, Permalink: "https://www.instagram.com/p/" + id + "/", Author: "Fixture", Text: text, Media: []map[string]any{{"kind": "image", "url": "https://instagram.example.fbcdn.net/" + id + ".jpg"}}}
}

func TestEngineVisionLaneCharacterizationRouting(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceInstagram, domain.SourceFacebook, domain.SourceLinkedIn, domain.SourceX, "unknown"} {
		for _, mode := range []string{"media-only", "whitespace", "caption", "video", "untrusted", "suffix-spoof", "http", "credentials", "port", "poster", "invalid-url-with-poster", "attachments-only", "no-media"} {
			t.Run(string(source)+"/"+mode, func(t *testing.T) {
				candidate := visionLaneBlock("visual", "")
				eligible := source == domain.SourceInstagram
				media := candidate.Media[0]
				switch mode {
				case "whitespace":
					candidate.Text = " \t\n"
				case "caption":
					candidate.Text, eligible = "Authored caption", false
				case "video":
					media["kind"], media["url"] = "video", "https://scontent.cdninstagram.com/visual.mp4"
				case "untrusted":
					media["url"], eligible = "https://evil.example/visual.jpg", false
				case "suffix-spoof":
					media["url"], eligible = "https://fbcdn.net.evil.example/visual.jpg", false
				case "http":
					media["url"], eligible = "http://instagram.example.fbcdn.net/visual.jpg", false
				case "credentials":
					media["url"], eligible = "https://user@instagram.example.fbcdn.net/visual.jpg", false
				case "port":
					media["url"], eligible = "https://instagram.example.fbcdn.net:443/visual.jpg", false
				case "poster":
					media["url"], media["posterUrl"] = " ", "https://scontent.cdninstagram.com/poster.jpg"
				case "invalid-url-with-poster":
					media["url"], media["posterUrl"], eligible = "https://evil.example/visual.jpg", "https://scontent.cdninstagram.com/poster.jpg", false
				case "attachments-only":
					candidate.Media, candidate.Attachments, eligible = nil, []domain.Attachment{{}}, false
				case "no-media":
					candidate.Media, eligible = nil, false
				}
				first, last := visionLaneBlock("first", "First text candidate"), visionLaneBlock("last", "Last text candidate")
				value := domain.Observation{Source: source, PageURL: "https://source.example/feed", PageTitle: "Fixture page", CapturedAt: "fixed-time", Coverage: map[string]any{"marker": "preserved"}, Snapshots: []domain.Snapshot{
					{Index: 4, ScrollY: 12},
					{Index: 8, ScrollY: 99, AdapterVersion: "fixture-adapter", SelectorCounts: map[string]int{"articles": 3}, Blocks: []domain.Block{first, candidate, candidate, last}},
				}}
				before := visionLaneJSON(t, value)
				got, held := splitVisionLaneCandidates(value)
				wantBlocks, wantHeld := value.Snapshots[1].Blocks, []domain.VisionEvaluationInput{}
				if eligible {
					wantBlocks = []domain.Block{first, last}
					wantHeld = []domain.VisionEvaluationInput{{EvidenceKey: candidate.EvidenceKey, Candidate: candidate}, {EvidenceKey: candidate.EvidenceKey, Candidate: candidate}}
				}
				want := value
				want.Snapshots = []domain.Snapshot{value.Snapshots[0], value.Snapshots[1]}
				want.Snapshots[0].Blocks, want.Snapshots[1].Blocks = []domain.Block{}, wantBlocks
				if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(held, wantHeld) {
					t.Fatalf("got=%+v held=%+v want=%+v held=%+v", got, held, want, wantHeld)
				}
				if visionLaneJSON(t, value) != before {
					t.Fatal("routing mutated input")
				}
			})
		}
	}
}

func TestEngineVisionLaneCharacterizationEmptyAndCopySemantics(t *testing.T) {
	for _, mode := range []string{"nil-snapshots", "empty-snapshots", "nil-blocks", "empty-blocks", "all-held"} {
		t.Run(mode, func(t *testing.T) {
			value := domain.Observation{Source: domain.SourceInstagram}
			switch mode {
			case "empty-snapshots":
				value.Snapshots = []domain.Snapshot{}
			case "nil-blocks":
				value.Snapshots = []domain.Snapshot{{}}
			case "empty-blocks":
				value.Snapshots = []domain.Snapshot{{Blocks: []domain.Block{}}}
			case "all-held":
				value.Snapshots = []domain.Snapshot{{Blocks: []domain.Block{visionLaneBlock("visual", "")}}}
			}
			got, held := splitVisionLaneCandidates(value)
			wantHeld := 0
			if mode == "all-held" {
				wantHeld = 1
			}
			if got.Snapshots == nil || held == nil || len(got.Snapshots) != len(value.Snapshots) || len(held) != wantHeld || got.Coverage != nil {
				t.Fatalf("got=%+v held=%+v", got, held)
			}
			for _, snapshot := range got.Snapshots {
				if snapshot.Blocks == nil || len(snapshot.Blocks) != 0 {
					t.Fatalf("empty snapshot=%+v", snapshot)
				}
			}
		})
	}
	text, media := visionLaneBlock("caption", "Original text"), visionLaneBlock("visual", "")
	media.EvidenceKey = " raw key "
	value := domain.Observation{Source: domain.SourceInstagram, Coverage: map[string]any{"before": true}, Snapshots: []domain.Snapshot{{ScrollY: 10, SelectorCounts: map[string]int{"count": 2}, Blocks: []domain.Block{text, media}}}}
	before := visionLaneJSON(t, value)
	got, held := splitVisionLaneCandidates(value)
	if visionLaneJSON(t, value) != before || held[0].EvidenceKey != " raw key " {
		t.Fatal("split mutated or normalized input")
	}
	got.Snapshots[0].ScrollY, got.Snapshots[0].Blocks[0].Text, held[0].Candidate.EvidenceKey = 20, "Changed copy", "Changed copy"
	if value.Snapshots[0].ScrollY != 10 || value.Snapshots[0].Blocks[0].Text != "Original text" || value.Snapshots[0].Blocks[1].EvidenceKey != " raw key " {
		t.Fatal("outer snapshot/block storage was shared")
	}
	// The caller writes coverage after splitting and before assigning the result.
	value.Coverage["visionEvaluation"] = map[string]any{"pending": 1}
	if !reflect.DeepEqual(got.Coverage["visionEvaluation"], value.Coverage["visionEvaluation"]) {
		t.Fatal("caller receipt alias was broken")
	}
	got.Coverage["shared"] = true
	got.Snapshots[0].SelectorCounts["count"] = 3
	held[0].Candidate.Media[0]["marker"] = "shared"
	if value.Coverage["shared"] != true || value.Snapshots[0].SelectorCounts["count"] != 3 || value.Snapshots[0].Blocks[1].Media[0]["marker"] != "shared" {
		t.Fatal("nested shallow aliases changed")
	}
}

func TestEngineVisionLaneCharacterizationSnapshotOrder(t *testing.T) {
	firstText, firstMedia := visionLaneBlock("firstText", "First caption"), visionLaneBlock("firstMedia", "")
	lastText, lastMedia := visionLaneBlock("lastText", "Last caption"), visionLaneBlock("lastMedia", "")
	value := domain.Observation{Source: domain.SourceInstagram, Coverage: map[string]any{"existing": "retained"}, Snapshots: []domain.Snapshot{
		{Index: 7, ScrollY: 100, Blocks: []domain.Block{firstMedia, firstText}},
		{Index: 3, ScrollY: 20, Blocks: []domain.Block{lastText, lastMedia}},
	}}
	before := visionLaneJSON(t, value)
	got, held := splitVisionLaneCandidates(value)
	want := value
	want.Snapshots = []domain.Snapshot{value.Snapshots[0], value.Snapshots[1]}
	want.Snapshots[0].Blocks, want.Snapshots[1].Blocks = []domain.Block{firstText}, []domain.Block{lastText}
	wantHeld := []domain.VisionEvaluationInput{{EvidenceKey: firstMedia.EvidenceKey, Candidate: firstMedia}, {EvidenceKey: lastMedia.EvidenceKey, Candidate: lastMedia}}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(held, wantHeld) || visionLaneJSON(t, value) != before {
		t.Fatalf("snapshot order changed: observation=%+v held=%+v", got, held)
	}
}

func TestEngineVisionLaneCharacterizationCoverage(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(fmt.Sprint(zero), func(t *testing.T) {
			value := domain.InboxVisionEvaluation{Policy: "manual_review", Available: true, Availability: "fixture availability", Capacity: 1, Active: 2, Pending: 3, Deferred: 4, Evaluating: 5, Retrying: 6, Ready: 7, Failed: 8, Jobs: []domain.VisionEvaluationJob{{ID: "must-not-leak"}}}
			want := map[string]any{"policy": "manual_review", "available": true, "availability": "fixture availability", "capacity": 1, "active": 2, "pending": 3, "deferred": 4, "evaluating": 5, "retrying": 6, "ready": 7, "failed": 8}
			if zero {
				value = domain.InboxVisionEvaluation{}
				want = map[string]any{"policy": "", "available": false, "availability": "", "capacity": 0, "active": 0, "pending": 0, "deferred": 0, "evaluating": 0, "retrying": 0, "ready": 0, "failed": 0}
			}
			before := visionLaneJSON(t, value)
			got := visionEvaluationCoverage(value)
			if !reflect.DeepEqual(got, want) || len(got) != 11 || visionLaneJSON(t, value) != before {
				t.Fatalf("coverage=%v want=%v", got, want)
			}
			got["policy"] = "changed"
			if !reflect.DeepEqual(visionEvaluationCoverage(value), want) {
				t.Fatal("coverage maps were shared between calls")
			}
		})
	}
}

type visionLaneProvider struct {
	reasoning.Deterministic
	t                 *testing.T
	fixture           bridgeCommandFixture
	planned, analyzed int
	wantVision        bool
}

func (p *visionLaneProvider) verify(ctx context.Context, observation domain.Observation) {
	p.t.Helper()
	keys := []string{}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			keys = append(keys, block.EvidenceKey)
		}
	}
	if !reflect.DeepEqual(keys, []string{"instagram:p:caption"}) {
		p.t.Fatalf("text provider received keys=%v", keys)
	}
	run, err := p.fixture.state.GetRun(ctx, p.fixture.run.ID)
	if err != nil {
		p.t.Fatal(err)
	}
	jobs, err := p.fixture.state.ListVisionEvaluations(ctx, run.ID, run.Source)
	if err != nil {
		p.t.Fatal(err)
	}
	if p.wantVision {
		if len(jobs) != 1 || run.Coverage["visionEvaluation"] == nil || observation.Coverage["visionEvaluation"] == nil {
			p.t.Fatalf("reasoning preceded durable vision receipt: jobs=%v coverage=%v", jobs, run.Coverage)
		}
	} else if len(jobs) != 0 || run.Coverage["visionEvaluation"] != nil || observation.Coverage["visionEvaluation"] != nil {
		p.t.Fatal("text-only capture unexpectedly entered vision lane")
	}
}

func (p *visionLaneProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	p.planned++
	p.verify(ctx, observation)
	_, telemetry, err := p.Deterministic.Plan(ctx, run, observation, knowledge)
	return reasoning.AcquisitionPlan{Decision: "finish", Reason: "offline bounded text evaluation"}, telemetry, err
}

func (p *visionLaneProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.analyzed++
	p.verify(ctx, observation)
	return p.Deterministic.Analyze(ctx, run, observation, knowledge)
}

func TestEngineVisionLaneCharacterizationProcess(t *testing.T) {
	for _, mode := range []string{"media-only", "mixed", "text-only", "capacity", "enqueue-error", "receipt-error"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t, domain.SourceInstagram)
			ctx := context.Background()
			settings, err := f.state.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.CalibrationEnabled, settings.AIDetectionEnabled = false, false
			if err := f.state.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			provider := &visionLaneProvider{t: t, fixture: f, wantVision: mode == "mixed"}
			f.e.provider = provider
			command := f.claim(t)
			blocks := []domain.Block{visionLaneBlock("visual", "")}
			if mode == "mixed" {
				blocks = append(blocks, visionLaneBlock("caption", "Independent text-bearing source evidence stays in text reasoning."))
			}
			if mode == "text-only" {
				blocks = []domain.Block{visionLaneBlock("caption", "Independent text-bearing source evidence stays in text reasoning.")}
			}
			if mode == "capacity" {
				for i := 1; i < 10; i++ {
					blocks = append(blocks, visionLaneBlock(fmt.Sprintf("visual%d", i), ""))
				}
			}
			value := domain.Observation{Source: domain.SourceInstagram, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: blocks}}, Coverage: map[string]any{"performedScrolls": float64(0)}}
			if err := validateObservation(value); err != nil {
				t.Fatal(err)
			}
			if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, value); err != nil {
				t.Fatal(err)
			}
			if mode == "enqueue-error" {
				f.exec(t, `CREATE TRIGGER vision_enqueue_abort BEFORE INSERT ON vision_evaluation_jobs BEGIN SELECT RAISE(ABORT,'offline vision enqueue rejected'); END`)
			}
			if mode == "receipt-error" {
				f.exec(t, `CREATE TRIGGER vision_receipt_abort BEFORE UPDATE OF coverage_json ON runs WHEN json_type(NEW.coverage_json,'$.visionEvaluation')='object' BEGIN SELECT RAISE(ABORT,'offline vision receipt rejected'); END`)
			}
			err = f.e.process(ctx, f.run.ID, true)
			run, loadErr := f.state.GetRun(ctx, f.run.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			jobs, loadErr := f.state.ListVisionEvaluations(ctx, run.ID, run.Source)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			var commands int
			if queryErr := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, run.ID).Scan(&commands); queryErr != nil {
				t.Fatal(queryErr)
			}
			if commands != 1 {
				t.Fatalf("unexpected follow-up commands=%d", commands)
			}
			if strings.HasSuffix(mode, "error") {
				prefix, message, wantJobs := "enqueue media-only vision candidates: ", "offline vision enqueue rejected", 0
				if mode == "receipt-error" {
					prefix, message, wantJobs = "save media-only vision queue receipt: ", "offline vision receipt rejected", 1
				}
				if err == nil || !strings.HasPrefix(err.Error(), prefix) || !strings.Contains(err.Error(), message) || len(jobs) != wantJobs || run.Status != "reasoning" || run.Coverage["visionEvaluation"] != nil || run.Coverage["acquisitionPlanning"] != nil || provider.planned != 0 || provider.analyzed != 0 {
					t.Fatalf("err=%v jobs=%v run=%+v calls=%d/%d", err, jobs, run, provider.planned, provider.analyzed)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 0
			if mode == "mixed" || mode == "text-only" {
				wantCalls = 1
			}
			if provider.planned != wantCalls || provider.analyzed != wantCalls || run.Status != "completed" {
				t.Fatalf("calls=%d/%d status=%s", provider.planned, provider.analyzed, run.Status)
			}
			wantJobs, wantPending, wantDeferred := 1, 1, 0
			if mode == "capacity" {
				wantJobs, wantPending, wantDeferred = 10, 8, 2
			}
			if mode == "text-only" {
				if len(jobs) != 0 || run.Coverage["visionEvaluation"] != nil {
					t.Fatal("text-only run queued vision work")
				}
				return
			}
			if len(jobs) != wantJobs {
				t.Fatalf("jobs=%d want=%d", len(jobs), wantJobs)
			}
			coverage, _ := run.Coverage["visionEvaluation"].(map[string]any)
			wantCoverage := map[string]any{"policy": "manual_review", "available": false, "availability": "Generated locally from trusted media metadata. No image is sent to an AI provider.", "capacity": float64(8), "active": float64(wantPending), "pending": float64(wantPending), "deferred": float64(wantDeferred), "evaluating": float64(0), "retrying": float64(0), "ready": float64(0), "failed": float64(0)}
			if !reflect.DeepEqual(coverage, wantCoverage) {
				t.Fatalf("coverage=%v want=%v", coverage, wantCoverage)
			}
			if wantCalls == 0 {
				session, err := f.state.GetSession(ctx, f.session.ID)
				if err != nil || len(session.Items) != 0 {
					t.Fatalf("media-only session=%+v err=%v", session, err)
				}
				receipt, _ := run.Coverage["acquisitionPlanning"].(map[string]any)
				if receipt["mode"] != "continuity_bypass" || receipt["decision"] != "finish" {
					t.Fatalf("media-only receipt=%v", receipt)
				}
			}
		})
	}
}
