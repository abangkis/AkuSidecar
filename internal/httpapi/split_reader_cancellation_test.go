package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type readerBrokerReceivedPause struct {
	received chan struct{}
	resume   chan struct{}
	once     sync.Once
	logs     bytes.Buffer
}

func (w *readerBrokerReceivedPause) Write(body []byte) (int, error) {
	n, err := w.logs.Write(body)
	if bytes.Contains(body, []byte("phase=received")) {
		w.once.Do(func() {
			close(w.received)
			<-w.resume
		})
	}
	return n, err
}

func TestSplitCaptureCloseCancellationWinsBeforeClosedCheck(t *testing.T) {
	s, _ := splitTestServer(t)
	pause := &readerBrokerReceivedPause{received: make(chan struct{}), resume: make(chan struct{})}
	s.logger = log.New(pause, "", 0)
	release := sync.OnceFunc(func() { close(pause.resume) })
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("f", 32), Source: "x", URL: "https://x.com/a/status/123"}
	done := make(chan error, 1)
	go func() {
		done <- s.HandleReaderBroker(ctx, request, func(readerbroker.Target) (readerbroker.Reply, error) {
			t.Error("closed waiter reached activation")
			return readerbroker.Reply{}, nil
		})
	}()
	select {
	case <-pause.received:
	case <-ctx.Done():
		t.Fatal("waiter did not reach the registered pre-loop barrier")
	}
	// Force close after registration, before the first transport-closed check.
	s.splitCapture.close()
	release()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("registered waiter lost cancellation to closed transport: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("registered waiter did not exit after close")
	}
	if !strings.Contains(pause.logs.String(), "phase=terminal outcome=cancelled") {
		t.Fatalf("close lost cancellation diagnostics: %s", pause.logs.String())
	}
	s.splitCapture.mu.Lock()
	conversation := s.splitCapture.readerBrokers[request.RequestID]
	tombstonePreserved := conversation != nil && conversation.cancelled && conversation.cancel == nil && time.Now().Before(conversation.expires)
	s.splitCapture.mu.Unlock()
	if !tombstonePreserved {
		t.Fatal("close lost the reader cancellation tombstone")
	}
	err := s.HandleReaderBroker(context.Background(), request, func(readerbroker.Target) (readerbroker.Reply, error) {
		t.Error("cancelled request replay reached activation")
		return readerbroker.Reply{}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request replay changed its error: %v", err)
	}
}

func TestClosedSplitCaptureRejectsNewReaderBroker(t *testing.T) {
	s, _ := splitTestServer(t)
	s.splitCapture.close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("a", 32), Source: "x", URL: "https://x.com/a/status/456"}
	err := s.HandleReaderBroker(ctx, request, func(readerbroker.Target) (readerbroker.Reply, error) {
		t.Error("closed transport admitted a new reader activation")
		return readerbroker.Reply{}, nil
	})
	if err == nil || err.Error() != "reader broker stopped" || ctx.Err() != nil {
		t.Fatalf("closed transport rejection changed: err=%v context=%v", err, ctx.Err())
	}
}
