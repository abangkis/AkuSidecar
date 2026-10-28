package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const xDailyPermissionTestScope = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const xDailyPermissionOtherScope = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func TestXDailyPermissionPersistsAcrossReopenAndDefaultsOff(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sidecar.db")
	defaults := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)

	state, err := Open(dbPath, defaults)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := state.XDailyPermission(ctx, xDailyPermissionTestScope)
	if err != nil || enabled {
		t.Fatalf("missing permission=(%t,%v), want false,nil", enabled, err)
	}
	if err := state.SaveXDailyPermission(ctx, xDailyPermissionTestScope, true); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dbPath, defaults)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	enabled, err = reopened.XDailyPermission(ctx, xDailyPermissionTestScope)
	if err != nil || !enabled {
		t.Fatalf("reopened permission=(%t,%v), want true,nil", enabled, err)
	}
	otherProfileEnabled, err := reopened.XDailyPermission(ctx, xDailyPermissionOtherScope)
	if err != nil || otherProfileEnabled {
		t.Fatalf("other profile permission=(%t,%v), want false,nil", otherProfileEnabled, err)
	}
}

func TestXDailyPermissionRevocationPersists(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sidecar.db")
	defaults := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := Open(dbPath, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SaveXDailyPermission(ctx, xDailyPermissionTestScope, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveXDailyPermission(ctx, xDailyPermissionTestScope, false); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dbPath, defaults)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	enabled, err := reopened.XDailyPermission(ctx, xDailyPermissionTestScope)
	if err != nil || enabled {
		t.Fatalf("revoked permission=(%t,%v), want false,nil", enabled, err)
	}
}

func TestXDailyPermissionIsIndependentOfSettingsAndAutoUpdate(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	autoUpdateEnabled := settings.AutoUpdateEnabled
	autoUpdateMode := settings.AutoUpdateMode
	if err := state.SaveXDailyPermission(ctx, xDailyPermissionTestScope, true); err != nil {
		t.Fatal(err)
	}
	settings.SingleImageFit = "contain"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	enabled, err := state.XDailyPermission(ctx, xDailyPermissionTestScope)
	if err != nil || !enabled {
		t.Fatalf("permission after SaveSettings=(%t,%v), want true,nil", enabled, err)
	}
	persisted, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.SingleImageFit != "contain" || persisted.AutoUpdateEnabled != autoUpdateEnabled || persisted.AutoUpdateMode != autoUpdateMode {
		t.Fatalf("settings after independent write=%+v; Auto Update was (%t,%q)", persisted, autoUpdateEnabled, autoUpdateMode)
	}
}

func TestXDailyPermissionRejectsMalformedStoredRecordsWithoutOverwriting(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	malformedRecords := []string{
		`{`,
		`{"version":2,"enabled":true,"profileScope":"` + xDailyPermissionTestScope + `"}`,
		`{"version":1,"enabled":"true","profileScope":"` + xDailyPermissionTestScope + `"}`,
		`{"version":1,"version":1,"enabled":true,"profileScope":"` + xDailyPermissionTestScope + `"}`,
		`{"version":1,"enabled":true}`,
		`{"version":1,"enabled":true,"profileScope":"` + xDailyPermissionTestScope + `","extra":false}`,
		`{"version":1,"enabled":null,"profileScope":"` + xDailyPermissionTestScope + `"}`,
		`{"version":1,"enabled":true,"profileScope":"ABCDEF"}`,
	}
	for _, raw := range malformedRecords {
		if _, err := state.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, xDailyPermissionMetaKey, raw); err != nil {
			t.Fatal(err)
		}
		enabled, err := state.XDailyPermission(ctx, xDailyPermissionTestScope)
		if err == nil || enabled {
			t.Fatalf("malformed record %q returned (%t,%v), want false,error", raw, enabled, err)
		}
		var stored string
		if err := state.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, xDailyPermissionMetaKey).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != raw {
			t.Fatalf("malformed record was overwritten: got %q, want %q", stored, raw)
		}
	}
}

func TestXDailyPermissionRejectsInvalidScopes(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	for _, scope := range []string{
		"",
		"0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
	} {
		if enabled, err := state.XDailyPermission(ctx, scope); err == nil || enabled {
			t.Fatalf("read with scope %q returned (%t,%v), want false,error", scope, enabled, err)
		}
		if err := state.SaveXDailyPermission(ctx, scope, true); err == nil {
			t.Fatalf("save with scope %q succeeded, want error", scope)
		}
	}
}
