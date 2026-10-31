//go:build windows

package appshell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestMainWorkerLifecycleDoesNotConsumeTerminalReply(t *testing.T) {
	c, writer, writes := syntheticMainBrowserClient(t)
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { _, err := c.CallSource(ctx, "instagram", "source_check", nil); done <- err }()
	select {
	case <-writes:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := io.WriteString(writer, workerLifecycleFixture+"\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(writer, `{"id":1,"ok":false,"error":{"code":"native_source_wire_unavailable"}}`); err != nil {
		t.Fatal(err)
	}
	var wire *MainBrowserError
	err := <-done
	if !errors.As(err, &wire) || wire.Code != "native_source_wire_unavailable" {
		t.Fatalf("terminal lost: %v", err)
	}
	if raw := MainBrowserWorkerFailureDiagnostic(err); !ValidateMainBrowserWorkerDiagnostic(raw) {
		t.Fatalf("worker cause lost: %s", raw)
	}
	if v := workerSnapshot(t, c.diagnostic); v.Reason != "command_timeout" || !v.Reported {
		t.Fatal("wrong worker cause")
	}
}
