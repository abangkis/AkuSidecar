package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Operational IDs on durable rows are provenance snapshots, not ownership.
// Capture presentation and the bounded displayed evidence before raw operational
// data disappears. A missing origin never implies a fabricated completed session.
const timelineLifecycleSQL = `
CREATE TRIGGER IF NOT EXISTS timeline_snapshot_session BEFORE DELETE ON sessions BEGIN
 UPDATE timeline_items SET
   origin_status=OLD.status,
   presentation=COALESCE(NULLIF(presentation,''),json_extract(OLD.coverage_json,'$.timelinePresentation'),''),
   presented_at=CASE
     WHEN COALESCE((SELECT state FROM auto_update_batches WHERE session_id=OLD.id),NULLIF(batch_state,''),'visible')='visible'
     THEN COALESCE(NULLIF(presented_at,''),(SELECT revealed_at FROM auto_update_batches WHERE session_id=OLD.id),OLD.completed_at,created_at)
     ELSE NULL END,
   batch_state=COALESCE(NULLIF(batch_state,''),(SELECT state FROM auto_update_batches WHERE session_id=OLD.id),'visible')
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

// Rebuild on one reserved connection with FK enforcement temporarily disabled
// outside the transaction. This is SQLite's safe parent-table rebuild sequence:
// no parent rename (which would rewrite child references), no cascaded deletes.
// All rows, extra columns, indexes and triggers are preserved, and the version
// advances in the same transaction. Open restores enforcement before any work.
func migrateSchema26To27(ctx context.Context, db *sql.DB) (err error) {
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
	// Triggers on other tables may refer to a rebuilt parent. Temporarily drop
	// and restore their exact definitions inside this same atomic transaction.
	triggerRows, err := tx.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='trigger' ORDER BY name`)
	if err != nil {
		return err
	}
	var triggerNames, triggerDefinitions []string
	for triggerRows.Next() {
		var name, definition string
		if err = triggerRows.Scan(&name, &definition); err != nil {
			triggerRows.Close()
			return err
		}
		triggerNames = append(triggerNames, name)
		triggerDefinitions = append(triggerDefinitions, definition)
	}
	err = triggerRows.Err()
	triggerRows.Close()
	if err != nil {
		return err
	}
	for _, name := range triggerNames {
		if _, err = tx.ExecContext(ctx, `DROP TRIGGER `+quoteIdentifier(name)); err != nil {
			return err
		}
	}
	for _, table := range []string{"timeline_items", "ai_assessments", "media_provenance_assessments", "feedback_events", "semantic_event_reports"} {
		var ddl string
		err = tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&ddl)
		if err == sql.ErrNoRows {
			continue
		} // canonical initialization supplies older optional tables
		if err != nil {
			return err
		}
		rows, queryErr := tx.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE tbl_name=? AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name`, table)
		if queryErr != nil {
			return queryErr
		}
		var objects []string
		for rows.Next() {
			var definition string
			if err = rows.Scan(&definition); err != nil {
				rows.Close()
				return err
			}
			objects = append(objects, definition)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		// Only the two audited operational reference clauses change. Required
		// identifiers remain as historical evidence, including already-detached rows.
		ddl = regexp.MustCompile(`(?i)\s+REFERENCES\s+(sessions|runs)\s*\(id\)\s+ON\s+DELETE\s+CASCADE`).ReplaceAllString(ddl, "")
		start := strings.Index(ddl, "(")
		if start < 0 {
			return fmt.Errorf("invalid definition for %s", table)
		}
		temporary := table + "_lifecycle_v27"
		ddl = `CREATE TABLE ` + quoteIdentifier(temporary) + ddl[start:]
		if _, err = tx.ExecContext(ctx, ddl); err != nil {
			return err
		}
		columnRows, columnErr := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if columnErr != nil {
			return columnErr
		}
		columns := []string{"rowid"}
		for columnRows.Next() {
			var column string
			if err = columnRows.Scan(&column); err != nil {
				columnRows.Close()
				return err
			}
			columns = append(columns, quoteIdentifier(column))
		}
		err = columnRows.Err()
		columnRows.Close()
		if err != nil {
			return err
		}
		columnList := strings.Join(columns, ",")
		if _, err = tx.ExecContext(ctx, `INSERT INTO `+quoteIdentifier(temporary)+` (`+columnList+`) SELECT `+columnList+` FROM `+quoteIdentifier(table)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DROP TABLE `+quoteIdentifier(table)+`; ALTER TABLE `+quoteIdentifier(temporary)+` RENAME TO `+quoteIdentifier(table)); err != nil {
			return err
		}
		for _, definition := range objects {
			if _, err = tx.ExecContext(ctx, definition); err != nil {
				return err
			}
		}
		var operationalFKs int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_list(?) WHERE "table" IN ('sessions','runs')`, table).Scan(&operationalFKs); err != nil {
			return err
		}
		if operationalFKs != 0 {
			return fmt.Errorf("unrecognized operational foreign key definition on %s", table)
		}
		if table == "timeline_items" {
			for _, column := range []string{"origin_status TEXT NOT NULL DEFAULT 'unavailable'", "presentation TEXT NOT NULL DEFAULT ''", "presented_at TEXT", "batch_state TEXT NOT NULL DEFAULT ''", "evidence_snapshot_json TEXT"} {
				name := strings.Fields(column)[0]
				var count int
				if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('timeline_items') WHERE name=?`, name).Scan(&count); err != nil {
					return err
				}
				if count == 0 {
					if _, err = tx.ExecContext(ctx, `ALTER TABLE timeline_items ADD COLUMN `+column); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, definition := range triggerDefinitions {
		if _, err = tx.ExecContext(ctx, definition); err != nil {
			return err
		}
	}
	var hasTimeline int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='timeline_items'`).Scan(&hasTimeline); err != nil {
		return err
	}
	if hasTimeline != 0 {
		if _, err = tx.ExecContext(ctx, timelineLifecycleSQL); err != nil {
			return err
		}
	}
	after, err := lifecycleForeignKeyIssues(ctx, tx)
	if err != nil {
		return err
	}
	for issue := range after {
		if !before[issue] {
			return fmt.Errorf("lifecycle migration introduced foreign key issue: %s", issue)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE meta SET value='27' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

// Existing damage remains inspectable by the maintenance UI; a migration may
// remove obsolete ownership violations but must never introduce new ones.
func lifecycleForeignKeyIssues(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := map[string]bool{}
	for rows.Next() {
		var table, parent string
		var row sql.NullInt64
		var fk int
		if err := rows.Scan(&table, &row, &parent, &fk); err != nil {
			return nil, err
		}
		issues[fmt.Sprintf("%s:%v:%s", table, row, parent)] = true
	}
	return issues, rows.Err()
}
