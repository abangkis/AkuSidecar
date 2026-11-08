package timelinecomposition

import (
	"reflect"
	"testing"
)

func TestPlanDiversityUsesFullInputBeforePruning(t *testing.T) {
	candidates := []Candidate{
		{Source: "x"}, {Source: "x"}, {Source: "x"}, {Source: "linkedin"},
		{Source: "x", SemanticRelation: "duplicate_report", SemanticEventID: "a"},
		{Source: "x", SemanticRelation: "duplicate_report", SemanticEventID: "b"},
		{Source: "linkedin", SemanticRelation: "duplicate_report", SemanticEventID: "c"},
	}
	before := append([]Candidate(nil), candidates...)
	want := []Decision{
		{Index: 0, Keep: true, Rank: 0}, {Index: 1, Keep: true, Rank: 1},
		{Index: 3, Rank: -1}, {Index: 2, Rank: -1},
		{Index: 4, Keep: true, Rank: 2}, {Index: 6, Keep: true, Rank: 3},
		{Index: 5, Rank: -1},
	}
	for attempt := 0; attempt < 2; attempt++ {
		if got := Plan(candidates, 2); !reflect.DeepEqual(got, want) {
			t.Fatalf("plan=%+v, want %+v", got, want)
		}
		if !reflect.DeepEqual(candidates, before) {
			t.Fatal("Plan changed its input")
		}
	}
}

func TestPlanDuplicateEventCapAndEvidenceFallback(t *testing.T) {
	candidates := []Candidate{
		{EvidenceKey: "fallback", SemanticRelation: "duplicate_report"},
		{EvidenceKey: "fallback", SemanticRelation: "duplicate_report"},
		{SemanticEventID: "fallback", SemanticRelation: "duplicate_report"},
		{EvidenceKey: "fallback", SemanticRelation: "duplicate_report"},
		{EvidenceKey: "other", SemanticRelation: "duplicate_report"},
		{SemanticEventID: "second", SemanticRelation: "duplicate_report"},
		{SemanticEventID: "third", SemanticRelation: "duplicate_report"},
		{SemanticRelation: "new_event"}, {SemanticRelation: ""},
		{SemanticRelation: "material_update"}, {SemanticRelation: "unknown"}, {}, {},
	}
	wantRanks := []int{0, 1, 2, -1, 3, 4, -1, 5, 6, 7, 8, 9, -1}
	got := Plan(candidates, 5)
	for index, want := range wantRanks {
		if got[index].Index != index || got[index].Rank != want || got[index].Keep != (want >= 0) {
			t.Fatalf("decision[%d]=%+v, want rank %d", index, got[index], want)
		}
	}
}

func TestPlanPreservesInputOrderWhenDiversityHasNoAlternative(t *testing.T) {
	candidates := []Candidate{{Source: "x", EvidenceKey: "z"}, {Source: "x", EvidenceKey: "a"}, {Source: "x", EvidenceKey: "m"}}
	want := []Decision{{Index: 0, Keep: true, Rank: 0}, {Index: 1, Keep: true, Rank: 1}, {Index: 2, Keep: true, Rank: 2}}
	if got := Plan(candidates, 3); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan=%+v, want %+v", got, want)
	}
}

func TestPlanEmptyAndNonpositiveLimits(t *testing.T) {
	if got := Plan(nil, 3); len(got) != 0 {
		t.Fatalf("empty plan=%+v", got)
	}
	for _, limit := range []int{0, -1} {
		got := Plan([]Candidate{{}, {SemanticRelation: "duplicate_report"}}, limit)
		want := []Decision{{Index: 0, Rank: -1}, {Index: 1, Rank: -1}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("limit=%d plan=%+v, want %+v", limit, got, want)
		}
	}
}
