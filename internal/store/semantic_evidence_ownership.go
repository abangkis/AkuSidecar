package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Reports own the compact reasoned item needed for semantic history and undo.
// timeline_id remains a historical identifier, not a storage ownership edge.
// Full captured content stays in independently retained Personal Memory.
const semanticEvidenceOwnershipSQL = `
CREATE TRIGGER IF NOT EXISTS semantic_report_snapshot_insert AFTER INSERT ON semantic_event_reports BEGIN
 UPDATE semantic_event_reports SET item_json=(SELECT item_json FROM timeline_items WHERE id=NEW.timeline_id)
 WHERE id=NEW.id AND item_json IS NULL;
END;
CREATE TRIGGER IF NOT EXISTS semantic_report_snapshot_eviction BEFORE DELETE ON timeline_items BEGIN
 UPDATE semantic_event_reports SET item_json=OLD.item_json WHERE timeline_id=OLD.id;
END;
`

// Called inside the schema-29 transaction on a reserved connection with FK
// enforcement disabled. Rebuild without renaming the original parent: child
// report references remain intact and no cascade can erase deltas or undo.
func migrateSemanticEvidenceOwnershipTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='trigger' ORDER BY name`)
	if err != nil {
		return err
	}
	var names, definitions []string
	for rows.Next() {
		var name, definition string
		if err = rows.Scan(&name, &definition); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
		definitions = append(definitions, definition)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err = tx.ExecContext(ctx, `DROP TRIGGER `+quoteIdentifier(name)); err != nil {
			return err
		}
	}
	for _, table := range []string{"semantic_event_reports", "semantic_event_corrections"} {
		var ddl string
		err = tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&ddl)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		objectRows, e := tx.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE tbl_name=? AND type='index' AND sql IS NOT NULL ORDER BY name`, table)
		if e != nil {
			return e
		}
		var indexes []string
		for objectRows.Next() {
			var definition string
			if e = objectRows.Scan(&definition); e != nil {
				objectRows.Close()
				return e
			}
			indexes = append(indexes, definition)
		}
		e = objectRows.Err()
		objectRows.Close()
		if e != nil {
			return e
		}
		ddl = regexp.MustCompile(`(?i)\s+REFERENCES\s+timeline_items\s*\(id\)\s+ON\s+DELETE\s+CASCADE`).ReplaceAllString(ddl, "")
		start := strings.Index(ddl, "(")
		if start < 0 {
			return fmt.Errorf("invalid semantic table definition: %s", table)
		}
		temporary := table + "_ownership_v29"
		if _, err = tx.ExecContext(ctx, `CREATE TABLE `+quoteIdentifier(temporary)+ddl[start:]); err != nil {
			return err
		}
		columnRows, e := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if e != nil {
			return e
		}
		columns := []string{"rowid"}
		for columnRows.Next() {
			var column string
			if e = columnRows.Scan(&column); e != nil {
				columnRows.Close()
				return e
			}
			columns = append(columns, quoteIdentifier(column))
		}
		e = columnRows.Err()
		columnRows.Close()
		if e != nil {
			return e
		}
		columnList := strings.Join(columns, ",")
		if _, err = tx.ExecContext(ctx, `INSERT INTO `+quoteIdentifier(temporary)+` (`+columnList+`) SELECT `+columnList+` FROM `+quoteIdentifier(table)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DROP TABLE `+quoteIdentifier(table)+`; ALTER TABLE `+quoteIdentifier(temporary)+` RENAME TO `+quoteIdentifier(table)); err != nil {
			return err
		}
		for _, definition := range indexes {
			if _, err = tx.ExecContext(ctx, definition); err != nil {
				return err
			}
		}
		var timelineFKs int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_list(?) WHERE "table"='timeline_items'`, table).Scan(&timelineFKs); err != nil {
			return err
		}
		if timelineFKs != 0 {
			return fmt.Errorf("unrecognized Timeline ownership reference on %s", table)
		}
		if table == "semantic_event_reports" {
			var snapshotColumn int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('semantic_event_reports') WHERE name='item_json'`).Scan(&snapshotColumn); err != nil {
				return err
			}
			if snapshotColumn == 0 {
				if _, err = tx.ExecContext(ctx, `ALTER TABLE semantic_event_reports ADD COLUMN item_json TEXT`); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `UPDATE semantic_event_reports SET item_json=(SELECT t.item_json FROM timeline_items t WHERE t.id=semantic_event_reports.timeline_id) WHERE EXISTS(SELECT 1 FROM timeline_items t WHERE t.id=semantic_event_reports.timeline_id)`); err != nil {
				return err
			}
		}
	}
	for _, definition := range definitions {
		if _, err = tx.ExecContext(ctx, definition); err != nil {
			return err
		}
	}
	return nil
}
