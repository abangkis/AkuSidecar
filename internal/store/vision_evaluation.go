package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const defaultVisionQueueLimit = 8

// EnqueueVisionEvaluations persists media-only candidates and returns their
// current durable states. It is intentionally idempotent: repeated captures
// of one native post with the same media do not create another job.
func (s *Store) EnqueueVisionEvaluations(ctx context.Context, run domain.Run, inputs []domain.VisionEvaluationInput, limit int) ([]domain.VisionEvaluationJob, error) {
	if run.ID == "" || run.SessionID == "" || !run.Source.Valid() {
		return nil, errors.New("vision evaluation queue requires a valid run")
	}
	if limit <= 0 {
		limit = defaultVisionQueueLimit
	}
	if err := s.promoteDeferredVisionEvaluations(ctx, run.Source, limit); err != nil {
		return nil, err
	}
	for inputIndex, input := range inputs {
		if strings.TrimSpace(input.EvidenceKey) == "" {
			continue
		}
		identity := visionCanonicalIdentity(run.Source, input.Candidate)
		mediaFingerprint := visionMediaFingerprint(run.Source, input.Candidate)
		if identity == "" || mediaFingerprint == "" {
			continue
		}
		candidateJSON, err := json.Marshal(input.Candidate)
		if err != nil {
			return nil, fmt.Errorf("encode vision candidate: %w", err)
		}
		status := "pending"
		reason := "media_only_bounded_lane"
		active, err := s.visionActiveCount(ctx, run.Source)
		if err != nil {
			return nil, err
		}
		if active >= limit {
			status = "deferred"
			reason = "capacity_full"
		}
		now := formatVisionTime(s.Now().UTC().Add(time.Duration(inputIndex) * time.Nanosecond))
		id := domain.NewID("vision")
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO vision_evaluation_jobs(
			  id,run_id,session_id,source,evidence_key,canonical_identity,
			  media_fingerprint,status,reason,attempt_count,queued_at,last_error,
			  candidate_json,result_json,created_at)
			VALUES(?,?,?,?,?,?,?, ?, ?,0,?, '',?,'{}',?)
			ON CONFLICT(source,canonical_identity,media_fingerprint) DO NOTHING`,
			id, run.ID, run.SessionID, run.Source, input.EvidenceKey, identity,
			mediaFingerprint, status, reason, now, candidateJSON, now)
		if err != nil {
			return nil, fmt.Errorf("enqueue vision evaluation: %w", err)
		}
	}
	if err := s.promoteDeferredVisionEvaluations(ctx, run.Source, limit); err != nil {
		return nil, err
	}
	return s.ListVisionEvaluations(ctx, run.ID, run.Source)
}

func (s *Store) visionActiveCount(ctx context.Context, source domain.Source) (int, error) {
	var count int
	// Deferred jobs wait outside the bounded worker capacity. Counting them as
	// active would make the queue permanently full and prevent promotion after
	// a pending/evaluating job completes.
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE source=? AND status IN ('pending','evaluating','retry_wait')`, source).Scan(&count)
	return count, err
}

