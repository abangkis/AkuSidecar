//go:build windows

// Package ownedprofile provides an OS-held lease for an existing application-owned profile.
// A lease is a filesystem lifetime guarantee, not browser-session authority.
package ownedprofile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"golang.org/x/sys/windows"
)

const lockFileName = ".aku-x-manual.lock"

// Descriptor identifies already-existing ordinary directories by canonical path and OS identity.
type Descriptor struct {
	Root        string
	Directory   string
	RootID      string
	DirectoryID string
}

// String and GoString deliberately redact paths and identity pins.
func (Descriptor) String() string   { return "Descriptor{private paths and identity pins redacted}" }
func (Descriptor) GoString() string { return "Descriptor{private paths and identity pins redacted}" }

// Lease holds non-delete-sharing handles to Root and Directory and an exclusive root lock file.
type Lease struct {
	mu         sync.Mutex
	descriptor Descriptor
	root       windows.Handle
	directory  windows.Handle
	lock       windows.Handle
	closed     bool
}

// String and GoString deliberately redact paths, identity pins, and OS handles.
func (*Lease) String() string   { return "Lease{private paths, identity pins, and handles redacted}" }
func (*Lease) GoString() string { return "Lease{private paths, identity pins, and handles redacted}" }

// Acquire pins existing ordinary directories and takes the exclusive application-root lock.
// It never creates a directory or grants browser, account, or session authority.
func Acquire(d Descriptor) (*Lease, error) {
	rootPath, err := ordinaryDirectoryPath(d.Root)
	if err != nil || d.RootID == "" {
		return nil, errors.New("owned profile root is invalid")
	}
	directoryPath, err := ordinaryDirectoryPath(d.Directory)
	if err != nil || d.DirectoryID == "" || !strictDescendant(rootPath, directoryPath) {
		return nil, errors.New("owned profile directory is invalid")
	}
	d.Root, d.Directory = rootPath, directoryPath

	// Check both identities before any lock-file open, then repeat from the pinned handles to
	// close the check/open race. The handles deny DELETE sharing but permit content writes.
	rootID, err := ownedprocess.DirectoryID(rootPath)
	if err != nil || rootID != d.RootID {
		return nil, errors.New("owned profile root identity mismatch")
	}
	directoryID, err := ownedprocess.DirectoryID(directoryPath)
	if err != nil || directoryID != d.DirectoryID {
		return nil, errors.New("owned profile directory identity mismatch")
	}

	rootHandle, rootID, err := pinDirectory(rootPath)
	if err != nil {
		return nil, errors.New("owned profile root pin unavailable")
	}
	cleanupRoot := true
	defer func() {
		if cleanupRoot {
			_ = windows.CloseHandle(rootHandle)
		}
	}()
	if rootID != d.RootID {
		return nil, errors.New("owned profile root changed before pin")
	}
	directoryHandle, pinnedDirectoryID, err := pinDirectory(directoryPath)
	if err != nil {
		return nil, errors.New("owned profile directory pin unavailable")
	}
	cleanupDirectory := true
	defer func() {
		if cleanupDirectory {
			_ = windows.CloseHandle(directoryHandle)
		}
	}()
	if pinnedDirectoryID != d.DirectoryID {
		return nil, errors.New("owned profile directory changed before pin")
	}

	lockPath := filepath.Join(rootPath, lockFileName)
	lockName, err := windows.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, errors.New("owned profile lock path invalid")
	}
	lockHandle, err := windows.CreateFile(lockName, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
		windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, errors.New("owned profile lease is unavailable")
	}
	var lockInfo windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(lockHandle, &lockInfo) != nil ||
		lockInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		lockInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		lockInfo.FileSizeHigh != 0 || lockInfo.FileSizeLow != 0 {
		_ = windows.CloseHandle(lockHandle)
		return nil, errors.New("owned profile lock file is not ordinary and empty")
	}

	lease := &Lease{descriptor: d, root: rootHandle, directory: directoryHandle, lock: lockHandle}
	if err := lease.validateLocked(); err != nil {
		_ = windows.CloseHandle(lockHandle)
		return nil, err
	}
	cleanupRoot, cleanupDirectory = false, false
	return lease, nil
}

