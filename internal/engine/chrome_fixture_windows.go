//go:build windows && auth_session_fixture

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	chromeFixtureJobInfoBasicProcessIDList = 3
	chromeFixtureMaxJobProcesses           = 4096
)

type chromeFixtureConfig struct {
	NodePath            string
	ChromePath          string
	Root                string
	Origin              string
	DriverPath          string
	NativeReaderStartup bool
}

type chromeFixtureBridge struct {
	config chromeFixtureConfig
	job    windows.Handle
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	pid    uint32

	rpcMu         sync.Mutex
	writeMu       sync.Mutex
	jobMu         sync.Mutex
	termMu        sync.Mutex
	mu            sync.Mutex
	nextID        int64
	pending       map[int64]chan chromeFixtureReply
	owners        map[int]*chromeFixtureProcess
	exitsEarly    map[int]struct{}
	closed        chan struct{}
	closeOnce     sync.Once
	rootErr       error
	cleanupErr    error
	verifiedEmpty bool
	launchCount   atomic.Int64
	releaseProofs atomic.Int64
}

type chromeFixtureReply struct {
	result json.RawMessage
	code   string
	err    error
}

type chromeFixtureProcess struct {
	bridge              *chromeFixtureBridge
	owner               int
	pid                 uint32
	role                string
	nativeReaderStartup bool
	done                chan error
	once                sync.Once
	exited              atomic.Bool
	rootExited          atomic.Bool
	transportUnknown    atomic.Bool
	retiring            atomic.Bool
	failCleanup         atomic.Bool
}

func startChromeFixtureBridge(ctx context.Context, config chromeFixtureConfig) (*chromeFixtureBridge, error) {
	root, err := validateChromeFixtureConfig(config)
	if err != nil {
		return nil, err
	}
	config.Root = root
	for _, path := range []string{config.NodePath, config.ChromePath, config.DriverPath} {
		if !filepath.IsAbs(path) {
			return nil, errors.New("fixture executable paths must be absolute")
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil {
			return nil, errors.New("fixture executable is unavailable")
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil || info.IsDir() {
			return nil, errors.New("fixture executable is unavailable")
		}
	}
	origin, originErr := url.Parse(config.Origin)
	if originErr != nil || origin.Scheme != "http" || origin.Hostname() != "127.0.0.1" || origin.Port() == "" ||
		origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || net.ParseIP(origin.Hostname()) == nil {
		return nil, errors.New("fixture origin is invalid")
	}

	job, err := newChromeFixtureJob()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(config.NodePath, config.DriverPath, config.ChromePath, config.Root, config.Origin)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, errors.New("fixture bridge output is unavailable")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, errors.New("fixture bridge input is unavailable")
	}
	if err = cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, errors.New("fixture bridge did not start")
	}
	b := &chromeFixtureBridge{config: config, job: job, cmd: cmd, stdin: stdin, pid: uint32(cmd.Process.Pid),
		pending: map[int64]chan chromeFixtureReply{}, owners: map[int]*chromeFixtureProcess{},
		exitsEarly: map[int]struct{}{}, closed: make(chan struct{})}
	ready := make(chan error, 1)
	go b.readOutput(stdout, ready)
	wait := time.NewTimer(8 * time.Second)
	defer wait.Stop()
	select {
	case readyErr := <-ready:
		if readyErr != nil {
			b.stopBeforeChrome()
			return nil, errors.New("fixture bridge handshake failed")
		}
	case <-ctx.Done():
		b.stopBeforeChrome()
		return nil, ctx.Err()
	case <-wait.C:
		b.stopBeforeChrome()
		return nil, errors.New("fixture bridge handshake timed out")
	}
	if err = assignChromeFixtureProcess(job, b.pid); err != nil {
		b.stopBeforeChrome()
		return nil, errors.New("fixture bridge Job Object assignment failed")
	}
	pids, err := b.jobPIDs()
	if err != nil || len(pids) != 1 || pids[0] != b.pid {
		b.stopBeforeChrome()
		return nil, errors.New("fixture bridge Job Object membership is unknown")
	}
	return b, nil
}

