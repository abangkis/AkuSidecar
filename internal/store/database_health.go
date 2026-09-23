package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var ErrDatabaseMaintenanceRequired = errors.New("database cleanup required; retention and new updates are paused")
var ErrDatabaseHealthChanged = errors.New("database issue changed; inspect it again before cleanup")

type healthDB interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type DatabaseHealthIssue struct {
	Table      string `json:"table"`
	Parent     string `json:"parent"`
	Kind       string `json:"kind"`
	Count      int    `json:"count"`
	Repairable bool   `json:"repairable"`
}

type DatabaseHealth struct {
	EffectiveDatabaseBytes int64                 `json:"effectiveDatabaseBytes"`
	LimitBytes             int64                 `json:"limitBytes"`
	StoragePressure        bool                  `json:"storagePressure"`
	Status                 string                `json:"status"`
	ForeignKeysEnabled     bool                  `json:"foreignKeysEnabled"`
	QuickCheck             string                `json:"quickCheck"`
	ForeignKeyViolations   int                   `json:"foreignKeyViolations"`
	OrphanRows             int                   `json:"orphanRows"`
	AffectedTables         []DatabaseHealthIssue `json:"affectedTables"`
	Repairable             bool                  `json:"repairable"`
	Fingerprint            string                `json:"fingerprint"`
}

type orphanRow struct {
	table  string
	rowid  int64
	parent string
	fk     int
	kind   string
}

// Explicitly audited operational relationships. Missing source definitions and
// durable memory/learning relationships are never repaired by deleting data.
var repairableRelations = map[string]string{
	"auto_update_batches": "sessions", "runs": "sessions", "bridge_commands": "runs",
	"observations": "runs bridge_commands", "capture_surface_events": "sessions runs",
	"content_continuity_occurrences": "runs", "run_stage_timings": "runs", "reasoning_invocations": "runs",
	"candidate_assessments": "runs", "ai_assessments": "timeline_items",
	"ai_detection_jobs": "sessions", "media_provenance_assessments": "timeline_items",
	"media_recaptures": "timeline_items", "vision_evaluation_jobs": "runs sessions",
	"timeline_evidence_overrides": "timeline_items media_recaptures", "calibration_sessions": "sessions",
	"calibration_samples": "calibration_sessions runs", "calibration_profile_snapshots": "calibration_sessions",
	"selection_corrections":  "timeline_items sessions runs",
	"semantic_event_reports": "semantic_events", "semantic_event_deltas": "semantic_events semantic_event_reports",
	"semantic_event_constraints": "semantic_events", "semantic_novelty_constraints": "semantic_events",
	"event_resolution_invocations": "sessions", "event_resolution_diagnostics": "event_resolution_invocations",
	"semantic_event_corrections": "semantic_event_reports",
	"content_identity_aliases":   "runs", "content_continuity": "runs",
}

type applicationRelation struct {
	table, column, parent, parentColumn string
	repairable                          bool
}

var applicationRelations = []applicationRelation{
	{"content_identity_aliases", "last_run_id", "runs", "id", true},
	{"content_continuity", "last_run_id", "runs", "id", true},
	{"memory_identity_aliases", "memory_item_id", "memory_items", "id", false},
	{"memory_tombstone_aliases", "memory_item_id", "memory_items", "id", false},
	{"memory_content_versions", "memory_item_id", "memory_items", "id", false},
	{"memory_provenance", "memory_item_id", "memory_items", "id", false},
	{"memory_actions", "memory_item_id", "memory_items", "id", false},
	{"memory_retention_claims", "memory_item_id", "memory_items", "id", false},
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

func requireForeignKeys(ctx context.Context, db healthDB) error {
	var enabled int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return fmt.Errorf("%w: foreign key enforcement is disabled", ErrDatabaseMaintenanceRequired)
	}
	return nil
}

