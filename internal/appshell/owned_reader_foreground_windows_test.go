//go:build windows && auth_session_dev

package appshell

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Fake HWND probes only: no native window, browser or foreground write.
func TestOwnedReaderForegroundPreparation(t *testing.T) {
	for _, mode := range []string{"foreground_after_ready", "changed", "lost", "ownership_failed", "canceled", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
			defer cancel()
			calls, ready := 0, 0
			err := waitOwnedReaderForeground(ctx, func() (uintptr, bool, error) {
				calls++
				if calls == 1 {
					return 42, false, nil
				}
				switch mode {
				case "foreground_after_ready":
					return 42, true, nil
				case "changed":
					return 43, true, nil
				case "lost":
					return 0, false, nil
				case "ownership_failed":
					return 42, true, errors.New("PRIVATE")
				default:
					return 42, false, nil
				}
			}, func() {
				ready++
				if mode == "canceled" {
					cancel()
				}
			})
			if (err == nil) != (mode == "foreground_after_ready") || ready != 1 {
				t.Fatal("invalid pre-capability foreground verdict")
			}
			if err != nil && strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("raw probe error escaped")
			}
		})
	}
}

func TestOwnedReaderForegroundRejectsInvalidObserverWithoutProbe(t *testing.T) {
	var b *OwnedReaderBinding
	if b.WaitForForeground(context.Background(), "AkuBrowser reader split_"+strings.Repeat("a", 32), nil) == nil {
		t.Fatal("nil owner accepted")
	}
	if waitOwnedReaderForeground(nil, func() (uintptr, bool, error) { t.Fatal("nil context probed"); return 0, false, nil }, nil) == nil {
		t.Fatal("nil context accepted")
	}
}
