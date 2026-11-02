package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func reasoningResultJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestEngineReasoningResultCharacterizationURLs(t *testing.T) {
	for _, source := range []struct {
		name      string
		source    domain.Source
		raw, want string
	}{
		{"x", domain.SourceX, "https://x.com/a/status/42/photo/1?tracking=fixture#detail", "https://x.com/a/status/42"},
		{"linkedin", domain.SourceLinkedIn, "https://www.linkedin.com/feed/update/urn:li:activity:42", "https://www.linkedin.com/feed/update/urn:li:activity:42"},
		{"instagram", domain.SourceInstagram, "https://www.instagram.com/p/FIXTURE42/", "https://www.instagram.com/p/FIXTURE42/"},
		{"facebook", domain.SourceFacebook, "https://m.facebook.com/watch/?v=42&tracking=fixture", "https://www.facebook.com/watch/?v=42"},
	} {
		for _, mode := range []string{"valid", "external", "absent", "other-source", "unknown-key"} {
			t.Run(source.name+"/"+mode, func(t *testing.T) {
				raw, want := source.raw, source.want
				switch mode {
				case "external":
					raw = "https://outside.example/post"
					want = ""
				case "absent":
					raw = ""
					want = ""
				case "other-source":
					raw = "https://x.com/a/status/43"
					if source.source == domain.SourceX {
						raw = "https://www.instagram.com/p/FIXTURE43/"
					}
					want = ""
				case "unknown-key":
					want = ""
				}
				observation := domain.Observation{Source: source.source, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "known", Permalink: raw}}}}}
				key := "known"
				if mode == "unknown-key" {
					key = "unknown"
				}
				original := domain.ReasonedItem{ID: "keep-id", EvidenceKey: key, Source: domain.SourceLinkedIn, SourceURL: "https://model.example/invented", SourceURLKind: "external_reference", WhatChanged: "keep body", Author: "keep author"}
				result := domain.ReasoningResult{Summary: "keep summary", Items: []domain.ReasonedItem{original}, CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: key}}, Limitations: []string{"keep limitation"}}
				before, observed := reasoningResultJSON(t, result), reasoningResultJSON(t, observation)
				alias := result.Items
				bindReasoningSourceURLs(observation, &result)
				item := result.Items[0]
				kind := ""
				if want != "" {
					kind = "native_post"
				}
				if item.Source != source.source || item.SourceURL != want || item.SourceURLKind != kind || alias[0].SourceURL != want {
					t.Fatalf("item=%+v want=%q", item, want)
				}
				result.Items[0].Source, result.Items[0].SourceURL, result.Items[0].SourceURLKind = original.Source, original.SourceURL, original.SourceURLKind
				if reasoningResultJSON(t, result) != before || reasoningResultJSON(t, observation) != observed {
					t.Fatal("binding changed unrelated fields or captured evidence")
				}
			})
		}
	}
	for _, mode := range []string{"later-valid", "later-invalid", "no-valid"} {
		t.Run("duplicate-key/"+mode, func(t *testing.T) {
			first, last, want := "https://x.com/a/status/41", "https://x.com/b/status/42", "https://x.com/b/status/42"
			if mode == "later-invalid" {
				last = "https://outside.example/post"
				want = first
			}
			if mode == "no-valid" {
				first = ""
				last = "https://outside.example/post"
				want = ""
			}
			observation := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "same", Permalink: first}}}, {Blocks: []domain.Block{{EvidenceKey: "same", Permalink: last}}}}}
			result := domain.ReasoningResult{Items: []domain.ReasonedItem{{EvidenceKey: "same"}, {EvidenceKey: "same"}}}
			bindReasoningSourceURLs(observation, &result)
			if result.Items[0].SourceURL != want || result.Items[1].SourceURL != want {
				t.Fatalf("duplicate URLs=%+v", result.Items)
			}
		})
	}
	var result domain.ReasoningResult
	bindReasoningSourceURLs(domain.Observation{}, &result)
	if result.Items != nil {
		t.Fatal("empty binding allocated items")
	}
}

