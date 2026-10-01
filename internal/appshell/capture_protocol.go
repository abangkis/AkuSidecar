package appshell

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	captureProtocolMaxFrame   = 1 << 20
	captureProtocolMaxRequest = 64 << 10
	captureProtocolMaxPending = 32
)

var (
	errCaptureProtocolUnavailable = errors.New("private CDP transport is unavailable")
	errCaptureProtocolRejected    = errors.New("private CDP command was rejected")
)

// CaptureProtocol sends bounded DevTools Protocol calls through the private
// Chrome pipe owned by one managed Window. sessionID is empty for browser
// commands and set to the CDP session returned by Target.attachToTarget for
// target-scoped commands.
type CaptureProtocol interface {
	Call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error)
}

type captureProtocolLaunch interface {
	args() []string
	configure(*exec.Cmd) error
	childStarted() error
	protocol() CaptureProtocol
	closeAfterOwnerExit() error
}

type captureProtocolQuarantineEntry struct {
	owner  *processOwnership
	launch captureProtocolLaunch
}

var captureProtocolQuarantine = struct {
	sync.Mutex
	entries []captureProtocolQuarantineEntry
}{}

// quarantineCaptureProtocol retains handles after startup cleanup could not
// verify that the owned process tree is empty. Entries intentionally live for
// the application process lifetime; finalizers must not close pipes still
// inherited by an unverified Chrome process.
func quarantineCaptureProtocol(owner processOwnership, launch captureProtocolLaunch) {
	if launch == nil {
		return
	}
	retainedOwner := new(processOwnership)
	*retainedOwner = owner
	captureProtocolQuarantine.Lock()
	captureProtocolQuarantine.entries = append(captureProtocolQuarantine.entries, captureProtocolQuarantineEntry{
		owner: retainedOwner, launch: launch,
	})
	captureProtocolQuarantine.Unlock()
}

type captureProtocolTransport struct {
	reader io.ReadCloser
	writer io.WriteCloser

	mu         sync.Mutex
	writeGate  chan struct{}
	nextID     int64
	pending    map[int64]chan captureProtocolReply
	failure    error
	readerDone chan struct{}
	closeOnce  sync.Once
}

type captureProtocolReply struct {
	result json.RawMessage
	err    error
}

func newCaptureProtocolTransport(reader io.ReadCloser, writer io.WriteCloser) *captureProtocolTransport {
	transport := &captureProtocolTransport{
		reader: reader, writer: writer,
		pending:    make(map[int64]chan captureProtocolReply),
		writeGate:  make(chan struct{}, 1),
		readerDone: make(chan struct{}),
	}
	go transport.readLoop()
	return transport
}

func (t *captureProtocolTransport) Call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	if t == nil || ctx == nil {
		return nil, errCaptureProtocolUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	method = strings.TrimSpace(method)
	if method == "" || len(method) > 256 || strings.IndexByte(method, 0) >= 0 || len(sessionID) > 512 || strings.IndexByte(sessionID, 0) >= 0 {
		return nil, errors.New("private CDP request metadata is invalid")
	}

	t.mu.Lock()
	if t.failure != nil {
		t.mu.Unlock()
		return nil, errCaptureProtocolUnavailable
	}
	if len(t.pending) >= captureProtocolMaxPending {
		t.mu.Unlock()
		return nil, errors.New("private CDP request limit reached")
	}
	t.nextID++
	id := t.nextID
	t.mu.Unlock()

	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if sessionID != "" {
		request["sessionId"] = sessionID
	}
	frame, err := json.Marshal(request)
	if err != nil || len(frame) > captureProtocolMaxRequest {
		return nil, errors.New("private CDP request is invalid or too large")
	}
	frame = append(frame, 0)
	reply := make(chan captureProtocolReply, 1)
	t.mu.Lock()
	if t.failure != nil {
		t.mu.Unlock()
		return nil, errCaptureProtocolUnavailable
	}
	if len(t.pending) >= captureProtocolMaxPending {
		t.mu.Unlock()
		return nil, errors.New("private CDP request limit reached")
	}
	t.pending[id] = reply
	t.mu.Unlock()

	select {
	case t.writeGate <- struct{}{}:
	case <-ctx.Done():
		t.removePending(id)
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-t.writeGate
		t.removePending(id)
		return nil, err
	}
	t.mu.Lock()
	unavailable := t.failure != nil || t.pending[id] == nil
	t.mu.Unlock()
	if unavailable {
		<-t.writeGate
		t.removePending(id)
		return nil, errCaptureProtocolUnavailable
	}
	writeDone := make(chan error, 1)
	go func() {
		writeErr := writeCaptureFrame(t.writer, frame)
		if writeErr != nil {
			t.fail("private CDP write failed")
		}
		<-t.writeGate
		writeDone <- writeErr
	}()
	select {
	case writeErr := <-writeDone:
		if writeErr != nil {
			t.removePending(id)
			t.fail("private CDP write failed")
			return nil, errCaptureProtocolUnavailable
		}
	case <-ctx.Done():
		t.removePending(id)
		return nil, ctx.Err()
	}

	select {
	case response := <-reply:
		return response.result, response.err
	case <-ctx.Done():
		t.removePending(id)
		return nil, ctx.Err()
	}
}

