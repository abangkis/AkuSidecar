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

func TestMaterialEngagementChangeUsesNormalizedCounts(t *testing.T) {
	if !materialEngagementChange(
		continuityEngagementScore(map[string]any{"like": "1.0K"}),
		continuityEngagementScore(map[string]any{"like": "1.3K"}),
	) {
		t.Fatal("a 30 percent rendered engagement increase should be material")
	}
	if materialEngagementChange(
		continuityEngagementScore(map[string]any{"like": "1.0K"}),
		continuityEngagementScore(map[string]any{"like": "1.1K"}),
	) {
		t.Fatal("a 10 percent rendered engagement increase should remain below the gate")
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
