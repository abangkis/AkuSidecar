package reasoning

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestBindEvidenceKeysByAliasPreservesIdentityAcrossIndependentPermutations(t *testing.T) {
	aliases := []string{"candidate_007", "candidate_008", "candidate_009"}
	keys := []string{"x:first", "x:second", "x:third"}
	result := domain.ReasoningResult{Summary: "summary", Limitations: []string{"limitation"}, RepeatedClaimsCollapsed: 2, DeferredByBudget: 1}
	for _, index := range []int{1, 2, 0} {
		result.Items = append(result.Items, domain.ReasonedItem{ID: "model-generated-id", EvidenceKey: aliases[index], WhatChanged: fmt.Sprintf("item-%d", index)})
	}
	for _, index := range []int{2, 0, 1} {
		result.CandidateAssessments = append(result.CandidateAssessments, domain.CandidateAssessment{EvidenceKey: aliases[index], Rationale: fmt.Sprintf("assessment-%d", index), Materiality: float64(index) / 10})
	}
	if err := bindEvidenceKeysByAlias(&result, aliases, keys); err != nil {
		t.Fatal(err)
	}
	for index, key := range keys {
		item, assessment := result.Items[index], result.CandidateAssessments[index]
		if item.ID != key || item.EvidenceKey != key || item.WhatChanged != fmt.Sprintf("item-%d", index) || assessment.EvidenceKey != key || assessment.Rationale != fmt.Sprintf("assessment-%d", index) || assessment.Materiality != float64(index)/10 {
			t.Fatalf("candidate %d was misbound: item=%+v assessment=%+v", index, item, assessment)
		}
	}
	if result.Summary != "summary" || !reflect.DeepEqual(result.Limitations, []string{"limitation"}) || result.RepeatedClaimsCollapsed != 2 || result.DeferredByBudget != 1 {
		t.Fatalf("result metadata changed: %+v", result)
	}
}

func TestBindEvidenceKeysByAliasRejectsInvalidIdentityWithoutMutation(t *testing.T) {
	for _, mode := range []string{
		"missing-item", "missing-assessment", "unknown-item", "unknown-assessment", "empty-item", "empty-assessment",
		"duplicate-item", "duplicate-assessment", "invented-native-keys", "request-count", "request-empty-alias", "request-empty-key", "request-duplicate-alias", "request-duplicate-key",
	} {
		t.Run(mode, func(t *testing.T) {
			aliases := []string{"candidate_007", "candidate_008"}
			keys := []string{"x:first", "x:second"}
			result := domain.ReasoningResult{
				Items:                []domain.ReasonedItem{{ID: "first", EvidenceKey: aliases[0]}, {ID: "second", EvidenceKey: aliases[1]}},
				CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: aliases[0]}, {EvidenceKey: aliases[1]}},
			}
			want := ""
			switch mode {
			case "missing-item":
				result.Items = result.Items[:1]
				want = "1 items for 2 candidates"
			case "missing-assessment":
				result.CandidateAssessments = result.CandidateAssessments[:1]
				want = "1 assessments for 2 candidates"
			case "unknown-item", "empty-item":
				result.Items[1].EvidenceKey = "candidate_001"
				if mode == "empty-item" {
					result.Items[1].EvidenceKey = ""
				}
				want = "item references unknown candidate alias"
			case "unknown-assessment", "empty-assessment":
				result.CandidateAssessments[1].EvidenceKey = "candidate_001"
				if mode == "empty-assessment" {
					result.CandidateAssessments[1].EvidenceKey = ""
				}
				want = "assessment references unknown candidate alias"
			case "duplicate-item":
				result.Items[1].EvidenceKey = aliases[0]
				want = "duplicate item alias"
			case "duplicate-assessment":
				result.CandidateAssessments[1].EvidenceKey = aliases[0]
				want = "duplicate assessment alias"
			case "invented-native-keys":
				result.Items[0].EvidenceKey = "x:invented"
				result.CandidateAssessments[0].EvidenceKey = "linkedin:invented"
				want = "item references unknown candidate alias"
			case "request-count":
				aliases = aliases[:1]
				want = "request has 1 aliases for 2 candidates"
			case "request-empty-alias":
				aliases[1] = ""
				want = "empty alias or evidence key"
			case "request-empty-key":
				keys[1] = ""
				want = "empty alias or evidence key"
			case "request-duplicate-alias":
				aliases[1] = aliases[0]
				want = "request has duplicate alias"
			case "request-duplicate-key":
				keys[1] = keys[0]
				want = "request has duplicate evidence key"
			}
			before, _ := json.Marshal(result)
			// Retain aliases to the original slices so a rejected bind cannot leave
			// even a partially rewritten caller-owned response behind.
			items, assessments := result.Items, result.CandidateAssessments
			err := bindEvidenceKeysByAlias(&result, aliases, keys)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v, want %q", err, want)
			}
			after, _ := json.Marshal(result)
			if string(after) != string(before) || !reflect.DeepEqual(items, result.Items) || !reflect.DeepEqual(assessments, result.CandidateAssessments) {
				t.Fatal("failed binding mutated the model result")
			}
		})
	}
}
