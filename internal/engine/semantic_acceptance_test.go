package engine

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type semanticAcceptanceFixture struct {
	source      domain.Source
	evidenceKey string
	author      string
	text        string
	whatChanged string
	eventKey    string
	topicTags   []string
}

type semanticAcceptanceProvider struct {
	reasoning.Deterministic
	fixtures map[string]semanticAcceptanceFixture
}

func (p semanticAcceptanceProvider) Analyze(_ context.Context, run domain.Run, observation domain.Observation, _ []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	result := domain.ReasoningResult{Summary: "semantic acceptance fixture", Limitations: []string{}}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			fixture, ok := p.fixtures[block.EvidenceKey]
			if !ok {
				continue
			}
			result.Items = append(result.Items, domain.ReasonedItem{
				ID:             domain.NewID("item"),
				WhatChanged:    fixture.whatChanged,
				WhyItMatters:   "Exercises the semantic event acceptance contract.",
				Source:         run.Source,
				SourceURL:      block.Permalink,
				SourceURLKind:  "native_post",
				EvidenceKey:    fixture.evidenceKey,
				EventKey:       fixture.eventKey,
				KnowledgeDelta: "new_event",
				Author:         fixture.author,
				PublishedAt:    block.PublishedAt,
				Confidence:     .98,
				EvidenceState:  "primary",
			})
			result.CandidateAssessments = append(result.CandidateAssessments, domain.CandidateAssessment{
				EvidenceKey: fixture.evidenceKey,
				TopicTags:   append([]string(nil), fixture.topicTags...),
				TopicFacets: []string{"ai_models"},
				ContentType: "news",
				Novelty:     .9, Urgency: .5, Actionability: .7, Materiality: .9, EvidenceStrength: .95,
				Rationale: "high-signal semantic acceptance fixture",
			})
		}
	}
	telemetry := domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: "candidate_evaluation", Provider: "semantic-acceptance", Model: "fixture", Effort: "none", Status: "completed", CreatedAt: domain.Now()}
	return result, telemetry, nil
}

type semanticAcceptanceResolver struct {
	mu      sync.Mutex
	calls   int
	resolve func([]domain.SemanticCandidate, []domain.SemanticEvent) domain.SemanticResolution
}

