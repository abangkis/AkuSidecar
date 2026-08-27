//go:build windows

package appshell

import (
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func spawnTestProcess(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	command := exec.Command(name, args...)
	if err := command.Start(); err != nil {
		t.Skipf("could not spawn %s: %v", name, err)
	}
	t.Cleanup(func() {
		if command.Process != nil && command.ProcessState == nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
	})
	return command
}

func attachTestOwnership(t *testing.T, command *exec.Cmd) processOwnership {
	t.Helper()
	owner, err := newProcessOwnership()
	if err != nil {
		t.Fatalf("newProcessOwnership: %v", err)
	}
	if err := owner.attach(command); err != nil {
		t.Fatalf("attach: %v", err)
	}
	return owner
}

func waitProcessExit(t *testing.T, command *exec.Cmd, timeout time.Duration) bool {
	t.Helper()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	return err == nil && event == uint32(windows.WAIT_OBJECT_0)
}

// The graceful stage must terminate a windowed process through WM_CLOSE
// without ever reaching the Job Object kill. A message box has a real
// top-level window and exits cleanly when it is closed.
func TestTerminateClosesWindowedProcessGracefully(t *testing.T) {
	command := spawnTestProcess(t, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.Form; $f.Text = 'aku-appshell-test'; $null = $f.ShowDialog()")
	owner := attachTestOwnership(t, command)
	defer owner.close()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if visibleTopLevelWindow(uint32(command.Process.Pid)) != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		_, _ = command.Process.Wait()
		close(done)
	}()
	go func() {
		owner.terminate(command.Process)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("terminate did not close the windowed process gracefully")
	}
}

// A windowless owned process must still die through the Job Object fallback.
func TestTerminateKillsWindowlessProcessViaJob(t *testing.T) {
	command := spawnTestProcess(t, "cmd", "/c", "ping", "-n", "60", "127.0.0.1")
	owner := attachTestOwnership(t, command)
	defer owner.close()

	go func() {
		_, _ = command.Process.Wait()
	}()
	owner.terminate(command.Process)
	if !waitProcessExit(t, command, 15*time.Second) {
		t.Fatal("terminate fallback did not stop the owned process")
	}
}
