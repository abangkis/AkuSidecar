//go:build windows && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

func TestProcessFixtureHeadedLauncherBeforeAssignment(t *testing.T) {
	testProcessFixtureHeadedLauncherBeforeAssignment(t, false)
}

func TestProcessFixtureHeadedLauncherStartupRejected(t *testing.T) {
	testProcessFixtureHeadedLauncherBeforeAssignment(t, true)
}

func testProcessFixtureHeadedLauncherBeforeAssignment(t *testing.T, reject bool) {
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
		t.Skip("requires canonical VerifyProcessRestart helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config := chromeFixtureConfigFromEnv(t, fixture.server.URL)
	executable := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "auth-session-process-fixture.exe")
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := processFixtureDirectoryID(config.Root)
	if err != nil {
		t.Fatal(err)
	}
	lineage := processFixtureLineage{Root: config.Root, RootID: rootID, Nonce: hex.EncodeToString(nonce[:]), Parent: parent}
	raw, _ := json.Marshal(lineage)
	if err = os.WriteFile(filepath.Join(config.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	barrier := "launcher_before_job"
	if reject {
		barrier = "launcher_abort_before_job"
	}
	child := startProcessFixtureChild(t, executable, processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, StartupBarrier: barrier})
	receipt := child.receive(t, "launcher_unassigned")
	if receipt.PID != child.identity.PID || !receipt.NodeMembershipKnown || receipt.LauncherRoot.PID == 0 || receipt.LauncherChild.PID != 0 || !receipt.LauncherLifetimeMember || !receipt.LauncherLifetimeNoInherit {
		t.Fatal("launcher barrier proof missing")
	}
	identities := []processFixtureIdentity{child.identity, receipt.LauncherRoot}
	handles := make([]windows.Handle, 0, len(identities))
	for index, identity := range identities {
		verifyProcessFixtureImage(t, identity, executable)
		rights := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE)
		if index == 0 {
			rights |= windows.PROCESS_TERMINATE
		}
		h, openErr := windows.OpenProcess(rights, false, identity.PID)
		if openErr != nil {
			t.Fatal(openErr)
		}
		handles = append(handles, h)
		member, membershipErr := chromeFixtureJobMembership(h, child.job)
		if membershipErr != nil || !member {
			t.Fatal("launcher peer outside controller containment")
		}
		if len(processFixtureWindows(identity.PID)) != 0 {
			t.Fatal("no-UI executable created a window")
		}
	}
	// Fallback runs only after the acceptance assertions. Pin-and-wait proof
	// also protects cleanup after a deliberately exposed failing baseline.
	t.Cleanup(func() {
		_ = windows.TerminateJobObject(child.job, 1)
		for _, h := range handles {
			status, waitErr := windows.WaitForSingleObject(h, 5000)
			if waitErr != nil || status != windows.WAIT_OBJECT_0 {
				processFixtureCleanupFailed.Store(true)
				t.Error("fallback did not release pinned synthetic peer")
			}
			windows.CloseHandle(h)
		}
	})
	for index, identity := range identities[1:] {
		actualParent, parentErr := readerbroker.ParentPID(identity.PID)
		if parentErr != nil || actualParent != identities[index].PID {
			t.Fatal("actual synthetic process parent mismatch")
		}
	}
	pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
	if err != nil || len(pids) != 2 {
		t.Fatal("outer Job must contain exactly Go+suspended root")
	}
	for _, pid := range pids {
		if pid != identities[0].PID && pid != identities[1].PID {
			t.Fatal("unexpected outer peer")
		}
	}
	for _, path := range []string{"shared", "fixture.sqlite", "process-recovery-consumed.json", "launcher-permit-consumed", "launcher-child.json"} {
		if _, statErr := os.Lstat(filepath.Join(config.Root, path)); !os.IsNotExist(statErr) {
			t.Fatal("unexpected profile/database/reopen work")
		}
	}
	if fixture.logins.Load() != 0 || fixture.sequence.Load() != 0 {
		t.Fatal("synthetic launcher performed authentication/capture")
	}
	t.Logf("launcher_barrier goPID=%d suspendedRootPID=%d rootCreated=%s actualLineage=true imageVerified=true pinnedHandles=true innerMembership=false lifetimeMembership=true lifetimeHandleNotInherited=true outerExactlyGoRoot=true rootNotExecuted=true windows=0 login=0 capture=0", identities[0].PID, identities[1].PID, identities[1].Created)
	if reject {
		if _, err = fmt.Fprintln(child.input, "abort"); err != nil {
			t.Fatal(err)
		}
		rejected := child.receive(t, "launcher_aborted")
		if !rejected.CleanupVerified || rejected.LauncherRoot != receipt.LauncherRoot {
			t.Fatal("startup rejection actual Wait proof unavailable")
		}
		rootStatus, rootErr := windows.WaitForSingleObject(handles[1], 0)
		goStatus, goErr := windows.WaitForSingleObject(handles[0], 0)
		pids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		if rootErr != nil || rootStatus != windows.WAIT_OBJECT_0 || goErr != nil || goStatus != uint32(windows.WAIT_TIMEOUT) || queryErr != nil || len(pids) != 1 || pids[0] != child.identity.PID {
			t.Fatal("rejected startup returned before root terminal or affected Go")
		}
		if _, statErr := os.Lstat(filepath.Join(config.Root, "launcher-permit-consumed")); !os.IsNotExist(statErr) {
			t.Fatal("rejected suspended root executed")
		}
		t.Log("startupRejected=true actualCmdWaitTerminal=true pinnedRootSignalled=true rootNeverExecuted=true innerJobEmpty=true GoAlive=true beforeFallback=true")
	}
	verifyProcessFixtureImage(t, child.identity, executable)
	if err = windows.TerminateProcess(handles[0], 76); err != nil {
		t.Fatal(err)
	}
	if status, waitErr := windows.WaitForSingleObject(handles[0], 3000); waitErr != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("actual Go root did not signal")
	}
	rootWait, rootErr := windows.WaitForSingleObject(handles[1], 3000)
	pids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
	t.Logf("launcher_parent_death goRootSignalled=true suspendedRootWait=%d outerPIDs=%v fallbackCleanupNotUsed=true", rootWait, pids)
	if rootErr != nil || rootWait != windows.WAIT_OBJECT_0 || queryErr != nil || len(pids) != 0 {
		t.Fatal("unassigned suspended launcher root survived Go death before fallback cleanup")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = processFixtureWait(ctx, func() bool {
		ids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		return queryErr == nil && len(ids) == 0
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-child.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("Go Wait did not complete")
	}
}

func TestProcessFixtureHeadedLauncherResumedTrees(t *testing.T) {
	for _, closeWindow := range []bool{false, true} {
		t.Run(fmt.Sprintf("close_window_%t", closeWindow), func(t *testing.T) {
			if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
				t.Skip("requires canonical VerifyProcessRestart helper")
			}
			fixture := startChromeFixtureHTTP(t)
			defer fixture.server.Close()
			config := chromeFixtureConfigFromEnv(t, fixture.server.URL)
			executable := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "auth-session-process-fixture.exe")
			var nonce [32]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				t.Fatal(err)
			}
			parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
			if err != nil {
				t.Fatal(err)
			}
			rootID, err := processFixtureDirectoryID(config.Root)
			if err != nil {
				t.Fatal(err)
			}
			lineage := processFixtureLineage{Root: config.Root, RootID: rootID, Nonce: hex.EncodeToString(nonce[:]), Parent: parent}
			raw, _ := json.Marshal(lineage)
			if err = os.WriteFile(filepath.Join(config.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			child := startProcessFixtureChild(t, executable, processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, StartupBarrier: "launcher_resumed_tree"})
			receipt := child.receive(t, "launcher_resumed")
			if receipt.PID != child.identity.PID || !receipt.LauncherTreeMembership || !receipt.LauncherLifetimeMember {
				t.Fatal("resumed two-tree ownership proof unavailable")
			}
			identities := []processFixtureIdentity{child.identity, receipt.LauncherRoot, receipt.LauncherChild, receipt.LauncherWorkerRoot, receipt.LauncherWorkerChild}
			var handles []windows.Handle
			t.Cleanup(func() {
				_ = windows.TerminateJobObject(child.job, 1)
				for _, h := range handles {
					status, err := windows.WaitForSingleObject(h, 5000)
					if err != nil || status != windows.WAIT_OBJECT_0 {
						processFixtureCleanupFailed.Store(true)
						t.Error("fallback synthetic peer cleanup failed")
					}
					windows.CloseHandle(h)
				}
			})
			for index, identity := range identities {
				verifyProcessFixtureImage(t, identity, executable)
				rights := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE)
				if index == 0 {
					rights |= windows.PROCESS_TERMINATE
				}
				h, err := windows.OpenProcess(rights, false, identity.PID)
				if err != nil {
					t.Fatal(err)
				}
				handles = append(handles, h)
				member, err := chromeFixtureJobMembership(h, child.job)
				if err != nil || !member || len(processFixtureWindows(identity.PID)) != 0 {
					t.Fatal("synthetic resumed peer escaped containment or made UI")
				}
				if index > 0 {
					wantParent := child.identity.PID
					if index == 2 || index == 4 {
						wantParent = identities[index-1].PID
					}
					actualParent, err := readerbroker.ParentPID(identity.PID)
					if err != nil || actualParent != wantParent {
						t.Fatal("resumed actual process lineage invalid")
					}
				}
			}
			pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
			if err != nil || len(pids) != 5 || fixture.logins.Load() != 0 || fixture.sequence.Load() != 0 {
				t.Fatal("resumed fixture must contain exactly five synthetic peers with no auth/capture")
			}
			t.Logf("resumed_trees goPID=%d launchRootPID=%d launchChildPID=%d workerRootPID=%d workerChildPID=%d actualLineage=true imagesVerified=true pinnedHandles=true bothInnerAndLifetimeMembership=true windows=0 login=0 capture=0", identities[0].PID, identities[1].PID, identities[2].PID, identities[3].PID, identities[4].PID)
			if closeWindow {
				if _, err = fmt.Fprintln(child.input, "close-window"); err != nil {
					t.Fatal(err)
				}
				isolated := child.receive(t, "launcher_isolated")
				if !isolated.LauncherIsolationVerified {
					t.Fatal("CloseForRetry isolation missing")
				}
				for index, h := range handles {
					want := uint32(windows.WAIT_TIMEOUT)
					if index == 1 || index == 2 {
						want = windows.WAIT_OBJECT_0
					}
					status, err := windows.WaitForSingleObject(h, 0)
					if err != nil || status != want {
						t.Fatalf("CloseForRetry peer %d wait=%d want=%d", index, status, want)
					}
				}
				pids, err = (&chromeFixtureBridge{job: child.job}).jobPIDs()
				if err != nil || len(pids) != 3 {
					t.Fatal("window close did not retain exactly Go+worker+child")
				}
				t.Log("close_for_retry=true launchRootAndChildSignalled=true goAndSiblingTreeAlive=true innerEmptyVerified=true")
			}
			if err = windows.TerminateProcess(handles[0], 76); err != nil {
				t.Fatal(err)
			}
			for index, h := range handles {
				status, err := windows.WaitForSingleObject(h, 3000)
				if err != nil || status != windows.WAIT_OBJECT_0 {
					t.Fatalf("Go-death peer %d did not signal: %d", index, status)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err = processFixtureWait(ctx, func() bool {
				pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
				return err == nil && len(pids) == 0
			}); err != nil {
				t.Fatal(err)
			}
			t.Log("onlyGoTerminated=true allPinnedPeersSignalled=true outerJobEmpty=true fallbackCleanupNotUsed=true")
		})
	}
}
