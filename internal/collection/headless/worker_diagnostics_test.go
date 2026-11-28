package headless

import (
	"bytes"
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"
)

const validWorkerLine = `{"operation":"headless_worker","phase":"request_failed","level":"error","sequence":1,"reason":"invalid_json","ignored":"private"}`

func TestWorkerDiagnosticFramingAndFiltering(t *testing.T) {
	events := make(chan workerEvent, 2)
	stream := &workerLines{events: events}
	for _, invalid := range []string{
		"private text", "{", strings.Replace(validWorkerLine, "headless_worker", "elsewhere", 1),
		strings.Replace(validWorkerLine, "request_failed", "private phase", 1), strings.Replace(validWorkerLine, "invalid_json", "private reason", 1),
		strings.Replace(validWorkerLine, "error", "info", 1), strings.Replace(validWorkerLine, ":1,", ":0,", 1),
		strings.Replace(validWorkerLine, ":1,", ":9007199254740992,", 1),
	} {
		io.WriteString(stream, invalid+"\n")
	}
	io.WriteString(stream, strings.Repeat("z", maxDiagnosticLine+1))
	io.WriteString(stream, validWorkerLine+"\n"+validWorkerLine[:20])
	if len(events) != 0 {
		t.Fatal("forwarded invalid, oversized or partial line")
	}
	io.WriteString(stream, validWorkerLine[20:]+"\r\n")
	if len(events) != 1 {
		t.Fatal("expected exactly one reassembled diagnostic")
	}
	e := <-events
	if e.Reason != "invalid_json" || e.Sequence != 1 {
		t.Fatalf("unexpected event: %+v", e)
	}
	io.WriteString(stream, strings.Repeat(validWorkerLine+"\n", 200))
	if len(events) != cap(events) {
		t.Fatal("bounded queue should be full")
	}
	io.WriteString(stream, validWorkerLine)
	if len(stream.line) > maxDiagnosticLine {
		t.Fatal("unbounded partial line")
	}
}

func TestWorkerDiagnosticsCapsOutputPerWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := make(chan workerEvent)
	done := make(chan struct{})
	var output bytes.Buffer
	go func() {
		defer close(done)
		forwardWorkerEvents(ctx, log.New(&output, "", 0), "quiet", events)
	}()
	for i := 0; i < 200; i++ {
		select {
		case events <- workerEvent{Phase: "request_failed", Reason: "invalid_json", Sequence: uint64(i + 1)}:
		case <-ctx.Done():
			t.Fatal("worker diagnostic drain stalled")
		}
	}
	cancel()
	<-done
	if got := bytes.Count(output.Bytes(), []byte("\n")); got != workerEventLimit {
		t.Fatalf("logged %d records; want %d", got, workerEventLimit)
	}
}
