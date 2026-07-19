package eventengine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	localCatalogLimit        = 2000
	historicalOverlapMinimum = 3
	intraCheckOverlapMinimum = 3
)

type Engine struct {
	store    *store.Store
	resolver Resolver
}

func New(state *store.Store, resolver Resolver) *Engine {
	return &Engine{store: state, resolver: resolver}
}

func (e *Engine) ProcessSession(ctx context.Context, sessionID string, settings domain.Settings) (domain.EventResolutionSummary, error) {
	return e.processSession(ctx, sessionID, settings, false)
}

// ProcessOnboardingSession establishes the local event index needed by later
// checks without spending a model turn comparing a brand-new calibration
// sample against itself. Exact native replay constraints still apply.
func (e *Engine) ProcessOnboardingSession(ctx context.Context, sessionID string, settings domain.Settings) (domain.EventResolutionSummary, error) {
	return e.processSession(ctx, sessionID, settings, true)
}

func (e *Engine) processSession(ctx context.Context, sessionID string, settings domain.Settings, onboardingFastPath bool) (domain.EventResolutionSummary, error) {
	if settings.SemanticEventMode == "show_all" {
		return domain.EventResolutionSummary{SessionID: sessionID, Status: "bypassed", Provider: "disabled", TriggerReason: "engine_disabled", CreatedAt: domain.Now()}, nil
	}
	candidates, err := e.store.SemanticCandidates(ctx, sessionID)
	if err != nil {
		return domain.EventResolutionSummary{}, err
	}
	summary := domain.EventResolutionSummary{SessionID: sessionID, Status: "completed", Provider: "local-index", Model: "none", Effort: "none", CandidateCount: len(candidates), UniqueItems: len(candidates), TriggerReason: "local_new_events", CreatedAt: domain.Now()}
	if len(candidates) == 0 {
		summary.TriggerReason = "no_candidates"
		_ = e.store.SaveEventResolutionSummary(ctx, summary)
		return summary, nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -settings.KnowledgeRetentionDays).Format(time.RFC3339Nano)
	catalog, err := e.store.ListSemanticEvents(ctx, cutoff, localCatalogLimit)
	if err != nil {
		return summary, err
	}
	constraints, err := e.store.SemanticConstraints(ctx, candidateEvidenceKeys(candidates))
	if err != nil {
		return summary, err
	}
	exactEventIDs, err := e.store.ExactSemanticEventIDs(ctx, candidateEvidenceKeys(candidates))
	if err != nil {
		return summary, err
	}
	resurfaceReevaluation, err := e.store.ResurfaceSemanticReevaluationKeys(ctx, sessionID)
	if err != nil {
		return summary, err
	}
	for evidenceKey := range resurfaceReevaluation {
		delete(exactEventIDs, evidenceKey)
	}
	applyExactReplayConstraints(constraints, exactEventIDs, catalog)
	resolverCandidates := candidatesRequiringResolution(candidates, constraints)
	summary.HistoricalEventCount = len(catalog)
	shortlist, historicalSignal := rankShortlist(resolverCandidates, catalog, settings.SemanticEventShortlist)
	summary.ShortlistCount = len(shortlist)
	intraCheckSignal := strongestIntraCheckSignal(resolverCandidates)
	triggerSignal := intraCheckSignal
	shouldResolve := intraCheckSignal.Strong
	if len(shortlist) > 0 {
		triggerSignal = historicalSignal
		shouldResolve = true
		summary.TriggerReason = "historical_shortlist"
	} else if intraCheckSignal.Strong {
		summary.TriggerReason = intraCheckSignal.Reason
	}
	summary.StrongestOverlap = triggerSignal.Overlap
	summary.TriggerTokens = append([]string(nil), triggerSignal.Tokens...)
	if onboardingFastPath {
		shortlist = nil
		shouldResolve = false
		summary.ShortlistCount = 0
		summary.TriggerReason = "onboarding_local_index"
		summary.StrongestOverlap = 0
		summary.TriggerTokens = nil
	}

	var resolution domain.SemanticResolution
	if len(resolverCandidates) == 0 {
		summary.TriggerReason = "exact_source_replays"
	}
	if e.resolver != nil && shouldResolve && len(resolverCandidates) > 0 {
		summary.ResolverInvoked = true
		model := e.modelForProfile(settings.ReasoningSemanticProfile)
		summary.Provider, summary.Model, summary.Effort = e.resolver.Name(), model.Model, model.Effort
		resolution, summary.Usage, summary.DurationMS, err = e.resolve(ctx, resolverCandidates, shortlist, settings.ReasoningSemanticProfile)
		if err != nil {
			summary.Status = "failed"
			summary.Error = &domain.Failure{Code: "semantic_resolution_failed", Stage: "semantic_event_resolution", Message: err.Error(), Retryable: true}
			_ = e.store.SaveEventResolutionSummary(context.Background(), summary)
			return summary, err
		}
	}

	reports := resolveReports(candidates, shortlist, catalog, resolution, constraints, settings.KnowledgeRetentionDays, settings.SemanticEventMergeThreshold)
	duplicates := 0
	for _, report := range reports {
		if report.Relation == "duplicate_report" {
			duplicates++
		}
	}
	summary.DuplicateReports = duplicates
	summary.UniqueItems = len(reports) - duplicates
	if err := e.store.SaveSemanticReports(ctx, reports); err != nil {
		return summary, err
	}
	if err := e.store.SaveEventResolutionSummary(ctx, summary); err != nil {
		return summary, err
	}
	return summary, nil
}

