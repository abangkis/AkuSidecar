package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestFallbackIdentityPromotesToNativeWithoutASecondEvaluation(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	text := strings.Repeat("A durable LinkedIn identity recovery example with exact author and content. ", 3)
	publishedAt := "2026-07-26T17:07:00.000Z"

	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:fallback-evidence",
		Author:      "Rivelino Hasugian",
		Text:        text,
		PublishedAt: &publishedAt,
		ContentKind: "post",
	})
	firstObservation := storedIdentityObservation(t, state, firstRun.ID)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstDecisions, err := state.ClassifyContentContinuity(ctx, firstRun, firstObservation, settings)
	if err != nil {
		t.Fatal(err)
	}
	if firstDecisions["linkedin:fallback-evidence"].Status != "fresh" {
		t.Fatalf("first decision=%+v", firstDecisions)
	}
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:native-evidence",
		Author:      "Rivelino Hasugian",
		Text:        text,
		PublishedAt: &publishedAt,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7411111111111111111",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111",
	})
	secondObservation := storedIdentityObservation(t, state, secondRun.ID)
	block := secondObservation.Snapshots[0].Blocks[0]
	if block.EvidenceKey != "linkedin:fallback-evidence" {
		t.Fatalf("canonical evidence key=%q", block.EvidenceKey)
	}
	if block.PlatformID == "" || block.Permalink == "" {
		t.Fatalf("native evidence was lost during promotion: %+v", block)
	}
	identity := secondObservation.Coverage["contentIdentity"].(map[string]any)
	if integerValue(identity["fallbacksPromoted"]) != 1 || integerValue(identity["aliasesReused"]) != 1 {
		t.Fatalf("identity receipt=%+v", identity)
	}

	secondDecisions, err := state.ClassifyContentContinuity(ctx, secondRun, secondObservation, settings)
	if err != nil {
		t.Fatal(err)
	}
	decision := secondDecisions["linkedin:fallback-evidence"]
	if decision.Status != "resurfaced_unchanged" || decision.Action != "fail_fast" {
		t.Fatalf("promoted continuity decision=%+v", decision)
	}
	diagnostic, err := state.inboxRun(ctx, secondRun)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.IdentityResolution == nil ||
		diagnostic.IdentityResolution.FallbacksPromoted != 1 ||
		diagnostic.IdentityResolution.AliasesReused != 1 {
		t.Fatalf("Inbox identity resolution=%+v", diagnostic.IdentityResolution)
	}
}

func TestFallbackIdentityDoesNotMergeARepublishedNativePost(t *testing.T) {
	text := strings.Repeat("An author can intentionally republish the same substantial wording as a distinct native post. ", 2)
	firstPublishedAt := "2026-07-26T17:07:00.000Z"
	secondPublishedAt := "2026-07-27T17:07:00.000Z"
	state := openTestStore(t)

	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:original-fallback",
		Author:      "Repeat Author",
		Text:        text,
		PublishedAt: &firstPublishedAt,
		ContentKind: "post",
	})
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:republished-native",
		Author:      "Repeat Author",
		Text:        text,
		PublishedAt: &secondPublishedAt,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7444444444444444444",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7444444444444444444",
	})
	observation := storedIdentityObservation(t, state, secondRun.ID)
	if got := observation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:republished-native" {
		t.Fatalf("republished native post was incorrectly merged to %q", got)
	}
	identity := observation.Coverage["contentIdentity"].(map[string]any)
	if integerValue(identity["ambiguousFallbacks"]) != 1 || integerValue(identity["fallbacksPromoted"]) != 0 {
		t.Fatalf("identity receipt=%+v", identity)
	}
}

func TestFallbackIdentityWithoutTimestampExpiresBeforeNativePromotion(t *testing.T) {
	state := openTestStore(t)
	text := strings.Repeat("A timestamp-free identity candidate remains eligible only during a bounded recovery window. ", 2)

	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:old-fallback",
		Author:      "Bounded Author",
		Text:        text,
		ContentKind: "post",
	})
	finishIdentityRun(t, state, firstRun)
	expired := time.Now().UTC().Add(-contentIdentityFallbackWindow - time.Minute).Format(time.RFC3339Nano)
	if _, err := state.db.ExecContext(context.Background(), `UPDATE content_identity_aliases SET first_seen_at=?`, expired); err != nil {
		t.Fatal(err)
	}

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:new-native",
		Author:      "Bounded Author",
		Text:        text,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7455555555555555555",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7455555555555555555",
	})
	observation := storedIdentityObservation(t, state, secondRun.ID)
	if got := observation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:new-native" {
		t.Fatalf("expired fallback was incorrectly promoted to %q", got)
	}
}

