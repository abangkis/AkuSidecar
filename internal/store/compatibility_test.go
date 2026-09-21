package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestDatabaseCompatibilityClassification(t *testing.T) {
	for _, tc := range []struct{ name, version, want string }{
		{"supported-old", "7", "migratable"}, {"native-trace-upgrade", "25", "migratable"}, {"current", "26", "current"}, {"legacy", "5", "unsupported"}, {"future", "27", "newer"}, {"invalid", "bad", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "state.db")
			db, e := sql.Open("sqlite", p)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec("CREATE TABLE meta(key TEXT,value TEXT); INSERT INTO meta VALUES('schema_version',?)", tc.version); e != nil {
				t.Fatal(e)
			}
			db.Close()
			if got := InspectDatabase(p); got.Status != tc.want {
				t.Fatalf("got %+v", got)
			}
		})
	}
	p := filepath.Join(t.TempDir(), "state.db")
	if r := InspectDatabase(p); r.Status != "absent" {
		t.Fatal(r)
	}
	os.WriteFile(p, []byte("not sqlite"), 0600)
	if r := InspectDatabase(p); r.Status != "unknown" {
		t.Fatal(r)
	}
}

func TestPrepareFreshPreservesBytesAndProfile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows offline preparation")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "state.db")
	original := []byte("unrecognized existing database")
	os.WriteFile(p, original, 0600)
	profile := filepath.Join(dir, "browser-profile")
	os.Mkdir(profile, 0700)
	os.WriteFile(filepath.Join(profile, "sentinel"), []byte("keep"), 0600)
	if _, e := PrepareDatabase(p, "fresh", false, domain.Settings{}); e == nil {
		t.Fatal("unconfirmed action allowed")
	}
	r, e := PrepareDatabase(p, "fresh", true, domain.Settings{})
	if e != nil {
		t.Fatal(e)
	}
	if r.Status != "absent" || r.BackupPath == "" {
		t.Fatal(r)
	}
	data, e := os.ReadFile(filepath.Join(r.BackupPath, "state.db"))
	if e != nil || string(data) != string(original) {
		t.Fatalf("backup: %q %v", data, e)
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatalf("database still active: %v", e)
	}
	if _, e = os.Stat(filepath.Join(profile, "sentinel")); e != nil {
		t.Fatal(e)
	}
	if stages, e := filepath.Glob(filepath.Join(dir, "database-preparation-*")); e != nil || len(stages) != 0 {
		t.Fatalf("successful fresh start retained staging copies: %v %v", stages, e)
	}
}

func TestPrepareRejectsOpenDatabase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows offline preparation")
	}
	p := filepath.Join(t.TempDir(), "state.db")
	os.WriteFile(p, []byte("original"), 0600)
	f, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = PrepareDatabase(p, "fresh", true, domain.Settings{}); e == nil {
		t.Fatal("active file was archived")
	}
}

func TestPrepareMigrationStagesAndPreservesOriginal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows offline preparation")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "state.db")
	defaults := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	s, err := Open(p, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE meta SET value='24' WHERE key='schema_version'"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	r, err := PrepareDatabase(p, "migrate", true, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "current" {
		t.Fatal(r)
	}
	if original := InspectDatabase(filepath.Join(r.BackupPath, "state.db")); original.DatabaseSchemaVersion != 24 {
		t.Fatal(original)
	}
	if current := InspectDatabase(p); current.Status != "current" {
		t.Fatal(current)
	}
	if stages, e := filepath.Glob(filepath.Join(dir, "database-preparation-*")); e != nil || len(stages) != 0 {
		t.Fatalf("successful migration retained staging copies: %v %v", stages, e)
	}
}

func TestFailedMigrationLeavesOriginal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows offline preparation")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "state.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE meta(key TEXT,value TEXT); INSERT INTO meta VALUES('schema_version','24')")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, _ := os.ReadFile(p)
	if _, err := PrepareDatabase(p, "migrate", true, domain.Settings{}); err == nil {
		t.Fatal("broken migration succeeded")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("failed migration modified original")
	}
	if stages, e := filepath.Glob(filepath.Join(dir, "database-preparation-*")); e != nil || len(stages) != 1 {
		t.Fatalf("failed migration should retain one diagnostic staging copy: %v %v", stages, e)
	}
}

func TestMigrationRegistryComplete(t *testing.T) {
	for version := MinimumMigratableSchema; version < SchemaVersion; version++ {
		if !canMigrateSchema(version) {
			t.Fatalf("schema %d has no complete migration path to %d", version, SchemaVersion)
		}
	}
	last := schemaMigrations["24"]
	delete(schemaMigrations, "24")
	defer func() { schemaMigrations["24"] = last }()
	if canMigrateSchema(7) {
		t.Fatal("incomplete path advertised as migratable")
	}
}

func TestPrepareRejectsChangedConfirmedDatabase(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	os.WriteFile(p, []byte("original corrupt database"), 0600)
	inspection := InspectDatabase(p)
	if inspection.Fingerprint == "" {
		t.Fatal("readable corrupt file needs fingerprint")
	}
	os.WriteFile(p, []byte("replacement database"), 0600)
	if _, err := PrepareDatabaseExpected(p, "fresh", true, inspection.Fingerprint, domain.Settings{}); err == nil {
		t.Fatal("changed database accepted")
	}
	data, _ := os.ReadFile(p)
	if string(data) != "replacement database" {
		t.Fatal("changed source mutated")
	}
}