func validateChromeFixtureConfig(config chromeFixtureConfig) (string, error) {
	if config.Root == "" || config.DriverPath == "" || config.NodePath == "" || config.ChromePath == "" || config.Origin == "" {
		return "", errors.New("fixture configuration is incomplete")
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return "", errors.New("fixture root is invalid")
	}
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("fixture root must be a new ordinary directory")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || !samePath(root, resolvedRoot) {
		return "", errors.New("fixture root resolves outside its requested path")
	}
	project, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", errors.New("fixture project root is unavailable")
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", errors.New("fixture project root is unavailable")
	}
	parent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	parentResolved, err := filepath.EvalSymlinks(parent)
	if err != nil || !samePath(parent, parentResolved) {
		return "", errors.New("fixture output parent is unavailable")
	}
	rel, err := filepath.Rel(parent, root)
	if err != nil || filepath.IsAbs(rel) || strings.Contains(rel, "..") || !strings.HasPrefix(filepath.Base(root), "run-") {
		return "", errors.New("fixture root is outside the project QA scope")
	}
	profile := filepath.Join(root, "shared")
	if _, err = os.Lstat(profile); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("fixture profile must be new")
	}
	return root, nil
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func newChromeFixtureJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create Chrome fixture Job Object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, errors.New("configure Chrome fixture Job Object failed")
	}
	return job, nil
}

func assignChromeFixtureProcess(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
}

