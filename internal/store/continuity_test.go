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
