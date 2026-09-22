package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	timelineProtectionDays          = 14
	timelineRoutineExpiryDays       = 30
	timelineMaxItems                = 500
	timelineMaxLogicalBytes   int64 = 10 * 1024 * 1024
	timelineReceiptLimit            = 128
)

const timelineRetentionSQL = `
CREATE TABLE IF NOT EXISTS timeline_retention_receipts (
  id TEXT PRIMARY KEY,
  mode TEXT NOT NULL CHECK (mode='observe'),
  evaluated_at TEXT NOT NULL,
  protection_days INTEGER NOT NULL CHECK (protection_days > 0),
  routine_expiry_days INTEGER NOT NULL CHECK (routine_expiry_days >= protection_days),
  max_items INTEGER NOT NULL CHECK (max_items > 0),
  max_logical_bytes INTEGER NOT NULL CHECK (max_logical_bytes > 0),
  total_items INTEGER NOT NULL CHECK (total_items >= 0),
  logical_bytes INTEGER NOT NULL CHECK (logical_bytes >= 0),
  protected_items INTEGER NOT NULL CHECK (protected_items >= 0),
  eligible_items INTEGER NOT NULL CHECK (eligible_items >= 0),
  routine_expired_items INTEGER NOT NULL CHECK (routine_expired_items >= 0),
  hidden_items INTEGER NOT NULL CHECK (hidden_items >= 0),
  missing_presentation_items INTEGER NOT NULL CHECK (missing_presentation_items >= 0),
  over_item_limit INTEGER NOT NULL CHECK (over_item_limit IN (0,1)),
  over_byte_limit INTEGER NOT NULL CHECK (over_byte_limit IN (0,1)),
  reclaimable_items INTEGER NOT NULL CHECK (reclaimable_items >= 0),
  reclaimable_bytes INTEGER NOT NULL CHECK (reclaimable_bytes >= 0),
  would_remove_items INTEGER NOT NULL CHECK (would_remove_items >= 0),
  would_remove_bytes INTEGER NOT NULL CHECK (would_remove_bytes >= 0),
  needs_attention INTEGER NOT NULL CHECK (needs_attention IN (0,1)),
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS timeline_retention_receipts_created
  ON timeline_retention_receipts(created_at DESC,id DESC);
`

const timelineRetentionMigrationSQL = `
CREATE TABLE IF NOT EXISTS timeline_retention_receipts (
  id TEXT PRIMARY KEY,
  mode TEXT NOT NULL CHECK (mode='observe'),
  evaluated_at TEXT NOT NULL,
  protection_days INTEGER NOT NULL CHECK (protection_days > 0),
  routine_expiry_days INTEGER NOT NULL CHECK (routine_expiry_days >= protection_days),
  max_items INTEGER NOT NULL CHECK (max_items > 0),
  max_logical_bytes INTEGER NOT NULL CHECK (max_logical_bytes > 0),
  total_items INTEGER NOT NULL CHECK (total_items >= 0),
  logical_bytes INTEGER NOT NULL CHECK (logical_bytes >= 0),
  protected_items INTEGER NOT NULL CHECK (protected_items >= 0),
  eligible_items INTEGER NOT NULL CHECK (eligible_items >= 0),
  routine_expired_items INTEGER NOT NULL CHECK (routine_expired_items >= 0),
  hidden_items INTEGER NOT NULL CHECK (hidden_items >= 0),
  missing_presentation_items INTEGER NOT NULL CHECK (missing_presentation_items >= 0),
  over_item_limit INTEGER NOT NULL CHECK (over_item_limit IN (0,1)),
  over_byte_limit INTEGER NOT NULL CHECK (over_byte_limit IN (0,1)),
  reclaimable_items INTEGER NOT NULL CHECK (reclaimable_items >= 0),
  reclaimable_bytes INTEGER NOT NULL CHECK (reclaimable_bytes >= 0),
  would_remove_items INTEGER NOT NULL CHECK (would_remove_items >= 0),
  would_remove_bytes INTEGER NOT NULL CHECK (would_remove_bytes >= 0),
  needs_attention INTEGER NOT NULL CHECK (needs_attention IN (0,1)),
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS timeline_retention_receipts_created
  ON timeline_retention_receipts(created_at DESC,id DESC);
`

