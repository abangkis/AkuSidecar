//go:build windows && auth_session_dev

package engine

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"golang.org/x/sys/windows"
)

const dailySyntheticDriver = `
import {createInterface} from 'node:readline';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
let scope,reader,collector,generation=0;
const launch=()=>spawn(scope.chromePath,['-test.run=^TestXApplicationFakeBrowser$'],{windowsHide:true,stdio:'ignore',env:{...process.env,AKU_X_APPLICATION_FAKE_BROWSER:'1'}});
for await(const line of createInterface({input:process.stdin})){
 const q=JSON.parse(line);let result;
 if(q.op==='daily_owner_bootstrap'){scope=q;console.log(JSON.stringify({ready:true,schema:q.schema,nonce:q.nonce}));continue;}
 if(q.op==='daily_reader_initialize'){reader=launch();result={owner:1,readerPID:reader.pid};}
 else if(q.op==='daily_reader_check')result={readerPID:reader.pid,readerProfile:scope.readerProfile};
 else if(q.op==='daily_batch_begin'){generation=q.generation;collector=launch();scope.collectorProfile=q.collectorProfile;result={owner:generation+1,readerPID:reader.pid,collectorPID:collector.pid};}
 else if(q.op==='daily_batch_seed')result={accountId:'1908123456789012345',readerPID:reader.pid,readerProfile:scope.readerProfile,collectorPID:collector.pid,collectorProfile:scope.collectorProfile,seeded:true,homeReady:true};
 else if(q.op==='daily_batch_end'){if(collector&&collector.exitCode===null){const ended=once(collector,'exit');collector.kill();await ended;}result={closed:true,cleanup:{readerControls:'verified',collectorControls:'verified',collector:'verified',enrollment:'verified',attachments:'verified',bootstrap:'not_needed'}};}
 else {console.log(JSON.stringify({id:q.id,ok:false,code:'manual_inspection_failed'}));continue;}
 console.log(JSON.stringify({id:q.id,ok:true,result}));
}
`

