//go:build windows

package appshell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"golang.org/x/sys/windows"
)

type mainBrowserTestWriter func([]byte) (int, error)

func (write mainBrowserTestWriter) Write(raw []byte) (int, error) { return write(raw) }

func syntheticMainBrowserClient(t *testing.T) (*mainBrowserClient, *io.PipeWriter, <-chan []byte) {
	t.Helper()
	reader, writer := io.Pipe()
	writes := make(chan []byte, 64)
	client := &mainBrowserClient{rpc: ownedrpc.New(mainBrowserTestWriter(func(raw []byte) (int, error) {
		writes <- append([]byte(nil), raw...)
		return len(raw), nil
	})), ready: make(chan error, 1), nonce: "synthetic-main-nonce"}
	go client.read(reader)
	if _, err := io.WriteString(writer, `{"ready":true,"schema":"`+mainBrowserSchema+`","nonce":"synthetic-main-nonce"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := <-client.ready; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close(); client.rpc.Close() })
	return client, writer, writes
}

func mainProgressFrame(id int64, result string) []byte {
	return []byte(fmt.Sprintf(`{"event":"main_x_progress","id":%d,"result":%s}`+"\n", id, result))
}

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

func TestMainBrowserProgressEnvelopeCorrelationBoundsAndRetirement(t *testing.T) {
	client, writer, writes := syntheticMainBrowserClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	progressCount := 0
	result := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		raw, err := client.CallXWithProgress(ctx, "daily_batch_begin", map[string]any{}, func([]byte) { progressCount++ })
		result <- raw
		errCh <- err
	}()
	var request struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(<-writes, &request); err != nil || request.ID < 1 {
		t.Fatal("synthetic X request missing correlation ID")
	}
	valid := `{"operation":"settings_wait","elapsedMS":12,"cause":""}`
	for _, raw := range [][]byte{
		mainProgressFrame(request.ID+100, valid),
		[]byte(fmt.Sprintf(`{"event":"main_x_progress","id":%d,"result":%s,"extra":"synthetic-secret"}`+"\n", request.ID, valid)),
		[]byte(fmt.Sprintf(`{"event":"main_x_progress","id":%d,"id":%d,"result":%s}`+"\n", request.ID, request.ID, valid)),
		[]byte(fmt.Sprintf(`{"event":"unknown_private_event","id":%d,"result":%s}`+"\n", request.ID, valid)),
		mainProgressFrame(request.ID, `"synthetic-secret"`),
	} {
		if _, err := writer.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 40; i++ {
		if _, err := writer.Write(mainProgressFrame(request.ID, valid)); err != nil {
			t.Fatal(err)
		}
	}
	terminal := fmt.Sprintf(`{"id":%d,"ok":true,"result":{"closed":true}}`+"\n", request.ID)
	if _, err := io.WriteString(writer, terminal); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(mainProgressFrame(request.ID, valid)); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if raw := <-result; string(raw) != `{"closed":true}` || progressCount != 32 {
		t.Fatal("progress was not bounded or terminal reply changed")
	}
}

func TestMainBrowserConcurrentProgressUsesEachCallCorrelation(t *testing.T) {
	client, writer, writes := syntheticMainBrowserClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var mu sync.Mutex
	seen := map[string]bool{}
	results := make(chan string, 2)
	start := func(operation string) {
		go func() {
			raw, err := client.CallXWithProgress(ctx, operation, map[string]any{}, func(raw []byte) {
				mu.Lock()
				want := `"operation":"settings_wait"`
				if operation == "daily_batch_begin_collector" {
					want = `"operation":"rendezvous"`
				}
				seen[operation] = bytes.Contains(raw, []byte(want))
				mu.Unlock()
			})
			if err != nil {
				results <- "error"
				return
			}
			var reply struct {
				Value string `json:"value"`
			}
			_ = json.Unmarshal(raw, &reply)
			results <- reply.Value
		}()
	}
	start("daily_batch_begin_reader")
	start("daily_batch_begin_collector")
	var ids = map[string]int64{}
	for range 2 {
		var request struct {
			Operation string `json:"operation"`
			ID        int64  `json:"id"`
		}
		if err := json.Unmarshal(<-writes, &request); err != nil {
			t.Fatal(err)
		}
		ids[request.Operation] = request.ID
	}
	reader := `{"operation":"settings_wait","elapsedMS":12,"cause":""}`
	collector := `{"operation":"rendezvous","elapsedMS":13,"cause":""}`
	if _, err := writer.Write(mainProgressFrame(ids["daily_batch_begin_collector"], collector)); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(mainProgressFrame(ids["daily_batch_begin_reader"], reader)); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id    int64
		value string
	}{{ids["daily_batch_begin_collector"], "collector"}, {ids["daily_batch_begin_reader"], "reader"}} {
		if _, err := io.WriteString(writer, fmt.Sprintf(`{"id":%d,"ok":true,"result":{"value":%q}}`+"\n", item.id, item.value)); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	for range 2 {
		got[<-results] = true
	}
	mu.Lock()
	defer mu.Unlock()
	if !got["reader"] || !got["collector"] || !seen["daily_batch_begin_reader"] || !seen["daily_batch_begin_collector"] {
		t.Fatal("concurrent progress or terminal replies crossed correlations")
	}
}
