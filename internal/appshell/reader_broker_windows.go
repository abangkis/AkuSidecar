//go:build windows

package appshell

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Session) ServeReaderBroker(ctx context.Context, handle func(context.Context, readerbroker.Request, func(readerbroker.Target) (readerbroker.Reply, error)) error) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.ctx != nil {
		stop := context.AfterFunc(s.ctx, cancel)
		defer stop()
	}
	return readerbroker.Serve(runCtx, s.authorizeReaderHelper, handle)
}

func (s *Session) authorizeReaderHelper(pid uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.window.(*Window)
	if !ok || w == nil {
		return errors.New("UI window unavailable")
	}
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	select {
	case <-w.closed:
		return errors.New("UI exited")
	default:
	}
	if pid == 0 {
		return errors.New("UI helper PID missing")
	}
	parent, err := readerbroker.ParentPID(pid)
	if err != nil || parent != uint32(w.PID()) {
		return errors.New("helper was not directly launched by UI")
	}
	image, err := readerbroker.ProcessImage(pid)
	self, selfErr := os.Executable()
	if err != nil || selfErr != nil || !strings.EqualFold(image, filepath.Join(filepath.Dir(self), "aku-reader-broker.exe")) {
		return errors.New("helper image mismatch")
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(p)
	if !processBelongsToJob(p, w.owner.job) {
		return errors.New("helper outside UI Job Object")
	}
	var created, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(p, &created, &exit, &kernel, &user); err != nil {
		return err
	}
	age := time.Since(time.Unix(0, created.Nanoseconds()))
	if age < 0 || age > readerbroker.Lifetime {
		return errors.New("helper lifetime expired")
	}
	return nil
}