func dailySyntheticConfig(t *testing.T) xApplicationOwnerConfig {
	t.Helper()
	cfg := applicationConfig(t, "normal")
	if err := os.WriteFile(cfg.driver.path, []byte(dailySyntheticDriver), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.driver = applicationAsset(t, cfg.driver.path)
	return cfg
}

func TestXDailyNativeCleanupUncertaintyStillReleasesExitedCollector(t *testing.T) {
	for _, failure := range []string{"controls", "bootstrap", "contradictory_receipt"} {
		t.Run(failure, func(t *testing.T) {
			cfg := dailySyntheticConfig(t)
			source := strings.Replace(dailySyntheticDriver, "closed:true,cleanup:{readerControls:'verified'", "closed:false,cleanup:{readerControls:'uncertain'", 1)
			wantReader, wantBootstrap := "uncertain", "not_needed"
			if failure == "bootstrap" {
				source = strings.Replace(dailySyntheticDriver, "closed:true,cleanup:", "closed:false,bootstrapDiagnostic:{stage:'absence',condition:'timeout'},cleanup:", 1)
				source = strings.Replace(source, "bootstrap:'not_needed'", "bootstrap:'uncertain'", 1)
				wantReader, wantBootstrap = "verified", "uncertain"
			}
			if failure == "contradictory_receipt" {
				source = strings.Replace(dailySyntheticDriver, "closed:true,cleanup:", "closed:true,bootstrapDiagnostic:{stage:'absence',condition:'timeout'},cleanup:", 1)
				wantReader, wantBootstrap = "verified", "not_needed"
			}
			if source == dailySyntheticDriver {
				t.Fatal("synthetic cleanup failure not injected")
			}
			if err := os.WriteFile(cfg.driver.path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			cfg.driver = applicationAsset(t, cfg.driver.path)
			e, state := testEngine(t)
			e.config.Dev = true
			e.config.Capture.XMode = config.XCaptureConcurrent
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			owner, err := newXDailyNativeOwner(ctx, ctx, cfg, e, nil)
			if err != nil {
				t.Fatal(err)
			}
			var closeChecked bool
			t.Cleanup(func() {
				cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				if err := owner.Close(cleanup); err != nil && !closeChecked {
					t.Error(err)
				}
			})
			settings, _ := state.GetSettings(ctx)
			settings.ActiveSources = []domain.Source{domain.SourceX}
			session, err := state.CreateUpdateSession(ctx, "synthetic cleanup separation", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			batch, startErr := owner.BeginBatch(ctx, ctx, 1, session.Runs[0])
			if startErr == nil || batch == nil {
				t.Fatal("fake transport manufactured startup authority or lost its partial batch")
			}
			cleanupErr := batch.Close(ctx)
			var diagnostic *xDailyCleanupError
			if !errors.As(cleanupErr, &diagnostic) || diagnostic.detail.Resources == nil || diagnostic.detail.Resources.ReaderControls != wantReader || diagnostic.detail.Resources.Bootstrap != wantBootstrap || diagnostic.detail.CollectorExit != "verified" || diagnostic.detail.PinRelease != "verified" {
				t.Fatalf("independent cleanup did not complete: %v", cleanupErr)
			}
			if _, err := os.Stat(batch.(*xDailyBatch).host.cfg.collector.Root); !os.IsNotExist(err) {
				t.Fatal("an exited collector profile was retained after unrelated control failure")
			}
			if err := owner.CheckReader(ctx); err != nil {
				t.Fatal("cleanup retired the process reader")
			}
			e.rememberXDailyFailure(startErr, "startup")
			e.rememberXDailyCleanup(cleanupErr)
			status := e.XCaptureStatus()
			if status.StartupDiagnostic == nil || status.CleanupDiagnostic == nil || status.FailurePhase != "startup" {
				t.Fatal("primary and cleanup diagnostics were not both retained")
			}
			if failure == "bootstrap" && (status.CleanupDiagnostic.Bootstrap == nil || status.CleanupDiagnostic.Bootstrap.Stage != "absence" || status.CleanupDiagnostic.Bootstrap.Condition != "timeout" || !strings.Contains(status.CleanupDiagnostic.String(), "bootstrap_condition=timeout")) {
				t.Fatal("bootstrap failure was lost between the Node receipt and status/log projection")
			}
			if failure == "contradictory_receipt" && (status.CleanupDiagnostic.BatchEnd != "uncertain" || status.CleanupDiagnostic.Condition != "receipt") {
				t.Fatal("contradictory bootstrap receipt manufactured cleanup proof")
			}
			closeErr := owner.Close(ctx)
			closeChecked = true
			var closedDetail *xDailyCleanupError
			if !errors.As(closeErr, &closedDetail) || closedDetail.detail.Resources.ReaderControls != wantReader || closedDetail.detail.Resources.Bootstrap != wantBootstrap {
				t.Fatal("owner shutdown erased prior batch cleanup uncertainty")
			}
		})
	}
}

func TestXDailyNativeOwnerExplicitResourcesAndScopedFailureCleanup(t *testing.T) {
	cfg := dailySyntheticConfig(t)
	e, state := testEngine(t)
	e.config.Dev = true
	e.config.Capture.XMode = config.XCaptureConcurrent
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, stop := context.WithTimeout(parent, 15*time.Second)
	defer stop()
	owner, err := newXDailyNativeOwner(parent, ctx, cfg, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := owner.(*xDailyOwner)
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if err := owner.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	readerPID := n.reader.chrome.PID
	if err := owner.CheckReader(ctx); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := state.CreateUpdateSession(ctx, "synthetic daily owner", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	// The fake transport deliberately cannot manufacture startup evidence. It
	// still exercises owned process pins, batch creation, seed receipt and cleanup.
	batch, err := owner.BeginBatch(ctx, parent, 1, session.Runs[0])
	if err == nil || batch == nil {
		t.Fatal("synthetic transport qualified or partial ownership lost")
	}
	diagnostic := xDailyStartupDetail(err)
	if diagnostic == nil || diagnostic.Stage != "broker_start" || diagnostic.Condition != "eligibility" || diagnostic.Origin == "" || diagnostic.BrokerReason == "" {
		t.Fatalf("broker startup failure lost domain diagnostic: %+v", diagnostic)
	}
	if err = batch.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(batch.(*xDailyBatch).host.cfg.collector.Root); !os.IsNotExist(err) {
		t.Fatal("disposable collector profile retained after verified cleanup")
	}
	if n.reader.chrome.PID != readerPID || owner.CheckReader(ctx) != nil {
		t.Fatal("batch cleanup retired process reader")
	}
	if state, err := windows.WaitForSingleObject(n.reader.chromeHandle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("reader exited during batch cleanup")
	}
	if _, err = owner.BeginBatch(ctx, parent, 2, session.Runs[0]); err == nil {
		t.Fatal("failed batch silently retried")
	}
}

func TestXDailyNativeBeginBatchRejectsWithExactPredicate(t *testing.T) {
	for _, tc := range []struct {
		name, old, replacement, stage, condition string
	}{
		{"begin-owner", "result={owner:generation+1,readerPID:reader.pid,collectorPID:collector.pid}", "result={owner:0,readerPID:reader.pid,collectorPID:collector.pid}", "batch_begin_reply", "owner"},
		{"begin-shape", "result={owner:generation+1,readerPID:reader.pid,collectorPID:collector.pid}", "result={owner:'invalid',readerPID:reader.pid,collectorPID:collector.pid}", "batch_begin_rpc", "decode"},
		{"seed-home", "seeded:true,homeReady:true", "seeded:true,homeReady:false", "seed_receipt", "home_ready"},
		{"seed-profile", "readerProfile:scope.readerProfile,collectorPID:collector.pid", "readerProfile:'private-invalid-profile',collectorPID:collector.pid", "seed_receipt", "reader_profile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := dailySyntheticConfig(t)
			changed := strings.Replace(dailySyntheticDriver, tc.old, tc.replacement, 1)
			if changed == dailySyntheticDriver {
				t.Fatal("synthetic fault not injected")
			}
			if err := os.WriteFile(cfg.driver.path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			cfg.driver = applicationAsset(t, cfg.driver.path)
			e, _ := testEngine(t)
			e.config.Dev = true
			e.config.Capture.XMode = config.XCaptureConcurrent
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			owner, err := newXDailyNativeOwner(context.Background(), ctx, cfg, e, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				if err := owner.Close(cleanup); err != nil {
					t.Error(err)
				}
			}()
			batch, err := owner.BeginBatch(ctx, context.Background(), 1, domain.Run{ID: "synthetic-run", SessionID: "synthetic-session", Source: domain.SourceX})
			d := xDailyStartupDetail(err)
			wantCause := "rejected"
			if tc.condition == "decode" {
				wantCause = "unavailable"
			}
			if d == nil || d.Stage != tc.stage || d.Condition != tc.condition || d.Cause != wantCause || batch == nil {
				t.Fatalf("failed predicate not retained: %+v", d)
			}
			if err := batch.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if owner.CheckReader(ctx) != nil {
				t.Fatal("failed collector retired reader")
			}
		})
	}
}

func TestXDailyNativeBeginBatchAdmissionDiagnostic(t *testing.T) {
	n := &xDailyOwner{}
	_, err := n.BeginBatch(context.Background(), context.Background(), 1, domain.Run{ID: "run", SessionID: "session", Source: domain.SourceLinkedIn})
	if d := xDailyStartupDetail(err); d == nil || d.Stage != "admission" || d.Condition != "source" {
		t.Fatal("admission predicate was not reported before native work")
	}
}

func TestXDailyPostOriginDoesNotWeakenManualEntry(t *testing.T) {
	f := &applicationPostFixture{now: time.Now(), pid: 123}
	marker := strings.Replace(applicationPostMarker(), ":12345", ":11122", 1)
	if _, err := prepareXApplicationPost(context.Background(), applicationPostRequest(), marker, f.ports()); err == nil {
		t.Fatal("manual path admitted main origin")
	}
	p := f.ports()
	p.allowMainOrigin = true
	prepared, err := prepareXApplicationPost(context.Background(), applicationPostRequest(), marker, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = prepared.VerifyForeground(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = prepared.NavigatePost(context.Background()); err != nil {
		t.Fatal(err)
	}
}
