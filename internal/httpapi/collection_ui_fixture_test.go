package httpapi

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

type collectionUIFixtureProcess struct {
	splitLeaseProcess
	mode string
}

func (p *collectionUIFixtureProcess) Driver() string { return p.mode }

type collectionUIFixtureCollector struct{}

func (collectionUIFixtureCollector) Capture(context.Context, domain.Source, map[string]any) (domain.Observation, error) {
	return domain.Observation{}, errors.New("rendered UI fixture cannot collect source data")
}

// This opt-in server exercises the real UI, Settings API and coordinator using
// fake capture processes. It cannot establish Chrome handoff, authentication or
// source parity. Its isolated store and loopback listener never use user data.
func TestCollectionSettingsRenderedFixture(t *testing.T) {
	fixtureRoot := os.Getenv("AKU_COLLECTION_UI_FIXTURE")
	if fixtureRoot == "" {
		t.Skip("set AKU_COLLECTION_UI_FIXTURE to a disposable project-local directory")
	}
	if !filepath.IsAbs(fixtureRoot) {
		t.Fatal("fixture directory must be absolute")
	}
	if err := os.MkdirAll(fixtureRoot, 0700); err != nil {
		t.Fatal(err)
	}
	s, _ := splitTestServer(t)
	settings, err := s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceFacebook}
	settings.CalibrationEnabled = false
	settings.ReasoningProvider = "deterministic"
	if err := s.store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	s.engine.RecordHeartbeat(engine.ExpectedHeartbeat())
	process := func(mode string) *collectionUIFixtureProcess {
		return &collectionUIFixtureProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, mode: mode}
	}
	owner, err := captureruntime.New(process("browser"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.engine.AttachCaptureRuntime(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	var coordinator *collection.Coordinator
	coordinator = collection.NewCoordinator(owner, func(_ context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		if mode == "browser" {
			coordinator.SetBrowserCollector(generation, collectionUIFixtureCollector{})
		}
		return process(mode), nil
	}, func() error { return nil })
	coordinator.SetBrowserCollector(owner.Snapshot().Generation, collectionUIFixtureCollector{})
	s.engine.AttachCollectionCoordinator(coordinator)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer owner.Terminate()
	coordinator.Start(ctx)
	s.http.Addr = net.JoinHostPort("127.0.0.1", "0")
	address, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := s.Stop(stopCtx); err != nil {
			t.Errorf("stop fixture: %v", err)
		}
	}()
	if err := os.WriteFile(filepath.Join(fixtureRoot, "url.txt"), []byte("http://"+address.String()), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("rendered fixture timed out without stop.txt")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(fixtureRoot, "stop.txt")); err == nil {
				persisted, err := s.store.GetSettings(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				allowed := map[domain.Source]bool{}
				for _, source := range persisted.ActiveSources {
					allowed[source] = true
				}
				if persisted.CollectionMode != "headless" || persisted.CaptureVisibility != "quiet" || len(persisted.ActiveSources) != 2 || !allowed[domain.SourceX] || !allowed[domain.SourceFacebook] {
					t.Fatalf("rendered journey did not preserve supported settings: mode=%s visibility=%s sources=%v", persisted.CollectionMode, persisted.CaptureVisibility, persisted.ActiveSources)
				}
				status := coordinator.Status()
				if status.Effective != "headless" || status.Pending || status.Generation < 4 {
					t.Fatalf("rendered switching journey incomplete: %+v", status)
				}
				return
			}
		}
	}
}
