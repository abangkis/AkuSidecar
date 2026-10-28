package collection

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

// ValidatePersistentBrowser binds ordinary interaction to the same retained
// owner that the Engine knows. Matching generation numbers alone are insufficient
// because independent managers may both start at generation one.
func (c *Coordinator) ValidatePersistentBrowser(owner *captureruntime.Manager) error {
	if owner == nil || c.owner != owner || !owner.Persistent() {
		return errors.New("persistent Browser action owner is unavailable")
	}
	snapshot := owner.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "browser" {
		return errors.New("persistent Browser action owner is not ready")
	}
	return nil
}

// BorrowPersistentBrowser reuses the existing interaction/reader intent and
// lease lifecycle. It grants no capture, account proof or session derivation.
func (c *Coordinator) BorrowPersistentBrowser(ctx context.Context, owner *captureruntime.Manager, nativeReader bool) (*captureruntime.Lease, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("persistent Browser action context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := c.ValidatePersistentBrowser(owner); err != nil {
		return nil, nil, err
	}
	borrow := c.BorrowBrowser
	if nativeReader {
		borrow = c.BorrowNativeReader
	}
	lease, release, err := borrow(ctx)
	if err != nil {
		return nil, nil, err
	}
	if lease == nil || lease.Driver() != "browser" || !owner.Accepts(lease.Generation()) {
		if lease != nil {
			lease.Release()
		}
		if release != nil {
			release()
		}
		return nil, nil, errors.New("persistent Browser action ownership changed")
	}
	return lease, release, nil
}