func TestEngineReasoningResultCharacterizationValidation(t *testing.T) {
	for _, mode := range []string{"complete", "unknown-assessment", "duplicate-assessment", "incomplete", "unknown-item", "unknown-before-duplicate", "duplicate-before-incomplete", "incomplete-before-item", "no-items", "duplicate-items", "duplicate-captured-key", "empty", "blank-key"} {
		t.Run(mode, func(t *testing.T) {
			observation := domain.Observation{Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "a"}, {EvidenceKey: "b"}}}}}
			result := domain.ReasoningResult{CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "a"}, {EvidenceKey: "b"}}, Items: []domain.ReasonedItem{{EvidenceKey: "a"}}}
			want := ""
			switch mode {
			case "unknown-assessment":
				result.CandidateAssessments[0].EvidenceKey = "foreign"
				want = "assessment references unknown evidence foreign"
			case "duplicate-assessment", "duplicate-before-incomplete":
				result.CandidateAssessments[1].EvidenceKey = "a"
				want = "duplicate assessment a"
			case "incomplete", "incomplete-before-item":
				result.CandidateAssessments = result.CandidateAssessments[:1]
				result.Items[0].EvidenceKey = "foreign"
				want = "reasoning assessed 1 of 2 candidates"
			case "unknown-item":
				result.Items[0].EvidenceKey = "foreign"
				want = "item references unknown evidence foreign"
			case "unknown-before-duplicate":
				result.CandidateAssessments = []domain.CandidateAssessment{{EvidenceKey: "foreign"}, {EvidenceKey: "a"}, {EvidenceKey: "a"}}
				want = "assessment references unknown evidence foreign"
			case "no-items":
				result.Items = nil
			case "duplicate-items":
				result.Items = append(result.Items, result.Items[0])
			case "duplicate-captured-key":
				observation.Snapshots = append(observation.Snapshots, observation.Snapshots[0])
			case "empty":
				observation = domain.Observation{}
				result = domain.ReasoningResult{}
			case "blank-key":
				observation.Snapshots[0].Blocks = []domain.Block{{}}
				result.CandidateAssessments = []domain.CandidateAssessment{{}}
				result.Items = []domain.ReasonedItem{{}}
			}
			before := reasoningResultJSON(t, []any{observation, result})
			err := validateReasoning(observation, result)
			if want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != want {
				t.Fatalf("err=%v want=%q", err, want)
			}
			if reasoningResultJSON(t, []any{observation, result}) != before {
				t.Fatal("validation mutated input")
			}
		})
	}
}

func TestEngineReasoningResultCharacterizationSelection(t *testing.T) {
	for _, mode := range []string{"ordered", "no-selected", "no-item", "nil-coverage", "duplicate-selected"} {
		t.Run(mode, func(t *testing.T) {
			run := domain.Run{ID: "run-owner", SessionID: "session-owner", Source: domain.SourceFacebook}
			result := domain.ReasoningResult{Items: []domain.ReasonedItem{{ID: "first", EvidenceKey: "a", Source: domain.SourceX}, {ID: "b-item", EvidenceKey: "b", Source: domain.SourceLinkedIn}, {ID: "last", EvidenceKey: "a", Source: domain.SourceInstagram}}}
			scored := []store.ScoredAssessment{{Assessment: domain.CandidateAssessment{EvidenceKey: "b", Rationale: "b rationale"}, Selected: true}, {Assessment: domain.CandidateAssessment{EvidenceKey: "a"}, Selected: false}, {Assessment: domain.CandidateAssessment{EvidenceKey: "absent"}, Selected: true}, {Assessment: domain.CandidateAssessment{EvidenceKey: "a", Rationale: "a rationale"}, Selected: true}}
			coverage := map[string]any{"receipt": "shared"}
			wantKeys := []string{"b", "a"}
			switch mode {
			case "no-selected":
				for i := range scored {
					scored[i].Selected = false
				}
				wantKeys = nil
			case "no-item":
				result.Items = nil
				wantKeys = nil
			case "nil-coverage":
				coverage = nil
			case "duplicate-selected":
				scored = append(scored, scored[0])
				wantKeys = append(wantKeys, "b")
			}
			before := reasoningResultJSON(t, []any{run, result, scored, coverage})
			items := selectedItems(run, result, scored, coverage)
			var keys []string
			ids := map[string]bool{}
			for _, item := range items {
				keys = append(keys, item.EvidenceKey)
				if !regexp.MustCompile(`^timeline_[0-9a-f]{32}$`).MatchString(item.ID) || ids[item.ID] {
					t.Fatalf("ID=%q", item.ID)
				}
				ids[item.ID] = true
				if item.SessionID != run.SessionID || item.RunID != run.ID || item.Source != run.Source || item.Rank != 0 || item.CreatedAt != "" || item.Evidence != nil {
					t.Fatalf("ownership/defaults=%+v", item)
				}
				if item.EvidenceKey == "a" && (item.Item.ID != "last" || item.Item.Source != domain.SourceInstagram || item.Assessment.Rationale != "a rationale") {
					t.Fatalf("last item/assessment=%+v", item)
				}
				if item.EvidenceKey == "b" && (item.Item.ID != "b-item" || item.Assessment.Rationale != "b rationale") {
					t.Fatalf("b item=%+v", item)
				}
				if !reflect.DeepEqual(item.Coverage, coverage) {
					t.Fatal("coverage changed")
				}
			}
			if !reflect.DeepEqual(keys, wantKeys) || (wantKeys == nil && items != nil) {
				t.Fatalf("keys=%v want=%v", keys, wantKeys)
			}
			if reasoningResultJSON(t, []any{run, result, scored, coverage}) != before {
				t.Fatal("mapping mutated input")
			}
			if len(items) > 0 && coverage != nil {
				items[0].Coverage["alias"] = "retained"
				if coverage["alias"] != "retained" || items[len(items)-1].Coverage["alias"] != "retained" {
					t.Fatal("coverage alias changed")
				}
			}
		})
	}
}

