package ownedrpc

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
)

func TestLateProgressIsPassiveAndBounded(t *testing.T) {
	writer, requests := captureWriter()
	c := New(writer)
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan string, 2)
	done := make(chan error, 1)
	go func() {
		_, err := c.CallWithLateProgress(ctx, "test", nil, func(raw []byte) { updates <- string(raw) })
		done <- err
	}()
	q := receive(t, requests)
	cancel()
	if !errors.Is(receive(t, done), context.Canceled) {
		t.Fatal("cancellation lost")
	}
	if c.Pending() != 0 {
		t.Fatal("observer kept operation pending")
	}
	c.Progress(q.ID, []byte("late"))
	if receive(t, updates) != "late" {
		t.Fatal("late update lost")
	}
	if c.Deliver(q.ID, Reply{}) {
		t.Fatal("call revived")
	}
	c.Close()
	c.Progress(q.ID, []byte("after close"))
	select {
	case <-updates:
		t.Fatal("EOF retained observer")
	default:
	}
}
func TestPreparedFieldsSampleAfterWriteAdmission(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var revision atomic.Int32
	wire := make(chan []byte, 2)
	c := New(writeFunc(func(b []byte) (int, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		wire <- append([]byte(nil), b...)
		return len(b), nil
	}))
	first := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _, e := c.Call(ctx, "first", nil, nil); first <- e }()
	<-entered
	second := make(chan error, 1)
	go func() {
		_, e := c.CallWithLateProgressPrepared(ctx, "second", nil, nil, func() (map[string]any, error) { return map[string]any{"revision": revision.Load()}, nil })
		second <- e
	}()
	revision.Store(7)
	close(release)
	receive(t, wire)
	raw := receive(t, wire)
	var q map[string]any
	json.Unmarshal(raw, &q)
	if q["revision"] != float64(7) {
		t.Fatal("fields sampled before writer admission")
	}
	c.Close()
	receive(t, first)
	receive(t, second)
}
func TestCanceledBeforeDispatchHasNoLateObserver(t *testing.T) {
	writer, _ := captureWriter()
	c := New(writer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = c.CallWithLateProgress(ctx, "test", nil, func([]byte) { t.Error("unstarted observer") })
	if len(c.late) != 0 {
		t.Fatal("retained unstarted observer")
	}
}
