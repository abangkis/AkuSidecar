//go:build windows

package readerbroker

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"testing"
	"time"
)

func TestReaderNativeRejectsInvalidBindingBeforeActivation(t *testing.T) {
	if result := activate(context.Background(), Target{}, 0); result.OK || result.Applied {
		t.Fatal("empty native target accepted")
	}
	if result := activate(context.Background(), Target{HWND: 123, PID: 1, Value: 1, Property: "AkuBrowser.ExplicitReader.1", Expires: time.Now().Add(-time.Second)}, 0); result.OK || result.Applied {
		t.Fatal("expired target activated")
	}
}

func TestReaderOverlappedPipeTransfersFramesAndCancelsIdleRead(t *testing.T) {
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\pipe\AkuBrowser.reader-test.%d.%d`, os.Getpid(), time.Now().UnixNano()))
	h, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT|0x8, 1, 8192, 8192, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	server := &pipeConn{h: h, ctx: ctx}
	ready := make(chan error, 1)
	go func() {
		_, err := server.operation(func(o *windows.Overlapped, _ *uint32) error { return windows.ConnectNamedPipe(h, o) })
		if err == windows.ERROR_PIPE_CONNECTED {
			err = nil
		}
		ready <- err
	}()
	clientHandle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(clientHandle)
	if err = <-ready; err != nil {
		t.Fatal(err)
	}
	client := &pipeConn{h: clientHandle, ctx: ctx}
	go func() { ready <- Write(client, Reply{OK: true, Ticket: "example"}) }()
	var value Reply
	if err = Read(server, &value); err != nil {
		t.Fatal(err)
	}
	if err = <-ready; err != nil {
		t.Fatal(err)
	}
	if value.Ticket != "example" {
		t.Fatal(value)
	}
	short, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	server.ctx = short
	if err = Read(server, &value); err == nil {
		t.Fatal("idle pipe read did not cancel")
	}
}