func TestFallbackIdentityDoesNotCrossContentKinds(t *testing.T) {
	state := openTestStore(t)
	text := strings.Repeat("The same author and wording can still represent different source-native content kinds. ", 2)

	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:post-fallback",
		Author:      "Format Author",
		Text:        text,
		ContentKind: "post",
	})
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:video-native",
		Author:      "Format Author",
		Text:        text,
		ContentKind: "video",
		PlatformID:  "urn:li:activity:7466666666666666666",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7466666666666666666",
	})
	observation := storedIdentityObservation(t, state, secondRun.ID)
	if got := observation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:video-native" {
		t.Fatalf("different content kind was incorrectly merged to %q", got)
	}
}

func TestDistinctNativeIdentitiesNeverMergeOnMatchingText(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	text := strings.Repeat("The same substantial syndicated wording can legitimately identify different native posts. ", 2)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:native-a",
		Author:      "Shared Author",
		Text:        text,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7411111111111111111",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111",
	})
	firstObservation := storedIdentityObservation(t, state, firstRun.ID)
	if _, err := state.ClassifyContentContinuity(ctx, firstRun, firstObservation, settings); err != nil {
		t.Fatal(err)
	}
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:native-b",
		Author:      "Shared Author",
		Text:        text,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7422222222222222222",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7422222222222222222",
	})
	secondObservation := storedIdentityObservation(t, state, secondRun.ID)
	if got := secondObservation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:native-b" {
		t.Fatalf("distinct native evidence was incorrectly merged to %q", got)
	}
	identity := secondObservation.Coverage["contentIdentity"].(map[string]any)
	if integerValue(identity["nativeConflicts"]) != 1 {
		t.Fatalf("identity conflict receipt=%+v", identity)
	}
	decisions, err := state.ClassifyContentContinuity(ctx, secondRun, secondObservation, settings)
	if err != nil {
		t.Fatal(err)
	}
	if decisions["linkedin:native-b"].Status != "fresh" {
		t.Fatalf("distinct native post was not fresh: %+v", decisions)
	}
	finishIdentityRun(t, state, secondRun)

	fallbackRun, fallbackCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, fallbackRun, fallbackCommand, domain.Block{
		EvidenceKey: "linkedin:ambiguous-fallback",
		Author:      "Shared Author",
		Text:        text,
		ContentKind: "post",
	})
	fallbackObservation := storedIdentityObservation(t, state, fallbackRun.ID)
	if got := fallbackObservation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:ambiguous-fallback" {
		t.Fatalf("ambiguous fallback was incorrectly merged to %q", got)
	}
	fallbackIdentity := fallbackObservation.Coverage["contentIdentity"].(map[string]any)
	if integerValue(fallbackIdentity["ambiguousFallbacks"]) != 1 {
		t.Fatalf("ambiguous fallback receipt=%+v", fallbackIdentity)
	}
}

func TestShortGenericFallbackIsNotPromoted(t *testing.T) {
	state := openTestStore(t)
	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:short-fallback",
		Author:      "Shared Author",
		Text:        "We are hiring.",
		ContentKind: "post",
	})
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:short-native",
		Author:      "Shared Author",
		Text:        "We are hiring.",
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7433333333333333333",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7433333333333333333",
	})
	secondObservation := storedIdentityObservation(t, state, secondRun.ID)
	if got := secondObservation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:short-native" {
		t.Fatalf("short generic content was promoted to %q", got)
	}
}

func TestNativeIdentityRepresentationsConvergeWithoutConflict(t *testing.T) {
	text := strings.Repeat("A native LinkedIn post can reveal its platform ID and canonical permalink in different capture rounds. ", 2)
	for _, test := range []struct {
		name  string
		first domain.Block
	}{
		{
			name: "permalink first",
			first: domain.Block{
				Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111",
			},
		},
		{
			name: "platform ID first",
			first: domain.Block{
				PlatformID: "urn:li:activity:7411111111111111111",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := openTestStore(t)
			firstRun, firstCommand := startClaimedIdentityRun(t, state)
			firstBlock := test.first
			firstBlock.EvidenceKey = "linkedin:first-native-representation"
			firstBlock.Author = "Shared Author"
			firstBlock.Text = text
			firstBlock.ContentKind = "post"
			saveIdentityObservation(t, state, firstRun, firstCommand, firstBlock)
			finishIdentityRun(t, state, firstRun)

			secondRun, secondCommand := startClaimedIdentityRun(t, state)
			saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
				EvidenceKey: "linkedin:complete-native-representation",
				Author:      "Shared Author",
				Text:        text,
				ContentKind: "post",
				PlatformID:  "urn:li:activity:7411111111111111111",
				Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111",
			})
			observation := storedIdentityObservation(t, state, secondRun.ID)
			if got := observation.Snapshots[0].Blocks[0].EvidenceKey; got != firstBlock.EvidenceKey {
				t.Fatalf("native representation resolved to %q want %q", got, firstBlock.EvidenceKey)
			}
			identity := observation.Coverage["contentIdentity"].(map[string]any)
			if integerValue(identity["nativeConflicts"]) != 0 || integerValue(identity["aliasesReused"]) != 1 {
				t.Fatalf("identity receipt=%+v", identity)
			}
		})
	}
}

