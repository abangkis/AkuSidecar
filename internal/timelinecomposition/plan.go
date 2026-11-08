// Package timelinecomposition plans the global order and capacity of a session
// without reading or writing persisted state.
package timelinecomposition

// Candidate contains only the fields used by composition. Candidates must arrive
// in the store's score, creation time, and existing rank order; Plan does not sort.
type Candidate struct {
	Source           string
	EvidenceKey      string
	SemanticEventID  string
	SemanticRelation string
}

// Decision refers to a candidate by its input index. Rank is zero-based for kept
// candidates and -1 for dropped candidates. Decisions include every input in
// diversified order, which is also the store's write order.
type Decision struct {
	Index int
	Keep  bool
	Rank  int
}

// Plan diversifies the full input before applying independent unique-information
// and duplicate-report budgets. Dropped candidates still influence diversity.
// The input is not modified.
func Plan(candidates []Candidate, limit int) []Decision {
	remaining := make([]int, len(candidates))
	for index := range candidates {
		remaining[index] = index
	}
	ordered := make([]int, 0, len(candidates))
	for len(remaining) > 0 {
		chosen := 0
		if len(ordered) >= 2 && candidates[ordered[len(ordered)-1]].Source == candidates[ordered[len(ordered)-2]].Source {
			for index, candidate := range remaining {
				if candidates[candidate].Source != candidates[ordered[len(ordered)-1]].Source {
					chosen = index
					break
				}
			}
		}
		ordered = append(ordered, remaining[chosen])
		remaining = append(remaining[:chosen], remaining[chosen+1:]...)
	}

	decisions := make([]Decision, 0, len(ordered))
	uniqueKept, duplicateKept, rank := 0, 0, 0
	duplicatesPerEvent := map[string]int{}
	for _, index := range ordered {
		candidate := candidates[index]
		decision := Decision{Index: index, Rank: -1}
		if candidate.SemanticRelation == "duplicate_report" {
			eventKey := candidate.SemanticEventID
			if eventKey == "" {
				eventKey = candidate.EvidenceKey
			}
			if duplicateKept < limit && duplicatesPerEvent[eventKey] < 3 {
				decision.Keep = true
				duplicateKept++
				duplicatesPerEvent[eventKey]++
			}
		} else if uniqueKept < limit {
			decision.Keep = true
			uniqueKept++
		}
		if decision.Keep {
			decision.Rank = rank
			rank++
		}
		decisions = append(decisions, decision)
	}
	return decisions
}
