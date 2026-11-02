package engine

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type fullResetFixture struct {
	e         *Engine
	state     *store.Store
	db        *sql.DB
	collector *collection.Coordinator
}

func newFullResetFixture(t *testing.T) fullResetFixture {
	t.Helper()
	e, state := testEngine(t)
	e.config.Capture = config.CaptureConfig{Profile: "standard", Visibility: "quiet", OpenMissingSource: true}
	e.config.Preference.Mode = "guarded_live"
	e.config.Reasoning.ActiveProvider = "deterministic"
	e.config.Reasoning.Providers = map[string]config.ProviderConfig{"deterministic": {}}
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	owner, err := captureruntime.New(&runtimeTestProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Terminate)
	collector := collection.NewCoordinator(owner, nil, func() error { return nil })
	collector.Request("headless")
	e.AttachCollectionCoordinator(collector)
	e.headlessAccess = []domain.Source{domain.SourceX}
	return fullResetFixture{e, state, db, collector}
}

func (f fullResetFixture) exec(t *testing.T, query string) {
	t.Helper()
	if _, err := f.db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func fullResetOperationHeld(t *testing.T, e *Engine) {
	t.Helper()
	if e.operation.TryLock() {
		e.operation.Unlock()
		t.Error("operation lock released during reset")
	}
}

func fullResetLocksReleased(t *testing.T, e *Engine) {
	t.Helper()
	if !e.operation.TryLock() {
		t.Fatal("operation lock retained after reset")
	}
	e.operation.Unlock()
	if !e.mu.TryLock() {
		t.Fatal("mu retained after reset")
	}
	e.mu.Unlock()
}

func (f fullResetFixture) assertUnchanged(t *testing.T, before domain.Settings, provider reasoning.Provider) {
	t.Helper()
	after, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed reset changed durable settings")
	}
	if f.e.provider != provider || f.collector.Status().Requested != "headless" || !reflect.DeepEqual(f.e.headlessAccess, []domain.Source{domain.SourceX}) {
		t.Fatal("failed reset changed runtime authority")
	}
	fullResetLocksReleased(t, f.e)
}

func TestEngineFullResetCharacterizationDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, target                           string
		emptyRegistry, executable, noCollector bool
	}{
		{name: "configured", target: "deterministic"},
		{name: "trimmed", target: "  deterministic\t"},
		{name: "fallback"},
		{name: "empty_registry_different_target", target: "gemini-flash-lite", emptyRegistry: true},
		{name: "runtime_executable", executable: true},
		{name: "no_collector", noCollector: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFullResetFixture(t)
			f.e.config.Reasoning.ActiveProvider = tc.target
			if tc.emptyRegistry {
				f.e.config.Reasoning.Providers = nil
			}
			if tc.executable {
				f.e.provider = &executableTestProvider{path: filepath.Join("fixture", "codex.exe")}
			}
			if tc.noCollector {
				f.e.collectionRuntime = nil
			}
			active := f.e.provider
			token, err := f.state.BridgeToken(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.e.FullReset(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.CompletedAt == "" || result.BackupFile == "" || filepath.Base(result.BackupFile) != result.BackupFile {
				t.Fatalf("result=%+v", result)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(f.state.Path()), "backups", result.BackupFile)); err != nil {
				t.Fatal(err)
			}
			want := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
			want.ReasoningProvider = strings.TrimSpace(tc.target)
			if want.ReasoningProvider == "" {
				want.ReasoningProvider = active.Name()
			}
			want.ReasoningExecutablePath = f.e.ReasoningRuntime().ExecutablePath
			want.Normalize()
			got, err := f.state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("defaults changed: got=%+v want=%+v", got, want)
			}
			if f.e.provider != active || f.e.headlessAccess != nil {
				t.Fatal("same-provider reset replaced provider or retained access")
			}
			if !tc.noCollector && f.collector.Status().Requested != "browser" {
				t.Fatal("browser transition not requested")
			}
			if gotToken, err := f.state.BridgeToken(context.Background()); err != nil || gotToken != token {
				t.Fatal("Bridge identity changed", err)
			}
			if pending, err := f.state.PendingAppProfileReset(context.Background()); err != nil || pending {
				t.Fatal("browser profile wipe staged", err)
			}
			fullResetLocksReleased(t, f.e)
		})
	}
}