func TestMatchingPlatformIdentityOutranksPermalinkSpellingDifferences(t *testing.T) {
	state := openTestStore(t)
	text := strings.Repeat("A stable LinkedIn platform identity can appear through harmless permalink spelling variations. ", 2)
	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:first-url",
		Author:      "Stable Author",
		Text:        text,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7477777777777777777",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7477777777777777777/",
	})
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:tracked-url",
		Author:      "Stable Author",
		Text:        text,
		ContentKind: "post",
		PlatformID:  "linkedin:activity:7477777777777777777",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7477777777777777777/?trackingId=bounded",
	})
	observation := storedIdentityObservation(t, state, secondRun.ID)
	if got := observation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:first-url" {
		t.Fatalf("matching platform identity resolved to %q", got)
	}
	identity := observation.Coverage["contentIdentity"].(map[string]any)
	if integerValue(identity["nativeConflicts"]) != 0 || integerValue(identity["aliasesReused"]) != 1 {
		t.Fatalf("identity receipt=%+v", identity)
	}
}

func TestEstimatedTimestampDriftUsesBoundedFallbackWindow(t *testing.T) {
	state := openTestStore(t)
	text := strings.Repeat("A relative timestamp estimate may drift while identity recovery remains bounded by capture time. ", 2)
	firstPublishedAt := "2026-07-26T17:00:00Z"
	secondPublishedAt := "2026-07-26T18:00:00Z"
	firstRun, firstCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, firstRun, firstCommand, domain.Block{
		EvidenceKey: "linkedin:estimated-fallback",
		Author:      "Estimated Author",
		Text:        text,
		PublishedAt: &firstPublishedAt,
		ContentKind: "post",
		Presentation: map[string]any{
			"timestampEstimated": true,
			"timestampPrecision": "hour",
		},
	})
	finishIdentityRun(t, state, firstRun)

	secondRun, secondCommand := startClaimedIdentityRun(t, state)
	saveIdentityObservation(t, state, secondRun, secondCommand, domain.Block{
		EvidenceKey: "linkedin:estimated-native",
		Author:      "Estimated Author",
		Text:        text,
		PublishedAt: &secondPublishedAt,
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7499999999999999999",
		Permalink:   "https://www.linkedin.com/feed/update/urn:li:activity:7499999999999999999",
		Presentation: map[string]any{
			"timestampEstimated": true,
			"timestampPrecision": "hour",
		},
	})
	observation := storedIdentityObservation(t, state, secondRun.ID)
	if got := observation.Snapshots[0].Blocks[0].EvidenceKey; got != "linkedin:estimated-fallback" {
		t.Fatalf("estimated timestamp drift resolved to %q", got)
	}
}

func startClaimedIdentityRun(t *testing.T, state *Store) (domain.Run, domain.BridgeCommand) {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "identity resolution", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("identity runs=%+v err=%v", runs, err)
	}
	command, err := state.StartRun(ctx, runs[0].ID, map[string]any{"source": domain.SourceLinkedIn})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := state.ClaimCommand(ctx, runs[0].ID, "identity-test")
	if err != nil || claimed == nil {
		t.Fatalf("identity claim=%+v err=%v", claimed, err)
	}
	return runs[0], command
}

func saveIdentityObservation(t *testing.T, state *Store, run domain.Run, command domain.BridgeCommand, block domain.Block) {
	t.Helper()
	observation := domain.Observation{
		Source:     run.Source,
		PageURL:    "https://www.linkedin.com/feed/",
		PageTitle:  "LinkedIn",
		CapturedAt: domain.Now(),
		Snapshots:  []domain.Snapshot{{Blocks: []domain.Block{block}}},
		Coverage:   map[string]any{"status": "partial"},
	}
	if err := state.SaveObservation(context.Background(), command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
}

func storedIdentityObservation(t *testing.T, state *Store, runID string) domain.Observation {
	t.Helper()
	values, err := state.Observations(context.Background(), runID)
	if err != nil || len(values) != 1 {
		t.Fatalf("stored observations=%+v err=%v", values, err)
	}
	return values[0]
}

func finishIdentityRun(t *testing.T, state *Store, run domain.Run) {
	t.Helper()
	now := domain.Now()
	if _, err := state.db.ExecContext(context.Background(), `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id=?`, now, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(context.Background(), `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, now, run.SessionID); err != nil {
		t.Fatal(err)
	}
}