func (s *Store) promoteDeferredVisionEvaluations(ctx context.Context, source domain.Source, limit int) error {
	if limit <= 0 {
		limit = defaultVisionQueueLimit
	}
	active, err := s.visionActiveCount(ctx, source)
	if err != nil {
		return err
	}
	available := limit - active
	if available <= 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE vision_evaluation_jobs SET status='pending',reason='capacity_available'
		WHERE id IN (
			SELECT id FROM vision_evaluation_jobs
			WHERE source=? AND status='deferred'
			ORDER BY queued_at,id LIMIT ?
		)`, source, available)
	return err
}

func (s *Store) ListVisionEvaluations(ctx context.Context, runID string, source domain.Source) ([]domain.VisionEvaluationJob, error) {
	query := `SELECT id,run_id,session_id,source,evidence_key,canonical_identity,media_fingerprint,status,reason,attempt_count,queued_at,COALESCE(next_attempt_at,''),last_error,candidate_json,created_at,COALESCE(started_at,''),COALESCE(completed_at,'') FROM vision_evaluation_jobs`
	args := []any{}
	where := []string{}
	if strings.TrimSpace(runID) != "" {
		where = append(where, "run_id=?")
		args = append(args, runID)
	}
	if source.Valid() {
		where = append(where, "source=?")
		args = append(args, source)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY queued_at,id"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.VisionEvaluationJob{}
	for rows.Next() {
		var value domain.VisionEvaluationJob
		var candidateJSON string
		if err := rows.Scan(&value.ID, &value.RunID, &value.SessionID, &value.Source, &value.EvidenceKey, &value.CanonicalIdentity, &value.MediaFingerprint, &value.Status, &value.Reason, &value.AttemptCount, &value.QueuedAt, &value.NextAttemptAt, &value.LastError, &candidateJSON, &value.CreatedAt, &value.StartedAt, &value.CompletedAt); err != nil {
			return nil, err
		}
		var candidate domain.Block
		if err := json.Unmarshal([]byte(candidateJSON), &candidate); err == nil {
			value.Candidate = map[string]any{}
			raw, _ := json.Marshal(candidate)
			_ = json.Unmarshal(raw, &value.Candidate)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func sourceVisionQueueLimit(source domain.Source) int {
	if descriptor, ok := domain.SourceByID(source); ok && descriptor.VisionQueueLimit > 0 {
		return descriptor.VisionQueueLimit
	}
	return defaultVisionQueueLimit
}

func (s *Store) VisionEvaluationSummary(ctx context.Context, runID string, source domain.Source) (domain.InboxVisionEvaluation, error) {
	jobs, err := s.ListVisionEvaluations(ctx, runID, source)
	if err != nil {
		return domain.InboxVisionEvaluation{}, err
	}
	value := domain.InboxVisionEvaluation{Policy: "bounded_vision", Available: false, Availability: "no pixel-capable provider transport is registered; metadata-only reasoning is not used as vision", Capacity: sourceVisionQueueLimit(source), Jobs: jobs}
	queuePosition := 0
	for index := range value.Jobs {
		status := value.Jobs[index].Status
		if status == "pending" || status == "evaluating" || status == "retry_wait" {
			value.Active++
		}
		switch status {
		case "pending":
			value.Pending++
		case "deferred":
			value.Deferred++
		case "evaluating":
			value.Evaluating++
		case "retry_wait":
			value.Retrying++
		case "ready":
			value.Ready++
		case "failed":
			value.Failed++
		}
		if status == "pending" || status == "deferred" || status == "retry_wait" {
			queuePosition++
			value.Jobs[index].QueuePosition = queuePosition
		}
	}
	return value, nil
}

// ClaimNextVisionEvaluation is the future evaluator's FIFO claim boundary.
// No caller is allowed to treat metadata as a successful pixel evaluation.
func (s *Store) ClaimNextVisionEvaluation(ctx context.Context, source domain.Source, worker string) (*domain.VisionEvaluationJob, error) {
	if strings.TrimSpace(worker) == "" {
		return nil, errors.New("vision evaluator worker is required")
	}
	if err := s.promoteDeferredVisionEvaluations(ctx, source, sourceVisionQueueLimit(source)); err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	nowText := formatVisionTime(now)
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM vision_evaluation_jobs WHERE source=? AND (status='pending' OR (status='retry_wait' AND (next_attempt_at IS NULL OR next_attempt_at<=?))) ORDER BY queued_at,id LIMIT 1`, source, nowText).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE vision_evaluation_jobs SET status='evaluating',attempt_count=attempt_count+1,started_at=?,last_error='' WHERE id=? AND (status='pending' OR (status='retry_wait' AND (next_attempt_at IS NULL OR next_attempt_at<=?)))`, nowText, id, nowText)
	if err != nil {
		return nil, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, nil
	}
	jobs, err := s.ListVisionEvaluations(ctx, "", source)
	if err != nil {
		return nil, err
	}
	for index := range jobs {
		if jobs[index].ID == id {
			return &jobs[index], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) CompleteVisionEvaluation(ctx context.Context, id string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	now := formatVisionTime(s.Now().UTC())
	var source domain.Source
	if err := s.db.QueryRowContext(ctx, `SELECT source FROM vision_evaluation_jobs WHERE id=?`, id).Scan(&source); err != nil {
		return err
	}
	execResult, err := s.db.ExecContext(ctx, `UPDATE vision_evaluation_jobs SET status='ready',result_json=?,completed_at=?,last_error='' WHERE id=? AND status='evaluating'`, encoded, now, id)
	if err != nil {
		return err
	}
	if changed, _ := execResult.RowsAffected(); changed != 1 {
		return sql.ErrNoRows
	}
	return s.promoteDeferredVisionEvaluations(ctx, source, sourceVisionQueueLimit(source))
}

// FailVisionEvaluation applies the bounded retry policy. The first failure is
// requeued at the tail after a short backoff; the second is terminal.
func (s *Store) FailVisionEvaluation(ctx context.Context, id, failure string) error {
	now := s.Now().UTC()
	status := "failed"
	nextAttempt := any(nil)
	reason := "vision_evaluation_failed"
	var attempts int
	var source domain.Source
	if err := s.db.QueryRowContext(ctx, `SELECT attempt_count,source FROM vision_evaluation_jobs WHERE id=? AND status='evaluating'`, id).Scan(&attempts, &source); err != nil {
		return err
	}
	if attempts < 2 {
		status = "retry_wait"
		retryAt := formatVisionTime(now.Add(30 * time.Second))
		nextAttempt = retryAt
		reason = "automatic_retry_pending"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE vision_evaluation_jobs SET status=?,reason=?,last_error=?,next_attempt_at=?,queued_at=? WHERE id=? AND status='evaluating'`, status, reason, strings.TrimSpace(failure), nextAttempt, formatVisionTime(now), id)
	if err != nil {
		return err
	}
	if status == "failed" {
		return s.promoteDeferredVisionEvaluations(ctx, source, sourceVisionQueueLimit(source))
	}
	return nil
}