// ProcessTimelineItem resolves one user-restored report against the retained
// global event catalog without reprocessing or spending tokens on the rest of
// its already-terminal session.
func (e *Engine) ProcessTimelineItem(ctx context.Context, timelineID string, settings domain.Settings) (domain.ResolvedSemanticReport, error) {
	candidate, err := e.store.SemanticCandidate(ctx, timelineID)
	if err != nil {
		return domain.ResolvedSemanticReport{}, err
	}
	if settings.SemanticEventMode == "show_all" {
		_ = e.store.RefreshEventResolutionCounts(ctx, candidate.SessionID)
		return domain.ResolvedSemanticReport{Candidate: candidate, Relation: "new_event", Confidence: 1, Reason: "Semantic event engine is disabled."}, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -settings.KnowledgeRetentionDays).Format(time.RFC3339Nano)
	catalog, err := e.store.ListSemanticEvents(ctx, cutoff, localCatalogLimit)
	if err != nil {
		return domain.ResolvedSemanticReport{}, err
	}
	candidates := []domain.SemanticCandidate{candidate}
	constraints, err := e.store.SemanticConstraints(ctx, []string{candidate.EvidenceKey})
	if err != nil {
		return domain.ResolvedSemanticReport{}, err
	}
	exactEventIDs, err := e.store.ExactSemanticEventIDs(ctx, []string{candidate.EvidenceKey})
	if err != nil {
		return domain.ResolvedSemanticReport{}, err
	}
	applyExactReplayConstraints(constraints, exactEventIDs, catalog)
	resolverCandidates := candidatesRequiringResolution(candidates, constraints)
	shortlist, _ := rankShortlist(resolverCandidates, catalog, settings.SemanticEventShortlist)
	var resolution domain.SemanticResolution
	if e.resolver != nil && len(shortlist) > 0 && len(resolverCandidates) > 0 {
		resolution, _, _, err = e.resolve(ctx, resolverCandidates, shortlist, settings.ReasoningSemanticProfile)
		if err != nil {
			return domain.ResolvedSemanticReport{}, err
		}
	}
	reports := resolveReports(candidates, shortlist, catalog, resolution, constraints, settings.KnowledgeRetentionDays, settings.SemanticEventMergeThreshold)
	if len(reports) != 1 {
		return domain.ResolvedSemanticReport{}, errors.New("selection correction produced no semantic report")
	}
	if err := e.store.SaveSemanticReports(ctx, reports); err != nil {
		return domain.ResolvedSemanticReport{}, err
	}
	if err := e.store.RefreshEventResolutionCounts(ctx, candidate.SessionID); err != nil {
		return domain.ResolvedSemanticReport{}, err
	}
	return reports[0], nil
}

