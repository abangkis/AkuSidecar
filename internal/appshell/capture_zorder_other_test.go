//go:build !windows

package appshell

import "testing"

func TestCaptureContainmentDisabledOutsideWindows(t *testing.T) {
	controller, err := (&Window{captureHost: true}).StartCaptureContainment(nil)
	if controller != nil || err != nil {
		t.Fatal("containment enabled outside Windows")
	}
}
