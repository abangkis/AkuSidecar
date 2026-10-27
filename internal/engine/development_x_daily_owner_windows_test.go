//go:build windows && auth_session_dev

package engine

import (
	"context"
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
 else if(q.op==='daily_batch_end'){if(collector&&collector.exitCode===null){const ended=once(collector,'exit');collector.kill();await ended;}result={closed:true};}
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