func (e *Engine) resolve(ctx context.Context, candidates []domain.SemanticCandidate, shortlist []domain.SemanticEvent, profileID string) (domain.SemanticResolution, domain.ModelUsage, int64, error) {
	var result domain.SemanticResolution
	var usage domain.ModelUsage
	var duration time.Duration
	var err error
	if profiled, ok := e.resolver.(ProfiledResolver); ok {
		result, usage, duration, err = profiled.ResolveWithProfile(ctx, candidates, shortlist, profileID)
	} else {
		result, usage, duration, err = e.resolver.Resolve(ctx, candidates, shortlist)
	}
	return result, usage, duration.Milliseconds(), err
}

func (e *Engine) modelForProfile(profileID string) config.ModelConfig {
	if profiled, ok := e.resolver.(ProfiledResolver); ok {
		return profiled.ModelForProfile(profileID)
	}
	return e.resolver.Model()
}

func candidateEvidenceKeys(candidates []domain.SemanticCandidate) []string {
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.EvidenceKey)
	}
	return result
}

func applyExactReplayConstraints(constraints map[string]map[string]string, exactEventIDs map[string]string, catalog []domain.SemanticEvent) {
	retained := make(map[string]bool, len(catalog))
	for _, event := range catalog {
		retained[event.ID] = true
	}
	for evidenceKey, eventID := range exactEventIDs {
		if !retained[eventID] {
			continue
		}
		if constraints[evidenceKey] == nil {
			constraints[evidenceKey] = map[string]string{}
		}
		if constraints[evidenceKey][eventID] == "must_not_merge" {
			continue
		}
		constraints[evidenceKey][eventID] = "exact_source_replay"
	}
}

func candidatesRequiringResolution(candidates []domain.SemanticCandidate, constraints map[string]map[string]string) []domain.SemanticCandidate {
	result := make([]domain.SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		deterministic := false
		for _, kind := range constraints[candidate.EvidenceKey] {
			if kind == "must_merge" || kind == "exact_source_replay" {
				deterministic = true
				break
			}
		}
		if !deterministic {
			result = append(result, candidate)
		}
	}
	return result
}

type rankedEvent struct {
	event  domain.SemanticEvent
	score  int
	tokens []string
}

type triggerSignal struct {
	Strong  bool
	Overlap int
	Tokens  []string
	Reason  string
}

func rankShortlist(candidates []domain.SemanticCandidate, catalog []domain.SemanticEvent, limit int) ([]domain.SemanticEvent, triggerSignal) {
	ranked := make([]rankedEvent, 0, len(catalog))
	for _, event := range catalog {
		best := 0
		var bestTokens []string
		for _, candidate := range candidates {
			shared := intersection(tokens(candidateText(candidate)), tokens(eventText(event)))
			score := len(shared)
			if score > best {
				best = score
				bestTokens = shared
			}
		}
		if best >= historicalOverlapMinimum {
			ranked = append(ranked, rankedEvent{event: event, score: best, tokens: bestTokens})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].event.LastSeenAt > ranked[j].event.LastSeenAt
		}
		return ranked[i].score > ranked[j].score
	})
	if limit > len(ranked) {
		limit = len(ranked)
	}
	result := make([]domain.SemanticEvent, 0, limit)
	for _, value := range ranked[:limit] {
		result = append(result, value.event)
	}
	signal := triggerSignal{Reason: "historical_shortlist"}
	if len(ranked) > 0 {
		signal.Strong = true
		signal.Overlap = ranked[0].score
		signal.Tokens = append([]string(nil), ranked[0].tokens...)
	}
	return result, signal
}

func strongestIntraCheckSignal(candidates []domain.SemanticCandidate) triggerSignal {
	strongest := triggerSignal{Reason: "weak_intra_check_overlap"}
	for left := 0; left < len(candidates); left++ {
		for right := left + 1; right < len(candidates); right++ {
			leftCandidate, rightCandidate := candidates[left], candidates[right]
			leftEventKey := normalizeEventKey(leftCandidate.EventKey)
			rightEventKey := normalizeEventKey(rightCandidate.EventKey)
			shared := intersection(tokens(candidateText(leftCandidate)), tokens(candidateText(rightCandidate)))
			anchors := intersection(tokens(candidateAnchorText(leftCandidate)), tokens(candidateAnchorText(rightCandidate)))
			strong := leftEventKey != "" && leftEventKey == rightEventKey
			reason := "matching_event_key"
			if !strong {
				strong = len(shared) >= intraCheckOverlapMinimum && len(anchors) > 0
				reason = "strong_intra_check_overlap"
			}
			if (strong && !strongest.Strong) || (strong == strongest.Strong && len(shared) > strongest.Overlap) {
				strongest = triggerSignal{Strong: strong, Overlap: len(shared), Tokens: shared, Reason: reason}
			}
		}
	}
	if !strongest.Strong {
		strongest.Reason = "weak_intra_check_overlap"
	}
	return strongest
}

