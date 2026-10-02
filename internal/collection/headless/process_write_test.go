package headless

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestWorkerWriteBackpressureUsesRequestDeadline(t *testing.T) {
	reader, writer := io.Pipe() // No reader: the write cannot complete normally.
	defer reader.Close()
	defer writer.Close()
	timer := time.NewTimer(20 * time.Millisecond)
	defer timer.Stop()
	result := make(chan error, 1)
	go func() { result <- writeWorkerFrame(writer, []byte("request\n"), timer.C) }()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected bounded write failure, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pipe backpressure bypassed request deadline")
	}
	// Production closes the owned worker tree on this error; closing the pipe
	// here releases the fixture's blocked writer as process cleanup would.
}

func TestWorkerWriteDoesNotResetReplyDeadline(t *testing.T) {
	var output strings.Builder
	timer := time.NewTimer(20 * time.Millisecond)
	defer timer.Stop()
	if err := writeWorkerFrame(&output, []byte("request\n"), timer.C); err != nil {
		t.Fatal(err)
	}
	if output.String() != "request\n" {
		t.Fatal("frame changed")
	}
	select {
	case <-timer.C:
	case <-time.After(time.Second):
		t.Fatal("reply no longer shares original request deadline")
	}
}
