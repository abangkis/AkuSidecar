package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func corruptSessionParent(t *testing.T, s *Store) string {
	t.Helper()
	session, timeline := insertSemanticTimelineFixture(t, s, "new_event")
	for _, q := range []string{`PRAGMA foreign_keys=OFF`, `DELETE FROM sessions WHERE id='` + session.ID + `'`, `PRAGMA foreign_keys=ON`} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return timeline
}

func TestForeignKeysSurviveConnectionReplacement(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	s.db.SetMaxIdleConns(0)
	for i := 0; i < 3; i++ {
		if err := requireForeignKeys(ctx, s.db); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO auto_update_batches(session_id,state,created_at) VALUES('missing','preparing','now')`); err == nil {
		t.Fatal("replacement connection accepted orphan")
	}
}

func TestOpenKeepsMaintenanceUIAvailableForRepairableDatabase(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	corruptSessionParent(t, s)
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatalf("open repairable database: %v", err)
	}
	defer reopened.Close()
	health, err := reopened.DatabaseHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "attention" || !health.Repairable || health.OrphanRows == 0 {
		t.Fatalf("health=%+v", health)
	}
	if err := reopened.RequireHealthyDatabase(ctx); !errors.Is(err, ErrDatabaseMaintenanceRequired) {
		t.Fatalf("new updates were not paused: %v", err)
	}
}

func TestDatabaseCleanupPreservesLearningAndMemory(t *testing.T) {
	for _, backup := range []bool{false, true} {
		t.Run(map[bool]string{false: "without_backup", true: "with_backup"}[backup], func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			memory, err := s.CreateMemoryRecallStub(ctx, memoryStubInput())
			if err != nil {
				t.Fatal(err)
			}
			timeline := corruptSessionParent(t, s)
			if _, err = s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.Exec(`INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) SELECT run_id,evidence_key,source,assessment_json,1,1,1,1,created_at FROM timeline_items WHERE id=?`, timeline); err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.Exec(`INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,created_at) SELECT 'repair-learning',id,session_id,run_id,evidence_key,'more',created_at FROM timeline_items WHERE id=?`, timeline); err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
				t.Fatal(err)
			}
			h, err := s.DatabaseHealth(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if h.Status != "attention" || !h.Repairable || h.ForeignKeyViolations == 0 {
				t.Fatalf("health=%+v", h)
			}
			settings, _ := s.GetSettings(ctx)
			if _, err := s.EnforceRetention(ctx, settings); !errors.Is(err, ErrDatabaseMaintenanceRequired) {
				t.Fatalf("retention error=%v", err)
			}
			before, _ := s.DatabaseHealth(ctx)
			if before.Fingerprint != h.Fingerprint {
				t.Fatal("preflight changed damaged data")
			}
			result, err := s.CleanDatabase(ctx, DatabaseCleanupRequest{Confirmed: true, Fingerprint: h.Fingerprint, Backup: backup})
			if err != nil {
				t.Fatal(err)
			}
			if result.Health.Status != "healthy" || result.Health.OrphanRows != 0 || result.RemovedRows == 0 {
				t.Fatalf("cleanup=%+v", result)
			}
			if result.Health.LimitBytes == 0 || result.Health.EffectiveDatabaseBytes == 0 {
				t.Fatalf("cleanup response omitted database footprint: %+v", result.Health)
			}
			if backup {
				if _, err := os.Stat(result.BackupPath); err != nil {
					t.Fatal(err)
				}
			} else if result.BackupPath != "" {
				t.Fatal("unexpected backup")
			}
			if _, err := s.MemoryItem(ctx, memory.ID); err != nil {
				t.Fatalf("memory lost: %v", err)
			}
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM preference_learning_ledger WHERE event_id='repair-learning'`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("learning lost: count=%d err=%v", count, err)
			}
			if _, err := s.EnforceRetention(ctx, settings); err != nil {
				t.Fatalf("retention did not resume: %v", err)
			}
		})
	}
}

func TestDatabaseCleanupRejectsStaleAndUnconfirmedActions(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	timeline := corruptSessionParent(t, s)
	h, err := s.DatabaseHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CleanDatabase(ctx, DatabaseCleanupRequest{Fingerprint: h.Fingerprint}); err == nil {
		t.Fatal("unconfirmed cleanup accepted")
	}
	if _, err := s.db.Exec(`UPDATE runs SET status='failed' WHERE id=(SELECT run_id FROM timeline_items WHERE id=?)`, timeline); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CleanDatabase(ctx, DatabaseCleanupRequest{Confirmed: true, Fingerprint: h.Fingerprint}); !errors.Is(err, ErrDatabaseHealthChanged) {
		t.Fatalf("stale cleanup error=%v", err)
	}
}

func TestDatabaseCleanupRejectsUnknownAndDurableOrphans(t *testing.T) {
	for _, query := range []string{
		`CREATE TABLE unknown_child(id INTEGER PRIMARY KEY,parent TEXT REFERENCES sessions(id)); INSERT INTO unknown_child VALUES(1,'missing')`,
		`INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at) VALUES('missing','saved','now')`,
	} {
		t.Run(query[:12], func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			corruptSessionParent(t, s)
			if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF; ` + query + `; PRAGMA foreign_keys=ON`); err != nil {
				t.Fatal(err)
			}
			h, err := s.DatabaseHealth(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if h.Repairable {
				t.Fatal("unknown/durable orphan marked repairable")
			}
			if _, err := s.CleanDatabase(ctx, DatabaseCleanupRequest{Confirmed: true, Fingerprint: h.Fingerprint}); err == nil {
				t.Fatal("unsafe repair accepted")
			}
			after, _ := s.DatabaseHealth(ctx)
			if after.Fingerprint != h.Fingerprint {
				t.Fatal("rejected repair changed data")
			}
		})
	}
}

func TestRetentionPostflightRollsBackApplicationOrphan(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	session, _ := insertSemanticTimelineFixture(t, s, "new_event")
	if _, err := s.db.Exec(`UPDATE sessions SET completed_at='2000-01-01T00:00:00Z' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER retention_corruption AFTER DELETE ON sessions BEGIN INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at) VALUES('missing','saved','now'); END`); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.GetSettings(ctx)
	if _, err := s.EnforceRetention(ctx, settings); !errors.Is(err, ErrDatabaseMaintenanceRequired) {
		t.Fatalf("postflight error=%v", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, session.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("deletion not rolled back: count=%d err=%v", count, err)
	}
}
