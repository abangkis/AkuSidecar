package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Active retention must not invent a presentation timestamp when an origin
// expires. Persist the live batch visibility before that batch disappears.
const timelineActiveLifecycleSQL = `
CREATE TRIGGER IF NOT EXISTS timeline_snapshot_session BEFORE DELETE ON sessions BEGIN
 UPDATE timeline_items SET
   origin_status=OLD.status,
   presentation=COALESCE(NULLIF(presentation,''),json_extract(OLD.coverage_json,'$.timelinePresentation'),''),
   presented_at=CASE
     WHEN COALESCE((SELECT state FROM auto_update_batches WHERE session_id=OLD.id),NULLIF(batch_state,''),'visible')='visible'
     THEN NULLIF(presented_at,'')
     ELSE NULL END,
   batch_state=COALESCE((SELECT state FROM auto_update_batches WHERE session_id=OLD.id),NULLIF(batch_state,''),'visible')
 WHERE session_id=OLD.id;
END;
CREATE TRIGGER IF NOT EXISTS timeline_snapshot_run BEFORE DELETE ON runs BEGIN
 UPDATE timeline_items SET evidence_snapshot_json=COALESCE(evidence_snapshot_json,(
   SELECT b.value FROM observations o,json_each(o.observation_json,'$.snapshots') s,json_each(s.value,'$.blocks') b
   WHERE o.run_id=OLD.id AND json_extract(b.value,'$.evidenceKey')=timeline_items.evidence_key
   ORDER BY o.created_at,s.key,b.key LIMIT 1
 )) WHERE run_id=OLD.id;
END;
`

func migrateSchema28To29(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() {
		_, restoreErr := conn.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
		if err == nil {
			err = restoreErr
		}
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	before, err := lifecycleForeignKeyIssues(ctx, tx)
	if err != nil {
		return err
	}
	if err = migrateSemanticEvidenceOwnershipTx(ctx, tx); err != nil {
		return err
	}
	// Receipts have no children. Rebuild their mode constraint while preserving
	// historical observations verbatim and initializing actual removals to zero.
	if _, err = tx.ExecContext(ctx, `DROP INDEX IF EXISTS timeline_retention_receipts_created;
 ALTER TABLE timeline_retention_receipts RENAME TO timeline_retention_receipts_v28;`+timelineRetentionSQL+`
 INSERT INTO timeline_retention_receipts(
 id,mode,evaluated_at,protection_days,routine_expiry_days,max_items,max_logical_bytes,
 total_items,logical_bytes,protected_items,eligible_items,routine_expired_items,hidden_items,missing_presentation_items,
 over_item_limit,over_byte_limit,reclaimable_items,reclaimable_bytes,would_remove_items,would_remove_bytes,needs_attention,created_at,
 remaining_items,remaining_bytes)
 SELECT id,mode,evaluated_at,protection_days,routine_expiry_days,max_items,max_logical_bytes,
 total_items,logical_bytes,protected_items,eligible_items,routine_expired_items,hidden_items,missing_presentation_items,
 over_item_limit,over_byte_limit,reclaimable_items,reclaimable_bytes,would_remove_items,would_remove_bytes,needs_attention,created_at,
 total_items,logical_bytes FROM timeline_retention_receipts_v28;
 DROP TABLE timeline_retention_receipts_v28;`); err != nil {
		return err
	}
	var hasTimeline int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='timeline_items'`).Scan(&hasTimeline); err != nil {
		return err
	}
	if hasTimeline > 0 {
		if _, err = tx.ExecContext(ctx, `DROP TRIGGER IF EXISTS timeline_snapshot_session;`+timelineActiveLifecycleSQL+semanticEvidenceOwnershipSQL); err != nil {
			return err
		}
	}
	after, err := lifecycleForeignKeyIssues(ctx, tx)
	if err != nil {
		return err
	}
	for issue := range after {
		if !before[issue] {
			return fmt.Errorf("semantic ownership migration introduced foreign key issue: %s", issue)
		}
	}
	if _, err = tx.ExecContext(ctx, splitActionAuditSQL); err != nil {
		return fmt.Errorf("create bounded split action audit: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE meta SET value='29' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}
