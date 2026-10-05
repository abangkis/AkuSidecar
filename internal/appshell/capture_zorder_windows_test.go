//go:build windows

package appshell

import (
	"context"
	"testing"
	"time"
)

func TestCaptureContainmentRejectsOrdinaryOrUnownedWindows(t *testing.T) {
	for _, w := range []*Window{{}, {captureHost: true}} {
		if controller, err := w.StartCaptureContainment(nil); err == nil || controller != nil {
			t.Fatal("unowned/ordinary window enabled containment")
		}
	}
}

func TestCaptureReaderCloseRejectsUntrackedWindow(t *testing.T) {
	c := &captureZOrder{job: 1, readerLifetimes: map[uintptr]struct{}{}}
	if err := c.CloseReaderWindow(context.Background(), 123); err == nil {
		t.Fatal("close accepted an HWND that is not owned by this native reader")
	}
}

func TestCaptureReaderIntentRejectsMissingOwnershipWithoutNativeWrite(t *testing.T) {
	c := &captureZOrder{}
	if c.owns(0) || c.owns(123) {
		t.Fatal("window accepted without Job Object")
	}
	if err := c.foregroundReader(context.Background(), 123, 1, time.Now().Add(time.Second)); err == nil {
		t.Fatal("unowned reader foreground accepted")
	}
	if _, err := c.PrepareReader(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid marker accepted")
	}
}