func (r *semanticAcceptanceResolver) Name() string { return "semantic-acceptance" }
func (r *semanticAcceptanceResolver) Model() config.ModelConfig {
	return config.ModelConfig{Model: "fixture", Effort: "none"}
}
func (r *semanticAcceptanceResolver) Resolve(_ context.Context, candidates []domain.SemanticCandidate, events []domain.SemanticEvent) (domain.SemanticResolution, domain.ModelUsage, time.Duration, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return r.resolve(candidates, events), domain.ModelUsage{}, time.Millisecond, nil
}
func (r *semanticAcceptanceResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestSemanticAcceptanceMatrixEndToEnd(t *testing.T) {
	t.Run("true cross-source duplicate collapses and does not consume unique capacity", func(t *testing.T) {
		fixtures := sameLaunchFixtures()
		resolver := acceptanceResolverFor("duplicate_report", .95)
		runtime, _ := semanticAcceptanceRuntime(t, fixtures, resolver, nil)
		runSemanticAcceptanceSession(t, runtime, fixtures)

		items := acceptanceTimeline(t, runtime)
		if len(items) != 2 || countSemanticRelation(items, "duplicate_report") != 1 {
			t.Fatalf("collapsed Timeline=%+v", items)
		}
		assertLatestSemanticCount(t, runtime, 1, 1)
		if resolver.callCount() != 1 {
			t.Fatalf("resolver calls=%d want=1", resolver.callCount())
		}
	})

	t.Run("same topic but different occurrence remains unique", func(t *testing.T) {
		fixtures := nearMissFixtures()
		resolver := acceptanceResolverFor("new_event", .99)
		runtime, _ := semanticAcceptanceRuntime(t, fixtures, resolver, nil)
		runSemanticAcceptanceSession(t, runtime, fixtures)

		items := acceptanceTimeline(t, runtime)
		if len(items) != 2 || countSemanticRelation(items, "new_event") != 2 || semanticEventID(items[0]) == semanticEventID(items[1]) {
			t.Fatalf("near-miss Timeline=%+v", items)
		}
		assertLatestSemanticCount(t, runtime, 2, 0)
	})

	t.Run("material update stays unique inside the same event thread", func(t *testing.T) {
		fixtures := sameLaunchFixtures()
		fixtures[1].whatChanged = "Moonshot published a verified Kimi K3 launch benchmark after the announcement."
		resolver := acceptanceResolverFor("material_update", .90)
		runtime, _ := semanticAcceptanceRuntime(t, fixtures, resolver, nil)
		runSemanticAcceptanceSession(t, runtime, fixtures)

		items := acceptanceTimeline(t, runtime)
		if len(items) != 2 || countSemanticRelation(items, "material_update") != 1 || semanticEventID(items[0]) != semanticEventID(items[1]) {
			t.Fatalf("material-update Timeline=%+v", items)
		}
		assertLatestSemanticCount(t, runtime, 2, 0)
	})

	t.Run("user can split attach and undo a semantic relationship", func(t *testing.T) {
		t.Run("not same event", func(t *testing.T) {
			fixtures := sameLaunchFixtures()
			runtime, _ := semanticAcceptanceRuntime(t, fixtures, acceptanceResolverFor("duplicate_report", .96), nil)
			runSemanticAcceptanceSession(t, runtime, fixtures)
			items := acceptanceTimeline(t, runtime)
			duplicate := itemWithSemanticRelation(t, items, "duplicate_report")
			correction, err := runtime.CorrectSemanticEvent(context.Background(), duplicate.ID, "not_same_event", "")
			if err != nil {
				t.Fatal(err)
			}
			if countSemanticRelation(acceptanceTimeline(t, runtime), "duplicate_report") != 0 {
				t.Fatal("false merge remained collapsed after Not the same event")
			}
			assertLatestSemanticCount(t, runtime, 2, 0)
			if _, err := runtime.UndoSemanticCorrection(context.Background(), correction.ID); err != nil {
				t.Fatal(err)
			}
			assertLatestSemanticCount(t, runtime, 1, 1)
		})

		t.Run("same event", func(t *testing.T) {
			fixtures := nearMissFixtures()
			runtime, _ := semanticAcceptanceRuntime(t, fixtures, acceptanceResolverFor("new_event", .99), nil)
			runSemanticAcceptanceSession(t, runtime, fixtures)
			items := acceptanceTimeline(t, runtime)
			targetID := semanticEventID(items[0])
			suggestions, err := runtime.SemanticEventSuggestions(context.Background(), items[1].ID, 3)
			if err != nil || !containsEventSuggestion(suggestions, targetID) {
				t.Fatalf("suggestions=%+v err=%v", suggestions, err)
			}
			correction, err := runtime.CorrectSemanticEvent(context.Background(), items[1].ID, "same_event", targetID)
			if err != nil {
				t.Fatal(err)
			}
			assertLatestSemanticCount(t, runtime, 1, 1)
			if _, err := runtime.UndoSemanticCorrection(context.Background(), correction.ID); err != nil {
				t.Fatal(err)
			}
			assertLatestSemanticCount(t, runtime, 2, 0)
		})
	})

	t.Run("show all bypasses the event engine", func(t *testing.T) {
		fixtures := sameLaunchFixtures()
		resolver := acceptanceResolverFor("duplicate_report", .99)
		runtime, state := semanticAcceptanceRuntime(t, fixtures, resolver, func(settings *domain.Settings) {
			settings.SemanticEventMode = "show_all"
		})
		runSemanticAcceptanceSession(t, runtime, fixtures)

		items := acceptanceTimeline(t, runtime)
		if resolver.callCount() != 0 || len(items) != 2 || countSemanticAnnotations(items) != 0 {
			t.Fatalf("show-all calls=%d items=%+v", resolver.callCount(), items)
		}
		events, err := state.ListSemanticEvents(context.Background(), "", 10)
		if err != nil || len(events) != 0 {
			t.Fatalf("show-all persisted events=%+v err=%v", events, err)
		}
		assertLatestSemanticCount(t, runtime, 2, 0)
	})

	t.Run("collapse hide and header counts stay consistent", func(t *testing.T) {
		fixtures := sameLaunchFixtures()
		runtime, _ := semanticAcceptanceRuntime(t, fixtures, acceptanceResolverFor("duplicate_report", .96), nil)
		runSemanticAcceptanceSession(t, runtime, fixtures)
		if items := acceptanceTimeline(t, runtime); len(items) != 2 || countSemanticRelation(items, "duplicate_report") != 1 {
			t.Fatalf("collapse items=%+v", items)
		}
		assertLatestSemanticCount(t, runtime, 1, 1)

		settings, _ := runtime.Settings(context.Background())
		settings.SemanticEventMode = "hide"
		if _, err := runtime.SaveSettings(context.Background(), settings); err != nil {
			t.Fatal(err)
		}
		if items := acceptanceTimeline(t, runtime); len(items) != 1 || countSemanticRelation(items, "duplicate_report") != 0 {
			t.Fatalf("hide items=%+v", items)
		}
		assertLatestSemanticCount(t, runtime, 1, 1)
	})
}

func TestSemanticMergeThresholdTunesOnlyInsideSafeBoundary(t *testing.T) {
	for _, test := range []struct {
		name       string
		threshold  float64
		unique     int
		duplicates int
	}{
		{name: "default rejects below threshold", threshold: .92, unique: 2, duplicates: 0},
		{name: "one step lower accepts the same resolver confidence", threshold: .90, unique: 1, duplicates: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixtures := sameLaunchFixtures()
			runtime, _ := semanticAcceptanceRuntime(t, fixtures, acceptanceResolverFor("duplicate_report", .91), func(settings *domain.Settings) {
				settings.SemanticEventMergeThreshold = test.threshold
			})
			runSemanticAcceptanceSession(t, runtime, fixtures)
			assertLatestSemanticCount(t, runtime, test.unique, test.duplicates)
		})
	}
}

