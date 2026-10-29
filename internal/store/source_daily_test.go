package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSourceDailyPermissionIsolationAndColdMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source-daily.db")
	defaults := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	s, err := Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveXDailyPermission(ctx, xDailyPermissionTestScope, true); err != nil {
		t.Fatal(err)
	}
	var xBefore string
	if err = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, xDailyPermissionMetaKey).Scan(&xBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE meta SET value='30' WHERE key='schema_version'; INSERT INTO meta(key,value) VALUES('migration_sentinel','unchanged')`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if info := InspectDatabase(path); info.Status != "migratable" || info.DatabaseSchemaVersion != 30 || info.TargetSchemaVersion != 31 {
		t.Fatalf("schema30 classification %+v", info)
	}
	s, err = Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	var version, sentinel, xAfter string
	for key, dest := range map[string]*string{"schema_version": &version, "migration_sentinel": &sentinel, xDailyPermissionMetaKey: &xAfter} {
		if err = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if version != "31" || sentinel != "unchanged" || xBefore != xAfter {
		t.Fatal("migration changed records or missed barrier")
	}
	if granted, err := s.DailySourcePermission(ctx, domain.SourceInstagram, xDailyPermissionTestScope); err != nil || granted {
		t.Fatal("X grant migrated into IG authority")
	}
	if err = s.SaveDailySourcePermission(ctx, domain.SourceInstagram, xDailyPermissionTestScope, true); err != nil {
		t.Fatal(err)
	}
	if granted, err := s.DailySourcePermission(ctx, domain.SourceInstagram, xDailyPermissionOtherScope); err != nil || granted {
		t.Fatal("IG grant crossed profile boundary")
	}
	if err = s.SaveDailySourcePermission(ctx, domain.SourceInstagram, xDailyPermissionTestScope, false); err != nil {
		t.Fatal(err)
	}
	if granted, err := s.XDailyPermission(ctx, xDailyPermissionTestScope); err != nil || !granted {
		t.Fatal("IG revocation changed X grant")
	}
	if err = s.SaveDailySourcePermission(ctx, domain.SourceFacebook, xDailyPermissionTestScope, true); err == nil {
		t.Fatal("unsupported source grant accepted")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if granted, err := s.DailySourcePermission(ctx, domain.SourceInstagram, xDailyPermissionTestScope); err != nil || granted {
		t.Fatal("IG revocation lost on cold reopen")
	}
	if info := InspectDatabase(path); info.DatabaseSchemaVersion != 31 || info.Status != "current" {
		t.Fatalf("schema31 reopen %+v", info)
	}
}
