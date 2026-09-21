//go:build !windows

package appshell

import "context"

// Never invoked on non-Windows platforms.
func (*processOwnership) minimizeInitialWindow(context.Context, int) error { return nil }