func resolveReports(candidates []domain.SemanticCandidate, shortlist, catalog []domain.SemanticEvent, resolution domain.SemanticResolution, constraints map[string]map[string]string, retentionDays int, mergeThreshold float64) []domain.ResolvedSemanticReport {
	decisions := map[string]domain.SemanticDecision{}
	for _, decision := range resolution.Decisions {
		if _, exists := decisions[decision.CandidateAlias]; !exists {
			decisions[decision.CandidateAlias] = decision
		}
	}
	eventAliases := map[string]domain.SemanticEvent{}
	for index, event := range shortlist {
		eventAliases[fmt.Sprintf("event_%03d", index+1)] = event
	}
	allEvents := map[string]domain.SemanticEvent{}
	for _, event := range catalog {
		allEvents[event.ID] = event
	}
	candidateEvents := map[string]domain.SemanticEvent{}
	reports := make([]domain.ResolvedSemanticReport, 0, len(candidates))
	for _, candidate := range candidates {
		decision, hasDecision := decisions[candidate.Alias]
		relation := "new_event"
		confidence := 1.0
		reason := "No plausible semantic overlap in the bounded local index."
		var target domain.SemanticEvent
		targetFound := false

		for eventID, kind := range constraints[candidate.EvidenceKey] {
			if kind == "must_merge" {
				if value, exists := allEvents[eventID]; exists {
					target, targetFound, relation, confidence, reason = value, true, "duplicate_report", 1, "User-confirmed semantic event."
				}
				break
			}
		}
		if !targetFound {
			for eventID, kind := range constraints[candidate.EvidenceKey] {
				if kind != "exact_source_replay" {
					continue
				}
				if value, exists := allEvents[eventID]; exists {
					target, targetFound, relation, confidence, reason = value, true, "duplicate_report", 1, "Exact native source post was already captured."
				}
				break
			}
		}
		if !targetFound && hasDecision && validRelation(decision.Relation) && decision.Relation != "new_event" && decision.TargetAlias != nil {
			if value, exists := eventAliases[*decision.TargetAlias]; exists {
				target, targetFound = value, true
			} else if value, exists := candidateEvents[*decision.TargetAlias]; exists {
				target, targetFound = value, true
			}
			if targetFound && constraints[candidate.EvidenceKey][target.ID] == "must_not_merge" {
				targetFound = false
			}
			if targetFound && !compatibleEventTime(candidate.PublishedAt, target, retentionDays) {
				targetFound = false
			}
			if targetFound && decision.Relation == "duplicate_report" && decision.Confidence < mergeThreshold {
				targetFound = false
			}
			if targetFound {
				relation, confidence, reason = decision.Relation, decision.Confidence, decision.Reason
			}
		}

		event := target
		if !targetFound {
			var proposed domain.SemanticEvent
			if hasDecision {
				proposed = decision.Event
				confidence = decision.Confidence
				if strings.TrimSpace(decision.Reason) != "" {
					reason = decision.Reason
				}
			}
			event = newEvent(candidate, proposed)
			relation = "new_event"
		}
		event.LastSeenAt = domain.Now()
		candidateEvents[candidate.Alias] = event
		reports = append(reports, domain.ResolvedSemanticReport{Candidate: candidate, Event: event, Relation: relation, Confidence: clampConfidence(confidence), Reason: reason})
	}
	return reports
}

