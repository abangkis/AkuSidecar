package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// FeedbackTiming records the stages of one routine Timeline feedback request.
// Pool wait deltas describe the whole DB pool during this request, including
// other concurrent work; they are not attributed solely to this request.
type FeedbackTiming struct {
	Lookup                time.Duration
	Hydration             time.Duration
	BeginTx               time.Duration
	Transaction           time.Duration
	Commit                time.Duration
	Total                 time.Duration
	PoolWaitCountDelta    int64
	PoolWaitDurationDelta time.Duration
}

type feedbackTimingKey struct{}

func WithFeedbackTiming(ctx context.Context, timing *FeedbackTiming) context.Context {
	return context.WithValue(ctx, feedbackTimingKey{}, timing)
}

func (s *Store) AddFeedback(ctx context.Context, timelineID string, input domain.Feedback) (domain.Feedback, error) {
	timing, _ := ctx.Value(feedbackTimingKey{}).(*FeedbackTiming)
	if timing != nil {
		started := time.Now()
		before := s.db.Stats()
		defer func() {
			after := s.db.Stats()
			timing.Total = time.Since(started)
			timing.PoolWaitCountDelta = after.WaitCount - before.WaitCount
			timing.PoolWaitDurationDelta = after.WaitDuration - before.WaitDuration
		}()
	}
	var sessionID, runID, evidenceKey, sessionStatus string
	lookupStarted := time.Now()
	err := s.db.QueryRowContext(ctx, `
		SELECT t.session_id,t.run_id,t.evidence_key,COALESCE(s.status,t.origin_status)
		FROM timeline_items t LEFT JOIN sessions s ON s.id=t.session_id
		WHERE t.id=?`, timelineID).Scan(&sessionID, &runID, &evidenceKey, &sessionStatus)
	if timing != nil {
		timing.Lookup = time.Since(lookupStarted)
	}
	if err != nil {
		return domain.Feedback{}, err
	}
	// AddFeedback is the authoritative routine Timeline action. Origin is
	// normalized below so a stale or forged client hint cannot suppress the
	// guaranteed recall projection. Calibration and selection corrections use
	// their distinct canonical store paths and never call AddFeedback.
	input.ID = domain.NewID("feedback")
	input.TimelineID = timelineID
	input.SessionID = sessionID
	input.RunID = runID
	input.EvidenceKey = evidenceKey
	input.Origin = "routine"
	input.CreatedAt = domain.Now()
	if err = input.Validate(); err != nil {
		return domain.Feedback{}, err
	}

	// A Timeline row only becomes a Personal Memory candidate once it belongs
	// to a terminal, visible session. ComposeSession already removes
	// non-survivors; the existence check above therefore forms the survivor
	// boundary while the terminal status prevents projection during capture or
	// reasoning.
	projectMemory := input.Direction == "more" &&
		(sessionStatus == "completed" || sessionStatus == "partial")
	retractMemory := false
	var timelineItem domain.TimelineItem
	if projectMemory || (input.Direction == "less" && (sessionStatus == "completed" || sessionStatus == "partial")) {
		hydrationStarted := time.Now()
		timelineItem, err = s.TimelineItem(ctx, timelineID)
		if timing != nil {
			timing.Hydration = time.Since(hydrationStarted)
		}
		if err != nil {
			return domain.Feedback{}, err
		}
	}
	var normalized normalizedMemoryInput
	var tombstoneKey []byte
	if projectMemory {
		memoryInput := routineMoreMemoryInput(timelineItem)
		normalized, err = normalizeMemoryInput(memoryInput)
		if err != nil {
			return domain.Feedback{}, fmt.Errorf("prepare routine More memory projection: %w", err)
		}
		tombstoneKey, err = s.memoryTombstoneKey(ctx)
		if err != nil {
			return domain.Feedback{}, err
		}
		normalized.IdentityDigest = memoryIdentityDigest(tombstoneKey, normalized.Identity)
	}

	beginStarted := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if timing != nil {
		timing.BeginTx = time.Since(beginStarted)
	}
	if err != nil {
		return domain.Feedback{}, fmt.Errorf("begin feedback transaction: %w", err)
	}
	if timing != nil {
		transactionStarted := time.Now()
		defer func() { timing.Transaction = time.Since(transactionStarted) }()
	}
	defer tx.Rollback()
	if input.Direction == "less" && timelineItem.ID != "" {
		var previousDirection string
		err = tx.QueryRowContext(ctx, `
			SELECT direction FROM feedback_events
			WHERE timeline_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, timelineID).Scan(&previousDirection)
		if errors.Is(err, sql.ErrNoRows) {
			previousDirection = ""
		} else if err != nil {
			return domain.Feedback{}, fmt.Errorf("read previous routine feedback: %w", err)
		}
		retractMemory = previousDirection == "more"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,reason,created_at) VALUES(?,?,?,?,?,?,?,?)`, input.ID, input.TimelineID, input.SessionID, input.RunID, input.EvidenceKey, input.Direction, input.Reason, input.CreatedAt); err != nil {
		return domain.Feedback{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO preference_learning_ledger(event_id,source,evidence_key,direction,reason,origin,created_at,assessment_json,active) SELECT ?,source,evidence_key,?,?,'routine',?,assessment_json,1 FROM timeline_items WHERE id=?`, input.ID, input.Direction, input.Reason, input.CreatedAt, timelineID); err != nil {
		return domain.Feedback{}, fmt.Errorf("persist routine preference learning: %w", err)
	}
	if projectMemory {
		if _, err = s.upsertMemoryRecallStubTx(ctx, tx, tombstoneKey, normalized); err != nil {
			// The feedback row and projection intentionally roll back together.
			return domain.Feedback{}, fmt.Errorf("project routine More memory: %w", err)
		}
	}
	if retractMemory {
		if _, err := retractRoutineMoreMemoryTx(ctx, tx, timelineItem); err != nil {
			return domain.Feedback{}, fmt.Errorf("retract routine More memory: %w", err)
		}
	}
	commitStarted := time.Now()
	err = tx.Commit()
	if timing != nil {
		timing.Commit = time.Since(commitStarted)
	}
	if err != nil {
		return domain.Feedback{}, fmt.Errorf("commit feedback transaction: %w", err)
	}
	return input, nil
}
