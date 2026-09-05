package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLivingTopicTemporalOverviewAndMaterialLifecycle(t *testing.T) {
	oldTime, newTime := "2026-08-31T00:00:00Z", "2026-09-05T07:00:00+07:00"
	items := []domain.MemoryItem{{ID: "old", PublishedAt: &oldTime}, {ID: "new", PublishedAt: &newTime}, {ID: "undated"}}
	claims := []domain.LivingTopicClaim{
		{Key: "milestone-reset", Text: "25 million milestone reset.", Assessment: "supported", Centrality: "central", TemporalStatus: "historical", EventStatus: "completed", EvidenceIDs: []string{"old"}},
		{Key: "credit-validity", Text: "Credit validity is unknown.", Assessment: "uncertain", Centrality: "central", TemporalStatus: "current", EventStatus: "unknown", EvidenceIDs: []string{"new"}},
		{Key: "astra-rollout", Text: "Rollout completed; final compensation announced.", Assessment: "supported", Centrality: "central", TemporalStatus: "current", EventStatus: "completed", EvidenceIDs: []string{"new"}},
	}
	asOf := annotateLivingTopicClaimTimes(claims, items)
	if asOf != "2026-09-05T00:00:00Z" {
		t.Fatalf("source date must be UTC, got %q", asOf)
	}
	overview := livingTopicCentralOverview(claims)
	if overview != "Rollout completed; final compensation announced." {
		t.Fatalf("historical/uncertain claims leaked into overview: %s", overview)
	}
	previous := []domain.LivingTopicClaim{{Key: "rollout", Text: "Rollout underway.", MaterialValue: "rollout", TemporalStatus: "current", EventStatus: "ongoing"}}
	current := append([]domain.LivingTopicClaim(nil), previous...)
	current[0].EventStatus = "completed"
	if !livingTopicClaimsMateriallyChanged(previous, current) {
		t.Fatal("lifecycle transition must create a material delta")
	}
	deltas := livingTopicClaimDeltas(&domain.LivingTopicSnapshot{ContractVersion: livingTopicUnderstandingContractVersion, Claims: previous}, nil, true)
	if len(deltas) != 1 || deltas[0].Kind != "removed" {
		t.Fatalf("disappearance is not completion: %+v", deltas)
	}
}

func TestLivingTopicUnknownDatesAndHistoricalOnlyAreTruthful(t *testing.T) {
	invalid := "not a timestamp"
	claims := []domain.LivingTopicClaim{{Key: "old", Text: "Reset announced.", TemporalStatus: "historical", Assessment: "supported", Centrality: "central", EvidenceIDs: []string{"unknown"}, LatestEvidenceAt: "model-invented"}}
	if asOf := annotateLivingTopicClaimTimes(claims, []domain.MemoryItem{{ID: "unknown", PublishedAt: &invalid}}); asOf != "" || claims[0].LatestEvidenceAt != "" {
		t.Fatalf("unknown publication date was invented: %s %+v", asOf, claims)
	}
	if overview := livingTopicCentralOverview(claims); strings.Contains(overview, "Reset announced") || !strings.Contains(overview, "does not establish") {
		t.Fatalf("historical-only evidence must not claim current applicability: %s", overview)
	}
}

func TestLivingTopicRoutingContextUsesAttachedEvidenceWithoutPriorProse(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	topic, err := state.CreateLivingTopic(ctx, "Codex reset")
	if err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"Milestone reset", "Banked reset during Astra rollout"} {
		input := livingTopicMemoryInput(title)
		input.Identity.CanonicalEvidenceKey += title
		input.Identity.CanonicalPermalink = fmt.Sprintf("https://x.com/example/status/%d", i+1)
		input.Identity.CanonicalPlatformID = fmt.Sprint(i + 1)
		input.Identity.ContentFingerprint += title
		date := []string{"2026-08-31T00:00:00Z", "2026-09-04T00:00:00Z"}[i]
		input.PublishedAt = &date
		item, err := state.CreateMemoryRecallStub(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.AddLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	withContext, err := runtime.livingTopicWithRoutingContext(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	if len(withContext.RoutingContext) != 2 || withContext.RoutingContext[0].Title != "Banked reset during Astra rollout" {
		t.Fatalf("latest attached source must anchor continuity: %+v", withContext.RoutingContext)
	}
}