func (b *chromeFixtureBridge) readOutput(stdout io.Reader, ready chan<- error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	readySent := false
	for scanner.Scan() {
		var message struct {
			Ready  bool            `json:"ready"`
			ID     int64           `json:"id"`
			OK     bool            `json:"ok"`
			Code   string          `json:"code"`
			Event  string          `json:"event"`
			Owner  int             `json:"owner"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			if !readySent {
				ready <- errors.New("invalid fixture bridge handshake")
				readySent = true
			}
			continue
		}
		if message.Ready {
			if !readySent {
				ready <- nil
				readySent = true
			}
			continue
		}
		if message.Event == "chrome_exit" && message.Owner > 0 {
			b.handleChromeExit(message.Owner)
			continue
		}
		if message.ID == 0 {
			continue
		}
		b.mu.Lock()
		response := b.pending[message.ID]
		delete(b.pending, message.ID)
		b.mu.Unlock()
		if response != nil {
			if message.OK {
				response <- chromeFixtureReply{result: message.Result}
			} else {
				response <- chromeFixtureReply{code: safeFixtureCode(message.Code)}
			}
		}
	}
	rootErr := scanner.Err()
	if rootErr == nil {
		rootErr = io.EOF
	}
	b.mu.Lock()
	b.rootErr = rootErr
	if !readySent {
		ready <- rootErr
	}
	for id, response := range b.pending {
		delete(b.pending, id)
		response <- chromeFixtureReply{err: errors.New("fixture bridge exited")}
	}
	b.mu.Unlock()
	b.closeOnce.Do(func() { close(b.closed) })
	_ = b.cmd.Wait()
}

func safeFixtureCode(value string) string {
	if value == "" {
		return "fixture_bridge_failed"
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return "fixture_bridge_failed"
		}
	}
	return value
}

func (b *chromeFixtureBridge) rpc(ctx context.Context, op string, fields map[string]any, out any) error {
	b.rpcMu.Lock()
	defer b.rpcMu.Unlock()
	if b.isClosed() {
		return errors.New("fixture bridge is closed")
	}
	id := atomic.AddInt64(&b.nextID, 1)
	message := map[string]any{"id": id, "op": op}
	for key, value := range fields {
		message[key] = value
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return errors.New("fixture bridge request invalid")
	}
	response := make(chan chromeFixtureReply, 1)
	b.mu.Lock()
	b.pending[id] = response
	b.mu.Unlock()
	b.writeMu.Lock()
	_, err = b.stdin.Write(append(encoded, '\n'))
	b.writeMu.Unlock()
	if err != nil {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return errors.New("fixture bridge request failed")
	}
	select {
	case result := <-response:
		if result.err != nil {
			return result.err
		}
		if result.code != "" {
			return errors.New(result.code)
		}
		if out != nil && len(result.result) > 0 {
			if err = json.Unmarshal(result.result, out); err != nil {
				return errors.New("fixture bridge response invalid")
			}
		}
		return nil
	case <-ctx.Done():
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		return ctx.Err()
	case <-b.closed:
		return errors.New("fixture bridge exited")
	}
}

func (b *chromeFixtureBridge) isClosed() bool {
	select {
	case <-b.closed:
		return true
	default:
		return false
	}
}

func (b *chromeFixtureBridge) handleChromeExit(owner int) {
	b.mu.Lock()
	process := b.owners[owner]
	if process == nil {
		b.exitsEarly[owner] = struct{}{}
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	process.rootExited.Store(true)
	go process.verifyNaturalExit()
}

func (b *chromeFixtureBridge) launch(ctx context.Context, role string) (*chromeFixtureProcess, error) {
	if role != "collector" && role != "owner" {
		return nil, errors.New("fixture role invalid")
	}
	if err := b.assertBridgeOnly(ctx); err != nil {
		return nil, fmt.Errorf("previous Chrome owner was not released: %w", err)
	}
	b.releaseProofs.Add(1)
	var launched struct {
		Owner               int    `json:"owner"`
		PID                 uint32 `json:"pid"`
		WindowCount         int    `json:"windowCount"`
		BootstrapCount      int    `json:"bootstrapCount"`
		NativeReaderStartup bool   `json:"nativeReaderStartup"`
	}
	nativeStartup := role == "owner" && b.config.NativeReaderStartup
	if err := b.rpc(ctx, "launch", map[string]any{"role": role, "nativeReaderStartup": nativeStartup}, &launched); err != nil {
		return nil, err
	}
	badOwnerWindows := role == "owner" && (launched.BootstrapCount != 1 || nativeStartup && launched.WindowCount != 1 || !nativeStartup && launched.WindowCount < 2)
	if launched.Owner <= 0 || launched.PID == 0 || launched.NativeReaderStartup != nativeStartup || badOwnerWindows {
		return nil, errors.New("fixture Chrome launch evidence invalid")
	}
	p := &chromeFixtureProcess{bridge: b, owner: launched.Owner, pid: launched.PID, role: role, nativeReaderStartup: nativeStartup, done: make(chan error, 1)}
	b.mu.Lock()
	b.owners[p.owner] = p
	_, exited := b.exitsEarly[p.owner]
	delete(b.exitsEarly, p.owner)
	b.mu.Unlock()
	if exited {
		p.rootExited.Store(true)
		go p.verifyNaturalExit()
	}
	if err := b.containsPID(p.pid); err != nil {
		return nil, errors.New("Chrome process is outside the fixture Job Object")
	}
	b.launchCount.Add(1)
	return p, nil
}

func (p *chromeFixtureProcess) Driver() string {
	if p.role == "collector" {
		return "headless"
	}
	return "browser"
}

func (p *chromeFixtureProcess) Done() <-chan error { return p.done }
func (p *chromeFixtureProcess) PID() int           { return int(p.pid) }
func (p *chromeFixtureProcess) Retiring() bool     { return p.retiring.Load() }
func (p *chromeFixtureProcess) OpenExtensionsPage(ctx context.Context) error {
	if p.role != "owner" {
		return errors.New("fixture reader is unavailable")
	}
	var result struct {
		WindowCount int `json:"windowCount"`
	}
	if err := p.bridge.rpc(ctx, "open_reader", map[string]any{"owner": p.owner}, &result); err != nil {
		return err
	}
	if result.WindowCount < 1 {
		return errors.New("fixture reader window was not observed")
	}
	return nil
}

func (p *chromeFixtureProcess) OpenFixtureIndependentWindow(ctx context.Context) error {
	if p.role != "owner" || !p.nativeReaderStartup {
		return errors.New("fixture independent window requires native reader startup")
	}
	var result struct {
		WindowCount    int `json:"windowCount"`
		BootstrapCount int `json:"bootstrapCount"`
	}
	if err := p.bridge.rpc(ctx, "open_independent_window", map[string]any{"owner": p.owner}, &result); err != nil {
		return err
	}
	if result.WindowCount != 2 || result.BootstrapCount != 1 {
		return errors.New("additional ownership window inventory was not verified")
	}
	return nil
}

func (p *chromeFixtureProcess) ReplacementReadiness(ctx context.Context) error {
	if p.transportUnknown.Load() {
		return errors.New("fixture Chrome inventory is unknown")
	}
	if p.exited.Load() || p.rootExited.Load() {
		return p.bridge.assertBridgeOnly(ctx)
	}
	var result struct {
		Exited         bool `json:"exited"`
		WindowCount    int  `json:"windowCount"`
		BootstrapCount int  `json:"bootstrapCount"`
	}
	if err := p.bridge.rpc(ctx, "inventory", map[string]any{"owner": p.owner}, &result); err != nil {
		p.transportUnknown.Store(true)
		return fmt.Errorf("fixture Chrome inventory is unknown: %w", err)
	}
	if result.Exited {
		p.rootExited.Store(true)
		return p.bridge.assertBridgeOnly(ctx)
	}
	if p.role == "owner" && result.WindowCount != 0 {
		return errors.New("fixture reader windows remain open")
	}
	return nil
}

func (p *chromeFixtureProcess) CloseForRetry(ctx context.Context) error {
	if err := p.ReplacementReadiness(ctx); err != nil {
		return err
	}
	if p.failCleanup.Load() {
		return errors.New("fixture cleanup fault was injected")
	}
	if p.rootExited.Load() && !p.exited.Load() {
		select {
		case <-p.done:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Second):
			return errors.New("fixture Chrome process tree exit timed out")
		}
	}
	if !p.exited.Load() {
		var result struct {
			Exited bool `json:"exited"`
		}
		if err := p.bridge.rpc(ctx, "release", map[string]any{"owner": p.owner}, &result); err != nil {
			return fmt.Errorf("fixture Chrome release failed: %w", err)
		}
		if !result.Exited {
			return errors.New("fixture Chrome exit was not observed")
		}
	}
	select {
	case <-p.done:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return errors.New("fixture Chrome process tree exit timed out")
	}
	if err := p.bridge.assertBridgeOnly(ctx); err != nil {
		return fmt.Errorf("fixture Chrome process tree is not released: %w", err)
	}
	return nil
}

func (p *chromeFixtureProcess) Terminate() {
	p.bridge.terminateOwnedTree()
}

func (p *chromeFixtureProcess) Evidence(ctx context.Context) (SyntheticSessionEvidence, error) {
	var result struct {
		Authenticated bool   `json:"authenticated"`
		Account       string `json:"account"`
		Revision      string `json:"revision"`
	}
	if err := p.bridge.rpc(ctx, "probe", map[string]any{"owner": p.owner}, &result); err != nil {
		return SyntheticSessionEvidence{}, err
	}
	return SyntheticSessionEvidence{Authenticated: result.Authenticated, AccountID: result.Account, Revision: result.Revision}, nil
}

func (p *chromeFixtureProcess) Login(ctx context.Context) error {
	var result struct {
		Status int `json:"status"`
	}
	if err := p.bridge.rpc(ctx, "login", map[string]any{"owner": p.owner}, &result); err != nil {
		return err
	}
	if result.Status != http.StatusOK {
		return errors.New("fixture login failed")
	}
	return nil
}

func (p *chromeFixtureProcess) ChangeAccount(ctx context.Context) error {
	var result struct {
		Status int `json:"status"`
	}
	if err := p.bridge.rpc(ctx, "account_change", map[string]any{"owner": p.owner}, &result); err != nil {
		return err
	}
	if result.Status != http.StatusOK {
		return errors.New("fixture account change failed")
	}
	return nil
}

func (p *chromeFixtureProcess) Logout(ctx context.Context) error {
	var result struct {
		Status int `json:"status"`
	}
	if err := p.bridge.rpc(ctx, "logout", map[string]any{"owner": p.owner}, &result); err != nil {
		return err
	}
	if result.Status != http.StatusOK {
		return errors.New("fixture logout failed")
	}
	return nil
}

func (p *chromeFixtureProcess) ClearFixtureCookie(ctx context.Context) error {
	return p.bridge.rpc(ctx, "clear_fixture_cookie", map[string]any{"owner": p.owner}, nil)
}

func (p *chromeFixtureProcess) FixtureInventory(ctx context.Context) (windows, bootstrap int, err error) {
	var result struct {
		WindowCount    int `json:"windowCount"`
		BootstrapCount int `json:"bootstrapCount"`
	}
	if err = p.bridge.rpc(ctx, "inventory", map[string]any{"owner": p.owner}, &result); err != nil {
		return 0, 0, err
	}
	return result.WindowCount, result.BootstrapCount, nil
}

func (p *chromeFixtureProcess) FixtureBootstrapWindows(ctx context.Context) ([]bool, error) {
	var result struct {
		Exited    bool   `json:"exited"`
		Bootstrap []bool `json:"bootstrap"`
	}
	if err := p.bridge.rpc(ctx, "inventory", map[string]any{"owner": p.owner}, &result); err != nil {
		p.transportUnknown.Store(true)
		return nil, err
	}
	if result.Exited || len(result.Bootstrap) == 0 {
		return nil, errors.New("fixture window inventory is unknown")
	}
	return result.Bootstrap, nil
}

func (p *chromeFixtureProcess) CloseFixtureWindow(ctx context.Context, index int) error {
	flags, err := p.FixtureBootstrapWindows(ctx)
	if err != nil {
		return fmt.Errorf("fixture close-window inventory: %w", err)
	}
	if index < 0 || index >= len(flags) {
		return errors.New("fixture window index is invalid")
	}
	if len(flags) == 1 {
		// This fixture-only actor signal models the user's explicit final-window
		// close so Coordinator can retry a naturally exiting headed owner.
		p.retiring.Store(true)
	}
	if err = p.bridge.rpc(ctx, "close_window", map[string]any{"owner": p.owner, "index": index}, nil); err != nil {
		members, jobErr := p.bridge.jobPIDs()
		chromePresent := false
		for _, member := range members {
			chromePresent = chromePresent || member == p.pid
		}
		return fmt.Errorf("fixture close-window request (root-exit=%t job-query-complete=%t job-members=%d chrome-in-job=%t): %w",
			p.rootExited.Load(), jobErr == nil, len(members), chromePresent, err)
	}
	return nil
}

func (p *chromeFixtureProcess) DisconnectFixtureTransport(ctx context.Context) error {
	p.transportUnknown.Store(true)
	if err := p.bridge.rpc(ctx, "disconnect", map[string]any{"owner": p.owner}, nil); err != nil {
		return err
	}
	return nil
}

func (b *chromeFixtureBridge) collect(ctx context.Context, owner int) error {
	var result struct {
		Status int  `json:"status"`
		Item   bool `json:"item"`
	}
	if err := b.rpc(ctx, "collect", map[string]any{"owner": owner}, &result); err != nil {
		return err
	}
	if result.Status != http.StatusOK || !result.Item {
		return errors.New("fixture collection failed")
	}
	return nil
}

func (p *chromeFixtureProcess) verifyNaturalExit() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	deadline := time.NewTicker(50 * time.Millisecond)
	defer deadline.Stop()
	for {
		if err := p.bridge.assertBridgeOnly(ctx); err == nil {
			p.exited.Store(true)
			p.once.Do(func() {
				p.done <- nil
				close(p.done)
			})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
		}
	}
}

func (b *chromeFixtureBridge) jobPIDs() ([]uint32, error) {
	b.jobMu.Lock()
	defer b.jobMu.Unlock()
	if b.job == 0 {
		return nil, errors.New("fixture Job Object is unavailable")
	}
	const pidOffset = 8
	size := pidOffset + chromeFixtureMaxJobProcesses*int(unsafe.Sizeof(uintptr(0)))
	buffer := make([]byte, size)
	var returned uint32
	if err := windows.QueryInformationJobObject(b.job, chromeFixtureJobInfoBasicProcessIDList,
		uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer)), &returned); err != nil {
		return nil, err
	}
	assigned := *(*uint32)(unsafe.Pointer(&buffer[0]))
	listed := *(*uint32)(unsafe.Pointer(&buffer[4]))
	if listed > chromeFixtureMaxJobProcesses || listed != assigned || returned < uint32(pidOffset+int(listed)*int(unsafe.Sizeof(uintptr(0)))) {
		return nil, errors.New("fixture Job Object membership is invalid")
	}
	pointers := unsafe.Slice((*uintptr)(unsafe.Pointer(&buffer[pidOffset])), int(listed))
	pids := make([]uint32, 0, len(pointers))
	for _, pid := range pointers {
		if pid == 0 || pid > uintptr(^uint32(0)) {
			return nil, errors.New("fixture Job Object returned an invalid process")
		}
		pids = append(pids, uint32(pid))
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	return pids, nil
}

func (b *chromeFixtureBridge) assertBridgeOnly(ctx context.Context) error {
	for {
		pids, err := b.jobPIDs()
		if err == nil && len(pids) == 1 && pids[0] == b.pid {
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return err
			}
			return errors.New("fixture Chrome descendants remain or ownership is unknown")
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (b *chromeFixtureBridge) containsPID(pid uint32) error {
	pids, err := b.jobPIDs()
	if err != nil {
		return err
	}
	for _, value := range pids {
		if value == pid {
			return nil
		}
	}
	return errors.New("Chrome PID not in fixture Job Object")
}

func (b *chromeFixtureBridge) terminateOwnedTree() {
	b.termMu.Lock()
	defer b.termMu.Unlock()
	b.jobMu.Lock()
	job := b.job
	b.jobMu.Unlock()
	if job == 0 {
		return
	}
	if err := windows.TerminateJobObject(job, 1); err != nil {
		b.mu.Lock()
		b.cleanupErr = errors.New("fixture owned tree termination failed")
		b.mu.Unlock()
		return
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pids, err := b.jobPIDs()
		if err == nil && len(pids) == 0 {
			_ = b.stdin.Close()
			b.jobMu.Lock()
			if b.job == job {
				_ = windows.CloseHandle(job)
				b.job = 0
				b.verifiedEmpty = true
			}
			b.jobMu.Unlock()
			b.closeOnce.Do(func() { close(b.closed) })
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	b.mu.Lock()
	b.cleanupErr = errors.New("fixture owned tree cleanup is unverified")
	b.mu.Unlock()
}

func (b *chromeFixtureBridge) cleanupError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cleanupErr
}

func (b *chromeFixtureBridge) cleanupVerified() bool {
	b.jobMu.Lock()
	defer b.jobMu.Unlock()
	return b.job == 0 && b.verifiedEmpty
}

func (b *chromeFixtureBridge) closeJobAfterVerifiedEmpty() error {
	pids, err := b.jobPIDs()
	if err != nil || len(pids) != 0 {
		return errors.New("fixture owned tree cleanup is unverified")
	}
	b.jobMu.Lock()
	defer b.jobMu.Unlock()
	if b.job == 0 {
		return nil
	}
	if err = windows.CloseHandle(b.job); err != nil {
		return errors.New("fixture Job Object close failed")
	}
	b.job = 0
	return nil
}

func (b *chromeFixtureBridge) stopBeforeChrome() {
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		select {
		case <-b.closed:
		case <-time.After(5 * time.Second):
		}
	}
	b.jobMu.Lock()
	if b.job != 0 {
		_ = windows.CloseHandle(b.job)
		b.job = 0
	}
	b.jobMu.Unlock()
}
