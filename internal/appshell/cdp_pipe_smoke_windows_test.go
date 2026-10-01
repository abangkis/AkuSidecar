//go:build windows

package appshell

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cdpPipeSmokeChromeVersion = "Chrome/152.0.7977.54"

type cdpPipeFrame struct {
	data []byte
	err  error
}

func TestCDPPipeWindowsSmoke(t *testing.T) {
	chrome := os.Getenv("AKU_CDP_PIPE_SMOKE_CHROME")
	if chrome == "" {
		t.Skip("explicit staged Chrome path required")
	}
	chrome, err := filepath.Abs(chrome)
	if err != nil {
		t.Fatal("resolve staged Chrome path")
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve project-local smoke directory")
	}
	artifactRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "build", "cdp-pipe-smoke"))
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("create project-local smoke directory")
	}
	profile, err := os.MkdirTemp(artifactRoot, "profile-")
	if err != nil {
		t.Fatal("create disposable Chrome profile")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hostServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pipe-host" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>CDP pipe smoke</title><p>fixture</p>"))
	}))
	defer hostServer.Close()
	hostURL := hostServer.URL + "/pipe-host"

	var childInputRead, parentInputWrite windows.Handle
	var parentOutputRead, childOutputWrite windows.Handle
	var parentInput, parentOutput *os.File
	var command *exec.Cmd
	var owner processOwnership
	var waitCh chan error
	rootExited, complete := false, false
	readerStop := make(chan struct{})
	var readerFrames <-chan cdpPipeFrame
	t.Cleanup(func() {
		close(readerStop)
		if !complete && owner.job != 0 {
			_ = windows.TerminateJobObject(owner.job, 1)
			if waitCh != nil && !rootExited {
				select {
				case <-waitCh:
				case <-time.After(3 * time.Second):
					t.Errorf("test Chrome root did not stop after Job termination")
				}
			}
			cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if !waitForJobDrain(cleanupCtx, &owner) {
				t.Errorf("test Chrome Job did not drain after termination")
			}
		}
		if owner.job != 0 {
			owner.close()
		}
		if parentInput != nil {
			_ = parentInput.Close()
		}
		if parentOutput != nil {
			_ = parentOutput.Close()
		}
		for _, handle := range []windows.Handle{childInputRead, parentInputWrite, parentOutputRead, childOutputWrite} {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
		_ = os.RemoveAll(profile)
	})

	attributes := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	if err := windows.CreatePipe(&childInputRead, &parentInputWrite, attributes, 0); err != nil {
		t.Fatal("create Chrome input pipe")
	}
	if err := windows.CreatePipe(&parentOutputRead, &childOutputWrite, attributes, 0); err != nil {
		t.Fatal("create Chrome output pipe")
	}
	if err := windows.SetHandleInformation(parentInputWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		t.Fatal("restrict Chrome input pipe inheritance")
	}
	if err := windows.SetHandleInformation(parentOutputRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		t.Fatal("restrict Chrome output pipe inheritance")
	}
	inputText, err := inheritedHandleText(childInputRead)
	if err != nil {
		t.Fatal("serialize Chrome input handle")
	}
	outputText, err := inheritedHandleText(childOutputWrite)
	if err != nil {
		t.Fatal("serialize Chrome output handle")
	}
	parentInput = os.NewFile(uintptr(parentInputWrite), "cdp-parent-input")
	parentInputWrite = 0
	parentOutput = os.NewFile(uintptr(parentOutputRead), "cdp-parent-output")
	parentOutputRead = 0
	reader := bufio.NewReaderSize(parentOutput, 1<<20)
	readerFrames = startCDPPipeReader(reader, readerStop)

	options := LaunchOptions{
		UserDataDir:    profile,
		URL:            hostURL,
		StartMinimized: true,
		ExtraArgs: []string{
			"--start-minimized",
			"--remote-debugging-pipe",
			"--remote-debugging-io-pipes=" + inputText + "," + outputText,
		},
	}
	command = exec.Command(chrome, buildArgs(options)...)
	prepareCommand(command)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		AdditionalInheritedHandles: []syscall.Handle{
			syscall.Handle(childInputRead), syscall.Handle(childOutputWrite),
		},
	}
	owner, err = newProcessOwnership()
	if err != nil {
		t.Fatal("create Chrome Job")
	}
	if err := command.Start(); err != nil {
		t.Fatal("start test Chrome")
	}
	if err := owner.attach(command); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		rootExited = true
		t.Fatal("assign test Chrome to Job")
	}
	waitCh = make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	if err := windows.CloseHandle(childInputRead); err != nil {
		t.Fatal("close parent copy of Chrome input handle")
	}
	childInputRead = 0
	if err := windows.CloseHandle(childOutputWrite); err != nil {
		t.Fatal("close parent copy of Chrome output handle")
	}
	childOutputWrite = 0
	if err := owner.minimizeInitialWindow(ctx, command.Process.Pid); err != nil {
		t.Fatal("minimize test Chrome app window")
	}

	versionResult, err := pipeCommand(ctx, parentInput, readerFrames, 1, "Browser.getVersion", nil)
	if err != nil {
		t.Fatal("read Chrome version through private pipe")
	}
	var version struct {
		Product string `json:"product"`
	}
	if err := json.Unmarshal(versionResult, &version); err != nil || version.Product != cdpPipeSmokeChromeVersion {
		t.Fatal("private pipe did not report the required Chrome version")
	}
	t.Logf("chrome_version=%s", version.Product)

	var targets struct {
		TargetInfos []struct {
			ID   string `json:"targetId"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"targetInfos"`
	}
	var hostTarget string
	var pageCount, emptyURLCount, expectedCount int
	probeFailed := false
	probeDeadline := time.Now().Add(2 * time.Second)
	nextID := 2
	for time.Now().Before(probeDeadline) {
		probeCtx, stopProbe := context.WithDeadline(ctx, probeDeadline)
		targetResult, err := pipeCommand(probeCtx, parentInput, readerFrames, nextID, "Target.getTargets", nil)
		stopProbe()
		nextID++
		if err != nil {
			probeFailed = true
			break
		}
		if err := json.Unmarshal(targetResult, &targets); err != nil {
			t.Fatal("decode test Chrome targets")
		}
		hostTarget, pageCount, emptyURLCount, expectedCount = "", 0, 0, 0
		for _, target := range targets.TargetInfos {
			if target.Type != "page" {
				continue
			}
			pageCount++
			if target.URL == "" {
				emptyURLCount++
			}
			if target.URL == hostURL {
				expectedCount++
				hostTarget = target.ID
			}
		}
		if expectedCount > 1 {
			t.Fatal("multiple exact fixture page targets; refusing ambiguous close")
		}
		if expectedCount == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Chrome target probe exceeded test deadline")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if expectedCount != 1 || hostTarget == "" {
		t.Fatalf("exact fixture page target unavailable within bounded probe: pages=%d empty_urls=%d exact_matches=%d probe_failed=%t", pageCount, emptyURLCount, expectedCount, probeFailed)
	}
	closeResult, err := pipeCommand(ctx, parentInput, readerFrames, nextID, "Target.closeTarget", map[string]string{"targetId": hostTarget})
	if err != nil {
		t.Fatal("close exact fixture app target through private pipe")
	}
	var closed struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(closeResult, &closed); err != nil || !closed.Success {
		t.Fatal("Chrome rejected exact app target close")
	}

	select {
	case waitErr := <-waitCh:
		rootExited = true
		if waitErr != nil {
			t.Fatal("Chrome root did not exit cleanly after closing its only app target")
		}
	case <-ctx.Done():
		t.Fatal("Chrome root remained alive after closing its only app target")
	}
	if !waitForJobDrain(ctx, &owner) {
		t.Fatal("Chrome Job did not naturally drain while parent pipe handles remained open")
	}
	if parentInput == nil || parentOutput == nil {
		t.Fatal("parent pipe handles were not retained through natural shutdown")
	}
	complete = true
	t.Log("pipe_roundtrip=true exact_target_closed=true root_exited=true job_empty=true parent_pipes_retained=true")
}

