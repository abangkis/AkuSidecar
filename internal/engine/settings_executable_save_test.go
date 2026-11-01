package engine

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type executableChangeProvider struct {
	reasoning.Deterministic
	name, path, resolved string
	discoverErr          error
	requests, uses       []string
	events               []string
	onDiscover           func(context.Context, string)
	onUse                func(string)
}

func (p *executableChangeProvider) Name() string           { return p.name }
func (p *executableChangeProvider) ExecutablePath() string { return p.path }
func (p *executableChangeProvider) DiscoverExecutable(ctx context.Context, requested string) (string, error) {
	p.requests = append(p.requests, requested)
	p.events = append(p.events, "discover")
	if p.onDiscover != nil {
		p.onDiscover(ctx, requested)
	}
	return p.resolved, p.discoverErr
}
func (p *executableChangeProvider) UseExecutable(path string) {
	p.uses = append(p.uses, path)
	p.events = append(p.events, "activate")
	if p.onUse != nil {
		p.onUse(path)
	}
	p.path = path
}
func (p *executableChangeProvider) Close() error {
	p.events = append(p.events, "close")
	return nil
}

func executableChangeFixture(t *testing.T) (*Engine, *store.Store, *executableChangeProvider, domain.Settings) {
	t.Helper()
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.ReasoningProvider = "test-executable"
	settings.ReasoningExecutablePath = `C:\fixture\old.exe`
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	p := &executableChangeProvider{name: settings.ReasoningProvider, path: settings.ReasoningExecutablePath, resolved: `C:\fixture\resolved.exe`}
	runtime := New(state, p, config.Config{}, log.New(io.Discard, "", 0))
	current, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return runtime, state, p, current
}

func assertExecutableChangeUnchanged(t *testing.T, state *store.Store, p *executableChangeProvider, before domain.Settings) {
	t.Helper()
	after, err := state.GetSettings(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) || p.path != before.ReasoningExecutablePath || len(p.uses) != 0 {
		t.Fatalf("failed change altered durable/runtime state: settings=%+v provider=%+v err=%v", after, p, err)
	}
}

func TestSaveSettingsExecutableChangeNoOp(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		name := "supported"
		if unsupported {
			name = "unsupported"
		}
		t.Run(name, func(t *testing.T) {
			runtime, state, p, before := executableChangeFixture(t)
			if unsupported {
				runtime.provider = reasoning.Deterministic{}
				before.ReasoningProvider = "deterministic"
				if err := state.SaveSettings(context.Background(), before); err != nil {
					t.Fatal(err)
				}
			}
			runtime.mu.Lock()
			runtime.work.active["busy"] = func() {}
			runtime.mu.Unlock()
			value := before
			value.ReasoningExecutablePath = " \t" + before.ReasoningExecutablePath + " \n"
			saved, err := runtime.SaveSettings(context.Background(), value)
			if err != nil || saved.ReasoningExecutablePath != value.ReasoningExecutablePath || len(p.requests) != 0 || len(p.uses) != 0 {
				t.Fatalf("trim-equal path did not bypass discovery/guards: saved=%+v provider=%+v err=%v", saved, p, err)
			}
			persisted, err := state.GetSettings(context.Background())
			if err != nil || persisted.ReasoningExecutablePath != value.ReasoningExecutablePath {
				t.Fatalf("no-op unexpectedly canonicalized the submitted path: settings=%+v err=%v", persisted, err)
			}
		})
	}
}

