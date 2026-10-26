//go:build windows && auth_session_dev

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"golang.org/x/sys/windows"
)

const applicationOwnerTestDriver = `
import {createInterface} from 'node:readline';
import {spawn} from 'node:child_process';
const mode=MODE;
let children=[];
for await(const line of createInterface({input:process.stdin})){
 const q=JSON.parse(line);
 if(q.op==='native_owner_bootstrap'){
  if(mode==='stall')continue;
  if(mode==='bad-nonce'){console.log(JSON.stringify({ready:true,schema:q.schema,nonce:'wrong'}));continue;}
  if(mode==='extra'){console.log(JSON.stringify({ready:true,schema:q.schema,nonce:q.nonce,canAuthorize:true}));continue;}
  if(mode==='duplicate'){console.log('{'+'"ready":true,"schema":'+JSON.stringify(q.schema)+',"nonce":"wrong","nonce":'+JSON.stringify(q.nonce)+'}');continue;}
  if(mode==='pair')children=[1,2].map(()=>spawn(q.chromePath,['-test.run=^TestXApplicationFakeBrowser$'],{windowsHide:true,stdio:'ignore',env:{...process.env,AKU_X_APPLICATION_FAKE_BROWSER:'1'}}));
  console.log(JSON.stringify({ready:true,schema:q.schema,nonce:q.nonce}));continue;
 }
 if(mode==='invalid-wire'){console.log('{not-json');continue;}
 if(mode==='failure'){console.log(JSON.stringify({id:q.id,ok:false,code:'manual_inspection_failed',diagnostic:{stage:'synthetic'}}));continue;}
 console.log(JSON.stringify({id:q.id,ok:true,result:children.length?{readerPID:children[0].pid,collectorPID:children[1].pid}:{closed:true}}));
}
`

func applicationAsset(t *testing.T, path string) xApplicationOwnerAsset {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("synthetic asset unavailable")
	}
	sum := sha256.Sum256(data)
	return xApplicationOwnerAsset{path: path, digest: hex.EncodeToString(sum[:])}
}
func applicationConfig(t *testing.T, mode string) xApplicationOwnerConfig {
	t.Helper()
	if os.Getenv("AKU_X_APPLICATION_OWNER_OFFLINE") != "1" {
		t.Skip("canonical accountless application owner helper required")
	}
	root := t.TempDir()
	descriptor := func(role string) ownedprofile.Descriptor {
		r := filepath.Join(root, role)
		p := filepath.Join(r, "profile")
		if os.MkdirAll(p, 0700) != nil {
			t.Fatal("synthetic profile setup failed")
		}
		rID, err := ownedprocess.DirectoryID(r)
		if err != nil {
			t.Fatal("synthetic root pin unavailable")
		}
		pID, err := ownedprocess.DirectoryID(p)
		if err != nil {
			t.Fatal("synthetic profile pin unavailable")
		}
		return ownedprofile.Descriptor{Root: r, Directory: p, RootID: rID, DirectoryID: pID}
	}
	encoded, _ := json.Marshal(mode)
	driver := filepath.Join(root, "owner.mjs")
	if os.WriteFile(driver, []byte(strings.Replace(applicationOwnerTestDriver, "MODE", string(encoded), 1)), 0600) != nil {
		t.Fatal("synthetic driver setup failed")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal("synthetic process image unavailable")
	}
	return xApplicationOwnerConfig{enabled: true, node: applicationAsset(t, os.Getenv("AKU_CHROME_FIXTURE_NODE")), chrome: applicationAsset(t, exe), driver: applicationAsset(t, driver), reader: descriptor("reader"), collector: descriptor("collector")}
}
func closeApplication(t *testing.T, b *xApplicationOwner) {
	t.Helper()
	if b == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup+time.Second)
	defer cancel()
	if b.Close(ctx) != nil {
		t.Error("synthetic owner cleanup unverified")
	}
}

func TestXApplicationDisabledBeforeAnyIO(t *testing.T) {
	b, err := startXApplicationOwner(context.Background(), context.Background(), xApplicationOwnerConfig{})
	if b != nil || err == nil || err.Error() != "application owner disabled" {
		t.Fatal("default construction did not reject before IO")
	}
}