func inspectDatabaseHealth(ctx context.Context, db healthDB) (DatabaseHealth, []orphanRow, error) {
	h := DatabaseHealth{Status: "healthy", Repairable: true, AffectedTables: []DatabaseHealthIssue{}}
	var enabled int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		return h, nil, err
	}
	h.ForeignKeysEnabled = enabled == 1
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&h.QuickCheck); err != nil {
		return h, nil, err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return h, nil, err
	}
	var violations []orphanRow
	for rows.Next() {
		var v orphanRow
		var rowid sql.NullInt64
		if err := rows.Scan(&v.table, &rowid, &v.parent, &v.fk); err != nil {
			rows.Close()
			return h, nil, err
		}
		if !rowid.Valid {
			h.Repairable = false
		}
		v.rowid, v.kind = rowid.Int64, "foreign_key"
		violations = append(violations, v)
		h.ForeignKeyViolations++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return h, nil, err
	}
	for _, relation := range applicationRelations {
		query := `SELECT c.rowid FROM ` + quoteIdentifier(relation.table) + ` c WHERE NOT EXISTS (SELECT 1 FROM ` + quoteIdentifier(relation.parent) + ` p WHERE p.` + quoteIdentifier(relation.parentColumn) + `=c.` + quoteIdentifier(relation.column) + `)`
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return h, nil, err
		}
		for rows.Next() {
			v := orphanRow{table: relation.table, parent: relation.parent, kind: "application"}
			if err := rows.Scan(&v.rowid); err != nil {
				rows.Close()
				return h, nil, err
			}
			violations = append(violations, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return h, nil, err
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		a, b := violations[i], violations[j]
		if a.table != b.table {
			return a.table < b.table
		}
		if a.rowid != b.rowid {
			return a.rowid < b.rowid
		}
		if a.parent != b.parent {
			return a.parent < b.parent
		}
		return a.fk < b.fk
	})
	hash := sha256.New()
	fmt.Fprintf(hash, "health-v1:%s\n", h.QuickCheck)
	unique := map[string]bool{}
	groups := map[string]*DatabaseHealthIssue{}
	for _, v := range violations {
		repairable := strings.Contains(" "+repairableRelations[v.table]+" ", " "+v.parent+" ")
		if !repairable {
			h.Repairable = false
		}
		key := v.table + ":" + v.parent + ":" + v.kind
		if groups[key] == nil {
			groups[key] = &DatabaseHealthIssue{Table: v.table, Parent: v.parent, Kind: v.kind, Repairable: repairable}
		}
		groups[key].Count++
		// VACUUM INTO can renumber implicit rowids in a backup. Natural keys
		// and affected row contents below identify the issue across that copy.
		fmt.Fprintf(hash, "%s:%s:%d:%s\n", v.table, v.parent, v.fk, v.kind)
		rowKey := fmt.Sprintf("%s:%d", v.table, v.rowid)
		if unique[rowKey] {
			continue
		}
		unique[rowKey] = true
		// Hash affected row contents as well as identity, without returning content
		// to the caller. A changed issue requires a fresh user decision.
		row, err := db.QueryContext(ctx, `SELECT * FROM `+quoteIdentifier(v.table)+` WHERE rowid=?`, v.rowid)
		if err != nil {
			return h, nil, err
		}
		columns, err := row.Columns()
		if err != nil {
			row.Close()
			return h, nil, err
		}
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if row.Next() {
			if err := row.Scan(targets...); err != nil {
				row.Close()
				return h, nil, err
			}
			encoded, err := json.Marshal(values)
			if err != nil {
				row.Close()
				return h, nil, err
			}
			hash.Write(encoded)
		}
		err = row.Err()
		row.Close()
		if err != nil {
			return h, nil, err
		}
	}
	h.OrphanRows = len(unique)
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		h.AffectedTables = append(h.AffectedTables, *groups[key])
	}
	h.Fingerprint = hex.EncodeToString(hash.Sum(nil))
	if h.QuickCheck != "ok" || !h.ForeignKeysEnabled {
		h.Repairable = false
	}
	if len(violations) > 0 || h.QuickCheck != "ok" || !h.ForeignKeysEnabled {
		h.Status = "attention"
	}
	return h, violations, nil
}