func writeCaptureFrame(writer io.Writer, frame []byte) error {
	for len(frame) > 0 {
		written, err := writer.Write(frame)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

func (t *captureProtocolTransport) readLoop() {
	defer close(t.readerDone)
	reader := bufio.NewReaderSize(t.reader, 32<<10)
	for {
		frame, oversized, err := readCaptureFrame(reader)
		if err != nil {
			t.fail("private CDP read failed")
			return
		}
		if oversized {
			t.fail("private CDP response exceeded the frame limit")
			continue // Keep draining complete NUL frames until owned Chrome exits.
		}
		var envelope struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(frame, &envelope); err != nil {
			t.fail("private CDP response was malformed")
			continue
		}
		if envelope.ID == nil { // DevTools events have no id and need no buffering.
			continue
		}
		if len(envelope.Error) != 0 && string(envelope.Error) != "null" {
			t.resolve(*envelope.ID, captureProtocolReply{err: errCaptureProtocolRejected})
			continue
		}
		t.resolve(*envelope.ID, captureProtocolReply{result: envelope.Result})
	}
}

func readCaptureFrame(reader *bufio.Reader) ([]byte, bool, error) {
	var frame []byte
	oversized := false
	for {
		part, err := reader.ReadSlice(0)
		if !oversized {
			if len(part) > captureProtocolMaxFrame-len(frame) {
				oversized = true
				frame = nil
			} else {
				frame = append(frame, part...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		if oversized {
			return nil, true, nil
		}
		if len(frame) == 0 || frame[len(frame)-1] != 0 {
			return nil, false, errors.New("private CDP frame terminator missing")
		}
		return frame[:len(frame)-1], false, nil
	}
}

func (t *captureProtocolTransport) resolve(id int64, response captureProtocolReply) {
	t.mu.Lock()
	reply := t.pending[id]
	delete(t.pending, id)
	t.mu.Unlock()
	if reply != nil {
		reply <- response
	}
}

func (t *captureProtocolTransport) removePending(id int64) {
	t.mu.Lock()
	delete(t.pending, id)
	t.mu.Unlock()
}

// fail rejects current and future requests but leaves the reader draining the
// pipe. Closing either endpoint here could disrupt a still-owned Chrome tree.
func (t *captureProtocolTransport) fail(message string) {
	t.mu.Lock()
	if t.failure == nil {
		t.failure = errors.New(message)
		for id, reply := range t.pending {
			delete(t.pending, id)
			reply <- captureProtocolReply{err: errCaptureProtocolUnavailable}
		}
	}
	t.mu.Unlock()
}

func (t *captureProtocolTransport) closeAfterOwnerExit() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		t.fail("private CDP transport closed after owner exit")
		if t.writer != nil {
			_ = t.writer.Close()
		}
		if t.reader != nil {
			_ = t.reader.Close()
		}
	})
	select {
	case <-t.readerDone:
		return nil
	case <-time.After(1 * time.Second):
		return errors.New("private CDP reader did not stop after owner exit")
	}
}