func (s *Store) RetryVisionEvaluation(ctx context.Context, id string) error {
	now := formatVisionTime(s.Now().UTC())
	var source domain.Source
	var currentStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT source,status FROM vision_evaluation_jobs WHERE id=?`, id).Scan(&source, &currentStatus); err != nil {
		return err
	}
	if currentStatus != "failed" {
		return fmt.Errorf("vision evaluation %s is %s, not failed", id, currentStatus)
	}
	if err := s.promoteDeferredVisionEvaluations(ctx, source, sourceVisionQueueLimit(source)); err != nil {
		return err
	}
	status := "pending"
	active, err := s.visionActiveCount(ctx, source)
	if err != nil {
		return err
	}
	if active >= sourceVisionQueueLimit(source) {
		status = "deferred"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE vision_evaluation_jobs SET status=?,reason='manual_retry',attempt_count=0,next_attempt_at=NULL,last_error='',queued_at=?,completed_at=NULL WHERE id=? AND status='failed'`, status, now, id)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func formatVisionTime(value time.Time) string {
	// Fixed-width fractional seconds preserve lexical FIFO ordering in SQLite
	// even when two jobs are enqueued within the same clock tick.
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
}

func visionCanonicalIdentity(source domain.Source, block domain.Block) string {
	if value := domain.NativeIdentityFromPermalink(source, block.Permalink); value != "" {
		return value
	}
	if value := domain.NormalizeNativeIdentity(source, block.PlatformID); value != "" {
		return value
	}
	if canonical, ok := domain.CanonicalSourceURL(source, block.Permalink); ok {
		return canonical
	}
	return strings.TrimSpace(block.EvidenceKey)
}

func visionMediaFingerprint(source domain.Source, block domain.Block) string {
	type mediaIdentity struct {
		Kind        string `json:"kind"`
		URL         string `json:"url"`
		PosterURL   string `json:"posterUrl"`
		PlaybackURL string `json:"playbackUrl"`
	}
	values := make([]mediaIdentity, 0, len(block.Media))
	for _, media := range block.Media {
		value, _ := media["url"].(string)
		if strings.TrimSpace(value) == "" {
			value, _ = media["posterUrl"].(string)
		}
		if !validatedVisionMedia(source, media, value) {
			continue
		}
		kind, _ := media["kind"].(string)
		poster, _ := media["posterUrl"].(string)
		playback, _ := media["playbackUrl"].(string)
		values = append(values, mediaIdentity{Kind: strings.ToLower(strings.TrimSpace(kind)), URL: canonicalMediaURL(value), PosterURL: canonicalMediaURL(poster), PlaybackURL: canonicalMediaURL(playback)})
	}
	if len(values) == 0 {
		return ""
	}
	raw, _ := json.Marshal(values)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validatedVisionMedia(source domain.Source, media map[string]any, raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Port() != "" {
		return false
	}
	trusted := false
	if descriptor, ok := domain.SourceByID(source); ok {
		host := strings.ToLower(parsed.Hostname())
		for _, suffix := range descriptor.TrustedMediaHostSuffixes {
			suffix = strings.ToLower(strings.TrimSpace(suffix))
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				trusted = true
				break
			}
		}
	}
	return trusted
}

func canonicalMediaURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	parsed.Fragment = ""
	return parsed.String()
}