func TestEngineFullResetCharacterizationCancellation(t *testing.T) {
	for _, path := range []string{"success", "cancelled_context", "different_provider"} {
		t.Run(path, func(t *testing.T) {
			f := newFullResetFixture(t)
			calls := map[string]int{}
			for _, key := range []string{"ai:a", "ai:b", "media-provenance", "ai", "media-provenance:other", "topics:a", "run:a"} {
				key := key
				f.e.work.active[key] = func() {
					fullResetOperationHeld(t, f.e)
					if f.e.mu.TryLock() {
						f.e.mu.Unlock()
						t.Error("cancellation must hold mu")
					}
					if f.collector.Status().Requested != "headless" || len(f.e.headlessAccess) != 1 {
						t.Error("runtime changed before cancellation")
					}
					calls[key]++
				}
			}
			ctx := context.Background()
			if path == "cancelled_context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if path == "different_provider" {
				f.e.config.Reasoning.ActiveProvider = "gemini-flash-lite"
			}
			result, err := f.e.FullReset(ctx)
			switch path {
			case "success":
				if err != nil {
					t.Fatal(err)
				}
			case "cancelled_context":
				if !errors.Is(err, context.Canceled) || result != (store.FullResetResult{}) {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			case "different_provider":
				if err == nil || err.Error() != "finish active reasoning work before switching the reasoning provider" || result != (store.FullResetResult{}) {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			if !reflect.DeepEqual(calls, map[string]int{"ai:a": 1, "ai:b": 1, "media-provenance": 1}) {
				t.Fatalf("cancellation scope=%v", calls)
			}
			if len(f.e.work.active) != 7 {
				t.Fatal("reset retired work instead of only cancelling it")
			}
			fullResetLocksReleased(t, f.e)
		})
	}
}

func TestEngineFullResetCharacterizationFailures(t *testing.T) {
	for _, kind := range []string{"same_provider_session", "different_provider_session", "pending_work", "shutdown", "unknown_provider", "construction", "backup", "transaction"} {
		t.Run(kind, func(t *testing.T) {
			f := newFullResetFixture(t)
			before, err := f.state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			active := f.e.provider
			want := ""
			switch kind {
			case "same_provider_session", "different_provider_session":
				f.exec(t, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES('reset-session','fixture','queued',1,1,'2026-11-02T00:00:00Z')`)
				want = "reset is unavailable while an update is running"
				if kind == "different_provider_session" {
					f.e.config.Reasoning.ActiveProvider = "gemini-flash-lite"
					want = "finish the active update before switching the reasoning provider"
				}
			case "pending_work":
				f.e.config.Reasoning.ActiveProvider = "gemini-flash-lite"
				f.e.work.pending["pending"] = true
				want = "finish active reasoning work before switching the reasoning provider"
			case "shutdown":
				f.e.config.Reasoning.ActiveProvider = "gemini-flash-lite"
				f.e.work.shuttingDown = true
				want = "reasoning provider cannot switch while shutting down"
			case "unknown_provider":
				f.e.config.Reasoning.ActiveProvider = "missing-reset-provider"
				want = `select reasoning provider "missing-reset-provider": `
			case "construction":
				f.e.config.Reasoning.ActiveProvider = "gemini-flash-lite"
				f.e.config.Reasoning.Providers["gemini-flash-lite"] = config.ProviderConfig{}
				want = `construct reasoning provider "gemini-flash-lite": `
			case "backup":
				if err := os.WriteFile(filepath.Join(filepath.Dir(f.state.Path()), "backups"), []byte("blocked"), 0o600); err != nil {
					t.Fatal(err)
				}
				want = "create reset backup directory: "
			case "transaction":
				// Construct a real offline candidate before the Store transaction fails.
				f.e.provider = &fullResetClosingProvider{name: "previous", close: func() error { t.Error("failed reset closed active provider"); return nil }}
				active = f.e.provider
				f.exec(t, `CREATE TRIGGER fail_reset BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT,'full-reset-transaction-failed'); END`)
				want = "constraint failed: full-reset-transaction-failed"
			}
			result, err := f.e.FullReset(context.Background())
			if err == nil || !strings.HasPrefix(err.Error(), want) || result != (store.FullResetResult{}) {
				t.Fatalf("result=%+v err=%v want prefix=%q", result, err, want)
			}
			f.assertUnchanged(t, before, active)
			if kind == "transaction" {
				files, err := filepath.Glob(filepath.Join(filepath.Dir(f.state.Path()), "backups", "pre-full-reset-*.db"))
				if err != nil || len(files) != 1 {
					t.Fatal("transaction failure must retain verified backup", files, err)
				}
				backup, err := sql.Open("sqlite", files[0])
				if err != nil {
					t.Fatal(err)
				}
				defer backup.Close()
				var raw string
				if err := backup.QueryRow(`SELECT value_json FROM settings WHERE key='runtime'`).Scan(&raw); err != nil || !strings.Contains(raw, "expanded") {
					t.Fatal("backup lost pre-reset settings", err)
				}
			}
		})
	}
}

type fullResetClosingProvider struct {
	reasoning.Deterministic
	name  string
	close func() error
}

func (p *fullResetClosingProvider) Name() string { return p.name }
func (p *fullResetClosingProvider) Close() error { return p.close() }

func TestEngineFullResetCharacterizationSwapOrder(t *testing.T) {
	for _, failClose := range []bool{false, true} {
		name := "close_success"
		if failClose {
			name = "close_error"
		}
		t.Run(name, func(t *testing.T) {
			f := newFullResetFixture(t)
			var messages bytes.Buffer
			f.e.logger = log.New(&messages, "", 0)
			calls := 0
			active := &fullResetClosingProvider{name: "previous"}
			active.close = func() error {
				calls++
				fullResetOperationHeld(t, f.e)
				if !f.e.mu.TryLock() {
					t.Fatal("swap apply must run after mu is released")
				}
				f.e.mu.Unlock()
				settings, err := f.state.GetSettings(context.Background())
				if err != nil || settings.LoadProfile != "standard" || settings.ReasoningProvider != "deterministic" {
					t.Fatal("swap applied before Store commit", settings, err)
				}
				if f.collector.Status().Requested != "browser" || f.e.headlessAccess != nil || f.e.provider != active {
					t.Fatal("runtime side-effect order changed")
				}
				if failClose {
					return errors.New("previous-close-fixture")
				}
				return nil
			}
			f.e.provider = active
			result, err := f.e.FullReset(context.Background())
			if err != nil || result.CompletedAt == "" || calls != 1 || f.e.ProviderName() != "deterministic" {
				t.Fatalf("result=%+v err=%v closes=%d provider=%s", result, err, calls, f.e.ProviderName())
			}
			if failClose && !strings.Contains(messages.String(), "previous reasoning provider previous shutdown failed: previous-close-fixture") {
				t.Fatal("Close failure was not logged")
			}
			if !strings.Contains(messages.String(), "reasoning provider switched previous -> deterministic") {
				t.Fatal("swap completion not logged")
			}
			fullResetLocksReleased(t, f.e)
		})
	}
}
