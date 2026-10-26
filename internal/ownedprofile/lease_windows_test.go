//go:build windows

package ownedprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"golang.org/x/sys/windows"
)

func profileFixture(t *testing.T) (Descriptor, string) {
	t.Helper()
	parent := os.Getenv("AKU_OWNED_PROFILE_TEST_ROOT")
	if !filepath.IsAbs(parent) || !strings.EqualFold(filepath.Clean(os.TempDir()), filepath.Clean(parent)) {
		t.Skip("canonical project-owned profile test helper required")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "profiles", "synthetic")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal("synthetic profile setup failed")
	}
	rootID, err := ownedprocess.DirectoryID(root)
	if err != nil {
		t.Fatal("synthetic root identity unavailable")
	}
	directoryID, err := ownedprocess.DirectoryID(directory)
	if err != nil {
		t.Fatal("synthetic directory identity unavailable")
	}
	return Descriptor{Root: root, Directory: directory, RootID: rootID, DirectoryID: directoryID}, directory
}

func TestLeaseContentionContentAndReacquire(t *testing.T) {
	d, directory := profileFixture(t)
	lease, err := Acquire(d)
	if err != nil {
		t.Fatal("synthetic lease acquisition failed")
	}
	defer lease.Close()
	if err := lease.Validate(); err != nil || !lease.Matches(d.Root, directory) {
		t.Fatal("open lease did not validate its pinned paths")
	}

	// The exclusive lock handle rejects a second opener, while ordinary profile content remains writable.
	lockName, _ := windows.UTF16PtrFromString(filepath.Join(d.Root, lockFileName))
	if h, openErr := windows.CreateFile(lockName, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT, 0); openErr == nil {
		_ = windows.CloseHandle(h)
		t.Fatal("exclusive lease lock was not held")
	}
	if err := os.WriteFile(filepath.Join(directory, "content.txt"), []byte("synthetic"), 0600); err != nil {
		t.Fatal("profile content write was blocked by the lease")
	}
	if err := os.Rename(directory, filepath.Join(filepath.Dir(directory), "renamed")); err == nil {
		t.Fatal("pinned profile directory was renamed")
	}

	if err := lease.Close(); err != nil {
		t.Fatal("lease close failed")
	}
	if err := lease.Close(); err != nil {
		t.Fatal("lease close was not idempotent")
	}
	if lease.Validate() == nil || lease.Matches(d.Root, directory) {
		t.Fatal("closed lease remained usable")
	}
	lockInfo, err := os.Stat(filepath.Join(d.Root, lockFileName))
	if err != nil || lockInfo.Size() != 0 {
		t.Fatal("empty lock file was not retained")
	}
	reacquired, err := Acquire(d)
	if err != nil {
		t.Fatal("lease could not be reacquired after close")
	}
	if err := reacquired.Close(); err != nil {
		t.Fatal("reacquired lease close failed")
	}
}

func TestAcquireRejectsWrongPinsAndPathsBeforeLockCreation(t *testing.T) {
	d, _ := profileFixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal("synthetic outside directory setup failed")
	}
	outsideID, err := ownedprocess.DirectoryID(outside)
	if err != nil {
		t.Fatal("synthetic outside identity unavailable")
	}
	file := filepath.Join(d.Root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal("synthetic file setup failed")
	}
	rootID, _ := ownedprocess.DirectoryID(d.Root)
	invalid := []Descriptor{
		{Root: d.Root, Directory: d.Directory, RootID: "wrong-root", DirectoryID: d.DirectoryID},
		{Root: d.Root, Directory: d.Directory, RootID: d.RootID, DirectoryID: "wrong-directory"},
		{Root: d.Root, Directory: outside, RootID: d.RootID, DirectoryID: outsideID},
		{Root: d.Root, Directory: d.Root, RootID: d.RootID, DirectoryID: rootID},
		{Root: d.Root, Directory: file, RootID: d.RootID, DirectoryID: "file"},
		{Root: d.Root, Directory: filepath.Join(d.Root, "missing"), RootID: d.RootID, DirectoryID: "missing"},
	}
	for _, candidate := range invalid {
		if lease, err := Acquire(candidate); err == nil {
			_ = lease.Close()
			t.Fatal("invalid synthetic descriptor was accepted")
		}
		if _, err := os.Lstat(filepath.Join(d.Root, lockFileName)); !os.IsNotExist(err) {
			t.Fatal("invalid descriptor created the lock file")
		}
	}
}

func TestAcquireRejectsDirectoryAndParentAliases(t *testing.T) {
	d, _ := profileFixture(t)
	parentAlias := filepath.Join(d.Root, "profiles-alias")
	if err := os.Symlink(filepath.Join(d.Root, "profiles"), parentAlias); err != nil {
		t.Log("symlink privilege unavailable; synthetic alias cases skipped")
		return
	}
	aliasPath := filepath.Join(parentAlias, "synthetic")
	if lease, err := Acquire(Descriptor{Root: d.Root, Directory: aliasPath, RootID: d.RootID, DirectoryID: d.DirectoryID}); err == nil {
		_ = lease.Close()
		t.Fatal("directory path through a reparse parent was accepted")
	}
	if _, err := os.Lstat(filepath.Join(d.Root, lockFileName)); !os.IsNotExist(err) {
		t.Fatal("aliased descriptor created the lock file")
	}
	directoryAlias := filepath.Join(d.Root, "profile-alias")
	if err := os.Symlink(d.Directory, directoryAlias); err != nil {
		t.Log("directory symlink unavailable; final-component alias case skipped")
		return
	}
	if lease, err := Acquire(Descriptor{Root: d.Root, Directory: directoryAlias, RootID: d.RootID, DirectoryID: d.DirectoryID}); err == nil {
		_ = lease.Close()
		t.Fatal("directory symlink was accepted")
	}
}

func TestLeaseStringRedactsPrivatePathsAndPins(t *testing.T) {
	d, _ := profileFixture(t)
	text := fmt.Sprintf("%s %#v", d, d)
	if strings.Contains(text, d.Root) || strings.Contains(text, d.Directory) || strings.Contains(text, d.RootID) || strings.Contains(text, d.DirectoryID) {
		t.Fatal("descriptor formatting exposed a private path or identity pin")
	}
	lease := &Lease{descriptor: d}
	text = fmt.Sprintf("%s %#v", lease, lease)
	if strings.Contains(text, d.Root) || strings.Contains(text, d.Directory) || strings.Contains(text, d.RootID) || strings.Contains(text, d.DirectoryID) {
		t.Fatal("lease formatting exposed a private path or identity pin")
	}
}
