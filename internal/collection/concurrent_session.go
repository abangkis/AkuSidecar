//go:build auth_session_dev && auth_session_fixture

package collection

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

// BindConcurrentSession binds only invented fixture owners before Start. The
// collector remains fixed and the reader uses a distinct Manager. No native
// process/provider constructor or production activation path is supplied here.
func (c *Coordinator) BindConcurrentSession(reader *captureruntime.Manager, generation uint64, backend CaptureBackend, beforeReader, beforeCapture func(context.Context) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fixtureHooks != nil || c.developmentSession || c.concurrentSession != nil || reader == nil || reader == c.owner || backend == nil || beforeReader == nil || beforeCapture == nil {
		return errors.New("invalid synthetic concurrent binding")
	}
	collectorState, readerState := c.owner.Snapshot(), reader.Snapshot()
	if collectorState.State != captureruntime.Ready || collectorState.Driver != "headless" || collectorState.Generation != generation || readerState.State != captureruntime.Ready || readerState.Driver != "browser" || c.owner.PID() == reader.PID() {
		return errors.New("synthetic concurrent owner unavailable")
	}
	c.concurrentSession = &syntheticConcurrentHooks{reader, beforeReader, beforeCapture}
	c.fixtureHeadlessGeneration, c.fixtureHeadless = generation, backend
	c.requested = "headless"
	return nil
}
