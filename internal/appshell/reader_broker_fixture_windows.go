//go:build windows && auth_session_fixture

package appshell

import (
	"context"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ReaderBrokerFixtureUI owns only the staged native fixture UI and its direct
// children. The stdin gate prevents descendants before Job assignment.
type ReaderBrokerFixtureUI struct {
	Session    *Session
	Input      io.WriteCloser
	Output     io.ReadCloser
	window     *Window
	waited     chan struct{}
	cleanup    sync.Once
	cleanupErr error
}

func LaunchReaderBrokerFixtureUI(ctx context.Context, pipe string) (*ReaderBrokerFixtureUI, error) {
	if err := readerbroker.ValidateFixturePipe(pipe); err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if root == "" || !strings.EqualFold(filepath.Dir(self), filepath.Clean(root)) || !strings.EqualFold(filepath.Base(self), "aku-sidecar.exe") {
		return nil, errors.New("fixture server staging identity rejected")
	}
	executable := filepath.Join(root, "aku-fixture-ui.exe")
	command := exec.Command(executable)
	prepareCommand(command)
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	owner, err := newProcessOwnership()
	if err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	if err = command.Start(); err != nil {
		owner.close()
		return nil, err
	}
	if err = owner.attach(command); err != nil {
		command.Process.Kill()
		command.Wait()
		owner.close()
		return nil, err
	}
	w := &Window{command: command, owner: owner, closed: make(chan struct{}), done: make(chan error, 1)}
	f := &ReaderBrokerFixtureUI{window: w, Input: input, Output: output, waited: make(chan struct{})}
	f.Session = &Session{window: w, ctx: ctx}
	go func() { err := command.Wait(); close(w.closed); w.done <- err; close(f.waited) }()
	return f, nil
}
func (f *ReaderBrokerFixtureUI) Close() error {
	f.cleanup.Do(func() {
		f.Input.Close()
		f.window.owner.terminate(f.window.command.Process)
		<-f.waited
		f.cleanupErr = f.window.owner.drain()
		if f.cleanupErr == nil {
			f.window.owner.close()
		}
		f.Output.Close()
	})
	return f.cleanupErr
}
func (s *Session) ServeReaderBrokerFixture(ctx context.Context, pipe string, handle func(context.Context, readerbroker.Request, func(readerbroker.Target) (readerbroker.Reply, error)) error) error {
	return readerbroker.ServeFixture(ctx, pipe, s.authorizeReaderHelper, handle)
}

// This borrowed binding owns no process and starts no containment monitor. It
// reuses the production HWND marker, Job membership, property and readback.
type ReaderBrokerFixtureBinding struct{ containment *captureZOrder }

func NewReaderBrokerFixtureBinding(job windows.Handle, pid uint32) (*ReaderBrokerFixtureBinding, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(process)
	if !processBelongsToJob(process, job) {
		return nil, errors.New("fixture reader is outside owned Job")
	}
	prop, _ := windows.UTF16PtrFromString("AkuBrowser.ExplicitReader." + fmt.Sprint(pid))
	return &ReaderBrokerFixtureBinding{containment: &captureZOrder{job: job, root: pid, readers: make(map[uintptr]captureReaderBinding), property: prop}}, nil
}
func (b *ReaderBrokerFixtureBinding) Prepare(ctx context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
	if !strings.HasPrefix(marker, "AkuBrowser reader split_fixture_") || len(marker) != len("AkuBrowser reader split_fixture_")+32 {
		return readerbroker.Target{}, nil, errors.New("invalid fixture marker")
	}
	return b.containment.PrepareBrokerReader(ctx, marker)
}
