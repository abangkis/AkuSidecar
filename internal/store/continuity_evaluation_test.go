package store

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/capture"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type persistedContinuityCase struct {
	Source     domain.Source              `json:"source"`
	Block      domain.Block               `json:"block"`
	Selected   bool                       `json:"selected"`
	Assessment domain.CandidateAssessment `json:"assessment"`
	Error      struct {
		HTTPStatus int `json:"httpStatus"`
	} `json:"error"`
}

func continuityAuditFixtures(t *testing.T) ([]persistedContinuityCase, []persistedContinuityCase) {
	t.Helper()
	raw, err := os.ReadFile("testdata/candidate-evaluation-continuity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Cases, Successes []persistedContinuityCase }
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Cases, fixture.Successes
}

func startContinuityRun(t *testing.T, state *Store, source domain.Source, block domain.Block) (domain.Run, domain.Observation, domain.Settings) {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources, settings.ResurfaceMode = []domain.Source{source}, "smart"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "continuity evaluation fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	run := runs[0]
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "continuity-fixture"); err != nil {
		t.Fatal(err)
	}
	observation := domain.Observation{Source: source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}}
	if err := state.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	return run, storedIdentityObservation(t, state, run.ID), settings
}

// Use the real atomic completion API, including a candidate assessment for each
// observed key. A completed status alone must never stand in for evaluation.
func completeContinuityRun(t *testing.T, state *Store, run domain.Run) {
	t.Helper()
	observations, err := state.Observations(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []domain.Snapshot
	for _, observation := range observations {
		snapshots = append(snapshots, observation.Snapshots...)
	}
	var scored []ScoredAssessment
	for _, block := range canonicalContinuityBlocks(domain.Observation{Snapshots: capture.ReconcileSnapshots(run.Source, snapshots)}) {
		scored = append(scored, ScoredAssessment{Assessment: domain.CandidateAssessment{
			EvidenceKey: block.EvidenceKey, TopicTags: []string{"unclassified"}, TopicFacets: []string{"other"},
			ContentType: "other", Novelty: .5, Urgency: .2, Actionability: .2, Materiality: .4, EvidenceStrength: .6,
			KnowledgeRelation: "unknown", Rationale: "Successful offline candidate evaluation.",
		}})
	}
	if err := state.CompleteRun(context.Background(), run, domain.ReasoningResult{}, scored, nil, map[string]any{}); err != nil {
		t.Fatal(err)
	}
}

func failContinuityRun(t *testing.T, state *Store, run domain.Run) {
	t.Helper()
	if err := state.FailRun(context.Background(), run.ID, domain.Failure{Code: "provider_unavailable", Stage: "candidate_evaluation", Message: "Persisted offline HTTP 503 fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := state.FinalizeSession(context.Background(), run.SessionID); err != nil {
		t.Fatal(err)
	}
}

func TestFailedCandidateEvaluationRemainsEligible(t *testing.T) {
	cases, _ := continuityAuditFixtures(t)
	for _, fixture := range cases {
		t.Run(fixture.Block.EvidenceKey, func(t *testing.T) {
			state := openTestStore(t)
			if fixture.Error.HTTPStatus != 503 {
				t.Fatal("fixture must represent persisted 503")
			}
			for attempt := 0; attempt < 3; attempt++ {
				run, observation, settings := startContinuityRun(t, state, fixture.Source, fixture.Block)
				decisions, err := state.ClassifyContentContinuity(context.Background(), run, observation, settings)
				if err != nil {
					t.Fatal(err)
				}
				key := observation.Snapshots[0].Blocks[0].EvidenceKey
				if decisions[key].Action != "evaluate" {
					t.Fatalf("attempt %d unexpectedly suppressed: %+v", attempt, decisions[key])
				}
				repeated, err := state.ClassifyContentContinuity(context.Background(), run, observation, settings)
				if err != nil || repeated[key].Action != decisions[key].Action || repeated[key].EvaluationPending != decisions[key].EvaluationPending {
					t.Fatalf("same-run classification lost eligibility: %v %v", repeated, err)
				}
				trace, err := state.InboxRunTrace(context.Background(), run.ID, "skipped", 20, 0)
				if err != nil || trace.Total != 0 || trace.Counts.Skipped != 0 {
					t.Fatalf("pending evaluation mislabeled skipped: %+v %v", trace, err)
				}
				failContinuityRun(t, state, run)
			}
		})
	}
}

func TestHistoricalSuccessfulCandidateVersionsStillSkip(t *testing.T) {
	_, successes := continuityAuditFixtures(t)
	for _, fixture := range successes {
		t.Run(fixture.Block.EvidenceKey, func(t *testing.T) {
			state := openTestStore(t)
			run, obs, settings := startContinuityRun(t, state, fixture.Source, fixture.Block)
			if _, err := state.ClassifyContentContinuity(context.Background(), run, obs, settings); err != nil {
				t.Fatal(err)
			}
			key := obs.Snapshots[0].Blocks[0].EvidenceKey
			assessment := fixture.Assessment
			assessment.EvidenceKey = key
			if err := state.CompleteRun(context.Background(), run, domain.ReasoningResult{}, []ScoredAssessment{{Assessment: assessment, Selected: fixture.Selected}}, nil, map[string]any{}); err != nil {
				t.Fatal(err)
			}
			if err := state.FinalizeSession(context.Background(), run.SessionID); err != nil {
				t.Fatal(err)
			}
			next, observation, settings := startContinuityRun(t, state, fixture.Source, fixture.Block)
			decisions, err := state.ClassifyContentContinuity(context.Background(), next, observation, settings)
			if err != nil || decisions[key].Action != "fail_fast" {
				t.Fatalf("existing success selected=%v must skip exact version: %+v %v", fixture.Selected, decisions[key], err)
			}
		})
	}
}

func TestContinuitySuccessIsBoundToCandidateAndVersion(t *testing.T) {
	for _, change := range []string{"content", "context", "quote-text", "unrelated-key", "empty-completed"} {
		t.Run(change, func(t *testing.T) {
			state := openTestStore(t)
			original := domain.Block{EvidenceKey: "x:versioned", Permalink: "https://x.com/fixture/status/12345", PlatformID: "x:status:12345", Author: "Owner", Text: strings.Repeat("Durable native source evidence. ", 4), ContentKind: "post"}
			if change == "quote-text" {
				original.RelationshipType = "quote"
				original.ParentPermalink = "https://x.com/quoted/status/54321"
				original.QuotedPost = map[string]any{"author": "Quoted", "text": "The first quoted claim."}
			}
			run, observation, settings := startContinuityRun(t, state, domain.SourceX, original)
			if _, err := state.ClassifyContentContinuity(context.Background(), run, observation, settings); err != nil {
				t.Fatal(err)
			}
			key := observation.Snapshots[0].Blocks[0].EvidenceKey
			if change == "unrelated-key" || change == "empty-completed" {
				var scored []ScoredAssessment
				if change == "unrelated-key" {
					scored = []ScoredAssessment{{Assessment: domain.CandidateAssessment{EvidenceKey: "x:someone-else"}}}
				}
				if err := state.CompleteRun(context.Background(), run, domain.ReasoningResult{}, scored, nil, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				completeContinuityRun(t, state, run)
			}
			if err := state.FinalizeSession(context.Background(), run.SessionID); err != nil {
				t.Fatal(err)
			}
			modified := original
			switch change {
			case "content":
				modified.Text += " Materially changed version."
			case "context":
				modified.Author = "Changed author"
			case "quote-text":
				modified.QuotedPost = map[string]any{"author": "Quoted", "text": "A materially different quoted claim."}
			}
			failed, observed, settings := startContinuityRun(t, state, domain.SourceX, modified)
			if _, err := state.ClassifyContentContinuity(context.Background(), failed, observed, settings); err != nil {
				t.Fatal(err)
			}
			failContinuityRun(t, state, failed)
			next, observed, settings := startContinuityRun(t, state, domain.SourceX, modified)
			decisions, err := state.ClassifyContentContinuity(context.Background(), next, observed, settings)
			if err != nil || decisions[key].Action != "evaluate" || !decisions[key].EvaluationPending {
				t.Fatalf("wrong success must not bless pending version: %+v %v", decisions[key], err)
			}
		})
	}
}

func TestContinuitySuccessfulVersionAfterCooldownStillEvaluates(t *testing.T) {
	state := openTestStore(t)
	block := domain.Block{EvidenceKey: "linkedin:cooldown", Author: "Owner", Text: strings.Repeat("An old but successfully evaluated source. ", 3), ContentKind: "post"}
	run, observation, settings := startContinuityRun(t, state, domain.SourceLinkedIn, block)
	if _, err := state.ClassifyContentContinuity(context.Background(), run, observation, settings); err != nil {
		t.Fatal(err)
	}
	completeContinuityRun(t, state, run)
	if err := state.FinalizeSession(context.Background(), run.SessionID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -settings.ResurfaceCooldownDays-1).UTC().Format(time.RFC3339Nano)
	if _, err := state.db.Exec(`UPDATE content_continuity SET last_seen_at=?`, old); err != nil {
		t.Fatal(err)
	}
	next, observation, settings := startContinuityRun(t, state, domain.SourceLinkedIn, block)
	decisions, err := state.ClassifyContentContinuity(context.Background(), next, observation, settings)
	if err != nil || decisions[block.EvidenceKey].Status != "resurfaced_after_cooldown" || decisions[block.EvidenceKey].Action != "evaluate" {
		t.Fatalf("cooldown=%v err=%v", decisions, err)
	}
}