type resultCharacterizationProvider struct {
	reasoning.Deterministic
	mode     string
	returned domain.ReasoningResult
}

var errResultCharacterizationProvider = errors.New("offline reasoning result error")

func (p *resultCharacterizationProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	result, telemetry, err := p.Deterministic.Analyze(ctx, run, observation, knowledge)
	for i := range result.Items {
		result.Items[i].Source = domain.SourceX
		result.Items[i].SourceURL = "https://model.example/invented"
		result.Items[i].SourceURLKind = "external_reference"
	}
	for i := range result.CandidateAssessments {
		a := &result.CandidateAssessments[i]
		a.Novelty = 1
		a.Urgency = 1
		a.Actionability = 1
		a.Materiality = 1
		a.EvidenceStrength = 1
	}
	result.CandidateAssessments[0], result.CandidateAssessments[1] = result.CandidateAssessments[1], result.CandidateAssessments[0]
	switch p.mode {
	case "provider-error":
		err = errResultCharacterizationProvider
	case "unknown-assessment":
		result.CandidateAssessments[0].EvidenceKey = "foreign"
	case "duplicate-assessment":
		result.CandidateAssessments[1].EvidenceKey = result.CandidateAssessments[0].EvidenceKey
	case "incomplete":
		result.CandidateAssessments = result.CandidateAssessments[:1]
	case "unknown-item":
		result.Items[0].EvidenceKey = "foreign"
	}
	p.returned = result
	return result, telemetry, err
}

