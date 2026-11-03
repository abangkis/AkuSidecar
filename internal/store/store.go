package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db                      *sql.DB
	path                    string
	clock                   Clock
	capturePermitMu         sync.RWMutex
	capturePermit           *accountCheckpointPermit
	fenceAttemptMu          sync.Mutex
	issuedFenceHoldID       string
	consumedFence           bool
	manualPilotMu           sync.Mutex
	manualPilots            map[string]*ManualPilotPermit
	manualPilotClosures     sync.Map
	manualPilotBeforeCommit func()
	manualPilotCommit       func(*sql.Tx) error
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func Open(path string, defaults domain.Settings) (*Store, error) {
	return OpenWithClock(path, defaults, systemClock{})
}

// OpenReadOnly opens an existing database without running initialization,
// migrations, retention, or any other write path. It is intended for bounded
// diagnostics and export tools. Callers must not use mutating Store methods
// on the returned handle.
func OpenReadOnly(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("read-only database path is required")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, clock: systemClock{}}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite read-only: %w", err)
	}
	return store, nil
}

func OpenWithClock(path string, defaults domain.Settings, clock Clock) (*Store, error) {
	return openWithClock(path, defaults, clock, true)
}

func openWithClock(path string, defaults domain.Settings, clock Clock, publishRuntimeVersion bool) (*Store, error) {
	if clock == nil {
		return nil, errors.New("store clock is required")
	}
	if err := validateDataRuntimeVersion(path, domain.ApplicationVersion); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := (&url.URL{Scheme: "file", Path: uriPath}).String() + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := requireForeignKeys(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db, path: path, clock: clock}
	if err := store.initialize(defaults); err != nil {
		db.Close()
		return nil, err
	}
	if err := requireForeignKeys(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	if publishRuntimeVersion {
		if err := writeDataRuntimeVersion(path, domain.ApplicationVersion); err != nil {
			db.Close()
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) Now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock.Now()
}

func (s *Store) initialize(defaults domain.Settings) error {
	ctx := context.Background()
	var metaTable int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&metaTable); err != nil {
		return fmt.Errorf("inspect database schema: %w", err)
	}
	if metaTable != 0 {
		var version string
		if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		for version != schemaVersion {
			migration, ok := schemaMigrations[version]
			if !ok {
				break
			}
			if err := migration.run(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema %s to %s: %w", version, migration.next, err)
			}
			version = migration.next
		}
		if version != schemaVersion {
			return fmt.Errorf("database schema %s is incompatible with required schema %s; start with a fresh database", version, schemaVersion)
		}
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}
	if err := s.syncSourceDefinitions(ctx); err != nil {
		return err
	}
	if err := s.backfillSemanticEventDeltas(ctx); err != nil {
		return fmt.Errorf("backfill semantic event deltas: %w", err)
	}
	if err := s.backfillContentContinuity(ctx); err != nil {
		return fmt.Errorf("backfill content continuity: %w", err)
	}
	if err := s.backfillContentIdentityAliases(ctx); err != nil {
		return fmt.Errorf("backfill content identity aliases: %w", err)
	}
	if metaTable == 0 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version',?)`, schemaVersion); err != nil {
			return fmt.Errorf("save schema version: %w", err)
		}
	}
	if _, err := s.bridgeToken(ctx); err != nil {
		return err
	}
	if _, err := s.memoryTombstoneKey(ctx); err != nil {
		return fmt.Errorf("initialize memory tombstone key: %w", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key='runtime'`).Scan(&count); err != nil {
		return err
	}
	fresh := count == 0
	if fresh {
		if err := s.SaveSettings(ctx, defaults); err != nil {
			return err
		}
	} else {
		settings, err := s.GetSettings(ctx)
		if err != nil {
			return err
		}
		adoptInstagram, err := s.shouldAdoptInstagramDefault(ctx, settings.ActiveSources)
		if err != nil {
			return err
		}
		if adoptInstagram {
			settings.ActiveSources = append(settings.ActiveSources, domain.SourceInstagram)
		}
		if err := s.SaveSettings(ctx, settings); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('source_default_instagram_v1','applied') ON CONFLICT(key) DO NOTHING`); err != nil {
		return fmt.Errorf("record Instagram default migration: %w", err)
	}
	if err := s.initializeOnboarding(ctx, fresh); err != nil {
		return err
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	_, err = s.EnforceRetention(ctx, settings)
	if errors.Is(err, ErrDatabaseMaintenanceRequired) {
		// Keep the application available so the health endpoint and explicit
		// cleanup action can guide the user through repair. New updates and
		// retention remain blocked by their own health preflight.
		return nil
	}
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

func decodeJSON(raw string, target any) {
	if raw == "" {
		return
	}
	_ = json.Unmarshal([]byte(raw), target)
}