func semanticAcceptanceRuntime(t *testing.T, fixtures []semanticAcceptanceFixture, resolver *semanticAcceptanceResolver, customize func(*domain.Settings)) (*Engine, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
	settings.CalibrationEnabled = false
	if customize != nil {
		customize(&settings)
	}
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		state.Close()
		t.Fatal(err)
	}
	fixtureMap := make(map[string]semanticAcceptanceFixture, len(fixtures))
	for _, fixture := range fixtures {
		fixtureMap[fixture.evidenceKey] = fixture
	}
	provider := semanticAcceptanceProvider{fixtures: fixtureMap}
	events := semanticengine.New(state, resolver)
	runtime := New(state, provider, config.Config{Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0), events)
	runtime.RecordHeartbeat(ExpectedHeartbeat())
	t.Cleanup(func() {
		runtime.Shutdown()
		state.Close()
	})
	return runtime, state
}

func runSemanticAcceptanceSession(t *testing.T, runtime *Engine, fixtures []semanticAcceptanceFixture) domain.Session {
	t.Helper()
	ctx := context.Background()
	session, err := runtime.StartSession(ctx, "Semantic acceptance matrix")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		current := waitSession(t, runtime, session.ID, func(value domain.Session) bool {
			for _, run := range value.Runs {
				if run.Source == fixture.source && run.Status == "waiting_for_bridge" {
					return true
				}
			}
			return false
		})
		var run domain.Run
		for _, value := range current.Runs {
			if value.Source == fixture.source {
				run = value
				break
			}
		}
		command, err := runtime.ClaimCommand(ctx, run.ID, "semantic-acceptance-bridge")
		if err != nil || command == nil {
			t.Fatalf("claim source=%s command=%+v err=%v", fixture.source, command, err)
		}
		published := "2026-07-17T00:00:00Z"
		permalink := "https://x.com/example/status/" + strings.TrimPrefix(fixture.evidenceKey, "x:")
		if fixture.source == domain.SourceLinkedIn {
			permalink = "https://www.linkedin.com/feed/update/urn:li:activity:" + strings.TrimPrefix(fixture.evidenceKey, "linkedin:")
		}
		observation := domain.Observation{
			Source: fixture.source, PageURL: "https://example.test/feed", CapturedAt: domain.Now(),
			Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: fixture.evidenceKey, Author: fixture.author, Text: fixture.text, PublishedAt: &published, Permalink: permalink}}}},
			Coverage:  map[string]any{"quality": "complete", "acceptanceFixture": true},
		}
		if _, err := runtime.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
			t.Fatal(err)
		}
	}
	return waitSession(t, runtime, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
}

