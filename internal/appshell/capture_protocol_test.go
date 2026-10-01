package appshell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"testing"
	"time"
)

type quarantinedCaptureLaunchFixture struct{ closed bool }

func (*quarantinedCaptureLaunchFixture) args() []string            { return nil }
func (*quarantinedCaptureLaunchFixture) configure(*exec.Cmd) error { return nil }
func (*quarantinedCaptureLaunchFixture) childStarted() error       { return nil }
func (*quarantinedCaptureLaunchFixture) protocol() CaptureProtocol { return nil }

func (f *quarantinedCaptureLaunchFixture) closeAfterOwnerExit() error {
	f.closed = true
	return nil
}

func TestUnverifiedCaptureLaunchIsRetainedByPackageQuarantine(t *testing.T) {
	launch := &quarantinedCaptureLaunchFixture{}
	quarantineCaptureProtocol(processOwnership{}, launch)
	captureProtocolQuarantine.Lock()
	defer captureProtocolQuarantine.Unlock()
	if len(captureProtocolQuarantine.entries) == 0 {
		t.Fatal("unverified private pipe launch was not retained")
	}
	entry := captureProtocolQuarantine.entries[len(captureProtocolQuarantine.entries)-1]
	if entry.launch != launch || entry.owner == nil || launch.closed {
		t.Fatal("unverified private pipe launch was not retained")
	}
}

func TestCaptureProtocolLateResponseDoesNotPoisonFutureCall(t *testing.T) {
	client, peer := net.Pipe()
	transport := newCaptureProtocolTransport(client, client)
	t.Cleanup(func() {
		_ = transport.closeAfterOwnerExit()
		_ = peer.Close()
	})
	firstRead := make(chan int64, 1)
	allowLate := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(peer)
		firstFrame, oversized, err := readCaptureFrame(reader)
		if err != nil || oversized {
			serverErr <- errors.New("read first fixture request")
			return
		}
		var first struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(firstFrame, &first) != nil {
			serverErr <- errors.New("decode first fixture request")
			return
		}
		firstRead <- first.ID
		<-allowLate
		late, _ := json.Marshal(map[string]any{"id": first.ID, "result": map[string]any{"late": true}})
		if _, err := peer.Write(append(late, 0)); err != nil {
			serverErr <- err
			return
		}
		secondFrame, oversized, err := readCaptureFrame(reader)
		if err != nil || oversized {
			serverErr <- errors.New("read second fixture request")
			return
		}
		var second struct {
			ID        int64  `json:"id"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(secondFrame, &second) != nil || second.SessionID != "fixture-session" {
			serverErr <- errors.New("second fixture request did not preserve the session id")
			return
		}
		response, _ := json.Marshal(map[string]any{"id": second.ID, "result": map[string]any{"ok": true}})
		if _, err := peer.Write(append(response, 0)); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancelReady := make(chan struct{})
	go func() {
		<-firstRead
		close(cancelReady)
		cancel()
	}()
	_, err := transport.Call(ctx, "Target.getTargets", nil, "")
	cancel()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request returned %v", err)
	}
	select {
	case <-cancelReady:
	case <-time.After(time.Second):
		t.Fatal("fixture request was not written before cancellation")
	}
	close(allowLate)
	secondCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result, err := transport.Call(secondCtx, "Page.navigate", map[string]string{"url": "about:blank"}, "fixture-session")
	if err != nil {
		t.Fatalf("future call failed after late response: %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Fatalf("future call returned unexpected raw result %s", result)
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("fixture server did not finish")
	}
}

func TestReadCaptureFrameDrainsOversizedFrame(t *testing.T) {
	input := make([]byte, captureProtocolMaxFrame+16)
	for i := range input {
		input[i] = 'x'
	}
	input = append(input, 0)
	input = append(input, []byte(`{"id":1,"result":{}}`)...)
	input = append(input, 0)
	reader := bufio.NewReaderSize(bytes.NewReader(input), 4096)
	_, oversized, err := readCaptureFrame(reader)
	if err != nil || !oversized {
		t.Fatalf("oversized frame result: oversized=%t err=%v", oversized, err)
	}
	frame, oversized, err := readCaptureFrame(reader)
	if err != nil || oversized || string(frame) != `{"id":1,"result":{}}` {
		t.Fatalf("next frame after oversized frame: frame=%q oversized=%t err=%v", frame, oversized, err)
	}
}
