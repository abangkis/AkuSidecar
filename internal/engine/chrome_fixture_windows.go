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

// Private QA reuse scope; it grants no Store or account-verifier authority.
type chromeFixtureQAProfileLease interface {
	validateFixtureProfile(string) error
	fixtureProfileScope() (string, bool)
}

type chromeFixtureConfig struct {
	NodePath            string
	ChromePath          string
	Root                string
	Origin              string
	DriverPath          string
	NativeReaderStartup bool
	processGrant        *processFixtureGrant
	processStart        *processFixtureStart
	qaProfileLease      chromeFixtureQAProfileLease
	beforeJobAssignment func(uint32, windows.Handle) error
	beforeReadySignal   func(uint32, windows.Handle) error
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
	nodeWaited    chan struct{}
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
	rootHandle          windows.Handle
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
	if config.beforeReadySignal != nil && (config.processGrant == nil || config.processGrant.Reopen) {
		return nil, errors.New("fixture pre-ready barrier requires fresh private process grant")
	}
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
		exitsEarly: map[int]struct{}{}, closed: make(chan struct{}), nodeWaited: make(chan struct{})}
	ready := make(chan error, 1)
	go b.readOutput(stdout, ready)
	wait := time.NewTimer(8 * time.Second)
	defer wait.Stop()
	bootstrap := map[string]any{"op": "fixture_bootstrap", "withholdReady": config.beforeReadySignal != nil}
	if config.beforeReadySignal != nil {
		bootstrap["nonce"] = config.processGrant.Nonce
		bootstrap["parentPID"] = os.Getpid()
		bootstrap["controllerPID"] = config.processGrant.Parent.PID
		bootstrap["controllerCreated"] = config.processGrant.Parent.Created
	}
	if err = json.NewEncoder(stdin).Encode(bootstrap); err != nil {
		b.stopBeforeChrome()
		return nil, errors.New("fixture bootstrap input unavailable")
	}
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
	if config.beforeJobAssignment != nil {
		if config.processGrant == nil || config.processGrant.Reopen {
			b.stopBeforeChrome()
			return nil, errors.New("fixture startup barrier requires fresh private process grant")
		}
		if err = config.beforeJobAssignment(b.pid, job); err != nil {
			b.stopBeforeChrome()
			return nil, err
		}
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
	if config.processGrant != nil {
		grant := config.processGrant
		validateScope := func() error {
			if config.processStart != nil {
				return validateProcessFixtureStartScope(config.Root, *config.processStart)
			}
			return validateProcessFixtureGrant(config.Root, grant)
		}
		if err = validateScope(); err != nil {
			b.terminateOwnedTree()
			return nil, errors.New("fixture process profile identity changed before scope")
		}
		preserve := grant.Reopen || config.processStart != nil && config.processStart.PolicyRecovery != nil
		if err = b.rpc(ctx, "process_profile_scope", map[string]any{"nonce": grant.Nonce, "reopen": preserve, "prepared": grant.profilePrepared}, nil); err != nil {
			b.terminateOwnedTree()
			return nil, fmt.Errorf("fixture process profile scope: %w", err)
		}
		if err = validateScope(); err != nil {
			b.terminateOwnedTree()
			return nil, errors.New("fixture process profile identity changed during scope")
		}
	}
	if config.qaProfileLease != nil {
		if err = config.qaProfileLease.validateFixtureProfile(config.Root); err != nil {
			b.terminateOwnedTree()
			return nil, errors.New("retained QA profile scope changed")
		}
		nonce, reuse := config.qaProfileLease.fixtureProfileScope()
		if err = b.rpc(ctx, "qa_profile_scope", map[string]any{"nonce": nonce, "reuse": reuse}, nil); err != nil {
			b.terminateOwnedTree()
			return nil, errors.New("retained QA profile scope rejected")
		}
		if err = config.qaProfileLease.validateFixtureProfile(config.Root); err != nil {
			b.terminateOwnedTree()
			return nil, errors.New("retained QA profile scope changed")
		}
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
	if config.qaProfileLease != nil {
		retainedDriver := samePath(config.DriverPath, filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-account-settings-driver.mjs")) ||
			samePath(config.DriverPath, filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-bootstrap-driver.mjs"))
		if config.processGrant != nil || config.processStart != nil || config.NativeReaderStartup ||
			!retainedDriver {
			return "", errors.New("retained QA scope cannot mix fixture permissions")
		}
		if err = config.qaProfileLease.validateFixtureProfile(root); err != nil {
			return "", errors.New("retained QA profile identity unavailable")
		}
		if samePath(config.DriverPath, filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-bootstrap-driver.mjs")) {
			if _, existing := config.qaProfileLease.fixtureProfileScope(); !existing {
				return "", errors.New("bootstrap probe requires the existing retained QA profile")
			}
		}
	}
	if config.processGrant != nil {
		if config.processStart != nil {
			err = validateProcessFixtureStartScope(root, *config.processStart)
		} else {
			err = validateProcessFixtureGrant(root, config.processGrant)
		}
		if err != nil {
			return "", err
		}
	}
	if _, err = os.Lstat(profile); !errors.Is(err, os.ErrNotExist) {
		preserve := config.processGrant != nil && (config.processGrant.Reopen || config.processStart != nil && config.processStart.PolicyRecovery != nil)
		prepared := config.processGrant != nil && config.processGrant.profilePrepared
		valid := false
		if config.processGrant != nil {
			if config.processStart != nil {
				valid = validateProcessFixtureStartScope(root, *config.processStart) == nil
			} else {
				valid = validateProcessFixtureGrant(root, config.processGrant) == nil
			}
		}
		retained := config.qaProfileLease != nil && config.qaProfileLease.validateFixtureProfile(root) == nil
		if !retained && (config.processGrant == nil || (!preserve && !prepared) || !valid) {
			return "", errors.New("fixture profile must be new")
		}
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

func chromeFixtureJobMembership(process, job windows.Handle) (bool, error) {
	if process == 0 || job == 0 {
		return false, errors.New("fixture Job membership handles unavailable")
	}
	var inJob int32
	result, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
	if result == 0 {
		return false, errors.New("fixture Job membership query unavailable")
	}
	return inJob != 0, nil
}

func (b *chromeFixtureBridge) readOutput(stdout io.Reader, ready chan<- error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	readySent := false
	stallSeen := false
	for scanner.Scan() {
		var message struct {
			Ready          bool            `json:"ready"`
			StartupStalled bool            `json:"startup_stalled"`
			ID             int64           `json:"id"`
			OK             bool            `json:"ok"`
			Code           string          `json:"code"`
			Event          string          `json:"event"`
			Owner          int             `json:"owner"`
			Result         json.RawMessage `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			if !readySent {
				ready <- errors.New("invalid fixture bridge handshake")
				readySent = true
			}
			continue
		}
		if message.StartupStalled {
			if readySent || stallSeen || b.config.beforeReadySignal == nil {
				if !readySent {
					ready <- errors.New("invalid fixture startup marker")
					readySent = true
				}
				continue
			}
			stallSeen = true
			if err := b.config.beforeReadySignal(b.pid, b.job); err != nil {
				ready <- err
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
	close(b.nodeWaited)
	// EOF alone is not process death. After Wait proves the Node bridge exited,
	// release its exact owned Job before signalling any Chrome owner as done.
	b.finishBridgeExit()
}

func (b *chromeFixtureBridge) finishBridgeExit() {
	b.mu.Lock()
	owners := make([]*chromeFixtureProcess, 0, len(b.owners))
	for _, owner := range b.owners {
		owners = append(owners, owner)
		if !owner.exited.Load() {
			owner.transportUnknown.Store(true)
		}
	}
	b.mu.Unlock()
	// Native windows are read only and limited to currently verified Job peers.
	var handles []uintptr
	if pids, err := b.jobPIDs(); err == nil {
		for _, pid := range pids {
			handles = append(handles, processFixtureWindows(pid)...)
		}
	}
	b.terminateOwnedTree()
	if b.cleanupError() != nil || !b.cleanupVerified() {
		return
	}
	// Job accounting can become empty before the root process handle signals.
	// Pin the handle obtained while its PID belonged to this Job; never reopen
	// a possibly recycled PID after bridge death to manufacture exit evidence.
	for _, owner := range owners {
		if owner.rootHandle != 0 {
			status, waitErr := windows.WaitForSingleObject(owner.rootHandle, 3000)
			if waitErr != nil || status != windows.WAIT_OBJECT_0 {
				b.mu.Lock()
				b.cleanupErr = errors.New("fixture Chrome root exit is unverified after bridge exit")
				b.mu.Unlock()
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if processFixtureWait(ctx, func() bool { return processFixtureWindowsGone(handles) }) != nil {
		b.mu.Lock()
		b.cleanupErr = errors.New("fixture bridge exited but native window release is unverified")
		b.mu.Unlock()
		return
	}
	for _, owner := range owners {
		owner.rootExited.Store(true)
		owner.exited.Store(true)
		owner.once.Do(func() { owner.done <- errors.New("fixture bridge exited"); close(owner.done) })
		if owner.rootHandle != 0 {
			_ = windows.CloseHandle(owner.rootHandle)
			owner.rootHandle = 0
		}
	}
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
	if b.config.qaProfileLease != nil && (role != "collector" || b.config.qaProfileLease.validateFixtureProfile(b.config.Root) != nil) {
		return nil, errors.New("retained QA launch scope unavailable")
	}
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
	if err := b.containsPID(launched.PID); err != nil {
		return nil, errors.New("Chrome process is outside the fixture Job Object")
	}
	rootHandle, handleErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, launched.PID)
	if handleErr != nil {
		return nil, errors.New("fixture Chrome root handle unavailable")
	}
	b.jobMu.Lock()
	ownedHandle, membershipErr := chromeFixtureJobMembership(rootHandle, b.job)
	b.jobMu.Unlock()
	if membershipErr != nil || !ownedHandle {
		_ = windows.CloseHandle(rootHandle)
		return nil, errors.New("fixture Chrome root handle is outside owned Job")
	}
	p := &chromeFixtureProcess{bridge: b, owner: launched.Owner, pid: launched.PID, rootHandle: rootHandle, role: role, nativeReaderStartup: nativeStartup, done: make(chan error, 1)}
	b.mu.Lock()
	if b.isClosed() {
		b.mu.Unlock()
		_ = windows.CloseHandle(rootHandle)
		return nil, errors.New("fixture bridge exited during Chrome launch")
	}
	b.owners[p.owner] = p
	_, exited := b.exitsEarly[p.owner]
	delete(b.exitsEarly, p.owner)
	b.mu.Unlock()
	if exited {
		p.rootExited.Store(true)
		go p.verifyNaturalExit()
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
		case <-b.nodeWaited:
		case <-time.After(5 * time.Second):
			b.mu.Lock()
			b.cleanupErr = errors.New("fixture Node process wait is unverified before Chrome startup")
			b.mu.Unlock()
		}
	}
	b.jobMu.Lock()
	if b.job != 0 {
		_ = windows.CloseHandle(b.job)
		b.job = 0
	}
	b.jobMu.Unlock()
}
