package store

import (
	"context"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestContinuityEngagementScoreParsesRenderedSocialCounts(t *testing.T) {
	value := map[string]any{
		"like":    "1.2K",
		"comment": "1,234",
		"share":   "2,5K",
		"nested":  map[string]any{"repost": float64(16)},
	}
	if got, want := continuityEngagementScore(value), int64(4_950); got != want {
		t.Fatalf("continuity engagement score=%d want=%d", got, want)
	}
}

func TestEngagementOnlyChangeRemainsUnchangedNativeResurface(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ResurfaceMode = "smart"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	previousSession, err := createVisibleUpdateSession(state, ctx, "previous X engagement snapshot", settings)
	if err != nil {
		t.Fatal(err)
	}
	previousRuns, err := state.listRuns(ctx, previousSession.ID)
	if err != nil || len(previousRuns) != 1 {
		t.Fatalf("previous X runs=%+v err=%v", previousRuns, err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id=?`, domain.Now(), previousRuns[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, domain.Now(), previousSession.ID); err != nil {
		t.Fatal(err)
	}
	currentSession, err := createVisibleUpdateSession(state, ctx, "current X engagement snapshot", settings)
	if err != nil {
		t.Fatal(err)
	}
	currentRuns, err := state.listRuns(ctx, currentSession.ID)
	if err != nil || len(currentRuns) != 1 {
		t.Fatalf("current X runs=%+v err=%v", currentRuns, err)
	}
	run := currentRuns[0]
	block := domain.Block{
		EvidenceKey: "x:03f742140c3d0ccc5bf3f66e",
		Author:      "Stable X author",
		Text:        "The same X post remains unchanged while its engagement grows.",
		Permalink:   "https://x.com/stable/status/123",
		PlatformID:  "123",
		ContentKind: "post",
		Engagement:  map[string]any{"likes": "622", "reposts": "0"},
	}
	previous := block
	previous.Engagement = map[string]any{"likes": "294", "reposts": "0"}
	priorCommand, err := state.StartRun(ctx, previousRuns[0].ID, map[string]any{"source": domain.SourceX})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, previousRuns[0].ID, "engagement-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveObservation(ctx, priorCommand.ID, previousRuns[0].ID, domain.Observation{
		Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{previous}}},
	}); err != nil {
		t.Fatal(err)
	}
	finishIdentityRun(t, state, previousRuns[0])
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO content_continuity(
		  source,evidence_key,content_fingerprint,context_fingerprint,engagement_score,
		  first_seen_at,last_seen_at,last_run_id,seen_count
		) VALUES(?,?,?,?,?,?,?,?,?)`,
		run.Source, block.EvidenceKey, continuityContentFingerprint(previous), continuityContextFingerprint(previous),
		continuityEngagementScore(previous.Engagement), domain.Now(), domain.Now(), previousRuns[0].ID, 1); err != nil {
		t.Fatal(err)
	}

	decisions, err := state.ClassifyContentContinuity(ctx, run, domain.Observation{
		Source:     domain.SourceX,
		CapturedAt: domain.Now(),
		Snapshots:  []domain.Snapshot{{Blocks: []domain.Block{block}}},
	}, settings)
	if err != nil {
		t.Fatal(err)
	}
	decision := decisions[block.EvidenceKey]
	if decision.Status != "resurfaced_unchanged" || decision.Action != "fail_fast" {
		t.Fatalf("engagement-only continuity decision=%+v", decision)
	}
}

func TestXQuoteContinuityWithIntermittentParentPermalink(t *testing.T) {
	const (
		firstQuote  = "https://x.com/quoted_author/status/2119894002057302330"
		secondQuote = "https://x.com/quoted_author/status/2119894002057302331"
		quoteText   = "Use one model for planning and a different model for coding."
		changedText = "The recommendation now includes a different model for coding."
	)
	type step struct {
		parent, text, conflict, status string
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"loss recovery and different parent", []step{
			{firstQuote, quoteText, "", "fresh"},
			{"", quoteText, "", "resurfaced_unchanged"},
			{"", quoteText, "", "resurfaced_unchanged"},
			{firstQuote, quoteText, "", "resurfaced_unchanged"},
			{"", quoteText, "", "resurfaced_unchanged"},
			{secondQuote, quoteText, "", "resurfaced_changed"},
		}},
		{"first verified parent enriches the same quote", []step{
			{"", quoteText, "", "fresh"},
			{firstQuote, quoteText, "", "resurfaced_unchanged"},
			{secondQuote, quoteText, "", "resurfaced_changed"},
		}},
		{"quote body changes while the parent is missing", []step{
			{firstQuote, quoteText, "", "fresh"},
			{"", changedText, "", "resurfaced_changed"},
		}},
		{"conflicting evidence is not treated as hydration", []step{
			{firstQuote, quoteText, "", "fresh"},
			{"", quoteText, "dom_identity_conflict", "resurfaced_changed"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
			settings.ActiveSources = []domain.Source{domain.SourceX}
			settings.ResurfaceMode = "smart"
			for i, step := range tt.steps {
				session, err := createVisibleUpdateSession(state, ctx, "X quote continuity fixture", settings)
				if err != nil {
					t.Fatal(err)
				}
				runs, err := state.listRuns(ctx, session.ID)
				if err != nil || len(runs) != 1 {
					t.Fatalf("X runs=%+v err=%v", runs, err)
				}
				run := runs[0]
				command, err := state.StartRun(ctx, run.ID, map[string]any{"source": domain.SourceX})
				if err != nil {
					t.Fatal(err)
				}
				if claimed, err := state.ClaimCommand(ctx, run.ID, "x-quote-fixture"); err != nil || claimed == nil {
					t.Fatalf("claim=%+v err=%v", claimed, err)
				}
				block := domain.Block{
					EvidenceKey: "x:quote-continuity-fixture", Author: "Fixture Author", Text: "I disagree with the hardware recommendation in this quote.",
					Permalink: "https://x.com/fixture_author/status/2119895564720435504", PlatformID: "x:status:2119895564720435504",
					ContentKind: "post", RelationshipType: "quote", ParentPermalink: step.parent,
					QuotedPost: map[string]any{"author": "Quoted Author", "text": step.text, "permalink": step.parent, "identityConflictReason": step.conflict},
				}
				if err := state.SaveObservation(ctx, command.ID, run.ID, domain.Observation{
					Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}},
				}); err != nil {
					t.Fatal(err)
				}
				observation := storedIdentityObservation(t, state, run.ID)
				key := observation.Snapshots[0].Blocks[0].EvidenceKey
				decisions, err := state.ClassifyContentContinuity(ctx, run, observation, settings)
				if err != nil {
					t.Fatal(err)
				}
				action := "evaluate"
				if step.status == "resurfaced_unchanged" {
					action = "fail_fast"
				}
				if decision := decisions[key]; decision.Status != step.status || decision.Action != action {
					t.Fatalf("capture %d got %+v, want %s/%s", i+1, decision, step.status, action)
				}
				finishIdentityRun(t, state, run)
			}
		})
	}
}

