package ownedrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(b []byte) (int, error) { return f(b) }

type request struct {
	ID int64
	Op string
}

func captureWriter() (io.Writer, <-chan request) {
	out := make(chan request, 32)
	return writeFunc(func(b []byte) (int, error) {
		var q request
		if err := json.Unmarshal(b, &q); err != nil {
			return 0, err
		}
		out <- q
		return len(b), nil
	}), out
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("private pipe test stalled")
		var zero T
		return zero
	}
}

func TestConcurrentReplyCorrelation(t *testing.T) {
	w, requests := captureWriter()
	c := New(w)
	defer c.Close()
	results := make(chan string, 12)
	for i := 0; i < 12; i++ {
		go func(i int) {
			reply, err := c.Call(context.Background(), fmt.Sprintf("op_%d", i), nil, nil)
			if err != nil {
				results <- "failed"
				return
			}
			results <- string(reply.Result)
		}(i)
	}
	queries := make([]request, 12)
	ids := map[int64]bool{}
	for i := range queries {
		queries[i] = receive(t, requests)
		if queries[i].ID <= 0 || ids[queries[i].ID] {
			t.Fatal("correlation reused")
		}
		ids[queries[i].ID] = true
	}
	c.Deliver(9999, Reply{Result: json.RawMessage(`"unknown"`)})
	for i := len(queries) - 1; i >= 0; i-- {
		q := queries[i]
		c.Deliver(q.ID, Reply{Result: json.RawMessage(q.Op)})
		c.Deliver(q.ID, Reply{Result: json.RawMessage(`"duplicate"`)})
	}
	seen := map[string]bool{}
	for range queries {
		seen[receive(t, results)] = true
	}
	for _, q := range queries {
		if !seen[q.Op] {
			t.Fatal("reply mixed across operations")
		}
	}
	if c.Pending() != 0 {
		t.Fatal("completed correlations retained")
	}
}

func TestProgressAndDiagnosticBounds(t *testing.T) {
	w, requests := captureWriter()
	c := New(w)
	defer c.Close()
	var count atomic.Int32
	done := make(chan Reply, 1)
	go func() {
		r, _ := c.Call(context.Background(), "inspect", nil, func(raw []byte) {
			if len(raw) > 512 {
				panic("unbounded progress")
			}
			count.Add(1)
		})
		done <- r
	}()
	q := receive(t, requests)
	c.Progress(q.ID+1, []byte(`{}`))
	c.Progress(q.ID, make([]byte, 513))
	c.Progress(q.ID, []byte(`{}`))
	c.Deliver(q.ID, Reply{Code: "closed_reason", Diagnostic: make([]byte, 513)})
	r := receive(t, done)
	c.Progress(q.ID, []byte(`{}`))
	if count.Load() != 1 || r.Code != "closed_reason" || r.Diagnostic != nil || c.Pending() != 0 {
		t.Fatal("diagnostic bounds or retirement failed")
	}
}

func TestCancellationRetiresCorrelation(t *testing.T) {
	w, requests := captureWriter()
	c := New(w)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var count atomic.Int32
	go func() { _, err := c.Call(ctx, "acquire", nil, func([]byte) { count.Add(1) }); done <- err }()
	q := receive(t, requests)
	cancel()
	if !errors.Is(receive(t, done), context.Canceled) || c.Pending() != 0 {
		t.Fatal("canceled call retained")
	}
	c.Progress(q.ID, []byte(`{}`))
	c.Deliver(q.ID, Reply{})
	if count.Load() != 0 {
		t.Fatal("retired progress accepted")
	}
}

func TestCloseRetiresInflightAndRejectsNewCalls(t *testing.T) {
	w, requests := captureWriter()
	c := New(w)
	done := make(chan error, 1)
	go func() { _, err := c.Call(context.Background(), "inspect", nil, nil); done <- err }()
	q := receive(t, requests)
	c.Close()
	c.Close()
	c.Deliver(q.ID, Reply{})
	if !errors.Is(receive(t, done), ErrClosed) || c.Pending() != 0 {
		t.Fatal("close failed to retire calls")
	}
	if _, err := c.Call(context.Background(), "new", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatal("closed client reused")
	}
}

func TestCanceledWriterWaitDoesNotDispatch(t *testing.T) {
	w, requests := captureWriter()
	c := New(w)
	defer c.Close()
	c.writeMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "canceled", nil, nil); done <- err }()
	end := time.Now().Add(time.Second)
	for c.Pending() == 0 && time.Now().Before(end) {
		time.Sleep(time.Millisecond)
	}
	if c.Pending() != 1 {
		c.writeMu.Unlock()
		cancel()
		t.Fatal("writer waiter was not registered")
	}
	cancel()
	c.writeMu.Unlock()
	err := receive(t, done)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("canceled context ignored")
	}
	select {
	case <-requests:
		t.Fatal("canceled operation dispatched")
	default:
	}
}

func TestRequestIntegrity(t *testing.T) {
	for _, fields := range []map[string]any{{"id": 1}, {"op": "override"}, {"unserializable": make(chan int)}} {
		w, requests := captureWriter()
		c := New(w)
		if _, err := c.Call(context.Background(), "inspect", fields, nil); !errors.Is(err, ErrRequest) {
			t.Fatal("invalid request accepted")
		}
		if c.Pending() != 0 {
			t.Fatal("invalid request retained")
		}
		select {
		case <-requests:
			t.Fatal("invalid request written")
		default:
		}
		c.Close()
	}
	c := New(io.Discard)
	c.next = int64(^uint64(0) >> 1)
	if _, err := c.Call(context.Background(), "inspect", nil, nil); !errors.Is(err, ErrRequest) {
		t.Fatal("ID exhaustion wrapped")
	}
	if _, err := c.Call(nil, "inspect", nil, nil); !errors.Is(err, ErrRequest) {
		t.Fatal("nil context accepted")
	}
	if _, err := c.Call(context.Background(), "", nil, nil); !errors.Is(err, ErrRequest) {
		t.Fatal("empty operation accepted")
	}
	if _, err := New(nil).Call(context.Background(), "inspect", nil, nil); !errors.Is(err, ErrRequest) {
		t.Fatal("missing pipe accepted")
	}
}

