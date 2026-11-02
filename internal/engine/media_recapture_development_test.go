//go:build auth_session_dev

package engine

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type recaptureReaderFixtureProcess struct{ *runtimeTestProcess }

func (*recaptureReaderFixtureProcess) PID() int { return 124 }

func TestEngineMediaRecaptureCharacterizationHoldFailureKeepsWaitingJob(t *testing.T) {
	f := newRecaptureEngineFixture(t, domain.SourceFacebook)
	manager := attachTestCapture(t, f.e)
	if err := manager.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) {
		return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	reader, err := captureruntime.New(&recaptureReaderFixtureProcess{&runtimeTestProcess{done: make(chan error, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Terminate)
	coordinator := collection.NewCoordinator(manager, nil, func() error { return nil })
	// The existing opt-in concurrent binding rejects a Browser collection hold.
	// No production hook or admission exception is introduced for this test.
	if err := coordinator.BindManualConcurrentSession(reader, manager.Snapshot().Generation, failingQuietBackend{}, func(context.Context) error { return nil }, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	f.e.AttachCollectionCoordinator(coordinator)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "headless"
	if err := f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	job := f.queue(t)
	admission, marked, err := store.MediaRecaptureAdmission(job.Payload)
	if err != nil || !marked || admission.Phase != "waiting" || job.Payload["captureRuntime"] != nil {
		t.Fatalf("hold rejection changed durable admission: %+v %v", job, err)
	}
	stored, err := f.e.MediaRecapture(context.Background(), job.ID)
	if err != nil || stored.Status != "queued" || f.count(t, "jobs") != 1 || manager.Snapshot().ActiveLeases != 0 || len(f.e.collectionIntents) != 0 {
		t.Fatalf("failed hold rolled back/activated the job: %+v %v", stored, err)
	}
	if claimed, err := f.e.ClaimMediaRecapture(context.Background(), job.ID, "early-bridge"); err != nil || claimed.ID != "" {
		t.Fatalf("waiting job became claimable: %+v %v", claimed, err)
	}
}