func (s *Store) DatabaseHealth(ctx context.Context) (DatabaseHealth, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DatabaseHealth{}, err
	}
	defer tx.Rollback()
	h, _, err := inspectDatabaseHealth(ctx, tx)
	if err != nil {
		return h, err
	}
	var pageCount, freePages, pageSize int64
	for _, v := range []struct {
		query  string
		target *int64
	}{{`PRAGMA page_count`, &pageCount}, {`PRAGMA freelist_count`, &freePages}, {`PRAGMA page_size`, &pageSize}} {
		if err := tx.QueryRowContext(ctx, v.query).Scan(v.target); err != nil {
			return h, err
		}
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE key='runtime'`).Scan(&raw); err != nil {
		return h, err
	}
	var settings domain.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return h, err
	}
	h.EffectiveDatabaseBytes = (pageCount - freePages) * pageSize
	h.LimitBytes = int64(settings.KnowledgeStorageLimitMB) * 1024 * 1024
	h.StoragePressure = h.EffectiveDatabaseBytes > h.LimitBytes
	return h, err
}

func (s *Store) RequireHealthyDatabase(ctx context.Context) error {
	h, err := s.DatabaseHealth(ctx)
	if err != nil {
		return err
	}
	if h.Status != "healthy" {
		return ErrDatabaseMaintenanceRequired
	}
	return nil
}

type DatabaseCleanupRequest struct {
	Confirmed   bool   `json:"confirmed"`
	Fingerprint string `json:"fingerprint"`
	Backup      bool   `json:"backup"`
}
type DatabaseCleanupResult struct {
	Health      DatabaseHealth `json:"health"`
	RemovedRows int64          `json:"removedRows"`
	BackupPath  string         `json:"backupPath,omitempty"`
}

func (s *Store) CleanDatabase(ctx context.Context, request DatabaseCleanupRequest) (DatabaseCleanupResult, error) {
	result := DatabaseCleanupResult{}
	if !request.Confirmed || request.Fingerprint == "" {
		return result, errors.New("explicit cleanup confirmation and fingerprint are required")
	}
	if err := s.requireIdle(ctx); err != nil {
		return result, err
	}
	initial, err := s.DatabaseHealth(ctx)
	if err != nil {
		return result, err
	}
	if initial.Fingerprint != request.Fingerprint {
		return result, ErrDatabaseHealthChanged
	}
	if !initial.Repairable || initial.Status != "attention" {
		return result, errors.New("database issue is not eligible for operational orphan cleanup")
	}
	if request.Backup {
		result.BackupPath, err = s.createMaintenanceBackup(ctx, initial.Fingerprint)
		if err != nil {
			return result, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	// Reserve the SQLite writer before checking the snapshot and deleting. This
	// also serializes other process writers for the entire repair transaction.
	if _, err = tx.ExecContext(ctx, `UPDATE meta SET value=value WHERE key='schema_version'`); err != nil {
		return result, err
	}
	if err = requireForeignKeys(ctx, tx); err != nil {
		return result, err
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE status IN ('queued','running')`).Scan(&active); err != nil {
		return result, err
	}
	if active != 0 {
		return result, errors.New("cleanup is unavailable while an update is running")
	}
	h, violations, err := inspectDatabaseHealth(ctx, tx)
	if err != nil {
		return result, err
	}
	if h.Fingerprint != request.Fingerprint {
		return result, ErrDatabaseHealthChanged
	}
	if !h.Repairable {
		return result, errors.New("database contains nonrepairable orphan relationships")
	}
	if err = syncPreferenceLearningLedgerTx(ctx, tx); err != nil {
		return result, fmt.Errorf("preserve learning before cleanup: %w", err)
	}
	before, err := operationalRowCount(ctx, tx)
	if err != nil {
		return result, err
	}
	// Deferred enforcement permits deleting connected damaged operational rows
	// in any order, while retaining commit-time FK enforcement.
	if _, err = tx.ExecContext(ctx, `PRAGMA defer_foreign_keys=ON`); err != nil {
		return result, err
	}
	for _, v := range violations {
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+quoteIdentifier(v.table)+` WHERE rowid=?`, v.rowid); err != nil {
			return result, err
		}
	}
	// Operational last-run pointers may become orphaned through cascading deletes.
	for _, r := range applicationRelations {
		if r.repairable {
			if _, err = tx.ExecContext(ctx, `DELETE FROM `+quoteIdentifier(r.table)+` WHERE NOT EXISTS (SELECT 1 FROM `+quoteIdentifier(r.parent)+` p WHERE p.`+quoteIdentifier(r.parentColumn)+`=`+quoteIdentifier(r.table)+`.`+quoteIdentifier(r.column)+`)`); err != nil {
				return result, err
			}
		}
	}
	result.Health, _, err = inspectDatabaseHealth(ctx, tx)
	if err != nil {
		return result, err
	}
	if result.Health.Status != "healthy" {
		return result, fmt.Errorf("%w: cleanup rolled back because postflight is not pristine", ErrDatabaseMaintenanceRequired)
	}
	after, err := operationalRowCount(ctx, tx)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	result.RemovedRows = before - after
	// Refresh outside the transaction so the response includes the real file
	// footprint and configured storage limit, not the transaction-only zeros.
	result.Health, err = s.DatabaseHealth(ctx)
	if err != nil {
		return result, err
	}
	return result, nil
}

func operationalRowCount(ctx context.Context, db healthDB) (int64, error) {
	var total int64
	for table := range repairableRelations {
		var count int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+quoteIdentifier(table)).Scan(&count); err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (s *Store) createMaintenanceBackup(ctx context.Context, fingerprint string) (string, error) {
	directory := filepath.Join(filepath.Dir(s.path), "backups")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(directory, "pre-orphan-cleanup-"+time.Now().UTC().Format("20060102-150405.000000000")+".db")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO '`+strings.ReplaceAll(target, "'", "''")+`'`); err != nil {
		return "", err
	}
	backup, err := OpenReadOnly(target)
	if err != nil {
		return "", err
	}
	defer backup.Close()
	h, err := backup.DatabaseHealth(ctx)
	if err != nil {
		return "", err
	}
	// A repair backup must preserve the existing violations, not reject them.
	if h.QuickCheck != "ok" || h.Fingerprint != fingerprint {
		return "", errors.New("maintenance backup verification failed; original database was not cleaned")
	}
	return target, nil
}
