package eventengine

import (
	"sort"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const semanticSignalReceiptVersion = "semantic-signal-receipt-v1"

// buildSemanticSignalReceipt computes only bounded, aggregate pre-resolution
// signals. It is deliberately separate from resolver input and never returns
// the token/string values used to compute these counts.
func buildSemanticSignalReceipt(candidates []domain.SemanticCandidate, shortlist, catalog []domain.SemanticEvent, trigger triggerSignal) domain.SemanticSignalReceipt {
	receipt := domain.SemanticSignalReceipt{
		Version:               semanticSignalReceiptVersion,
		CandidateCount:        len(candidates),
		ShortlistedEventCount: len(shortlist),
		TopOverlap:            trigger.Overlap,
		TriggerTokenTotal:     len(trigger.Tokens),
	}
	frequencies := receiptTokenFrequencies(catalog)
	commonThreshold := len(catalog) / 5
	if commonThreshold < 2 {
		commonThreshold = 2
	}
	for _, token := range trigger.Tokens {
		frequency := frequencies[token]
		switch {
		case frequency == 0:
			receipt.TriggerUnknownTokenCount++
		case frequency >= commonThreshold:
			receipt.TriggerCommonTokenCount++
		default:
			receipt.TriggerRareTokenCount++
		}
	}
	for _, candidate := range candidates {
		if len(shortlist) == 0 {
			receipt.CandidateActorUnavailableCount++
			receipt.CandidateObjectUnavailableCount++
			receipt.CandidateTimeUnavailableCount++
			receipt.CandidateExactEventKeyUnavailableCount++
			continue
		}
		countCandidateActor(&receipt, candidate, shortlist)
		countCandidateObject(&receipt, candidate, shortlist)
		countCandidateTime(&receipt, candidate, shortlist)
		countCandidateEventKey(&receipt, candidate, shortlist)
	}
	return receipt
}

func receiptTokenFrequencies(events []domain.SemanticEvent) map[string]int {
	frequencies := map[string]int{}
	for _, event := range events {
		text := strings.Join([]string{event.CanonicalClaim, event.Actor, event.Action, event.Object, strings.Join(event.Aliases, " ")}, " ")
		for token := range tokens(text) {
			frequencies[token]++
		}
	}
	return frequencies
}

func countCandidateActor(receipt *domain.SemanticSignalReceipt, candidate domain.SemanticCandidate, shortlist []domain.SemanticEvent) {
	if strings.TrimSpace(candidate.Author) == "" {
		receipt.CandidateActorUnavailableCount++
		return
	}
	comparable := false
	matched := false
	for _, event := range shortlist {
		if strings.TrimSpace(event.Actor) == "" {
			continue
		}
		comparable = true
		if len(intersection(tokens(candidate.Author), tokens(event.Actor))) > 0 {
			matched = true
			break
		}
	}
	if !comparable {
		receipt.CandidateActorUnavailableCount++
	} else if matched {
		receipt.CandidateActorOverlapCount++
	}
}

func countCandidateObject(receipt *domain.SemanticSignalReceipt, candidate domain.SemanticCandidate, shortlist []domain.SemanticEvent) {
	if strings.TrimSpace(candidate.WhatChanged) == "" {
		receipt.CandidateObjectUnavailableCount++
		return
	}
	comparable := false
	matched := false
	for _, event := range shortlist {
		if strings.TrimSpace(event.Object) == "" {
			continue
		}
		comparable = true
		if len(intersection(tokens(candidate.WhatChanged), tokens(event.Object))) > 0 {
			matched = true
			break
		}
	}
	if !comparable {
		receipt.CandidateObjectUnavailableCount++
	} else if matched {
		receipt.CandidateObjectOverlapCount++
	}
}

func countCandidateTime(receipt *domain.SemanticSignalReceipt, candidate domain.SemanticCandidate, shortlist []domain.SemanticEvent) {
	if candidate.PublishedAt == nil {
		receipt.CandidateTimeUnavailableCount++
		return
	}
	candidateTime, candidateErr := time.Parse(time.RFC3339, *candidate.PublishedAt)
	if candidateErr != nil {
		receipt.CandidateTimeUnavailableCount++
		return
	}
	comparable := false
	minimumDistance := time.Duration(0)
	minimumDistanceSet := false
	for _, event := range shortlist {
		start, end, ok := semanticEventTimeWindow(event)
		if !ok {
			continue
		}
		comparable = true
		distance := time.Duration(0)
		if candidateTime.Before(start) {
			distance = start.Sub(candidateTime)
		} else if candidateTime.After(end) {
			distance = candidateTime.Sub(end)
		}
		if !minimumDistanceSet || distance < minimumDistance {
			minimumDistance = distance
			minimumDistanceSet = true
		}
	}
	if !comparable {
		receipt.CandidateTimeUnavailableCount++
	} else if minimumDistance <= 24*time.Hour {
		receipt.CandidateTimeWithin24HoursCount++
	} else if minimumDistance <= 7*24*time.Hour {
		receipt.CandidateTimeWithin7DaysCount++
	} else {
		receipt.CandidateTimeBeyond7DaysCount++
	}
}

func semanticEventTimeWindow(event domain.SemanticEvent) (time.Time, time.Time, bool) {
	if event.EventStart == nil {
		return time.Time{}, time.Time{}, false
	}
	start, err := time.Parse(time.RFC3339, *event.EventStart)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	end := start
	if event.EventEnd != nil {
		if parsed, parseErr := time.Parse(time.RFC3339, *event.EventEnd); parseErr == nil {
			end = parsed
		}
	}
	if end.Before(start) {
		start, end = end, start
	}
	return start, end, true
}

func countCandidateEventKey(receipt *domain.SemanticSignalReceipt, candidate domain.SemanticCandidate, shortlist []domain.SemanticEvent) {
	key := receiptEventKey(candidate.EventKey)
	if key == "" {
		receipt.CandidateExactEventKeyUnavailableCount++
		return
	}
	comparable := false
	matched := false
	for _, event := range shortlist {
		for _, alias := range event.Aliases {
			aliasKey := receiptEventKey(alias)
			if aliasKey == "" {
				continue
			}
			comparable = true
			if aliasKey == key {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !comparable {
		receipt.CandidateExactEventKeyUnavailableCount++
	} else if matched {
		receipt.CandidateExactEventKeyMatchCount++
	}
}

func receiptEventKey(value string) string {
	words := tokens(value)
	ordered := make([]string, 0, len(words))
	for word := range words {
		ordered = append(ordered, word)
	}
	sort.Strings(ordered)
	return strings.Join(ordered, " ")
}