// Validate confirms the lease remains open and both pinned paths still resolve to the expected directories.
func (l *Lease) Validate() error {
	if l == nil {
		return errors.New("owned profile lease unavailable")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.validateLocked()
}

func (l *Lease) validateLocked() error {
	if l.closed || l.root == 0 || l.directory == 0 || l.lock == 0 {
		return errors.New("owned profile lease is closed")
	}
	rootID, err := directoryIDFromHandle(l.root)
	if err != nil || rootID != l.descriptor.RootID {
		return errors.New("owned profile root pin changed")
	}
	directoryID, err := directoryIDFromHandle(l.directory)
	if err != nil || directoryID != l.descriptor.DirectoryID {
		return errors.New("owned profile directory pin changed")
	}
	rootPath, err := ordinaryDirectoryPath(l.descriptor.Root)
	if err != nil {
		return errors.New("owned profile root path changed")
	}
	directoryPath, err := ordinaryDirectoryPath(l.descriptor.Directory)
	if err != nil || !strictDescendant(rootPath, directoryPath) {
		return errors.New("owned profile directory path changed")
	}
	currentRootID, err := directoryIDForPinnedPath(rootPath)
	if err != nil || currentRootID != l.descriptor.RootID {
		return errors.New("owned profile root path identity changed")
	}
	currentDirectoryID, err := directoryIDForPinnedPath(directoryPath)
	if err != nil || currentDirectoryID != l.descriptor.DirectoryID {
		return errors.New("owned profile directory path identity changed")
	}
	return nil
}

// Matches reports whether root/profile are the same canonical paths held by this open lease.
func (l *Lease) Matches(root, profile string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.validateLocked() != nil {
		return false
	}
	rootPath, rootErr := ordinaryDirectoryPath(root)
	profilePath, profileErr := ordinaryDirectoryPath(profile)
	return rootErr == nil && profileErr == nil &&
		strings.EqualFold(rootPath, l.descriptor.Root) && strings.EqualFold(profilePath, l.descriptor.Directory)
}

// Close releases the OS handles. The empty lock file is intentionally retained so future leases
// cannot race against lock-file deletion and replacement.
func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	var failed bool
	for _, h := range []windows.Handle{l.lock, l.directory, l.root} {
		if h != 0 && windows.CloseHandle(h) != nil {
			failed = true
		}
	}
	l.lock, l.directory, l.root = 0, 0, 0
	if failed {
		return errors.New("owned profile lease handle close failed")
	}
	return nil
}

func ordinaryDirectoryPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || !strings.EqualFold(path, filepath.Clean(path)) {
		return "", errors.New("directory path is not absolute and canonical")
	}
	clean := filepath.Clean(path)
	abs, err := filepath.Abs(clean)
	if err != nil || !strings.EqualFold(abs, clean) {
		return "", errors.New("directory path is not absolute and canonical")
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("directory is not an existing ordinary directory")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || !strings.EqualFold(filepath.Clean(resolved), clean) {
		return "", errors.New("directory path resolves through an alias")
	}
	return clean, nil
}

func strictDescendant(root, directory string) bool {
	rel, err := filepath.Rel(root, directory)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func pinDirectory(path string) (windows.Handle, string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, "", err
	}
	h, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, "", err
	}
	id, err := directoryIDFromHandle(h)
	if err != nil {
		_ = windows.CloseHandle(h)
		return 0, "", err
	}
	return h, id, nil
}

func directoryIDFromHandle(h windows.Handle) (string, error) {
	var data windows.ByHandleFileInformation
	if h == 0 || windows.GetFileInformationByHandle(h, &data) != nil ||
		data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		data.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", errors.New("ordinary directory handle unavailable")
	}
	return fmt.Sprintf("%08x:%08x%08x:%08x%08x", data.VolumeSerialNumber, data.FileIndexHigh, data.FileIndexLow, data.CreationTime.HighDateTime, data.CreationTime.LowDateTime), nil
}

// Use the same no-delete-sharing convention as the held directory pins.
func directoryIDForPinnedPath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	return directoryIDFromHandle(h)
}
