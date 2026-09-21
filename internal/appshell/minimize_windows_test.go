//go:build windows

package appshell

import (
	"context"
	"testing"
)

func TestCaptureMinimizeRejectsMissingOwnershipBeforeWindowAccess(t *testing.T) {
	owner := processOwnership{}
	if err := owner.minimizeInitialWindow(context.Background(), 123); err == nil {
		t.Fatal("minimize accepted an unowned process")
	}
}