func TestSaveSettingsExecutableChangeGuards(t *testing.T) {
	for _, mode := range []string{"unsupported", "queued_session", "running_session", "active_work"} {
		t.Run(mode, func(t *testing.T) {
			runtime, state, p, before := executableChangeFixture(t)
			wantError := "finish the active update or AI Deep Detection before changing the reasoning executable"
			switch mode {
			case "unsupported":
				runtime.provider = reasoning.Deterministic{}
				wantError = "deterministic does not expose an editable executable"
				fallthrough
			case "active_work":
				runtime.mu.Lock()
				runtime.work.active["busy"] = func() {}
				runtime.mu.Unlock()
			default:
				settings := before
				settings.ActiveSources = []domain.Source{domain.SourceX}
				session, err := state.CreateUpdateSession(context.Background(), "executable guard fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
				if err != nil || len(session.Runs) != 1 {
					t.Fatalf("create guard session: %+v err=%v", session, err)
				}
				wantStatus := "queued"
				if mode == "running_session" {
					if _, err := state.StartRun(context.Background(), session.Runs[0].ID, map[string]any{}); err != nil {
						t.Fatal(err)
					}
					wantStatus = "running"
				}
				active, err := state.ActiveSession(context.Background())
				if err != nil || active == nil || active.Status != wantStatus {
					t.Fatalf("guard fixture status: active=%+v err=%v", active, err)
				}
			}
			value := before
			value.ReasoningExecutablePath = "changed"
			_, err := runtime.SaveSettings(context.Background(), value)
			if err == nil || err.Error() != wantError || len(p.requests) != 0 {
				t.Fatalf("guard contract changed: provider=%+v err=%v want=%q", p, err, wantError)
			}
			assertExecutableChangeUnchanged(t, state, p, before)
		})
	}
}

func TestSaveSettingsExecutableChangeDoesNotAddIdleGuards(t *testing.T) {
	for _, mode := range []string{"pending_only", "shutdown_only"} {
		t.Run(mode, func(t *testing.T) {
			runtime, _, p, before := executableChangeFixture(t)
			runtime.mu.Lock()
			if mode == "pending_only" {
				runtime.work.pending["pending"] = false
			} else {
				runtime.work.shuttingDown = true
			}
			runtime.mu.Unlock()
			value := before
			value.ReasoningExecutablePath = "changed"
			if _, err := runtime.SaveSettings(context.Background(), value); err != nil || len(p.requests) != 1 || len(p.uses) != 1 {
				t.Fatalf("executable change gained a provider-swap idle guard: provider=%+v err=%v", p, err)
			}
		})
	}
}

func TestSaveSettingsExecutableChangeDiscoveryAndActivationOrder(t *testing.T) {
	for _, tc := range []struct{ name, requested, resolved string }{
		{name: "trimmed_request", requested: " \tnew-executable \n", resolved: `C:\fixture\resolved.exe`},
		{name: "case_change", requested: `c:\fixture\old.exe`, resolved: `C:\fixture\resolved.exe`},
		{name: "empty_request", requested: "", resolved: `C:\fixture\discovered.exe`},
		{name: "empty_successful_resolution", requested: "changed", resolved: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, state, p, before := executableChangeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.resolved = tc.resolved
			p.onDiscover = func(got context.Context, requested string) {
				if got != ctx {
					t.Error("discovery context changed")
				}
				if runtime.operation.TryLock() {
					runtime.operation.Unlock()
					t.Error("operation lock was released before discovery")
				}
				if !runtime.mu.TryLock() {
					t.Error("work read lock leaked into discovery")
				} else {
					runtime.mu.Unlock()
				}
				assertExecutableChangeUnchanged(t, state, p, before)
			}
			p.onUse = func(path string) {
				persisted, err := state.GetSettings(context.Background())
				if err != nil || persisted.ReasoningExecutablePath != path {
					t.Errorf("activation preceded persistence: settings=%+v err=%v", persisted, err)
				}
				if runtime.operation.TryLock() {
					runtime.operation.Unlock()
					t.Error("operation lock was released before activation")
				}
			}
			value := before
			value.ReasoningExecutablePath = tc.requested
			saved, err := runtime.SaveSettings(ctx, value)
			wantRequest := tc.requested
			if tc.name == "trimmed_request" {
				wantRequest = "new-executable"
			}
			if err != nil || saved.ReasoningExecutablePath != tc.resolved || p.path != tc.resolved || !reflect.DeepEqual(p.requests, []string{wantRequest}) || !reflect.DeepEqual(p.uses, []string{tc.resolved}) || !reflect.DeepEqual(p.events, []string{"discover", "activate"}) {
				t.Fatalf("discovery/activation contract changed: saved=%+v provider=%+v err=%v", saved, p, err)
			}
		})
	}
}

func TestSaveSettingsExecutableChangeFailures(t *testing.T) {
	for _, mode := range []string{"discovery", "persistence_cancelled"} {
		t.Run(mode, func(t *testing.T) {
			runtime, state, p, before := executableChangeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := errors.New("discovery sentinel")
			wantError := "validate reasoning executable: discovery sentinel"
			if mode == "discovery" {
				p.discoverErr = cause
			} else {
				cause = context.Canceled
				wantError = "context canceled"
				p.onDiscover = func(context.Context, string) { cancel() }
			}
			value := before
			value.ReasoningExecutablePath = "changed"
			saved, err := runtime.SaveSettings(ctx, value)
			if err == nil || err.Error() != wantError || !errors.Is(err, cause) || !reflect.DeepEqual(saved, domain.Settings{}) || len(p.requests) != 1 {
				t.Fatalf("error contract changed: settings=%+v provider=%+v err=%v", saved, p, err)
			}
			assertExecutableChangeUnchanged(t, state, p, before)
		})
	}
}

func TestSaveSettingsExecutableChangeUsesCurrentProviderBeforeSwap(t *testing.T) {
	runtime, state := swapTestEngine(t)
	before, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before.ReasoningExecutablePath = `C:\fixture\old.exe`
	if err := state.SaveSettings(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	p := &executableChangeProvider{name: "deterministic", path: before.ReasoningExecutablePath, resolved: `C:\fixture\resolved.exe`}
	runtime.provider = p
	t.Cleanup(func() {
		if closer, ok := runtime.provider.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})
	p.onUse = func(path string) {
		persisted, err := state.GetSettings(context.Background())
		if err != nil || persisted.ReasoningProvider != "gemini-flash-lite" || persisted.ReasoningExecutablePath != path || runtime.provider != p {
			t.Errorf("executable activation moved across persistence/provider swap: settings=%+v err=%v", persisted, err)
		}
	}
	value := before
	value.ReasoningProvider = "gemini-flash-lite"
	value.ReasoningExecutablePath = "changed"
	saved, err := runtime.SaveSettings(context.Background(), value)
	if err != nil || saved.ReasoningExecutablePath != p.resolved || runtime.ProviderName() != "gemini-flash-lite" || !reflect.DeepEqual(p.events, []string{"discover", "activate", "close"}) {
		t.Fatalf("current-provider executable ownership changed: settings=%+v provider=%+v err=%v", saved, p, err)
	}
}
