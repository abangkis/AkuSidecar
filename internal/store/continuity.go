package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type continuityRecord struct {
	contentFingerprint string
	contextFingerprint string
	engagementScore    int64
	lastSeenAt         string
	lastRunID          string
}

func (s *Store) ClassifyContentContinuity(ctx context.Context, run domain.Run, observation domain.Observation, settings domain.Settings) (map[string]domain.ContentContinuityDecision, error) {
	blocks := canonicalContinuityBlocks(observation)
	decisions := make(map[string]domain.ContentContinuityDecision, len(blocks))
	if len(blocks) == 0 {
		return decisions, nil
	}
	observedAt := strings.TrimSpace(observation.CapturedAt)
	if _, err := time.Parse(time.RFC3339Nano, observedAt); err != nil {
		observedAt = domain.Now()
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -settings.ResurfaceCooldownDays)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, block := range blocks {
		key := strings.TrimSpace(block.EvidenceKey)
		contentFingerprint := continuityContentFingerprint(block)
		contextFingerprint := continuityContextFingerprint(block)
		engagementScore := continuityEngagementScore(block.Engagement)
		decision := domain.ContentContinuityDecision{
			EvidenceKey: key,
			Status:      "fresh",
			Action:      "evaluate",
			ObservedAt:  observedAt,
			Reason:      "First observation of this native source item within retained local memory.",
		}

		var previous continuityRecord
		err := tx.QueryRowContext(ctx, `
			SELECT content_fingerprint,context_fingerprint,engagement_score,last_seen_at,last_run_id
			FROM content_continuity WHERE source=? AND evidence_key=?`, run.Source, key).
			Scan(&previous.contentFingerprint, &previous.contextFingerprint, &previous.engagementScore, &previous.lastSeenAt, &previous.lastRunID)
		switch {
		case err == nil && previous.lastRunID == run.ID:
			var status, action, priorSeen, reason string
			if occurrenceErr := tx.QueryRowContext(ctx, `
				SELECT status,action,COALESCE(previous_seen_at,''),reason
				FROM content_continuity_occurrences WHERE run_id=? AND evidence_key=?`, run.ID, key).
				Scan(&status, &action, &priorSeen, &reason); occurrenceErr == nil {
				decision.Status, decision.Action, decision.PreviousSeenAt, decision.Reason = status, action, priorSeen, reason
			}
			decisions[key] = decision
			continue
		case err == nil:
			decision.PreviousSeenAt = previous.lastSeenAt
			previousTime, parseErr := time.Parse(time.RFC3339Nano, previous.lastSeenAt)
			insideCooldown := parseErr == nil && !previousTime.Before(cutoff)
			changed := contentFingerprint != previous.contentFingerprint || contextFingerprint != previous.contextFingerprint || materialEngagementChange(previous.engagementScore, engagementScore)
			switch {
			case changed:
				decision.Status = "resurfaced_changed"
				decision.Reason = "The same native source item resurfaced with materially changed content, context, or engagement."
			case insideCooldown:
				decision.Status = "resurfaced_unchanged"
				decision.Reason = fmt.Sprintf("The same native source item resurfaced without a material change inside the %d-day cooldown.", settings.ResurfaceCooldownDays)
				if settings.ResurfaceMode == "smart" {
					decision.Action = "fail_fast"
				}
			default:
				decision.Status = "resurfaced_after_cooldown"
				decision.Reason = fmt.Sprintf("The same native source item resurfaced after the %d-day cooldown and is eligible for a fresh evaluation.", settings.ResurfaceCooldownDays)
			}
		case err != nil && err != sql.ErrNoRows:
			return nil, err
		}

		if _, err = tx.ExecContext(ctx, `
			INSERT INTO content_continuity(source,evidence_key,content_fingerprint,context_fingerprint,engagement_score,first_seen_at,last_seen_at,last_run_id,seen_count)
			VALUES(?,?,?,?,?,?,?,?,1)
			ON CONFLICT(source,evidence_key) DO UPDATE SET
			  content_fingerprint=excluded.content_fingerprint,
			  context_fingerprint=excluded.context_fingerprint,
			  engagement_score=excluded.engagement_score,
			  last_seen_at=excluded.last_seen_at,
			  last_run_id=excluded.last_run_id,
			  seen_count=content_continuity.seen_count+1`,
			run.Source, key, contentFingerprint, contextFingerprint, engagementScore, observedAt, observedAt, run.ID); err != nil {
			return nil, err
		}
		var prior any
		if decision.PreviousSeenAt != "" {
			prior = decision.PreviousSeenAt
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO content_continuity_occurrences(run_id,evidence_key,status,action,previous_seen_at,observed_at,reason)
			VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(run_id,evidence_key) DO UPDATE SET
			  status=excluded.status,action=excluded.action,previous_seen_at=excluded.previous_seen_at,
			  observed_at=excluded.observed_at,reason=excluded.reason`,
			run.ID, key, decision.Status, decision.Action, prior, observedAt, decision.Reason); err != nil {
			return nil, err
		}
		decisions[key] = decision
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return decisions, nil
}

func (s *Store) ContentContinuityDecisions(ctx context.Context, runID string) (map[string]domain.ContentContinuityDecision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT evidence_key,status,action,COALESCE(previous_seen_at,''),observed_at,reason
		FROM content_continuity_occurrences WHERE run_id=?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]domain.ContentContinuityDecision{}
	for rows.Next() {
		var value domain.ContentContinuityDecision
		if err := rows.Scan(&value.EvidenceKey, &value.Status, &value.Action, &value.PreviousSeenAt, &value.ObservedAt, &value.Reason); err != nil {
			return nil, err
		}
		values[value.EvidenceKey] = value
	}
	return values, rows.Err()
}

func (s *Store) ResurfaceSemanticReevaluationKeys(ctx context.Context, sessionID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.evidence_key
		FROM content_continuity_occurrences o JOIN runs r ON r.id=o.run_id
		WHERE r.session_id=? AND o.status IN ('resurfaced_changed','resurfaced_after_cooldown') AND o.action='evaluate'`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		values[key] = true
	}
	return values, rows.Err()
}

func (s *Store) SaveRunStageTiming(ctx context.Context, runID, stage string, duration time.Duration) error {
	if stage != "captured" && stage != "evaluated" && stage != "selected" && stage != "added" {
		return fmt.Errorf("invalid run timing stage %q", stage)
	}
	milliseconds := duration.Milliseconds()
	if milliseconds < 0 {
		milliseconds = 0
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO run_stage_timings(run_id,stage,duration_ms,completed_at) VALUES(?,?,?,?)
		ON CONFLICT(run_id,stage) DO UPDATE SET duration_ms=run_stage_timings.duration_ms+excluded.duration_ms,completed_at=excluded.completed_at`,
		runID, stage, milliseconds, domain.Now())
	return err
}

func (s *Store) backfillContentContinuity(ctx context.Context) error {
	var existing int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_continuity`).Scan(&existing); err != nil || existing > 0 {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.run_id,o.source,o.observation_json,o.captured_at
		FROM observations o JOIN runs r ON r.id=o.run_id
		ORDER BY o.created_at,r.ordinal`)
	if err != nil {
		return err
	}
	type historicalObservation struct {
		runID       string
		source      domain.Source
		observation domain.Observation
		capturedAt  string
	}
	values := []historicalObservation{}
	for rows.Next() {
		var value historicalObservation
		var raw string
		if err := rows.Scan(&value.runID, &value.source, &raw, &value.capturedAt); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(raw), &value.observation); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, value := range values {
		for _, block := range canonicalContinuityBlocks(value.observation) {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO content_continuity(source,evidence_key,content_fingerprint,context_fingerprint,engagement_score,first_seen_at,last_seen_at,last_run_id,seen_count)
				VALUES(?,?,?,?,?,?,?,?,1)
				ON CONFLICT(source,evidence_key) DO UPDATE SET
				  content_fingerprint=excluded.content_fingerprint,context_fingerprint=excluded.context_fingerprint,
				  engagement_score=excluded.engagement_score,last_seen_at=excluded.last_seen_at,
				  last_run_id=excluded.last_run_id,seen_count=content_continuity.seen_count+1`,
				value.source, block.EvidenceKey, continuityContentFingerprint(block), continuityContextFingerprint(block),
				continuityEngagementScore(block.Engagement), value.capturedAt, value.capturedAt, value.runID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func canonicalContinuityBlocks(observation domain.Observation) []domain.Block {
	seen := map[string]bool{}
	blocks := []domain.Block{}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			key := strings.TrimSpace(block.EvidenceKey)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func continuityContentFingerprint(block domain.Block) string {
	return continuityHash(struct {
		Text        string `json:"text"`
		ContentKind string `json:"contentKind"`
		PlatformID  string `json:"platformId"`
	}{
		Text:        strings.ToLower(strings.Join(strings.Fields(block.Text), " ")),
		ContentKind: strings.TrimSpace(block.ContentKind),
		PlatformID:  strings.TrimSpace(block.PlatformID),
	})
}

func continuityContextFingerprint(block domain.Block) string {
	return continuityHash(struct {
		Author           string `json:"author"`
		RelationshipType string `json:"relationshipType"`
		ParentPermalink  string `json:"parentPermalink"`
	}{
		Author:           strings.ToLower(strings.Join(strings.Fields(block.Author), " ")),
		RelationshipType: strings.TrimSpace(block.RelationshipType),
		ParentPermalink:  strings.TrimSpace(block.ParentPermalink),
	})
}

func continuityHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func continuityEngagementScore(value any) int64 {
	switch typed := value.(type) {
	case map[string]any:
		var total int64
		for _, nested := range typed {
			total += continuityEngagementScore(nested)
		}
		return total
	case []any:
		var total int64
		for _, nested := range typed {
			total += continuityEngagementScore(nested)
		}
		return total
	case float64:
		if typed > 0 {
			return int64(math.Round(typed))
		}
	case float32:
		if typed > 0 {
			return int64(math.Round(float64(typed)))
		}
	case int:
		if typed > 0 {
			return int64(typed)
		}
	case int64:
		if typed > 0 {
			return typed
		}
	case json.Number:
		if number, err := typed.Int64(); err == nil && number > 0 {
			return number
		}
	}
	return 0
}

func materialEngagementChange(previous, current int64) bool {
	delta := current - previous
	if delta < 0 {
		delta = -delta
	}
	if delta < 10 {
		return false
	}
	if previous == 0 {
		return current >= 10
	}
	return float64(delta)/float64(previous) >= 0.25
}