func newEvent(candidate domain.SemanticCandidate, proposed domain.SemanticEvent) domain.SemanticEvent {
	now := domain.Now()
	claim := strings.TrimSpace(proposed.CanonicalClaim)
	if claim == "" {
		claim = strings.TrimSpace(candidate.WhatChanged)
	}
	actor := strings.TrimSpace(proposed.Actor)
	if actor == "" {
		actor = strings.TrimSpace(candidate.Author)
	}
	aliases := uniqueAliases(append(append(proposed.Aliases, candidate.EventKey, candidate.Author), candidate.TopicTags...))
	start := proposed.EventStart
	if start == nil {
		start = candidate.PublishedAt
	}
	return domain.SemanticEvent{ID: domain.NewID("event"), CanonicalClaim: claim, Actor: actor, Action: strings.TrimSpace(proposed.Action), Object: strings.TrimSpace(proposed.Object), EventKind: defaultValue(proposed.EventKind, "other"), EventStart: start, EventEnd: proposed.EventEnd, Aliases: aliases, ReportCount: 1, FirstSeenAt: now, LastSeenAt: now}
}

func compatibleEventTime(published *string, event domain.SemanticEvent, retentionDays int) bool {
	if published == nil || event.EventStart == nil {
		return true
	}
	candidateTime, candidateErr := time.Parse(time.RFC3339, *published)
	eventTime, eventErr := time.Parse(time.RFC3339, *event.EventStart)
	if candidateErr != nil || eventErr != nil {
		return true
	}
	delta := candidateTime.Sub(eventTime)
	if delta < 0 {
		delta = -delta
	}
	return delta <= time.Duration(retentionDays)*24*time.Hour
}

func validRelation(value string) bool {
	switch value {
	case "new_event", "duplicate_report", "material_update", "contradiction", "new_consequence", "context_only":
		return true
	default:
		return false
	}
}

func uniqueAliases(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, 8)
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
		if len(result) == 8 {
			break
		}
	}
	return result
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func candidateText(value domain.SemanticCandidate) string {
	return strings.Join(append([]string{value.WhatChanged, value.EventKey}, value.TopicTags...), " ")
}

func candidateAnchorText(value domain.SemanticCandidate) string {
	return strings.Join(append([]string{value.EventKey}, value.TopicTags...), " ")
}

func eventText(value domain.SemanticEvent) string {
	return strings.Join(append([]string{value.CanonicalClaim, value.Actor, value.Action, value.Object, value.EventKind}, value.Aliases...), " ")
}

var urlPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)\S+`)

var stopWords = map[string]bool{
	"about": true, "after": true, "again": true, "akan": true, "adalah": true, "also": true, "and": true, "are": true, "atau": true,
	"bahwa": true, "bisa": true, "but": true, "can": true, "com": true, "could": true, "dari": true, "dengan": true, "did": true,
	"does": true, "feed": true, "for": true, "from": true, "full": true, "had": true, "has": true, "have": true, "here": true,
	"how": true, "http": true, "https": true, "ini": true, "instead": true, "into": true, "its": true, "itu": true, "karena": true,
	"linkedin": true, "lnkd": true, "may": true, "more": true, "most": true, "never": true, "new": true, "news": true, "not": true,
	"now": true, "one": true, "only": true, "our": true, "pada": true, "post": true, "posts": true, "reported": true, "said": true,
	"saat": true, "say": true, "says": true, "saya": true, "share": true, "shared": true, "should": true, "status": true, "still": true,
	"sudah": true, "than": true, "that": true, "the": true, "their": true, "them": true, "then": true, "there": true, "they": true,
	"think": true, "this": true, "through": true, "tidak": true, "time": true, "toward": true, "twitter": true, "under": true,
	"untuk": true, "update": true, "very": true, "via": true, "was": true, "were": true, "will": true, "with": true, "would": true,
	"www": true, "yang": true, "year": true, "you": true, "your": true,
}

func tokens(value string) map[string]bool {
	value = urlPattern.ReplaceAllString(value, " ")
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	result := map[string]bool{}
	for _, word := range words {
		if len([]rune(word)) >= 3 && !stopWords[word] {
			result[word] = true
		}
	}
	return result
}

func intersection(left, right map[string]bool) []string {
	result := make([]string, 0)
	for value := range left {
		if right[value] {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func normalizeEventKey(value string) string {
	return strings.Join(intersection(tokens(value), tokens(value)), "-")
}
