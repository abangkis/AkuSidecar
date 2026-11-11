package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/capture"
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
	priorXBlocks := map[string]map[string]domain.Block{}
	evaluations, err := completedContinuityEvaluations(ctx, tx, run.Source, blocks)
	if err != nil {
		return nil, err
	}
	evaluatedBlocks := map[string]map[string]domain.Block{}
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
			if settings.ResurfaceMode == "smart" && decision.Status == "resurfaced_unchanged" && decision.Action == "evaluate" {
				evaluated, err := continuityVersionEvaluated(ctx, tx, run.Source, block, evaluations[key], evaluatedBlocks)
				if err != nil {
					return nil, err
				}
				decision.EvaluationPending = !evaluated
			}
			decisions[key] = decision
			continue
		case err == nil:
			decision.PreviousSeenAt = previous.lastSeenAt
			previousTime, parseErr := time.Parse(time.RFC3339Nano, previous.lastSeenAt)
			insideCooldown := parseErr == nil && !previousTime.Before(cutoff)
			contentChanged := contentFingerprint != previous.contentFingerprint &&
				legacyContinuityContentFingerprint(block) != previous.contentFingerprint
			contextChanged := contextFingerprint != previous.contextFingerprint
			if !contentChanged && contextChanged && run.Source == domain.SourceX && strings.TrimSpace(block.RelationshipType) == "quote" {
				priorBlocks, loaded := priorXBlocks[previous.lastRunID]
				if !loaded {
					priorBlocks, err = priorXContinuityBlocks(ctx, tx, previous.lastRunID)
					if err != nil {
						return nil, err
					}
					priorXBlocks[previous.lastRunID] = priorBlocks
				}
				if reconciled, ok := reconcileXQuoteFingerprint(priorBlocks[key], block, previous.contextFingerprint, contextFingerprint); ok {
					contextFingerprint, contextChanged = reconciled, false
				}
			}
			// Engagement counters are retained for continuity telemetry, but their
			// volatility alone must not turn an otherwise stable native post into a
			// resurfaced change. Content and context remain the material signals.
			switch {
			case contentChanged || contextChanged:
				decision.Status = "resurfaced_changed"
				decision.Reason = "The same native source item resurfaced with materially changed content, context, or engagement."
			case insideCooldown:
				decision.Status = "resurfaced_unchanged"
				decision.Reason = fmt.Sprintf("The same native source item resurfaced without a material change inside the %d-day cooldown.", settings.ResurfaceCooldownDays)
				if settings.ResurfaceMode == "smart" {
					evaluated, err := continuityVersionEvaluated(ctx, tx, run.Source, block, evaluations[key], evaluatedBlocks)
					if err != nil {
						return nil, err
					}
					if evaluated {
						decision.Action = "fail_fast"
						decision.Reason += " This content and context version already has a completed candidate evaluation."
					} else {
						decision.EvaluationPending = true
						decision.Reason += " No completed candidate evaluation exists for this content and context version; it remains eligible."
					}
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

func priorXContinuityBlocks(ctx context.Context, tx *sql.Tx, runID string) (map[string]domain.Block, error) {
	observations, err := observationsFrom(ctx, tx, runID)
	if err != nil {
		return nil, fmt.Errorf("read prior X quote evidence: %w", err)
	}
	var snapshots []domain.Snapshot
	for _, observation := range observations {
		snapshots = append(snapshots, observation.Snapshots...)
	}
	blocks := make(map[string]domain.Block)
	for _, block := range canonicalContinuityBlocks(domain.Observation{Snapshots: capture.ReconcileSnapshots(domain.SourceX, snapshots)}) {
		blocks[strings.TrimSpace(block.EvidenceKey)] = block
	}
	return blocks, nil
}

func reconcileXQuoteFingerprint(prior, current domain.Block, stored, observed string) (string, bool) {
	priorPost := domain.ContextObjectIdentity(domain.ContextObject{Kind: "post", Permalink: prior.Permalink})
	currentPost := domain.ContextObjectIdentity(domain.ContextObject{Kind: "post", Permalink: current.Permalink})
	if priorPost == "" || priorPost != currentPost || strings.TrimSpace(prior.RelationshipType) != "quote" ||
		continuityContentFingerprint(prior) != continuityContentFingerprint(current) ||
		!sameXQuoteText(prior, current) {
		return "", false
	}
	priorLink, priorOK := xQuoteParentID(prior, priorPost)
	currentLink, currentOK := xQuoteParentID(current, currentPost)
	if !priorOK || !currentOK || priorLink != "" && currentLink != "" && priorLink != currentLink {
		return "", false
	}
	priorObserved := continuityContextFingerprint(prior)
	prior.ParentPermalink, current.ParentPermalink = "", ""
	if continuityContextFingerprint(prior) != continuityContextFingerprint(current) {
		return "", false
	}
	// Keep the last known parent across missing observations. A later, different
	// known parent must still compare against that identity, not against absence.
	if currentLink == "" && (priorLink == "" || stored == priorObserved) {
		return stored, true
	}
	if currentLink != "" && stored == priorObserved {
		return observed, true
	}
	return "", false
}

func xQuoteParentID(block domain.Block, primaryID string) (string, bool) {
	reason, _ := block.QuotedPost["identityConflictReason"].(string)
	status, _ := block.QuotedPost["identityStatus"].(string)
	if reason != "" || status == "conflicting_identity" {
		return "", false
	}
	parent := strings.TrimSpace(block.ParentPermalink)
	quoteURL, _ := block.QuotedPost["permalink"].(string)
	quoteURL = strings.TrimSpace(quoteURL)
	if parent == "" {
		return "", quoteURL == ""
	}
	id := domain.ContextObjectIdentity(domain.ContextObject{Kind: "post", Permalink: parent})
	return id, id != "" && id != primaryID && (quoteURL == "" || id == domain.ContextObjectIdentity(domain.ContextObject{Kind: "post", Permalink: quoteURL}))
}

func sameXQuoteText(prior, current domain.Block) bool {
	priorAuthor, _ := prior.QuotedPost["author"].(string)
	currentAuthor, _ := current.QuotedPost["author"].(string)
	priorText, _ := prior.QuotedPost["text"].(string)
	currentText, _ := current.QuotedPost["text"].(string)
	priorTime, _ := prior.QuotedPost["publishedAt"].(string)
	currentTime, _ := current.QuotedPost["publishedAt"].(string)
	priorAuthor = strings.Join(strings.Fields(priorAuthor), " ")
	priorText = strings.Join(strings.Fields(priorText), " ")
	return priorAuthor != "" && priorText != "" &&
		strings.EqualFold(priorAuthor, strings.Join(strings.Fields(currentAuthor), " ")) &&
		priorText == strings.Join(strings.Fields(currentText), " ") &&
		(priorTime == "" || currentTime == "" || priorTime == currentTime)
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
		WHERE r.session_id=? AND o.action='evaluate' AND (
		  o.status IN ('resurfaced_changed','resurfaced_after_cooldown') OR
		  (o.status='resurfaced_unchanged' AND r.status='completed' AND EXISTS (
		    SELECT 1 FROM json_each(r.coverage_json,'$.continuityEvaluationPending') p
		    WHERE p.value=o.evidence_key
		  ) AND EXISTS (
		    SELECT 1 FROM candidate_assessments a
		    WHERE a.run_id=r.id AND a.source=r.source AND a.evidence_key=o.evidence_key
		  ))
		)`, sessionID)
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
	return continuityContentFingerprintWithPlatformID(block, "")
}

func legacyContinuityContentFingerprint(block domain.Block) string {
	return continuityContentFingerprintWithPlatformID(block, strings.TrimSpace(block.PlatformID))
}

func continuityContentFingerprintWithPlatformID(block domain.Block, platformID string) string {
	return continuityHash(struct {
		Text        string `json:"text"`
		ContentKind string `json:"contentKind"`
		PlatformID  string `json:"platformId"`
	}{
		Text:        strings.ToLower(strings.Join(strings.Fields(block.Text), " ")),
		ContentKind: strings.TrimSpace(block.ContentKind),
		PlatformID:  platformID,
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
	case string:
		return parseContinuityEngagementCount(typed)
	}
	return 0
}

func parseContinuityEngagementCount(value string) int64 {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return 0
	}
	multiplier := float64(1)
	switch normalized[len(normalized)-1] {
	case 'K':
		multiplier = 1_000
		normalized = strings.TrimSpace(normalized[:len(normalized)-1])
	case 'M':
		multiplier = 1_000_000
		normalized = strings.TrimSpace(normalized[:len(normalized)-1])
	case 'B':
		multiplier = 1_000_000_000
		normalized = strings.TrimSpace(normalized[:len(normalized)-1])
	}
	if strings.Contains(normalized, ",") {
		if multiplier > 1 && !strings.Contains(normalized, ".") {
			normalized = strings.ReplaceAll(normalized, ",", ".")
		} else {
			normalized = strings.ReplaceAll(normalized, ",", "")
		}
	}
	number, err := strconv.ParseFloat(normalized, 64)
	if err != nil || number <= 0 {
		return 0
	}
	return int64(math.Round(number * multiplier))
}
