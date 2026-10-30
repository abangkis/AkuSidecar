//go:build windows

package appshell

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"golang.org/x/sys/windows"
)

const mainBrowserSchema = "aku.main-browser-transport.v1"

type mainBrowserPipe struct {
	handles              [2]windows.Handle
	rawReader, rawWriter *os.File // Lifetime pins only. Go never reads/writes these.
	client               *mainBrowserClient
	argsv                []string
}
type mainBrowserClient struct {
	worker  *OwnedCommand
	rpc     *ownedrpc.Client
	control *mainControlConn
	ready   chan error
	nonce   string
	once    sync.Once
}
type mainControlConn struct {
	handle windows.Handle
	ctx    context.Context
	cancel context.CancelFunc
}

func verifyMainAsset(path, expected string) error {
	if !filepath.IsAbs(path) || len(expected) != 64 {
		return errors.New("main browser packaged asset invalid")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("main browser packaged asset unavailable")
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil || hex.EncodeToString(hash.Sum(nil)) != expected {
		return errors.New("main browser asset pin rejected")
	}
	return nil
}
func newMainBrowserProtocolLaunch(parent context.Context, cfg MainBrowserTransportOptions) (captureProtocolLaunch, error) {
	if err := verifyMainAsset(cfg.Node, cfg.NodeSHA256); err != nil {
		return nil, err
	}
	if err := verifyMainAsset(cfg.Driver, cfg.DriverSHA256); err != nil {
		return nil, err
	}
	timeout := cfg.StartupTimeout
	if timeout <= 0 || timeout > 60*time.Second {
		timeout = 18 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(nonce[:])
	p := &mainBrowserPipe{}
	// Chrome inherits server endpoints. Go retains unread client copies; Node
	// receives ordinary stdio copies. A Node crash cannot publish CDP EOF.
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	cleanupRaw := func() {
		for _, h := range []windows.Handle{inputRead, inputWrite, outputRead, outputWrite} {
			if h != 0 {
				windows.CloseHandle(h)
			}
		}
	}
	if err := windows.CreatePipe(&inputRead, &inputWrite, sa, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outputRead, &outputWrite, sa, 0); err != nil {
		cleanupRaw()
		return nil, err
	}
	for _, h := range []windows.Handle{inputWrite, outputRead} {
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
			cleanupRaw()
			return nil, err
		}
	}
	p.handles = [2]windows.Handle{inputRead, outputWrite}
	inputRead = 0
	outputWrite = 0
	p.rawWriter = os.NewFile(uintptr(inputWrite), "main-cdp-client-write-pin")
	inputWrite = 0
	p.rawReader = os.NewFile(uintptr(outputRead), "main-cdp-client-read-pin")
	outputRead = 0
	closeRaw := func() {
		p.rawReader.Close()
		p.rawWriter.Close()
		for i, h := range p.handles {
			if h != 0 {
				windows.CloseHandle(h)
				p.handles[i] = 0
			}
		}
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		closeRaw()
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		closeRaw()
		return nil, err
	}
	acl := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	pipeName := `\\.\pipe\AkuBrowser.main-transport.` + token
	text, _ := windows.UTF16PtrFromString(pipeName)
	control, err := windows.CreateNamedPipe(text, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_FIRST_PIPE_INSTANCE|windows.FILE_FLAG_OVERLAPPED, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT|0x8, 1, 65536, 65536, 0, acl)
	if err != nil {
		closeRaw()
		return nil, errors.New("main transport control pipe unavailable")
	}
	cmd := exec.Command(cfg.Node, cfg.Driver, pipeName, token)
	cmd.Stdin = p.rawReader
	cmd.Stdout = p.rawWriter
	cmd.Stderr = io.Discard
	worker, err := StartOwnedCommand(cmd)
	if err != nil {
		windows.CloseHandle(control)
		closeRaw()
		return nil, err
	}
	fail := func(err error) (captureProtocolLaunch, error) {
		worker.Terminate()
		wait, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = worker.WaitForCleanup(wait)
		windows.CloseHandle(control)
		closeRaw()
		return nil, err
	}
	if err = connectMainTransportPipe(ctx, control, uint32(worker.PID())); err != nil {
		return fail(err)
	}
	life, stop := context.WithCancel(context.Background())
	conn := &mainControlConn{handle: control, ctx: life, cancel: stop}
	c := &mainBrowserClient{worker: worker, control: conn, rpc: ownedrpc.New(conn), ready: make(chan error, 1), nonce: token}
	p.client = c
	go c.read(conn)
	if json.NewEncoder(conn).Encode(map[string]any{"op": "main_transport_bootstrap", "schema": mainBrowserSchema, "nonce": token, "transport": "inherited-stdio"}) != nil {
		return fail(errors.New("main transport bootstrap write failed"))
	}
	select {
	case err = <-c.ready:
		if err != nil {
			return fail(err)
		}
	case <-ctx.Done():
		return fail(errors.New("main transport bootstrap timeout"))
	}
	in, _ := formatCDPPipeHandle(p.handles[0])
	out, _ := formatCDPPipeHandle(p.handles[1])
	p.argsv = []string{"--remote-debugging-pipe", "--remote-debugging-io-pipes=" + in + "," + out}
	return p, nil
}
func connectMainTransportPipe(ctx context.Context, h windows.Handle, pid uint32) error {
	_, err := mainControlOperation(ctx, h, func(ov *windows.Overlapped, _ *uint32) error { return windows.ConnectNamedPipe(h, ov) })
	if err != nil && err != windows.ERROR_PIPE_CONNECTED {
		return errors.New("main transport control connection failed")
	}
	var actual uint32
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeClientProcessId")
	ok, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&actual)))
	if ok == 0 || actual != pid {
		return errors.New("main transport client identity rejected")
	}
	return nil
}
func mainControlOperation(ctx context.Context, h windows.Handle, start func(*windows.Overlapped, *uint32) error) (uint32, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	var n uint32
	err = start(&ov, &n)
	if err == windows.ERROR_IO_PENDING {
		for {
			if ctx.Err() != nil {
				windows.CancelIoEx(h, &ov)
				_ = windows.GetOverlappedResult(h, &ov, &n, true)
				return n, ctx.Err()
			}
			state, e := windows.WaitForSingleObject(event, 20)
			if e != nil {
				return n, e
			}
			if state == windows.WAIT_OBJECT_0 {
				err = windows.GetOverlappedResult(h, &ov, &n, false)
				return n, err
			}
		}
	}
	return n, err
}
func (c *mainControlConn) Read(b []byte) (int, error) {
	n, err := mainControlOperation(c.ctx, c.handle, func(ov *windows.Overlapped, n *uint32) error { return windows.ReadFile(c.handle, b, n, ov) })
	return int(n), err
}
func (c *mainControlConn) Write(b []byte) (int, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 18*time.Second)
	defer cancel()
	n, err := mainControlOperation(ctx, c.handle, func(ov *windows.Overlapped, n *uint32) error { return windows.WriteFile(c.handle, b, n, ov) })
	return int(n), err
}
func (c *mainControlConn) Close() error {
	c.cancel()
	windows.CancelIoEx(c.handle, nil)
	return windows.CloseHandle(c.handle)
}
func (p *mainBrowserPipe) args() []string { return append([]string(nil), p.argsv...) }
func (p *mainBrowserPipe) configure(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	for _, h := range p.handles {
		cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(h))
	}
	return nil
}
func (p *mainBrowserPipe) childStarted() error {
	for i, h := range p.handles {
		if err := windows.CloseHandle(h); err != nil {
			return err
		}
		p.handles[i] = 0
	}
	return nil
}
func (p *mainBrowserPipe) protocol() CaptureProtocol { return p.client }
func (p *mainBrowserPipe) closeAfterOwnerExit() error {
	p.client.once.Do(func() { p.client.rpc.Close(); p.client.control.Close(); p.client.worker.Terminate() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.client.worker.WaitForCleanup(ctx); err != nil {
		return errors.New("main transport cleanup uncertain")
	}
	p.rawReader.Close()
	p.rawWriter.Close()
	for i, h := range p.handles {
		if h != 0 {
			windows.CloseHandle(h)
			p.handles[i] = 0
		}
	}
	return nil
}
func (c *mainBrowserClient) read(input io.Reader) {
	defer c.rpc.Close()
	scan := bufio.NewScanner(input)
	scan.Buffer(make([]byte, 4096), 8<<20)
	hello := false
	for scan.Scan() {
		if !hello {
			var h struct {
				Ready  bool   `json:"ready"`
				Schema string `json:"schema"`
				Nonce  string `json:"nonce"`
			}
			if json.Unmarshal(scan.Bytes(), &h) != nil || !h.Ready || h.Schema != mainBrowserSchema || h.Nonce != c.nonce {
				break
			}
			hello = true
			c.ready <- nil
			continue
		}
		if id, progress, found, valid := mainBrowserProgressFrame(scan.Bytes()); found {
			if valid {
				c.rpc.Progress(id, progress)
			}
			continue
		}
		if !mainBrowserTerminalFrame(scan.Bytes()) {
			continue
		}
		var r struct {
			ID     int64           `json:"id"`
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  struct {
				Code       string          `json:"code"`
				Diagnostic json.RawMessage `json:"diagnostic"`
			} `json:"error"`
		}
		if json.Unmarshal(scan.Bytes(), &r) != nil || r.ID < 1 {
			break
		}
		reply := ownedrpc.Reply{Result: r.Result}
		if !r.OK {
			reply.Result = nil
			reply.Code = r.Error.Code
			if reply.Code == "" || len(reply.Code) > 1024 {
				reply.Code = "main_transport_rejected"
			}
			reply.Diagnostic = r.Error.Diagnostic
		}
		c.rpc.Deliver(r.ID, reply)
	}
	if !hello {
		select {
		case c.ready <- errors.New("main transport handshake rejected"):
		default:
		}
	}
	// Failure revokes callers only. Unread endpoint copies preserve Chrome.
}

// Progress frames have an exact top-level envelope. Unknown, duplicate, late,
// and malformed frames are ignored and cannot retire or replace a terminal reply.
func mainBrowserProgressFrame(raw []byte) (id int64, progress []byte, found, valid bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return 0, nil, false, false
	}
	fields := make(map[string]json.RawMessage, 3)
	seen := make(map[string]bool, 3)
	malformed := false
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return 0, nil, found, false
		}
		if key == "event" {
			found = true
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return 0, nil, found, false
		}
		if seen[key] || key != "event" && key != "id" && key != "result" {
			malformed = true
		}
		seen[key] = true
		fields[key] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return 0, nil, found, false
	}
	if _, err = d.Token(); err != io.EOF {
		return 0, nil, found, false
	}
	if !found {
		return 0, nil, false, false
	}
	if malformed || len(fields) != 3 || !seen["id"] || !seen["result"] {
		return 0, nil, true, false
	}
	var event string
	if json.Unmarshal(fields["event"], &event) != nil || event != "main_x_progress" || json.Unmarshal(fields["id"], &id) != nil || id < 1 {
		return 0, nil, true, false
	}
	progress = bytes.TrimSpace(fields["result"])
	if len(progress) == 0 || len(progress) > 512 || progress[0] != '{' {
		return 0, nil, true, false
	}
	return id, append([]byte(nil), progress...), true, true
}