func TestLegacyNativeFingerprintMigratesWithoutFalseContentChange(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	run, command := startClaimedIdentityRun(t, state)
	block := domain.Block{
		EvidenceKey: "linkedin:legacy-native",
		Author:      "Legacy Author",
		Text:        strings.Repeat("A retained native LinkedIn post must not look changed when identity leaves the content fingerprint. ", 2),
		ContentKind: "post",
		PlatformID:  "urn:li:activity:7411111111111111111",
	}
	saveIdentityObservation(t, state, run, command, block)
	observation := storedIdentityObservation(t, state, run.ID)
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO content_continuity(
		  source,evidence_key,content_fingerprint,context_fingerprint,engagement_score,
		  first_seen_at,last_seen_at,last_run_id,seen_count
		) VALUES(?,?,?,?,?,?,?,?,1)`,
		run.Source, block.EvidenceKey, legacyContinuityContentFingerprint(block),
		continuityContextFingerprint(block), 0, domain.Now(), domain.Now(), "legacy-run"); err != nil {
		t.Fatal(err)
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := state.ClassifyContentContinuity(ctx, run, observation, settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := decisions[block.EvidenceKey].Status; got != "resurfaced_unchanged" {
		t.Fatalf("legacy fingerprint status=%q", got)
	}
	var migrated string
	if err := state.db.QueryRowContext(ctx, `
		SELECT content_fingerprint FROM content_continuity
		WHERE source=? AND evidence_key=?`, run.Source, block.EvidenceKey).Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if migrated != continuityContentFingerprint(block) {
		t.Fatalf("content fingerprint was not migrated: %q", migrated)
	}
}