func TestWriteFailureIsSanitizedAndClosesLane(t *testing.T) {
	for _, w := range []io.Writer{writeFunc(func([]byte) (int, error) { return 0, errors.New("PRIVATE_TOKEN") }), writeFunc(func(b []byte) (int, error) { return len(b) - 1, nil })} {
		c := New(w)
		if _, err := c.Call(context.Background(), "inspect", nil, nil); !errors.Is(err, ErrWrite) || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("write error leaked or ignored")
		}
		if c.Pending() != 0 {
			t.Fatal("failed request retained")
		}
		if _, err := c.Call(context.Background(), "new", nil, nil); !errors.Is(err, ErrClosed) {
			t.Fatal("corrupt stream reused")
		}
	}
}

func TestCanceledReplyCannotWin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var c *Client
	c = New(writeFunc(func(b []byte) (int, error) {
		var q request
		_ = json.Unmarshal(b, &q)
		cancel()
		c.Deliver(q.ID, Reply{})
		return len(b), nil
	}))
	if _, err := c.Call(ctx, "inspect", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("reply bypassed cancellation")
	}
}

func TestReplyFormattingIsPrivate(t *testing.T) {
	r := Reply{Result: json.RawMessage(`"PRIVATE_TOKEN"`), Code: "PRIVATE_CODE", Diagnostic: json.RawMessage(`"PRIVATE_BODY"`)}
	for _, v := range []string{fmt.Sprint(r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r)} {
		if strings.Contains(v, "PRIVATE") {
			t.Fatal("reply formatting leaked payload")
		}
	}
}

func TestDeliveredReplySurvivesImmediateEOF(t *testing.T) {
	var c *Client
	c = New(writeFunc(func(b []byte) (int, error) {
		var q request
		_ = json.Unmarshal(b, &q)
		c.Deliver(q.ID, Reply{Code: "first_failure", Diagnostic: json.RawMessage(`{"stage":"worker"}`)})
		c.Close()
		return len(b), nil
	}))
	r, err := c.Call(context.Background(), "inspect", nil, nil)
	if err != nil || r.Code != "first_failure" || string(r.Diagnostic) != `{"stage":"worker"}` {
		t.Fatal("EOF hid an already-delivered terminal reply")
	}
}

func TestConcurrentCloseCannotHideAcceptedTerminalReply(t *testing.T) {
	for i := 0; i < 200; i++ {
		w, requests := captureWriter()
		c := New(w)
		type result struct {
			reply Reply
			err   error
		}
		done := make(chan result, 1)
		go func() { r, err := c.Call(context.Background(), "inspect", nil, nil); done <- result{r, err} }()
		q := receive(t, requests)
		accepted := make(chan bool, 1)
		if i%2 == 0 {
			accepted <- c.Deliver(q.ID, Reply{Code: "first_failure"})
		} else {
			go func() { accepted <- c.Deliver(q.ID, Reply{Code: "first_failure"}) }()
		}
		c.Close()
		out := receive(t, done)
		delivered := receive(t, accepted)
		if delivered && (out.err != nil || out.reply.Code != "first_failure") {
			t.Fatal("close hid accepted terminal reply")
		}
		if !delivered && !errors.Is(out.err, ErrClosed) {
			t.Fatal("retired correlation produced a reply")
		}
	}
}

func TestFailedWriteClosesBeforeQueuedWrite(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	c := New(writeFunc(func([]byte) (int, error) { writes.Add(1); close(entered); <-release; return 0, io.ErrClosedPipe }))
	done := make(chan error, 2)
	go func() { _, err := c.Call(context.Background(), "first", nil, nil); done <- err }()
	receive(t, entered)
	go func() { _, err := c.Call(context.Background(), "queued", nil, nil); done <- err }()
	end := time.Now().Add(time.Second)
	for c.Pending() < 2 && time.Now().Before(end) {
		time.Sleep(time.Millisecond)
	}
	if c.Pending() != 2 {
		close(release)
		t.Fatal("second call did not reach the writer queue")
	}
	close(release)
	errorsSeen := []error{receive(t, done), receive(t, done)}
	writeFailed, closed := false, false
	for _, err := range errorsSeen {
		writeFailed = writeFailed || errors.Is(err, ErrWrite)
		closed = closed || errors.Is(err, ErrClosed)
	}
	if !writeFailed || !closed || writes.Load() != 1 || c.Pending() != 0 {
		t.Fatal("queued request entered a corrupt stream")
	}
}

func TestCloseDoesNotWaitForExternalIO(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	c := New(writeFunc(func(b []byte) (int, error) { close(entered); <-release; return len(b), nil }))
	done := make(chan error, 1)
	go func() { _, err := c.Call(context.Background(), "admitted", nil, nil); done <- err }()
	receive(t, entered)
	c.Close()
	if _, err := c.Call(context.Background(), "new", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatal("closed admission accepted a new write")
	}
	close(release)
	if !errors.Is(receive(t, done), ErrClosed) {
		t.Fatal("closed unresolved call returned success")
	}
}
