//go:build windows && auth_session_fixture && auth_session_dev

package engine

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

// Faults live in the loopback authority, not in a runtime flag or startup bypass.
// The actual main must have persisted its fence and launched its owned browser
// before it requests authority. No reader UI or physical input is involved.
func TestDevelopmentSessionMainStartupFailures(t *testing.T) {
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
		t.Skip("requires explicit VerifyDevelopment helper")
	}
	for _, mode := range []string{"parent_death", "authority_timeout", "account_changed"} {
		t.Run(mode, func(t *testing.T) {
			var random [32]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}
			nonce := hex.EncodeToString(random[:])
			entered, release := make(chan struct{}), make(chan struct{})
			canceled := make(chan struct{}, 1)
			var enteredOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			fixture := &chromeFixtureHTTP{}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/process-authority" {
					fixture.serveHTTP(w, r)
					return
				}
				if r.Header.Get("X-Fixture-Key") != nonce {
					http.Error(w, "denied", http.StatusForbidden)
					return
				}
				enteredOnce.Do(func() { close(entered) })
				select {
				case <-r.Context().Done():
					select {
					case canceled <- struct{}{}:
					default:
					}
					return
				case <-release:
				}
				value, _ := fixture.AuthorityEvidence(r.Context())
				value.AccountID = "different-synthetic-account"
				_ = json.NewEncoder(w).Encode(value)
			}))
			server.Listener = listener
			server.Start()
			defer server.Close()
			defer unblock()
			fixture.server = server
			cfg := chromeFixtureConfigFromEnv(t, server.URL)
			parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
			if err != nil {
				t.Fatal(err)
			}
			rootID, err := processFixtureDirectoryID(cfg.Root)
			if err != nil {
				t.Fatal(err)
			}
			lineage := processFixtureLineage{Root: cfg.Root, RootID: rootID, Nonce: nonce, Parent: parent}
			raw, err := json.Marshal(lineage)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(cfg.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			controller, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "aku-sidecar-dev.exe")
			child := startProcessFixtureChild(t, executable, processFixtureStart{
				Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: cfg.NodePath,
				ChromePath: cfg.ChromePath, DriverPath: cfg.DriverPath, Origin: cfg.Origin,
				AuthorityKey: nonce, ControllerImage: controller,
			})
			select {
			case <-entered:
			case <-time.After(30 * time.Second):
				t.Fatal("startup never requested authority")
			}
			started := time.Now()
			path := filepath.Join(cfg.Root, "fixture.sqlite")
			checkpoint := readDevelopmentPendingCheckpoint(t, path)
			pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
			if err != nil || len(pids) < 3 {
				t.Fatalf("expected owned Sidecar, Node and Chrome: %v %v", pids, err)
			}
			var handles []windows.Handle
			defer func() {
				for _, handle := range handles {
					_ = windows.CloseHandle(handle)
				}
			}()
			for _, pid := range pids {
				handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
				if err != nil {
					t.Fatal(err)
				}
				handles = append(handles, handle)
			}
			settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
			state, err := store.Open(path, settings)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			settings, err = state.GetSettings(ctx)
			if err != nil {
				state.Close()
				t.Fatal(err)
			}
			session, err := state.CreateUpdateSession(ctx, "synthetic startup failure", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				state.Close()
				t.Fatal(err)
			}
			run, err := state.AdvanceSession(ctx, session.ID)
			if err != nil || run == nil {
				state.Close()
				t.Fatalf("seed run: %v", err)
			}
			payload := capturePayload(*run, session.ID, settings, 1, nil, "")
			payload["captureRuntime"] = map[string]any{"driver": "headless", "epoch": "synthetic-unclaimed-owner", "generation": 1}
			payload["captureCollector"] = collectorStamp("headless")
			command, err := state.StartRun(ctx, run.ID, payload)
			if err != nil {
				state.Close()
				t.Fatal(err)
			}
			if claimed, err := state.ClaimCommandForCollector(ctx, run.ID, "before-failure", "headless"); err != nil || claimed != nil {
				state.Close()
				t.Fatal("pending checkpoint admitted command", err)
			}
			if err := state.Close(); err != nil {
				t.Fatal(err)
			}
			h := &syntheticSidecarHarness{ctx: ctx, path: path}
			before := readDevelopmentQueuedCommandSnapshot(t, h, command.ID)
			if before.status != "queued" || before.claimedAt.Valid || before.claimedBy.Valid {
				t.Fatalf("seed not queued: %+v", before)
			}
			if mode == "parent_death" {
				child.stop(t, true, executable, nil)
			} else {
				if mode == "account_changed" {
					unblock()
				}
				select {
				case exitErr := <-child.exited:
					if exitErr == nil {
						t.Fatal("failed verification exited successfully")
					}
				case <-time.After(15 * time.Second):
					t.Fatal("failed startup did not exit within deadline")
				}
				// Wait for the stdout reader before inspecting its diagnostic buffer.
				for reply := range child.replies {
					t.Fatalf("unexpected successful startup receipt: %s", reply.Stage)
				}
				if !strings.Contains(child.diagnostics.String(), "fixture binder:") {
					t.Fatalf("failure was not verifier rejection: %s", child.diagnostics.String())
				}
				if mode == "authority_timeout" {
					// The binding's two-second deadline intentionally wins over the
					// HTTP client's five-second timeout. No server response is sent.
					select {
					case <-canceled:
					case <-time.After(time.Second):
						t.Fatal("verifier did not cancel stalled authority request")
					}
					if time.Since(started) < time.Second {
						t.Fatal("stalled probe failed before exercising its deadline")
					}
				}
			}
			waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := processFixtureWait(waitCtx, func() bool {
				ids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
				return err == nil && len(ids) == 0
			}); err != nil {
				t.Fatal("startup failure left owned tree alive")
			}
			for _, handle := range handles {
				if result, err := windows.WaitForSingleObject(handle, 3000); err != nil || result != windows.WAIT_OBJECT_0 {
					t.Fatal("owned process handle not signalled", err)
				}
			}
			if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
				t.Fatal("unexpected login/capture count")
			}
			if after := readDevelopmentPendingCheckpoint(t, path); after != checkpoint {
				t.Fatal("failed verifier changed expected account checkpoint")
			}
			for i := 0; i < 2; i++ {
				cold, err := store.Open(path, settings)
				if err != nil {
					t.Fatal(err)
				}
				claimed, claimErr := cold.ClaimCommandForCollector(ctx, run.ID, "cold-open", "headless")
				closeErr := cold.Close()
				if claimErr != nil || closeErr != nil || claimed != nil {
					t.Fatal("cold-open admitted queued command", claimErr, closeErr)
				}
				if after := readDevelopmentQueuedCommandSnapshot(t, h, command.ID); after != before {
					t.Fatal("failure/cold-open mutated queued payload or claim state")
				}
			}
			t.Logf("mode=%s pendingCheckpointBeforeFault=true verifierRejectedOrKilled=true ownedProcessesSignalled=%d wholeTreeEmpty=true loginCount=1 captures=0 sameDatabaseColdOpens=2 queueUnchanged=true expectedAccountUnchanged=true", mode, len(handles))
		})
	}
}

func readDevelopmentPendingCheckpoint(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var raw, fence string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='synthetic_account_checkpoint'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='synthetic_account_checkpoint_fence'`).Scan(&fence); err != nil {
		t.Fatal(err)
	}
	var value store.SyntheticAccountCheckpoint
	var guard struct {
		State string `json:"state"`
	}
	if json.Unmarshal([]byte(raw), &value) != nil || json.Unmarshal([]byte(fence), &guard) != nil || guard.State != "pending" || value.OwnerEpoch != "" || value.OwnerPID != 0 || value.VerificationTime != "" || len(value.AcceptedRevisions) != 0 || len(value.ExpectedAccounts) != 2 {
		t.Fatal("checkpoint was not pending and expected-only")
	}
	for _, identity := range value.ExpectedAccounts {
		if identity.AccountID != "alpha" {
			t.Fatal("unexpected enrolled account")
		}
	}
	return raw
}