func TestEngineReasoningResultCharacterizationProcess(t *testing.T) {
	for _, mode := range []string{"valid", "provider-error", "unknown-assessment", "duplicate-assessment", "incomplete", "unknown-item", "write-error"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t, domain.SourceLinkedIn)
			settings, err := f.state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings.PreferenceEligibilityMode = "rank_only"
			settings.CalibrationEnabled = false
			settings.AIDetectionEnabled = false
			settings.MaxItemsPerSource = 4
			if err := f.state.SaveSettings(context.Background(), settings); err != nil {
				t.Fatal(err)
			}
			command := f.claim(t)
			observation := domain.Observation{Source: domain.SourceLinkedIn, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{
				{EvidenceKey: "linkedin:a", Text: "First captured item has independent primary evidence.", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:41"},
				{EvidenceKey: "linkedin:b", Text: "Second captured item has distinct primary evidence.", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:42"},
			}}}, Coverage: map[string]any{"status": "complete", "observedBlockCount": 2}}
			if err := validateObservation(observation); err != nil {
				t.Fatal(err)
			}
			// Persist through the real capture fence, then invoke process synchronously.
			if err := f.state.SaveObservation(context.Background(), command.ID, f.run.ID, observation); err != nil {
				t.Fatal(err)
			}
			provider := &resultCharacterizationProvider{mode: mode}
			f.e.provider = provider
			// Completion must follow evaluation telemetry, and the transaction must roll back on item failure.
			f.exec(t, `CREATE TRIGGER result_completion_order BEFORE INSERT ON candidate_assessments WHEN NOT EXISTS(SELECT 1 FROM reasoning_invocations WHERE run_id=NEW.run_id AND phase='candidate_evaluation') BEGIN SELECT RAISE(ABORT,'completion preceded telemetry'); END`)
			if mode == "write-error" {
				f.exec(t, `CREATE TRIGGER result_write_abort BEFORE INSERT ON timeline_items BEGIN SELECT RAISE(ABORT,'offline result persistence rejected'); END`)
			}
			err = f.e.process(context.Background(), f.run.ID, false)
			want := ""
			switch mode {
			case "provider-error":
				if !errors.Is(err, errResultCharacterizationProvider) {
					t.Fatalf("provider error=%v", err)
				}
			case "unknown-assessment":
				want = "assessment references unknown evidence foreign"
			case "duplicate-assessment":
				want = "duplicate assessment linkedin:b"
			case "incomplete":
				want = "reasoning assessed 1 of 2 candidates"
			case "unknown-item":
				want = "item references unknown evidence foreign"
			case "write-error":
				if err == nil || !strings.Contains(err.Error(), "offline result persistence rejected") {
					t.Fatalf("write error=%v", err)
				}
			}
			if want != "" && (err == nil || err.Error() != want) {
				t.Fatalf("err=%v want=%q", err, want)
			}
			if mode == "valid" && err != nil {
				t.Fatal(err)
			}
			if len(provider.returned.Items) != 2 {
				t.Fatal("provider did not run")
			}
			if mode != "provider-error" {
				if provider.returned.Items[1].Source != domain.SourceLinkedIn || provider.returned.Items[1].SourceURL != "https://www.linkedin.com/feed/update/urn:li:activity:42" {
					t.Fatal("binding did not precede validation/completion")
				}
			} else if provider.returned.Items[1].SourceURL != "https://model.example/invented" {
				t.Fatal("binding ran after provider error")
			}
			var invocations, assessments, items int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM reasoning_invocations WHERE run_id=? AND phase='candidate_evaluation'`, f.run.ID).Scan(&invocations); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM candidate_assessments WHERE run_id=?`, f.run.ID).Scan(&assessments); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, f.run.ID).Scan(&items); err != nil {
				t.Fatal(err)
			}
			wantCount := 0
			if mode == "valid" {
				wantCount = 2
			}
			if invocations != 1 || assessments != wantCount || items != wantCount {
				t.Fatalf("telemetry=%d assessments=%d items=%d", invocations, assessments, items)
			}
			run, err := f.state.GetRun(context.Background(), f.run.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := "reasoning"
			if mode == "valid" {
				wantStatus = "completed"
			}
			if run.Status != wantStatus {
				t.Fatalf("run=%s want=%s", run.Status, wantStatus)
			}
			if mode == "valid" {
				rows, err := f.db.Query(`SELECT evidence_key,rank,item_json,coverage_json FROM timeline_items WHERE run_id=? ORDER BY rank`, f.run.ID)
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				var keys []string
				for rows.Next() {
					var key, raw, coverageRaw string
					var rank int
					if err := rows.Scan(&key, &rank, &raw, &coverageRaw); err != nil {
						t.Fatal(err)
					}
					if rank != len(keys) {
						t.Fatal("Store rank changed")
					}
					keys = append(keys, key)
					var item domain.ReasonedItem
					if err := json.Unmarshal([]byte(raw), &item); err != nil {
						t.Fatal(err)
					}
					if item.Source != domain.SourceLinkedIn || !strings.HasPrefix(item.SourceURL, "https://www.linkedin.com/feed/update/") {
						t.Fatalf("item=%+v", item)
					}
					if !strings.Contains(coverageRaw, `"status":"complete"`) {
						t.Fatalf("coverage=%s", coverageRaw)
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(keys, []string{"linkedin:b", "linkedin:a"}) {
					t.Fatalf("selection order=%v", keys)
				}
			}
		})
	}
}
