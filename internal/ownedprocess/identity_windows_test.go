//go:build windows

package ownedprocess

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestProcessIdentityPinsCreation(t *testing.T) {
	actual, err := ProcessIdentity(uint32(os.Getpid()))
	if err != nil || actual.PID != uint32(os.Getpid()) || len(actual.Created) != 16 {
		t.Fatal("process identity unavailable")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	fromHandle, err := IdentityFromHandle(h)
	if err != nil || actual != fromHandle {
		t.Fatal("process creation pin changed")
	}
	if _, err := IdentityFromHandle(0); err == nil {
		t.Fatal("missing handle accepted")
	}
	if _, err := ProcessIdentity(0); err == nil {
		t.Fatal("missing process accepted")
	}
}

func TestDirectoryIdentityRejectsAliasAndFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "profile")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := DirectoryID(dir)
	if err != nil || first == "" {
		t.Fatal("directory identity unavailable")
	}
	if again, err := DirectoryID(dir); err != nil || again != first {
		t.Fatal("directory identity unstable")
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(root, "missing")} {
		if _, err := DirectoryID(path); err == nil {
			t.Fatal("non-directory accepted")
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Log("symlink privilege unavailable; alias subcase skipped")
		return
	}
	if _, err := DirectoryID(alias); err == nil {
		t.Fatal("symlink profile accepted")
	}
}

func TestJobPolicyAndUnknownHandles(t *testing.T) {
	job, err := NewJob()
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	var flags uint32
	result, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation").Call(uintptr(job), uintptr(unsafe.Pointer(&flags)))
	if result == 0 || flags&windows.HANDLE_FLAG_INHERIT != 0 {
		t.Fatal("Job handle inheritable")
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil) != nil || info.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatal("Job lacks lifetime cleanup")
	}
	if pids, err := JobPIDs(job); err != nil || len(pids) != 0 {
		t.Fatal("empty Job accounting failed")
	}
	if _, err := JobPIDs(0); err == nil {
		t.Fatal("missing Job accepted")
	}
	if _, err := Membership(0, job); err == nil {
		t.Fatal("unknown process membership accepted")
	}
	if _, err := Membership(windows.CurrentProcess(), 0); err == nil {
		t.Fatal("unknown Job membership accepted")
	}
}

func TestJobCloseKillsOnlyOwnedChild(t *testing.T) {
	job, err := NewJob()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if job != 0 {
			windows.CloseHandle(job)
		}
	}()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestOwnedProcessChild$")
	cmd.Env = append(os.Environ(), "AKU_OWNED_PROCESS_TEST_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	defer func() {
		_ = cmd.Process.Kill()
		select {
		case <-waited:
		case <-time.After(time.Second):
		}
	}()
	if err = Assign(job, uint32(cmd.Process.Pid)); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(stdout); ready <- scanner.Scan() && scanner.Text() == "ready" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("owned test child not ready")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owned test child startup stalled")
	}
	pids, err := JobPIDs(job)
	if err != nil || len(pids) != 1 || pids[0] != uint32(cmd.Process.Pid) {
		t.Fatal("owned child accounting mismatch")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if member, err := Membership(h, job); err != nil || !member {
		t.Fatal("owned child missing from Job")
	}
	if member, err := Membership(windows.CurrentProcess(), job); err != nil || member {
		t.Fatal("unowned parent included in child Job")
	}
	if err = windows.CloseHandle(job); err != nil {
		t.Fatal(err)
	}
	job = 0
	if status, err := windows.WaitForSingleObject(h, 3000); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("child outlived lifetime Job")
	}
}

func TestOwnedProcessChild(t *testing.T) {
	if os.Getenv("AKU_OWNED_PROCESS_TEST_CHILD") != "1" {
		t.Skip("private accountless subprocess helper")
	}
	fmt.Fprintln(os.Stdout, "ready")
	time.Sleep(20 * time.Second)
}