func TestXApplicationWorkerDoesNotInheritNodePreload(t *testing.T) {
	cfg := applicationConfig(t, "normal")
	marker := filepath.Join(cfg.collector.Root, "untrusted-preload-ran")
	preload := filepath.Join(cfg.collector.Root, "untrusted.cjs")
	encoded, _ := json.Marshal(marker)
	if os.WriteFile(preload, []byte("require('node:fs').writeFileSync("+string(encoded)+",'synthetic');"), 0600) != nil {
		t.Fatal("synthetic preload setup failed")
	}
	t.Setenv("NODE_OPTIONS", "--require=\""+filepath.ToSlash(preload)+"\"")
	t.Setenv("NODE_PATH", cfg.collector.Root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, err := startXApplicationOwner(ctx, ctx, cfg)
	if err != nil {
		t.Fatal("isolated worker failed to start")
	}
	defer closeApplication(t, b)
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("worker inherited untrusted startup code")
	}
	for _, entry := range xApplicationOwnerEnvironment(cfg.collector.Root) {
		if strings.HasPrefix(entry, "NODE_") {
			t.Fatal("worker startup override leaked")
		}
	}
}

func TestXApplicationStartupRequestDoesNotOwnReaderLifetime(t *testing.T) {
	cfg := applicationConfig(t, "normal")
	parent, stopParent := context.WithCancel(context.Background())
	defer stopParent()
	request, cancelRequest := context.WithCancel(parent)
	b, err := startXApplicationOwner(parent, request, cfg)
	if err != nil {
		cancelRequest()
		t.Fatal("synthetic owner failed to start")
	}
	defer closeApplication(t, b)
	cancelRequest()
	var result struct {
		Closed bool `json:"closed"`
	}
	if b.isClosed() || b.rpcRoundTripObserved(parent, "manual_inspect", nil, &result, nil) != nil || !result.Closed {
		t.Fatal("completed startup request retired live owner")
	}
	stopParent()
	closeApplication(t, b)
	if !b.isClosed() {
		t.Fatal("owner survived its parent lifetime")
	}
}
func TestXApplicationRejectsAssetAndLeaseDrift(t *testing.T) {
	for _, fault := range []string{"asset", "lease", "overlap", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			cfg := applicationConfig(t, "normal")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "asset":
				cfg.driver.digest = strings.Repeat("0", 64)
			case "lease":
				cfg.reader.DirectoryID = "wrong"
			case "overlap":
				cfg.collector = cfg.reader
			case "canceled":
				cancel()
			}
			b, err := startXApplicationOwner(ctx, ctx, cfg)
			if b != nil {
				closeApplication(t, b)
			}
			if err == nil {
				t.Fatal("invalid construction admitted")
			}
			if _, err := os.Stat(filepath.Join(cfg.reader.Root, ".aku-x-manual.lock")); !os.IsNotExist(err) {
				t.Fatal("invalid construction reached profile lock")
			}
		})
	}
}
func TestXApplicationHandshakeFailureReleasesLeases(t *testing.T) {
	for _, fault := range []string{"bad-nonce", "extra", "duplicate", "stall"} {
		t.Run(fault, func(t *testing.T) {
			cfg := applicationConfig(t, fault)
			ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
			defer cancel()
			b, err := startXApplicationOwner(ctx, ctx, cfg)
			if b != nil {
				closeApplication(t, b)
			}
			if err == nil {
				t.Fatal("invalid handshake admitted")
			}
			for _, d := range []ownedprofile.Descriptor{cfg.reader, cfg.collector} {
				l, err := ownedprofile.Acquire(d)
				if err != nil {
					t.Fatal("failed startup leaked its profile lease")
				}
				l.Close()
			}
		})
	}
}
func TestXApplicationTransportScopeAndCleanup(t *testing.T) {
	cfg := applicationConfig(t, "normal")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, err := startXApplicationOwner(ctx, ctx, cfg)
	if err != nil {
		t.Fatal("synthetic owner did not start")
	}
	defer closeApplication(t, b)
	if !b.nativeManualScope(cfg.reader.Root, cfg.reader.Directory) || !b.nativeManualScope(cfg.collector.Root, cfg.collector.Directory) || b.nativeManualScope(cfg.reader.Root, cfg.collector.Directory) {
		t.Fatal("profile scope crossed roles")
	}
	if _, err := ownedprofile.Acquire(cfg.reader); err == nil {
		t.Fatal("active factory did not retain profile lease")
	}
	var reply struct {
		Closed bool `json:"closed"`
	}
	if b.rpcRoundTripObserved(ctx, "manual_inspect", nil, &reply, nil) != nil || !reply.Closed {
		t.Fatal("correlated private operation failed")
	}
	if b.rpcRoundTripObserved(ctx, "arbitrary_script", nil, nil, nil) == nil {
		t.Fatal("unapproved operation reached private pipe")
	}
	if strings.Contains(fmt.Sprintf("%#v %v", cfg, b), cfg.reader.Directory) {
		t.Fatal("owner formatting leaked a path")
	}
	cancel()
	closeApplication(t, b)
	if !b.isClosed() || b.nativeManualScope(cfg.reader.Root, cfg.reader.Directory) {
		t.Fatal("closed owner retained admission")
	}
	if l, err := ownedprofile.Acquire(cfg.reader); err != nil {
		t.Fatal("verified cleanup did not release profile lease")
	} else {
		l.Close()
	}
}
func TestXApplicationRPCFailureAndProtocolLoss(t *testing.T) {
	for _, mode := range []string{"failure", "invalid-wire"} {
		t.Run(mode, func(t *testing.T) {
			cfg := applicationConfig(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			b, err := startXApplicationOwner(ctx, ctx, cfg)
			if err != nil {
				t.Fatal("synthetic owner did not start")
			}
			defer closeApplication(t, b)
			err = b.rpcRoundTripObserved(ctx, "manual_inspect", nil, nil, nil)
			if err == nil {
				t.Fatal("failed operation accepted")
			}
			if mode == "failure" {
				d, ok := err.(interface{ diagnosticJSON() []byte })
				if !ok || string(d.diagnosticJSON()) != `{"stage":"synthetic"}` {
					t.Fatal("separate diagnostic lost")
				}
			}
		})
	}
}
func TestXApplicationRoleBindsRealOSFactsWithoutAccountAuthority(t *testing.T) {
	cfg := applicationConfig(t, "pair")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := startXApplicationOwner(ctx, ctx, cfg)
	if err != nil {
		t.Fatal("synthetic owner pair did not start")
	}
	defer closeApplication(t, b)
	var pids struct {
		ReaderPID    uint32 `json:"readerPID"`
		CollectorPID uint32 `json:"collectorPID"`
	}
	if b.rpcRoundTripObserved(ctx, "manual_inventory", nil, &pids, nil) != nil {
		t.Fatal("synthetic owner inventory unavailable")
	}
	if _, err = b.bindRole(ctx, "reader", uint32(os.Getpid()), 1); err == nil {
		t.Fatal("unowned process bound")
	}
	r, err := b.bindRole(ctx, "reader", pids.ReaderPID, 1)
	if err != nil {
		t.Fatal("reader OS facts rejected")
	}
	c, err := b.bindRole(ctx, "collector", pids.CollectorPID, 1)
	if err != nil {
		t.Fatal("collector OS facts rejected")
	}
	if _, err = b.bindRole(ctx, "reader", pids.CollectorPID, 1); err == nil {
		t.Fatal("duplicate or crossed role bound")
	}
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"reader": r, "collector": c}}
	if _, err = backend.snapshot(ctx, "collector"); err != nil {
		t.Fatal("live OS binding unavailable")
	}
	if err = b.nativeManualJob(func(job windows.Handle) error {
		members, err := ownedprocess.JobPIDs(job)
		if err != nil {
			return err
		}
		present := map[uint32]bool{}
		for _, pid := range members {
			present[pid] = true
		}
		if !present[b.nativeManualNodePID()] || !present[pids.ReaderPID] || !present[pids.CollectorPID] || present[uint32(os.Getpid())] {
			return fmt.Errorf("synthetic process membership mismatch")
		}
		return nil
	}); err != nil {
		t.Fatal("owned tree membership unavailable")
	}
	closeApplication(t, b)
	if _, err = backend.snapshot(ctx, "collector"); err == nil {
		t.Fatal("retired owner accepted")
	}
}
func TestXApplicationFakeBrowser(t *testing.T) {
	if os.Getenv("AKU_X_APPLICATION_FAKE_BROWSER") != "1" {
		t.Skip("private accountless process helper")
	}
	time.Sleep(20 * time.Second)
}