func mainBrowserTerminalFrame(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	fields := make(map[string]json.RawMessage, 3)
	seen := make(map[string]bool, 3)
	malformed := false
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
		if seen[key] || key != "id" && key != "ok" && key != "result" && key != "error" {
			malformed = true
		}
		seen[key] = true
		fields[key] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return false
	}
	if _, err = d.Token(); err != io.EOF || malformed || !seen["id"] || !seen["ok"] {
		return false
	}
	var id int64
	var ok bool
	if json.Unmarshal(fields["id"], &id) != nil || id < 1 || json.Unmarshal(fields["ok"], &ok) != nil {
		return false
	}
	if ok {
		return len(fields) == 3 && seen["result"] && !seen["error"]
	}
	return len(fields) == 3 && seen["error"] && !seen["result"]
}

func (c *mainBrowserClient) request(ctx context.Context, op string, fields map[string]any) (json.RawMessage, error) {
	return c.requestWithProgress(ctx, op, fields, nil)
}
func (c *mainBrowserClient) requestWithProgress(ctx context.Context, op string, fields map[string]any, progress func([]byte)) (json.RawMessage, error) {
	r, err := c.rpc.Call(ctx, op, fields, progress)
	if err != nil {
		return nil, err
	}
	if r.Code != "" {
		clear(r.Result)
		return nil, &MainBrowserError{Code: r.Code, Diagnostic: r.Diagnostic}
	}
	return r.Result, nil
}
func (c *mainBrowserClient) Call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	if params == nil {
		params = map[string]any{}
	}
	ms := int64(15000)
	if d, ok := ctx.Deadline(); ok && time.Until(d).Milliseconds() < ms {
		ms = time.Until(d).Milliseconds()
	}
	if ms < 1 {
		return nil, context.DeadlineExceeded
	}
	return c.request(ctx, "main_call", map[string]any{"method": method, "params": params, "sessionId": sessionID, "timeoutMS": ms})
}
func (c *mainBrowserClient) CallX(ctx context.Context, operation string, params map[string]any) (json.RawMessage, error) {
	return c.request(ctx, "main_x", map[string]any{"operation": operation, "params": params})
}
func (c *mainBrowserClient) CallSource(ctx context.Context, source, operation string, params map[string]any) (json.RawMessage, error) {
	if source != "instagram" {
		return nil, errors.New("main browser source rejected")
	}
	return c.request(ctx, "main_source", map[string]any{"source": source, "operation": operation, "params": params})
}
func (c *mainBrowserClient) CallXWithProgress(ctx context.Context, operation string, params map[string]any, progress func([]byte)) (json.RawMessage, error) {
	var mu sync.Mutex
	count := 0
	boundedProgress := func(raw []byte) {
		if progress == nil || len(raw) > 512 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if count >= 32 {
			return
		}
		count++
		progress(append([]byte(nil), raw...))
	}
	return c.requestWithProgress(ctx, "main_x", map[string]any{"operation": operation, "params": params}, boundedProgress)
}
func (c *mainBrowserClient) TransportWorker() *OwnedCommand { return c.worker }

func (c *mainBrowserClient) DiagnoseInstagramBootstrap(ctx context.Context, nonce string) (json.RawMessage, error) {
	if !validInstagramDiagnosticNonce(nonce) {
		return nil, &MainBrowserError{Code: "instagram_diagnostic_invalid_request"}
	}
	raw, err := c.request(ctx, "main_instagram_bootstrap_diagnostic", map[string]any{"nonce": nonce})
	if err != nil {
		return nil, &MainBrowserError{Code: "instagram_diagnostic_rejected"}
	}
	return ValidateInstagramBootstrapDiagnostic(raw)
}

func (c *mainBrowserClient) DiagnoseInstagramReader(ctx context.Context, nonce string) (json.RawMessage, error) {
	if !validInstagramDiagnosticNonce(nonce) {
		return nil, &MainBrowserError{Code: "instagram_diagnostic_invalid_request"}
	}
	raw, err := c.request(ctx, "main_instagram_reader_diagnostic", map[string]any{"nonce": nonce})
	if err != nil {
		// Transport diagnostics are untrusted and may contain private browser data.
		return nil, &MainBrowserError{Code: "instagram_diagnostic_rejected"}
	}
	return validateInstagramDiagnosticProjection(raw)
}