// The canary measures the durable Timeline card projection itself. Child
// learning/audit tables remain independently durable and are not attributed to
// this soft budget until their ownership is explicitly classified.
const timelineItemLogicalBytesSQL = `
  length(CAST(id AS BLOB))+length(CAST(session_id AS BLOB))+length(CAST(run_id AS BLOB))+
  length(CAST(source AS BLOB))+length(CAST(evidence_key AS BLOB))+8+
  length(CAST(item_json AS BLOB))+length(CAST(assessment_json AS BLOB))+length(CAST(coverage_json AS BLOB))+
  length(CAST(origin_status AS BLOB))+length(CAST(presentation AS BLOB))+COALESCE(length(CAST(presented_at AS BLOB)),0)+
  length(CAST(batch_state AS BLOB))+COALESCE(length(CAST(evidence_snapshot_json AS BLOB)),0)+length(CAST(created_at AS BLOB))`

type retentionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type timelineRetentionCandidate struct {
	presentedAt time.Time
	bytes       int64
	expired     bool
}

func migrateSchema27To28(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, timelineRetentionMigrationSQL); err != nil {
		return err
	}
	var receiptColumns int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('timeline_retention_receipts') WHERE name IN ('id','mode','evaluated_at','protection_days','routine_expiry_days','max_items','max_logical_bytes','total_items','logical_bytes','protected_items','eligible_items','routine_expired_items','hidden_items','missing_presentation_items','over_item_limit','over_byte_limit','reclaimable_items','reclaimable_bytes','would_remove_items','would_remove_bytes','needs_attention','created_at')`).Scan(&receiptColumns); err != nil {
		return err
	}
	if receiptColumns != 22 {
		return fmt.Errorf("timeline retention receipt schema is incompatible: found %d of 22 required columns", receiptColumns)
	}
	var hasTimeline int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='timeline_items'`).Scan(&hasTimeline); err != nil {
		return err
	}
	if hasTimeline == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE meta SET value='28' WHERE key='schema_version'`); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS timeline_presented ON timeline_items(presented_at DESC)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DROP TRIGGER IF EXISTS timeline_snapshot_session;`+timelineLifecycleSQL); err != nil {
		return err
	}
	// Existing visible cards receive the best durable approximation of the
	// moment they entered Timeline. Hidden prepared/expired batches stay
	// unpresented and therefore fail safe as protected.
	if _, err = tx.ExecContext(ctx, `
		UPDATE timeline_items
		SET origin_status=COALESCE((SELECT s.status FROM sessions s WHERE s.id=timeline_items.session_id),NULLIF(origin_status,''),'unavailable'),
		    presentation=CASE
		      WHEN COALESCE((SELECT b.state FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),NULLIF(batch_state,''),'visible')='visible'
		      THEN COALESCE(NULLIF(presentation,''),(SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),'prepend')
		      ELSE presentation END,
		    presented_at=CASE
		      WHEN COALESCE((SELECT b.state FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),NULLIF(batch_state,''),'visible')='visible'
		      THEN COALESCE(NULLIF(presented_at,''),(SELECT b.revealed_at FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),(SELECT s.completed_at FROM sessions s WHERE s.id=timeline_items.session_id),created_at)
		      ELSE NULL END,
		    batch_state=COALESCE((SELECT b.state FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),NULLIF(batch_state,''),'visible')`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE meta SET value='28' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func timelineRetentionPolicy() domain.TimelineRetentionPolicy {
	return domain.TimelineRetentionPolicy{
		ProtectionDays: timelineProtectionDays, RoutineExpiryDays: timelineRoutineExpiryDays,
		MaxItems: timelineMaxItems, MaxLogicalBytes: timelineMaxLogicalBytes,
	}
}

func measureTimelineStorage(ctx context.Context, q retentionQueryer, now time.Time) (domain.TimelineStorageStatus, error) {
	status := domain.TimelineStorageStatus{
		Mode: "observe", EvaluatedAt: now.UTC().Format(time.RFC3339Nano), Policy: timelineRetentionPolicy(),
	}
	rows, err := q.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(t.presented_at,''),''),
		       COALESCE((SELECT b.state FROM auto_update_batches b WHERE b.session_id=t.session_id),NULLIF(t.batch_state,''),'visible'),
		       `+timelineItemLogicalBytesSQL+`
		FROM timeline_items t`)
	if err != nil {
		return status, err
	}
	defer rows.Close()
	protectionCutoff := now.UTC().AddDate(0, 0, -timelineProtectionDays)
	expiryCutoff := now.UTC().AddDate(0, 0, -timelineRoutineExpiryDays)
	candidates := make([]timelineRetentionCandidate, 0)
	for rows.Next() {
		var presentedAtRaw, batchState string
		var logicalBytes int64
		if err := rows.Scan(&presentedAtRaw, &batchState, &logicalBytes); err != nil {
			return status, err
		}
		status.TotalItems++
		status.LogicalBytes += logicalBytes
		if batchState != "visible" {
			status.HiddenItems++
			status.ProtectedItems++
			continue
		}
		presentedAt, parseErr := time.Parse(time.RFC3339Nano, presentedAtRaw)
		if parseErr != nil {
			status.MissingPresentationItems++
			status.ProtectedItems++
			continue
		}
		if presentedAt.After(protectionCutoff) {
			status.ProtectedItems++
			continue
		}
		status.EligibleItems++
		status.ReclaimableItems++
		status.ReclaimableBytes += logicalBytes
		expired := !presentedAt.After(expiryCutoff)
		if expired {
			status.RoutineExpiredItems++
		}
		candidates = append(candidates, timelineRetentionCandidate{presentedAt: presentedAt, bytes: logicalBytes, expired: expired})
	}
	if err := rows.Err(); err != nil {
		return status, err
	}
	status.OverItemLimit = status.TotalItems > timelineMaxItems
	status.OverByteLimit = status.LogicalBytes > timelineMaxLogicalBytes
	remainingItems, remainingBytes := status.TotalItems, status.LogicalBytes
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].presentedAt.Before(candidates[j].presentedAt) })
	for _, candidate := range candidates {
		if candidate.expired || remainingItems > timelineMaxItems || remainingBytes > timelineMaxLogicalBytes {
			remainingItems--
			remainingBytes -= candidate.bytes
			status.WouldRemoveItems++
			status.WouldRemoveBytes += candidate.bytes
		}
	}
	status.NeedsAttention = remainingItems > timelineMaxItems || remainingBytes > timelineMaxLogicalBytes
	return status, nil
}

func (s *Store) TimelineStorageStatus(ctx context.Context) (domain.TimelineStorageStatus, error) {
	status, err := measureTimelineStorage(ctx, s.db, s.Now())
	if err != nil {
		return status, err
	}
	status.DatabaseEffectiveBytes, err = s.databaseEffectiveFootprint(ctx)
	if err != nil {
		return status, err
	}
	status.DatabaseAllocatedBytes = s.databaseFootprint()
	return status, nil
}

func (s *Store) recordTimelineRetentionObservationTx(ctx context.Context, tx *sql.Tx, now time.Time) (domain.TimelineStorageStatus, error) {
	status, err := measureTimelineStorage(ctx, tx, now)
	if err != nil {
		return status, err
	}
	status.ReceiptID = domain.NewID("timeline-retention")
	boolInt := func(value bool) int {
		if value {
			return 1
		}
		return 0
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO timeline_retention_receipts(
		id,mode,evaluated_at,protection_days,routine_expiry_days,max_items,max_logical_bytes,
		total_items,logical_bytes,protected_items,eligible_items,routine_expired_items,hidden_items,missing_presentation_items,
		over_item_limit,over_byte_limit,reclaimable_items,reclaimable_bytes,would_remove_items,would_remove_bytes,needs_attention,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		status.ReceiptID, status.Mode, status.EvaluatedAt, status.Policy.ProtectionDays, status.Policy.RoutineExpiryDays,
		status.Policy.MaxItems, status.Policy.MaxLogicalBytes, status.TotalItems, status.LogicalBytes, status.ProtectedItems,
		status.EligibleItems, status.RoutineExpiredItems, status.HiddenItems, status.MissingPresentationItems,
		boolInt(status.OverItemLimit), boolInt(status.OverByteLimit), status.ReclaimableItems, status.ReclaimableBytes,
		status.WouldRemoveItems, status.WouldRemoveBytes, boolInt(status.NeedsAttention), status.EvaluatedAt)
	if err != nil {
		return status, err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM timeline_retention_receipts WHERE id NOT IN (SELECT id FROM timeline_retention_receipts ORDER BY created_at DESC,id DESC LIMIT ?)`, timelineReceiptLimit)
	return status, err
}
