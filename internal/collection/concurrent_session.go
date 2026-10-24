//go:build auth_session_dev && auth_session_fixture

package collection

import (
	"context"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

// BindConcurrentSession binds only invented fixture owners before Start. The
// collector remains fixed and the reader uses a distinct Manager. No native
// process/provider constructor or production activation path is supplied here.
func (c *Coordinator) BindConcurrentSession(reader *captureruntime.Manager, generation uint64, backend CaptureBackend, beforeReader, beforeCapture func(context.Context) error) error {
	return c.bindManualConcurrentSession(reader, generation, backend, beforeReader, beforeCapture)
}
