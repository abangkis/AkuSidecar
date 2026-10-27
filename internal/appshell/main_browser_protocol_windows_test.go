//go:build windows

package appshell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Explicit fixtures run the real Windows control handshake and sealed Node
// entry. The Chrome-facing endpoints are synthetic; no browser is launched.
func TestMainBrowserTransportWindowsSynthetic(t *testing.T) {
	node, driver := os.Getenv("AKU_CHROME_FIXTURE_NODE"), os.Getenv("AKU_MAIN_TRANSPORT_DRIVER")
	if node == "" && driver == "" {
		t.Skip("explicit packaged Node and main transport driver required")
	}
	if node == "" || driver == "" {
		t.Fatal("both AKU_CHROME_FIXTURE_NODE and AKU_MAIN_TRANSPORT_DRIVER are required")
	}
	pin := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		return hex.EncodeToString(h[:])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	launch, err := newMainBrowserProtocolLaunch(ctx, MainBrowserTransportOptions{
		Node: node, Driver: driver, NodeSHA256: pin(node), DriverSHA256: pin(driver), StartupTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := launch.(*mainBrowserPipe)
	t.Cleanup(func() {
		if err := p.closeAfterOwnerExit(); err != nil {
			t.Error(err)
		}
	})
	duplicate := func(h windows.Handle, name string) *os.File {
		var copy windows.Handle
		if err := windows.DuplicateHandle(windows.CurrentProcess(), h, windows.CurrentProcess(), &copy, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
			t.Fatal(err)
		}
		f := os.NewFile(uintptr(copy), name)
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	chromeRead := duplicate(p.handles[0], "synthetic-chrome-input")
	chromeWrite := duplicate(p.handles[1], "synthetic-chrome-output")
	peek := func(h windows.Handle) (uint32, error) {
		var available uint32
		ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("PeekNamedPipe").Call(
			uintptr(h), 0, 0, 0, uintptr(unsafe.Pointer(&available)), 0)
		if ok == 0 {
			return 0, callErr
		}
		return available, nil
	}
	served := make(chan error, 1)
	go func() {
		var frame []byte
		for ctx.Err() == nil {
			n, err := peek(windows.Handle(chromeRead.Fd()))
			if err != nil {
				served <- err
				return
			}
			if n == 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			part := make([]byte, n)
			count, err := chromeRead.Read(part)
			if err != nil {
				served <- err
				return
			}
			frame = append(frame, part[:count]...)
			if end := bytes.IndexByte(frame, 0); end >= 0 {
				var q struct {
					ID     int64  `json:"id"`
					Method string `json:"method"`
				}
				if err := json.Unmarshal(frame[:end], &q); err != nil || q.Method != "Browser.getVersion" {
					served <- fmt.Errorf("unexpected synthetic CDP request")
					return
				}
				response, _ := json.Marshal(map[string]any{"id": q.ID, "result": map[string]any{
					"product": "Chrome/synthetic", "cookies": "synthetic-secret", "headers": map[string]string{"Cookie": "synthetic-secret"},
				}})
				_, err = chromeWrite.Write(append(response, 0))
				served <- err
				return
			}
		}
		served <- ctx.Err()
	}()
	raw, err := p.protocol().Call(ctx, "Browser.getVersion", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("Chrome/synthetic")) || bytes.Contains(raw, []byte("synthetic-secret")) {
		t.Fatal("public projection leaked or lost its result")
	}
	if _, err = p.protocol().Call(ctx, "Network.getCookies", map[string]any{}, ""); err == nil {
		t.Fatal("raw cookie method admitted")
	}
	if n, err := peek(windows.Handle(chromeRead.Fd())); err != nil || n != 0 {
		t.Fatal("rejected call reached raw Chrome pipe")
	}
	// Terminate only the synthetic transport worker. Neither Go pin is consumed
	// or closed, so a real Chrome owner would not receive EOF from this event.
	p.client.worker.Terminate()
	stop, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := p.client.worker.WaitForCleanup(stop); err != nil {
		t.Fatal(err)
	}
	for _, f := range []*os.File{p.rawReader, p.rawWriter} {
		var flags uint32
		ok, _, handleErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation").Call(f.Fd(), uintptr(unsafe.Pointer(&flags)))
		if ok == 0 {
			t.Fatal("raw endpoint lifetime pin lost", handleErr)
		}
	}
	if _, err := peek(windows.Handle(chromeRead.Fd())); err != nil {
		t.Fatal("Node exit published Chrome input EOF", err)
	}
	if _, err := chromeWrite.Write([]byte{0}); err != nil {
		t.Fatal("Node exit closed Chrome output reader", err)
	}
	dead, deadCancel := context.WithTimeout(context.Background(), time.Second)
	defer deadCancel()
	if _, err := p.protocol().Call(dead, "Browser.getVersion", nil, ""); err == nil {
		t.Fatal("dead Node transport accepted a call")
	}
	_ = chromeRead.Close()
	_ = chromeWrite.Close()
	if err := p.closeAfterOwnerExit(); err != nil {
		t.Fatal(err)
	}
}
