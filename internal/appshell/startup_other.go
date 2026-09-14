//go:build !windows

package appshell

// The fallback is a native Windows surface; other platforms keep the HTML
// watchdog and the same acknowledgement contract without opening another app.
func (s *Startup) Start(reportError func(error)) {}
