//go:build auth_session_dev

package collection

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

// BindManualConcurrentSession binds a fixed collector and a separate reader.
// Admission comes from the private manual broker; this supplies no native
// backend, credential transfer, process launch or ordinary-build constructor.
func (c *Coordinator) BindManualConcurrentSession(reader *captureruntime.Manager, generation uint64, backend CaptureBackend, beforeReader, beforeCapture func(context.Context) error) error {
	return c.bindManualConcurrentSession(reader, generation, backend, beforeReader, beforeCapture)
}

func (c *Coordinator) bindManualConcurrentSession(reader *captureruntime.Manager, generation uint64, backend CaptureBackend, beforeReader, beforeCapture func(context.Context) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fixtureHooks != nil || c.developmentSession || c.concurrentSession != nil || reader == nil || reader == c.owner || backend == nil || beforeReader == nil || beforeCapture == nil {
		return errors.New("invalid manual concurrent binding")
	}
	collectorState, readerState := c.owner.Snapshot(), reader.Snapshot()
	if collectorState.State != captureruntime.Ready || collectorState.Driver != "headless" || collectorState.Generation != generation || readerState.State != captureruntime.Ready || readerState.Driver != "browser" || c.owner.PID() <= 0 || reader.PID() <= 0 || c.owner.PID() == reader.PID() {
		return errors.New("manual concurrent owner unavailable")
	}
	c.concurrentSession = &syntheticConcurrentHooks{reader, beforeReader, beforeCapture}
	c.fixtureHeadlessGeneration, c.fixtureHeadless = generation, backend
	c.requested = "headless"
	return nil
}