func inheritedHandleText(handle windows.Handle) (string, error) {
	value := uint64(handle)
	if handle == 0 || value > uint64(^uint32(0)) {
		return "", errors.New("invalid inherited handle")
	}
	return strconv.FormatUint(value, 10), nil
}

func startCDPPipeReader(reader *bufio.Reader, stop <-chan struct{}) <-chan cdpPipeFrame {
	frames := make(chan cdpPipeFrame, 16)
	go func() {
		for {
			frame, err := reader.ReadSlice(0)
			if err == nil {
				if len(frame) == 0 || len(frame) > 1<<20 {
					err = errors.New("CDP frame exceeds limit")
				} else {
					frame = append([]byte(nil), frame[:len(frame)-1]...)
				}
			}
			select {
			case frames <- cdpPipeFrame{data: frame, err: err}:
			case <-stop:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return frames
}

func pipeCommand(ctx context.Context, input *os.File, frames <-chan cdpPipeFrame, id int, method string, params any) (json.RawMessage, error) {
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > 64*1024 {
		return nil, errors.New("invalid private pipe request")
	}
	encoded = append(encoded, 0)
	if _, err := input.Write(encoded); err != nil {
		return nil, err
	}
	for readCount := 0; readCount < 64; readCount++ {
		select {
		case frame := <-frames:
			if frame.err != nil {
				return nil, frame.err
			}
			var response struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(frame.data, &response) != nil || response.ID != id {
				continue
			}
			if len(response.Error) != 0 {
				return nil, errors.New("private pipe command rejected")
			}
			return response.Result, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("too many unsolicited private pipe messages")
}

func waitForJobDrain(ctx context.Context, owner *processOwnership) bool {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		drained, err := owner.naturallyDrained()
		if err != nil {
			return false
		}
		if drained {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
