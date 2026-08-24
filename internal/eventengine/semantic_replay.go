package eventengine

import (
	"context"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// SemanticReplayReader is deliberately narrower than Store so the aggregate
// logic can be tested with synthetic, privacy-safe fixtures without opening a
// provider or a writable database.
type SemanticReplayReader interface {
	ReadSemanticReplay(context.Context, int) ([]store.SemanticReplaySession, []store.SemanticReplayEvent, error)
}

// SemanticReplayReport is an assessment-only aggregate. It intentionally has
// no identifiers, source text, URLs, author names, evidence keys, prompts, or
// model explanations. The report is safe to print as JSON for local review.
type SemanticReplayReport struct {
	SchemaVersion           string                         `json:"schemaVersion"`
	AssessmentOnly          bool                           `json:"assessmentOnly"`
	SessionsAnalyzed        int                            `json:"sessionsAnalyzed"`
	ReportsAnalyzed         int                            `json:"reportsAnalyzed"`
	CorpusEventsObserved    int                            `json:"corpusEventsObserved"`
	CorpusEventsCapped      bool                           `json:"corpusEventsCapped"`
	SessionStatuses         map[string]int                 `json:"sessionStatuses"`
	InvocationStatuses      map[string]int                 `json:"invocationStatuses"`
	Relations               map[string]int                 `json:"relations"`
	TriggerOverlapBuckets   map[string]int                 `json:"triggerOverlapBuckets"`
	TriggerRarityBuckets    map[string]int                 `json:"triggerRarityBuckets"`
	SignalReceipt           SemanticSignalReceiptAggregate `json:"signalReceipt"`
	HistoricalCompatibility SemanticReplayCompatibility    `json:"historicalCompatibility"`
	Corrections             SemanticReplayCorrections      `json:"corrections"`
	Classification          SemanticReplayClassification   `json:"classification"`
	UnavailableSignals      map[string]int                 `json:"unavailableSignals"`
	Limitations             []string                       `json:"limitations"`
}

// The assigned semantic event is post-resolution state. It is not a
// reconstructable historical candidate, so every compatibility dimension is
// explicitly unavailable rather than being reported as a misleading match.
type SemanticReplayCompatibility struct {
	Actor         int `json:"actorUnavailable"`
	Object        int `json:"objectUnavailable"`
	Time          int `json:"timeUnavailable"`
	ExactEventKey int `json:"exactEventKeyUnavailable"`
}

type SemanticReplayCorrections struct {
	ActiveUserCorrections int `json:"activeUserCorrections"`
	UndoneUserCorrections int `json:"undoneUserCorrections"`
}

type SemanticReplayClassification struct {
	ObservedLocalBypassSessions           int `json:"observedLocalBypassSessions"`
	CounterfactualReviewCandidateSessions int `json:"counterfactualReviewCandidateSessions"`
	ReceiptBackedReviewCandidateSessions  int `json:"receiptBackedReviewCandidateSessions"`
	LegacyAllNewReviewCandidateSessions   int `json:"legacyAllNewReviewCandidateSessions"`
	RequiresModelSessions                 int `json:"requiresModelSessions"`
	InsufficientDataSessions              int `json:"insufficientDataSessions"`
}

type SemanticSignalReceiptAggregate struct {
	SessionsWithReceipt                    int `json:"sessionsWithReceipt"`
	CandidateCount                         int `json:"candidateCount"`
	ShortlistedEventCount                  int `json:"shortlistedEventCount"`
	TopOverlapMax                          int `json:"topOverlapMax"`
	TriggerTokenTotal                      int `json:"triggerTokenTotal"`
	TriggerRareTokenCount                  int `json:"triggerRareTokenCount"`
	TriggerCommonTokenCount                int `json:"triggerCommonTokenCount"`
	TriggerUnknownTokenCount               int `json:"triggerUnknownTokenCount"`
	CandidateActorOverlapCount             int `json:"candidateActorOverlapCount"`
	CandidateActorUnavailableCount         int `json:"candidateActorUnavailableCount"`
	CandidateObjectOverlapCount            int `json:"candidateObjectOverlapCount"`
	CandidateObjectUnavailableCount        int `json:"candidateObjectUnavailableCount"`
	CandidateTimeWithin24HoursCount        int `json:"candidateTimeWithin24HoursCount"`
	CandidateTimeWithin7DaysCount          int `json:"candidateTimeWithin7DaysCount"`
	CandidateTimeBeyond7DaysCount          int `json:"candidateTimeBeyond7DaysCount"`
	CandidateTimeUnavailableCount          int `json:"candidateTimeUnavailableCount"`
	CandidateExactEventKeyMatchCount       int `json:"candidateExactEventKeyMatchCount"`
	CandidateExactEventKeyUnavailableCount int `json:"candidateExactEventKeyUnavailableCount"`
}

// AnalyzeSemanticReplay reads only already-loaded durable projections. It does
// not construct a provider, invoke inference, or write to the database. The
// labels are review evidence and never runtime policy.
func AnalyzeSemanticReplay(ctx context.Context, state SemanticReplayReader, limit int) (SemanticReplayReport, error) {
	sessions, corpus, err := state.ReadSemanticReplay(ctx, limit)
	if err != nil {
		return SemanticReplayReport{}, err
	}
	report := SemanticReplayReport{
		SchemaVersion:         "semantic-replay-v1",
		AssessmentOnly:        true,
		SessionsAnalyzed:      len(sessions),
		CorpusEventsObserved:  len(corpus),
		CorpusEventsCapped:    len(corpus) == 1000,
		SessionStatuses:       map[string]int{},
		InvocationStatuses:    map[string]int{},
		Relations:             map[string]int{},
		TriggerOverlapBuckets: map[string]int{"unavailable": 0, "none": 0, "low": 0, "high": 0},
		TriggerRarityBuckets:  map[string]int{"unavailable": 0, "rare": 0, "mixed": 0, "common": 0, "unknown": 0},
		UnavailableSignals:    map[string]int{},
		Limitations: []string{
			"Historical candidate text and event descriptors are read internally only; no raw evidence is emitted.",
			"The assigned semantic event is post-resolution state, not the exact historical shortlist; actor, object, time, and event-key compatibility are therefore unavailable.",
			"The ledger does not retain the exact pre-resolution shortlist for every session, so replay cannot reproduce the original model prompt.",
			"Legacy rows without a semantic signal receipt remain review evidence only; their all-new classification cannot be attributed to pre-resolution signals.",
			"Review-candidate and local-bypass labels are assessment evidence only and do not alter semantic runtime policy.",
		},
	}
	for _, session := range sessions {
		report.SessionStatuses[session.Status]++
		report.InvocationStatuses[session.InvocationStatus]++
		overlap := session.StrongestOverlap
		overlapAvailable := session.DiagnosticsAvailable
		if session.SignalReceipt == nil {
			report.TriggerRarityBuckets["unavailable"]++
		} else {
			report.SignalReceipt.SessionsWithReceipt++
			overlap = session.SignalReceipt.TopOverlap
			overlapAvailable = true
			report.TriggerRarityBuckets[receiptRarityBucket(*session.SignalReceipt)]++
			aggregateSignalReceipt(&report.SignalReceipt, *session.SignalReceipt)
		}
		report.TriggerOverlapBuckets[replayOverlapBucket(overlap, overlapAvailable)]++
		report.Corrections.ActiveUserCorrections += session.ActiveCorrections
		report.Corrections.UndoneUserCorrections += session.UndoneCorrections
		if !session.ResolverInvoked && (session.InvocationStatus == "completed" || session.InvocationStatus == "bypassed") {
			report.Classification.ObservedLocalBypassSessions++
		}
		if len(session.Reports) == 0 {
			report.Classification.InsufficientDataSessions++
			continue
		}
		allNew := true
		for _, value := range session.Reports {
			report.ReportsAnalyzed++
			report.Relations[value.Relation]++
			if value.Relation != "new_event" {
				allNew = false
			}
			// These are intentionally counts of unavailable historical signals,
			// not outcomes derived from the assigned event.
			report.HistoricalCompatibility.Actor++
			report.HistoricalCompatibility.Object++
			report.HistoricalCompatibility.Time++
			report.HistoricalCompatibility.ExactEventKey++
		}
		if session.ResolverInvoked {
			report.Classification.RequiresModelSessions++
		}
		if session.ResolverInvoked && session.InvocationStatus == "completed" && allNew && session.ActiveCorrections == 0 && session.UndoneCorrections == 0 {
			report.Classification.CounterfactualReviewCandidateSessions++
			if session.SignalReceipt == nil {
				report.Classification.LegacyAllNewReviewCandidateSessions++
			} else {
				report.Classification.ReceiptBackedReviewCandidateSessions++
			}
		}
	}
	report.UnavailableSignals["historical_actor_compatibility"] = report.HistoricalCompatibility.Actor
	report.UnavailableSignals["historical_object_compatibility"] = report.HistoricalCompatibility.Object
	report.UnavailableSignals["historical_time_compatibility"] = report.HistoricalCompatibility.Time
	report.UnavailableSignals["historical_exact_event_key_compatibility"] = report.HistoricalCompatibility.ExactEventKey
	report.UnavailableSignals["trigger_rarity"] = report.TriggerRarityBuckets["unavailable"]
	report.UnavailableSignals["signal_receipt"] = report.SessionsAnalyzed - report.SignalReceipt.SessionsWithReceipt
	return report, nil
}

func replayOverlapBucket(overlap int, available bool) string {
	if !available {
		return "unavailable"
	}
	switch {
	case overlap == 0:
		return "none"
	case overlap <= 2:
		return "low"
	default:
		return "high"
	}
}

func aggregateSignalReceipt(aggregate *SemanticSignalReceiptAggregate, receipt domain.SemanticSignalReceipt) {
	aggregate.CandidateCount += receipt.CandidateCount
	aggregate.ShortlistedEventCount += receipt.ShortlistedEventCount
	if receipt.TopOverlap > aggregate.TopOverlapMax {
		aggregate.TopOverlapMax = receipt.TopOverlap
	}
	aggregate.TriggerTokenTotal += receipt.TriggerTokenTotal
	aggregate.TriggerRareTokenCount += receipt.TriggerRareTokenCount
	aggregate.TriggerCommonTokenCount += receipt.TriggerCommonTokenCount
	aggregate.TriggerUnknownTokenCount += receipt.TriggerUnknownTokenCount
	aggregate.CandidateActorOverlapCount += receipt.CandidateActorOverlapCount
	aggregate.CandidateActorUnavailableCount += receipt.CandidateActorUnavailableCount
	aggregate.CandidateObjectOverlapCount += receipt.CandidateObjectOverlapCount
	aggregate.CandidateObjectUnavailableCount += receipt.CandidateObjectUnavailableCount
	aggregate.CandidateTimeWithin24HoursCount += receipt.CandidateTimeWithin24HoursCount
	aggregate.CandidateTimeWithin7DaysCount += receipt.CandidateTimeWithin7DaysCount
	aggregate.CandidateTimeBeyond7DaysCount += receipt.CandidateTimeBeyond7DaysCount
	aggregate.CandidateTimeUnavailableCount += receipt.CandidateTimeUnavailableCount
	aggregate.CandidateExactEventKeyMatchCount += receipt.CandidateExactEventKeyMatchCount
	aggregate.CandidateExactEventKeyUnavailableCount += receipt.CandidateExactEventKeyUnavailableCount
}

func receiptRarityBucket(receipt domain.SemanticSignalReceipt) string {
	if receipt.TriggerTokenTotal == 0 {
		return "unavailable"
	}
	if receipt.TriggerUnknownTokenCount > 0 {
		return "unknown"
	}
	switch {
	case receipt.TriggerRareTokenCount > 0 && receipt.TriggerCommonTokenCount > 0:
		return "mixed"
	case receipt.TriggerCommonTokenCount > 0:
		return "common"
	case receipt.TriggerRareTokenCount > 0:
		return "rare"
	default:
		return "unavailable"
	}
}
