package eventengine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const AutoMergeThreshold = 0.92

type Engine struct {
	store    *store.Store
	resolver Resolver
}

func New(state *store.Store, resolver Resolver) *Engine {
	return &Engine{store: state, resolver: resolver}
}

func (e *Engine) ProcessSession(ctx context.Context, sessionID string, settings domain.Settings) (domain.EventResolutionSummary, error) {
	if settings.SemanticEventMode == "show_all" {
		return domain.EventResolutionSummary{SessionID: sessionID, Status: "bypassed", Provider: "disabled", CreatedAt: domain.Now()}, nil
	}
	candidates, err := e.store.SemanticCandidates(ctx, sessionID)
	if err != nil {
		return domain.EventResolutionSummary{}, err
	}
	summary := domain.EventResolutionSummary{SessionID: sessionID, Status: "completed", Provider: "local-index", Model: "none", Effort: "none", CandidateCount: len(candidates), UniqueItems: len(candidates), CreatedAt: domain.Now()}
	if len(candidates) == 0 {
		_ = e.store.SaveEventResolutionSummary(ctx, summary)
		return summary, nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -settings.KnowledgeRetentionDays).Format(time.RFC3339Nano)
	catalog, err := e.store.ListSemanticEvents(ctx, cutoff, 2000)
	if err != nil {
		return summary, err
	}
	constraints, err := e.store.SemanticConstraints(ctx, candidateEvidenceKeys(candidates))
	if err != nil {
		return summary, err
	}
	shortlist := rankShortlist(candidates, catalog, settings.SemanticEventShortlist, constraints)
	summary.ShortlistCount = len(shortlist)

	var resolution domain.SemanticResolution
	if e.resolver != nil && needsResolver(candidates, shortlist) {
		model := e.resolver.Model()
		summary.Provider, summary.Model, summary.Effort = e.resolver.Name(), model.Model, model.Effort
		resolution, summary.Usage, summary.DurationMS, err = e.resolve(ctx, candidates, shortlist)
		if err != nil {
			summary.Status = "failed"
			summary.Error = &domain.Failure{Code: "semantic_resolution_failed", Stage: "semantic_event_resolution", Message: err.Error(), Retryable: true}
			_ = e.store.SaveEventResolutionSummary(context.Background(), summary)
			return summary, err
		}
	}

	reports := resolveReports(candidates, shortlist, catalog, resolution, constraints, settings.KnowledgeRetentionDays)
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

func (e *Engine) resolve(ctx context.Context, candidates []domain.SemanticCandidate, shortlist []domain.SemanticEvent) (domain.SemanticResolution, domain.ModelUsage, int64, error) {
	result, usage, duration, err := e.resolver.Resolve(ctx, candidates, shortlist)
	return result, usage, duration.Milliseconds(), err
}

func candidateEvidenceKeys(candidates []domain.SemanticCandidate) []string {
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.EvidenceKey)
	}
	return result
}

type rankedEvent struct {
	event domain.SemanticEvent
	score int
}

func rankShortlist(candidates []domain.SemanticCandidate, catalog []domain.SemanticEvent, limit int, constraints map[string]map[string]string) []domain.SemanticEvent {
	ranked := make([]rankedEvent, 0, len(catalog))
	for _, event := range catalog {
		best := 0
		for _, candidate := range candidates {
			score := overlap(tokens(candidateText(candidate)), tokens(eventText(event)))
			if constraints[candidate.EvidenceKey][event.ID] == "must_merge" {
				score += 1000
			}
			if score > best {
				best = score
			}
		}
		if best > 0 {
			ranked = append(ranked, rankedEvent{event: event, score: best})
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
	return result
}

func needsResolver(candidates []domain.SemanticCandidate, shortlist []domain.SemanticEvent) bool {
	if len(shortlist) > 0 {
		return true
	}
	for left := 0; left < len(candidates); left++ {
		for right := left + 1; right < len(candidates); right++ {
			if overlap(tokens(candidateText(candidates[left])), tokens(candidateText(candidates[right]))) >= 2 {
				return true
			}
		}
	}
	return false
}

func resolveReports(candidates []domain.SemanticCandidate, shortlist, catalog []domain.SemanticEvent, resolution domain.SemanticResolution, constraints map[string]map[string]string, retentionDays int) []domain.ResolvedSemanticReport {
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
		if !targetFound && hasDecision && validRelation(decision.Relation) && decision.Relation != "new_event" && decision.TargetAlias != nil {
			if value, exists := eventAliases[*decision.TargetAlias]; exists {
				target, targetFound = value, true
			} else if value, exists := candidateEvents[*decision.TargetAlias]; exists {
				target, targetFound = value, true
			}
			if targetFound && constraints[candidate.EvidenceKey][target.ID] == "must_not_merge" {
				targetFound = false
			}
			if targetFound && (!compatibleEventTime(candidate.PublishedAt, target, retentionDays) || decision.Confidence < AutoMergeThreshold) {
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
	return strings.Join(append([]string{value.Author, value.Text, value.WhatChanged, value.EventKey}, value.TopicTags...), " ")
}

func eventText(value domain.SemanticEvent) string {
	return strings.Join(append([]string{value.CanonicalClaim, value.Actor, value.Action, value.Object, value.EventKind}, value.Aliases...), " ")
}

var stopWords = map[string]bool{"about": true, "after": true, "again": true, "also": true, "and": true, "are": true, "dari": true, "dengan": true, "for": true, "from": true, "ini": true, "into": true, "itu": true, "karena": true, "new": true, "post": true, "that": true, "the": true, "their": true, "this": true, "untuk": true, "was": true, "were": true, "with": true}

func tokens(value string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	result := map[string]bool{}
	for _, word := range words {
		if len([]rune(word)) >= 3 && !stopWords[word] {
			result[word] = true
		}
	}
	return result
}

func overlap(left, right map[string]bool) int {
	count := 0
	for value := range left {
		if right[value] {
			count++
		}
	}
	return count
}