func sameLaunchFixtures() []semanticAcceptanceFixture {
	return []semanticAcceptanceFixture{
		{source: domain.SourceX, evidenceKey: "x:semantic-acceptance-launch", author: "Moonshot AI", text: "Moonshot AI launched the Kimi K3 open-weight model today.", whatChanged: "Moonshot AI launched the Kimi K3 open-weight model.", eventKey: "moonshot-kimi-k3-launch", topicTags: []string{"moonshot", "kimi-k3", "model-launch"}},
		{source: domain.SourceLinkedIn, evidenceKey: "linkedin:semantic-acceptance-launch", author: "AI Industry Reporter", text: "Moonshot AI launched its Kimi K3 open-weight model today.", whatChanged: "A second author reported Moonshot AI's Kimi K3 open-weight model launch.", eventKey: "moonshot-kimi-k3-launch", topicTags: []string{"moonshot", "kimi-k3", "model-launch"}},
	}
}

func nearMissFixtures() []semanticAcceptanceFixture {
	return []semanticAcceptanceFixture{
		{source: domain.SourceX, evidenceKey: "x:semantic-acceptance-near-miss", author: "Moonshot AI", text: "Moonshot AI launched the Kimi K3 open-weight model.", whatChanged: "Moonshot AI launched the Kimi K3 open-weight model.", eventKey: "moonshot-kimi-k3-launch", topicTags: []string{"moonshot", "kimi-k3", "model"}},
		{source: domain.SourceLinkedIn, evidenceKey: "linkedin:semantic-acceptance-near-miss", author: "Benchmark Lab", text: "Benchmark Lab published a new Kimi K3 reasoning benchmark two weeks later.", whatChanged: "Benchmark Lab published a new Kimi K3 reasoning benchmark.", eventKey: "kimi-k3-reasoning-benchmark", topicTags: []string{"moonshot", "kimi-k3", "model"}},
	}
}

func acceptanceResolverFor(relation string, confidence float64) *semanticAcceptanceResolver {
	return &semanticAcceptanceResolver{resolve: func(candidates []domain.SemanticCandidate, _ []domain.SemanticEvent) domain.SemanticResolution {
		decisions := make([]domain.SemanticDecision, 0, len(candidates))
		for index, candidate := range candidates {
			decision := domain.SemanticDecision{
				CandidateAlias: candidate.Alias,
				Relation:       "new_event",
				Confidence:     .99,
				Reason:         "Acceptance fixture creates a distinct event.",
				Event:          domain.SemanticEvent{CanonicalClaim: candidate.WhatChanged, Actor: candidate.Author, Action: "reports", Object: candidate.EventKey, EventKind: "release"},
			}
			if index == 1 && relation != "new_event" {
				target := candidates[0].Alias
				decision.Relation = relation
				decision.TargetAlias = &target
				decision.Confidence = confidence
				decision.Reason = "Acceptance fixture relates the second report to the first event."
			}
			decisions = append(decisions, decision)
		}
		return domain.SemanticResolution{Decisions: decisions}
	}}
}

func acceptanceTimeline(t *testing.T, runtime *Engine) []domain.TimelineItem {
	t.Helper()
	items, err := runtime.Timeline(context.Background(), 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func assertLatestSemanticCount(t *testing.T, runtime *Engine, unique, duplicates int) {
	t.Helper()
	latest, err := runtime.LatestTimelineCheck(context.Background())
	if err != nil || latest == nil || latest.AddedItems != unique || latest.DuplicateReports != duplicates {
		t.Fatalf("latest=%+v err=%v want unique=%d duplicates=%d", latest, err, unique, duplicates)
	}
}

func countSemanticRelation(items []domain.TimelineItem, relation string) int {
	count := 0
	for _, item := range items {
		if item.SemanticEvent != nil && item.SemanticEvent.Relation == relation {
			count++
		}
	}
	return count
}

func countSemanticAnnotations(items []domain.TimelineItem) int {
	count := 0
	for _, item := range items {
		if item.SemanticEvent != nil {
			count++
		}
	}
	return count
}

func itemWithSemanticRelation(t *testing.T, items []domain.TimelineItem, relation string) domain.TimelineItem {
	t.Helper()
	for _, item := range items {
		if item.SemanticEvent != nil && item.SemanticEvent.Relation == relation {
			return item
		}
	}
	t.Fatalf("no Timeline item with semantic relation %q: %+v", relation, items)
	return domain.TimelineItem{}
}

func semanticEventID(item domain.TimelineItem) string {
	if item.SemanticEvent == nil {
		return ""
	}
	return item.SemanticEvent.EventID
}

func containsEventSuggestion(values []domain.EventSuggestion, eventID string) bool {
	for _, value := range values {
		if value.EventID == eventID {
			return true
		}
	}
	return false
}
