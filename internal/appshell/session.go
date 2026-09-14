package appshell

import (
	"context"
	"errors"
	"sync"
	"time"
)

type sessionWindow interface {
	Done() <-chan error
	Terminate()
	CloseForRetry(context.Context) error
	OpenExtensionsPage(context.Context) error
}

// Session distinguishes a user-requested replacement from normal window exit.
// It alone launches replacements, after the previous ownership boundary drains.
type Session struct {
	mu      sync.Mutex
	window  sessionWindow
	startup *Startup
	done    chan error
	cancel  context.CancelFunc
}

func LaunchSession(ctx context.Context, options LaunchOptions, attach func(*Startup), report func(error)) (*Session, error) {
	return launchSession(ctx, options, attach, report, func(ctx context.Context, options LaunchOptions) (sessionWindow, error) { return Launch(ctx, options) }, func(s *Startup) { s.Start(report) })
}

func launchSession(ctx context.Context, options LaunchOptions, attach func(*Startup), report func(error), launch func(context.Context, LaunchOptions) (sessionWindow, error), show func(*Startup)) (*Session, error) {
	startup, err := NewStartup(options.URL)
	if err != nil {
		return nil, err
	}
	startup.onReady = options.OnStartupReady
	if attach != nil {
		attach(startup)
	}
	if !options.SuppressStartupWindow {
		show(startup)
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Session{startup: startup, done: make(chan error, 1), cancel: cancel}
	target := options.URL
	options.Startup = nil // The session, not an individual Chromium, owns status.
	options.URL = startup.LaunchURL(target)
	s.window, err = launch(ctx, options)
	if err != nil {
		startup.Stop()
		cancel()
		return nil, err
	}
	go s.run(ctx, target, options, launch, report)
	return s, nil
}

func (s *Session) run(ctx context.Context, target string, options LaunchOptions, launch func(context.Context, LaunchOptions) (sessionWindow, error), report func(error)) {
	defer close(s.done)
	defer s.startup.Stop()
	defer s.cancel()
	var blocked error
	var pendingCleanup sessionWindow
	pendingExited := false
	for {
		s.mu.Lock()
		window := s.window
		s.mu.Unlock()
		var exited <-chan error
		if window != nil {
			exited = window.Done()
		}
		if pendingCleanup != nil {
			window = pendingCleanup
			if !pendingExited {
				exited = pendingCleanup.Done()
			}
		}
		var recoveryClosed <-chan struct{}
		if window == nil || (pendingCleanup != nil && pendingExited) {
			recoveryClosed = s.startup.stopped
		}
		select {
		case <-ctx.Done():
			if window != nil {
				window.Terminate()
			}
			return
		case err := <-exited:
			if pendingCleanup != nil {
				pendingExited = true
				continue
			}
			s.done <- err // Ordinary close retains the existing Sidecar shutdown.
			return
		case <-recoveryClosed:
			if window != nil {
				window.Terminate()
			}
			return // No Chromium and no recovery surface: end Sidecar normally.
		case <-s.startup.retry:
		}
		// A close/shutdown already observed wins over a queued retry click.
		if ctx.Err() != nil {
			if window != nil {
				window.Terminate()
			}
			return
		}
		select {
		case err := <-exited:
			if pendingCleanup != nil {
				pendingExited = true
			} else {
				s.done <- err
				return
			}
		default:
		}
		if blocked != nil {
			if report != nil {
				report(blocked)
			}
			s.startup.retryComplete(true)
			continue
		}
		s.mu.Lock()
		s.window = nil
		s.mu.Unlock() // Disable extension actions during replacement.
		if window != nil {
			closeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := window.CloseForRetry(closeCtx)
			cancel()
			if err != nil {
				if report != nil {
					report(err)
				}
				s.startup.retryComplete(true)
				if pendingCleanup == nil {
					pendingExited = false
				}
				pendingCleanup = window // No new shell until cleanup is verified.
				continue
			}
			pendingCleanup = nil
			pendingExited = false
		}
		if ctx.Err() != nil {
			return
		}
		if err := s.startup.resetAttempt(target); err != nil {
			if report != nil {
				report(err)
			}
			s.startup.retryComplete(true)
			continue
		}
		options.URL = s.startup.LaunchURL(target)
		// Allow the newly launched page to acknowledge while native icon setup
		// completes, but keep duplicate button requests gated until Launch returns.
		next, err := launch(ctx, options)
		if err != nil {
			if report != nil {
				report(err)
			}
			if errors.Is(err, errCleanupUnverified) {
				blocked = err
			}
			s.startup.retryComplete(true)
			continue // Remain available; another attempt requires another click.
		}
		s.mu.Lock()
		s.window = next
		s.mu.Unlock()
		s.startup.retryComplete(false)
	}
}

func (s *Session) Done() <-chan error {
	if s == nil {
		return nil
	}
	return s.done
}
func (s *Session) PID() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if window, ok := s.window.(interface{ PID() int }); ok {
		return window.PID()
	}
	return 0
}
func (s *Session) Cancel() {
	if s != nil {
		s.cancel()
	}
}
func (s *Session) Terminate() {
	if s != nil {
		s.cancel()
		<-s.done
	}
}
func (s *Session) OpenExtensionsPage(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.window == nil {
		return (&Window{}).OpenExtensionsPage(ctx)
	}
	return s.window.OpenExtensionsPage(ctx)
}
