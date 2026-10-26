//go:build windows && auth_session_dev

package engine

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"golang.org/x/sys/windows"
)

const xRetainedQAProfileManifestName = "x-retained-native-qa.json"

var (
	xRetainedQARunRootPattern = regexp.MustCompile(`^run-x-two-phase-[0-9a-f]{32}$`)
	xRetainedQAIDPattern      = regexp.MustCompile(`^[0-9a-f]{8}:[0-9a-f]{16}:[0-9a-f]{16}$`)
)

// xRetainedQAProfile is a lease over the one project-owned native QA profile.
// Nonce exists only in memory and is intentionally absent from the manifest.
type xRetainedQAProfile struct {
	Root      string
	RootID    string
	ProfileID string
	Nonce     string
	Existing  bool

	mu           sync.Mutex
	parent       string
	parentID     string
	manifestPath string
	lockPath     string
	parentPin    windows.Handle
	rootPin      windows.Handle
	profilePin   windows.Handle
	lock         windows.Handle
	released     bool
	releaseErr   error
}

type xRetainedQAManifest struct {
	Version   int
	Root      string
	RootID    string
	ProfileID string
	State     string
}

// acquireXRetainedQAProfile is the legacy bootstrap adapter. It binds the
// shared lease to this source checkout and the fixed project QA directory.
func acquireXRetainedQAProfile(project string) (*xRetainedQAProfile, error) {
	canonicalProject, err := xRetainedQAProjectRoot()
	if err != nil {
		return nil, err
	}
	projectPath, err := filepath.Abs(filepath.Clean(project))
	if err != nil || !xRetainedSamePath(projectPath, canonicalProject) {
		return nil, errors.New("retained QA project path rejected")
	}
	if _, _, err = xRetainedOrdinaryDirectory(projectPath); err != nil {
		return nil, errors.New("retained QA project directory unavailable")
	}
	parent := filepath.Join(canonicalProject, "runtime", "qa", "auth-session-sidecar")
	if _, _, err = xRetainedOrdinaryDirectory(parent); err != nil {
		return nil, errors.New("retained QA directory unavailable")
	}
	return acquireXRetainedQAProfileAtParent(parent)
}

// acquireXRetainedQAProfileAtParent is shared by the production entry point
// and offline tests. Tests pass a fresh ordinary directory below
// AKU_CHROME_FIXTURE_ROOT so they never inspect or modify the user manifest.
func acquireXRetainedQAProfileAtParent(parent string) (*xRetainedQAProfile, error) {
	return acquireXRetainedProfileAtParent(parent, false)
}

func acquireXRetainedProfileAtParent(parent string, existingOnly bool) (*xRetainedQAProfile, error) {
	parent, parentID, err := xRetainedOrdinaryDirectory(parent)
	if err != nil {
		return nil, errors.New("retained QA parent rejected")
	}
	manifestPath := filepath.Join(parent, xRetainedQAProfileManifestName)
	lockPath := manifestPath + ".lock"
	lock, err := xRetainedOpenExclusiveLock(lockPath)
	if err != nil {
		return nil, errors.New("retained QA lease is already held or unavailable")
	}
	parentPin, err := xRetainedPinDirectory(parent)
	if err != nil {
		_ = windows.CloseHandle(lock)
		return nil, errors.New("retained QA parent could not be pinned")
	}
	parentIDAfterPin, err := ownedprocess.DirectoryID(parent)
	if err != nil || parentIDAfterPin != parentID {
		_ = windows.CloseHandle(parentPin)
		_ = windows.CloseHandle(lock)
		return nil, errors.New("retained QA parent identity changed while pinning")
	}

	lease := &xRetainedQAProfile{parent: parent, parentID: parentID, parentPin: parentPin,
		manifestPath: manifestPath, lockPath: lockPath, lock: lock}
	leaseReady := false
	defer func() {
		if !leaseReady {
			lease.xRetainedCloseHandles()
		}
	}()
	var nonceBytes [32]byte
	if _, err = rand.Read(nonceBytes[:]); err != nil {
		return nil, errors.New("retained QA nonce unavailable")
	}
	lease.Nonce = hex.EncodeToString(nonceBytes[:])

	manifest, manifestErr := xRetainedReadManifest(manifestPath)
	if existingOnly && (manifestErr != nil || manifest.ProfileID == "") {
		return nil, errors.New("retained QA existing profile required")
	}
	if manifestErr == nil {
		if manifest.State != "clean" {
			return nil, errors.New("retained QA profile has an active or unknown prior lease")
		}
		root := filepath.Join(parent, manifest.Root)
		if !xRetainedValidRootPath(parent, root) {
			return nil, errors.New("retained QA root scope rejected")
		}
		rootID, idErr := ownedprocess.DirectoryID(root)
		if idErr != nil || rootID != manifest.RootID {
			return nil, errors.New("retained QA root identity changed")
		}
		rootPin, pinErr := xRetainedPinDirectory(root)
		if pinErr != nil {
			return nil, errors.New("retained QA root could not be pinned")
		}
		rootIDAfterPin, afterPinErr := ownedprocess.DirectoryID(root)
		if afterPinErr != nil || rootIDAfterPin != manifest.RootID {
			_ = windows.CloseHandle(rootPin)
			return nil, errors.New("retained QA root identity changed while pinning")
		}
		lease.Root = root
		lease.RootID = rootIDAfterPin
		lease.rootPin = rootPin
		if manifest.ProfileID != "" {
			shared := filepath.Join(root, "shared")
			profileID, profileErr := ownedprocess.DirectoryID(shared)
			if profileErr != nil || profileID != manifest.ProfileID {
				_ = windows.CloseHandle(rootPin)
				lease.rootPin = 0
				return nil, errors.New("retained QA profile identity changed")
			}
			profilePin, profilePinErr := xRetainedPinDirectory(shared)
			if profilePinErr != nil {
				_ = windows.CloseHandle(rootPin)
				lease.rootPin = 0
				return nil, errors.New("retained QA profile could not be pinned")
			}
			profileIDAfterPin, afterPinErr := ownedprocess.DirectoryID(shared)
			if afterPinErr != nil || profileIDAfterPin != manifest.ProfileID {
				_ = windows.CloseHandle(profilePin)
				_ = windows.CloseHandle(rootPin)
				lease.rootPin = 0
				return nil, errors.New("retained QA profile identity changed while pinning")
			}
			lease.ProfileID = profileIDAfterPin
			lease.profilePin = profilePin
			lease.Existing = true
		} else if err = xRetainedRequireAbsent(filepath.Join(root, "shared")); err != nil {
			_ = windows.CloseHandle(rootPin)
			lease.rootPin = 0
			return nil, errors.New("retained QA profile exists without a saved identity")
		}
	} else if !errors.Is(manifestErr, os.ErrNotExist) {
		return nil, errors.New("retained QA manifest is invalid or unavailable")
	} else {
		root, rootID, rootPin, createErr := xRetainedCreateRoot(parent)
		if createErr != nil {
			return nil, createErr
		}
		lease.Root = root
		lease.RootID = rootID
		lease.rootPin = rootPin
	}

	if err = lease.xRetainedPersist("active"); err != nil {
		return nil, errors.New("retained QA active manifest could not be saved")
	}
	leaseReady = true
	return lease, nil
}

// validateFixtureProfile is called by the fixture before it uses this root.
// It verifies the exact leased root and the saved identity of shared, when one
// exists, while the exclusive manifest lock and directory pins remain held.
func (p *xRetainedQAProfile) validateFixtureProfile(root string) error {
	if p == nil {
		return errors.New("retained QA lease unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.lock == 0 || p.parentPin == 0 || p.rootPin == 0 {
		return errors.New("retained QA lease is not held")
	}
	parentID, err := ownedprocess.DirectoryID(p.parent)
	if err != nil || parentID != p.parentID {
		return errors.New("retained QA parent identity changed")
	}
	rootPath, err := filepath.Abs(filepath.Clean(root))
	if err != nil || !xRetainedSamePath(rootPath, p.Root) {
		return errors.New("fixture root does not match retained QA lease")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil || !xRetainedSamePath(resolvedRoot, p.Root) || !xRetainedValidRootPath(p.parent, rootPath) {
		return errors.New("retained QA root path is not canonical")
	}
	rootID, err := ownedprocess.DirectoryID(rootPath)
	if err != nil || rootID != p.RootID {
		return errors.New("retained QA root identity changed")
	}
	shared := filepath.Join(rootPath, "shared")
	if p.ProfileID == "" {
		return xRetainedRequireAbsent(shared)
	}
	if p.profilePin == 0 {
		return errors.New("retained QA profile is not pinned")
	}
	profileID, err := ownedprocess.DirectoryID(shared)
	if err != nil || profileID != p.ProfileID {
		return errors.New("retained QA profile identity changed")
	}
	return nil
}

// fixtureProfileScope returns the private RPC scope while the lease is held.
func (p *xRetainedQAProfile) fixtureProfileScope() (nonce string, existing bool) {
	if p == nil {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.lock == 0 {
		return "", false
	}
	return p.Nonce, p.Existing
}

// RememberProfile pins and records the shared directory created by the first
// Chrome launch. It never reads profile contents.
func (p *xRetainedQAProfile) RememberProfile() error {
	if p == nil {
		return errors.New("retained QA lease unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released || p.lock == 0 || p.parentPin == 0 || p.rootPin == 0 {
		return errors.New("retained QA lease is not held")
	}
	parentID, err := ownedprocess.DirectoryID(p.parent)
	if err != nil || parentID != p.parentID {
		return errors.New("retained QA parent identity changed")
	}
	rootID, err := ownedprocess.DirectoryID(p.Root)
	if err != nil || rootID != p.RootID {
		return errors.New("retained QA root identity changed")
	}
	shared := filepath.Join(p.Root, "shared")
	if p.ProfileID != "" {
		profileID, idErr := ownedprocess.DirectoryID(shared)
		if idErr != nil || profileID != p.ProfileID || p.profilePin == 0 {
			return errors.New("retained QA profile identity changed")
		}
		return nil
	}
	profileID, err := ownedprocess.DirectoryID(shared)
	if err != nil || !xRetainedValidRootPath(p.parent, p.Root) {
		return errors.New("new retained QA profile directory unavailable")
	}
	profilePin, err := xRetainedPinDirectory(shared)
	if err != nil {
		return errors.New("new retained QA profile could not be pinned")
	}
	profileIDAfterPin, err := ownedprocess.DirectoryID(shared)
	if err != nil || profileIDAfterPin != profileID {
		_ = windows.CloseHandle(profilePin)
		return errors.New("new retained QA profile identity changed while pinning")
	}
	p.ProfileID = profileIDAfterPin
	p.profilePin = profilePin
	if err = p.xRetainedPersist("active"); err != nil {
		return errors.New("retained QA profile identity could not be saved")
	}
	return nil
}

// Release records a clean lease only after the caller verifies its Job cleanup.
// The retained profile and its root are never deleted.
func (p *xRetainedQAProfile) Release(cleanupVerified bool) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released {
		return p.releaseErr
	}
	var releaseErr error
	if !cleanupVerified {
		releaseErr = errors.New("retained QA cleanup was not verified; reuse denied")
	} else if err := p.xRetainedValidateHeldIDs(); err != nil {
		releaseErr = err
	} else if err := p.xRetainedPersist("clean"); err != nil {
		releaseErr = errors.New("retained QA clean manifest could not be saved")
	}
	p.xRetainedCloseHandles()
	p.released = true
	p.releaseErr = releaseErr
	return releaseErr
}

func (p *xRetainedQAProfile) xRetainedValidateHeldIDs() error {
	if p.lock == 0 || p.parentPin == 0 || p.rootPin == 0 {
		return errors.New("retained QA lease is not held")
	}
	parentID, err := ownedprocess.DirectoryID(p.parent)
	if err != nil || parentID != p.parentID {
		return errors.New("retained QA parent identity changed")
	}
	rootID, err := ownedprocess.DirectoryID(p.Root)
	if err != nil || rootID != p.RootID || !xRetainedValidRootPath(p.parent, p.Root) {
		return errors.New("retained QA root identity changed")
	}
	shared := filepath.Join(p.Root, "shared")
	if p.ProfileID == "" {
		return xRetainedRequireAbsent(shared)
	}
	if p.profilePin == 0 {
		return errors.New("retained QA profile is not pinned")
	}
	profileID, err := ownedprocess.DirectoryID(shared)
	if err != nil || profileID != p.ProfileID {
		return errors.New("retained QA profile identity changed")
	}
	return nil
}

func (p *xRetainedQAProfile) xRetainedPersist(state string) error {
	parentID, parentErr := ownedprocess.DirectoryID(p.parent)
	if p.lock == 0 || p.parentPin == 0 || parentErr != nil || parentID != p.parentID ||
		(state != "active" && state != "clean") || !xRetainedValidRootPath(p.parent, p.Root) ||
		!xRetainedValidID(p.RootID) || p.ProfileID != "" && !xRetainedValidID(p.ProfileID) {
		return errors.New("retained QA manifest state rejected")
	}
	manifest := xRetainedQAManifest{Version: 1, Root: filepath.Base(p.Root), RootID: p.RootID, ProfileID: p.ProfileID, State: state}
	return xRetainedWriteManifest(p.parent, p.manifestPath, manifest)
}

func (p *xRetainedQAProfile) xRetainedCloseHandles() {
	if p.profilePin != 0 {
		_ = windows.CloseHandle(p.profilePin)
		p.profilePin = 0
	}
	if p.rootPin != 0 {
		_ = windows.CloseHandle(p.rootPin)
		p.rootPin = 0
	}
	if p.parentPin != 0 {
		_ = windows.CloseHandle(p.parentPin)
		p.parentPin = 0
	}
	if p.lock != 0 {
		_ = windows.CloseHandle(p.lock)
		p.lock = 0
	}
}

func xRetainedQAProjectRoot() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("retained QA source root unavailable")
	}
	project, err := filepath.Abs(filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..")))
	if err != nil {
		return "", errors.New("retained QA source root unavailable")
	}
	resolved, err := filepath.EvalSymlinks(project)
	if err != nil || !xRetainedSamePath(project, resolved) {
		return "", errors.New("retained QA source root is not canonical")
	}
	if _, _, err = xRetainedOrdinaryDirectory(project); err != nil {
		return "", errors.New("retained QA source root is unavailable")
	}
	return project, nil
}

func xRetainedOrdinaryDirectory(path string) (string, string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("directory is not ordinary")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || !xRetainedSamePath(abs, resolved) {
		return "", "", errors.New("directory path is not canonical")
	}
	id, err := ownedprocess.DirectoryID(abs)
	if err != nil || !xRetainedValidID(id) {
		return "", "", errors.New("directory identity unavailable")
	}
	return abs, id, nil
}

func xRetainedValidRootPath(parent, root string) bool {
	if !xRetainedValidBase(filepath.Base(root)) || !xRetainedSamePath(filepath.Dir(filepath.Clean(root)), parent) {
		return false
	}
	if _, _, err := xRetainedOrdinaryDirectory(parent); err != nil {
		return false
	}
	rootPath, _, err := xRetainedOrdinaryDirectory(root)
	return err == nil && xRetainedSamePath(rootPath, root)
}

func xRetainedValidBase(base string) bool {
	return xRetainedQARunRootPattern.MatchString(base) && filepath.Base(base) == base && !strings.ContainsAny(base, `/\:`)
}

func xRetainedValidID(value string) bool {
	return xRetainedQAIDPattern.MatchString(value)
}

func xRetainedRequireAbsent(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return errors.New("retained QA shared directory must be absent")
}

func xRetainedCreateRoot(parent string) (string, string, windows.Handle, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", "", 0, errors.New("retained QA root nonce unavailable")
		}
		root := filepath.Join(parent, "run-x-two-phase-"+hex.EncodeToString(raw[:]))
		if err := os.Mkdir(root, 0700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", "", 0, errors.New("retained QA root could not be created")
		}
		root, rootID, err := xRetainedOrdinaryDirectory(root)
		if err != nil {
			return "", "", 0, errors.New("new retained QA root is not ordinary")
		}
		pin, err := xRetainedPinDirectory(root)
		if err != nil {
			return "", "", 0, errors.New("new retained QA root could not be pinned")
		}
		currentID, idErr := ownedprocess.DirectoryID(root)
		if idErr != nil || currentID != rootID {
			_ = windows.CloseHandle(pin)
			return "", "", 0, errors.New("new retained QA root identity changed while pinning")
		}
		return root, rootID, pin, nil
	}
	return "", "", 0, errors.New("retained QA root name collisions exceeded limit")
}

func xRetainedOpenExclusiveLock(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, errors.New("retained QA lock path rejected")
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.NumberOfLinks != 1 || info.FileSizeHigh != 0 || info.FileSizeLow != 0 {
		_ = windows.CloseHandle(h)
		return 0, errors.New("retained QA lock file rejected")
	}
	fileID, idErr := xRetainedFileID(path)
	if idErr != nil || fileID != xRetainedFileIDFromInfo(info) {
		_ = windows.CloseHandle(h)
		return 0, errors.New("retained QA lock path changed")
	}
	return h, nil
}

func xRetainedReadManifest(path string) (xRetainedQAManifest, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return xRetainedQAManifest{}, os.ErrNotExist
	} else if err != nil {
		return xRetainedQAManifest{}, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return xRetainedQAManifest{}, errors.New("retained QA manifest path rejected")
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return xRetainedQAManifest{}, err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.NumberOfLinks != 1 {
		return xRetainedQAManifest{}, errors.New("retained QA manifest is not an ordinary file")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size == 0 || size > 4096 {
		return xRetainedQAManifest{}, errors.New("retained QA manifest size rejected")
	}
	fileID, idErr := xRetainedFileID(path)
	if idErr != nil || fileID != xRetainedFileIDFromInfo(info) {
		return xRetainedQAManifest{}, errors.New("retained QA manifest path changed")
	}
	data := make([]byte, int(size))
	var total uint32
	for total < uint32(len(data)) {
		var read uint32
		err = windows.ReadFile(h, data[total:], &read, nil)
		total += read
		if err != nil {
			if errors.Is(err, windows.ERROR_HANDLE_EOF) {
				break
			}
			return xRetainedQAManifest{}, errors.New("retained QA manifest read failed")
		}
		if read == 0 {
			break
		}
	}
	if total != uint32(len(data)) {
		return xRetainedQAManifest{}, errors.New("retained QA manifest changed while reading")
	}
	fileIDAfterRead, afterReadErr := xRetainedFileID(path)
	if afterReadErr != nil || fileIDAfterRead != fileID {
		return xRetainedQAManifest{}, errors.New("retained QA manifest path changed while reading")
	}
	manifest, err := xRetainedDecodeManifest(data)
	if err != nil {
		return xRetainedQAManifest{}, err
	}
	return manifest, nil
}

func xRetainedDecodeManifest(data []byte) (xRetainedQAManifest, error) {
	var manifest xRetainedQAManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return manifest, errors.New("retained QA manifest schema rejected")
	}
	seen := make(map[string]bool, 5)
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok || seen[key] {
			return manifest, errors.New("retained QA manifest schema rejected")
		}
		seen[key] = true
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return manifest, errors.New("retained QA manifest schema rejected")
		}
		switch key {
		case "version":
			err = json.Unmarshal(raw, &manifest.Version)
		case "root":
			err = xRetainedDecodeString(raw, &manifest.Root)
		case "rootId":
			err = xRetainedDecodeString(raw, &manifest.RootID)
		case "profileId":
			err = xRetainedDecodeString(raw, &manifest.ProfileID)
		case "state":
			err = xRetainedDecodeString(raw, &manifest.State)
		default:
			return manifest, errors.New("retained QA manifest has an unknown field")
		}
		if err != nil {
			return manifest, errors.New("retained QA manifest field rejected")
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return manifest, errors.New("retained QA manifest schema rejected")
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return manifest, errors.New("retained QA manifest has trailing data")
	}
	if len(seen) != 5 || !seen["version"] || !seen["root"] || !seen["rootId"] || !seen["profileId"] || !seen["state"] ||
		manifest.Version != 1 || !xRetainedValidBase(manifest.Root) || !xRetainedValidID(manifest.RootID) ||
		(manifest.ProfileID != "" && !xRetainedValidID(manifest.ProfileID)) ||
		(manifest.State != "active" && manifest.State != "clean") {
		return manifest, errors.New("retained QA manifest values rejected")
	}
	return manifest, nil
}

func xRetainedDecodeString(raw json.RawMessage, destination *string) error {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return errors.New("manifest field is not a string")
	}
	return json.Unmarshal(raw, destination)
}

func xRetainedWriteManifest(parent, path string, manifest xRetainedQAManifest) error {
	if !xRetainedSamePath(filepath.Dir(filepath.Clean(path)), parent) || filepath.Base(path) != xRetainedQAProfileManifestName {
		return errors.New("retained QA manifest target rejected")
	}
	data, err := json.Marshal(struct {
		Version   int    `json:"version"`
		Root      string `json:"root"`
		RootID    string `json:"rootId"`
		ProfileID string `json:"profileId"`
		State     string `json:"state"`
	}{manifest.Version, manifest.Root, manifest.RootID, manifest.ProfileID, manifest.State})
	if err != nil || len(data) == 0 || len(data) > 4096 {
		return errors.New("retained QA manifest encoding rejected")
	}
	file, err := os.CreateTemp(parent, ".x-retained-native-qa-*.pending")
	if err != nil {
		return errors.New("retained QA manifest staging unavailable")
	}
	temp := file.Name()
	defer func() { _ = os.Remove(temp) }()
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("retained QA manifest staging failed")
	}
	tempID, idErr := xRetainedFileID(temp)
	if idErr != nil || tempID == "" {
		return errors.New("retained QA manifest staging identity rejected")
	}
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return errors.New("retained QA manifest staging path rejected")
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("retained QA manifest target path rejected")
	}
	if err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return errors.New("retained QA manifest replacement failed")
	}
	return nil
}

func xRetainedFileIDFromInfo(info windows.ByHandleFileInformation) string {
	return fmt.Sprintf("%08x:%08x%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow,
		info.CreationTime.HighDateTime, info.CreationTime.LowDateTime)
}

func xRetainedPinDirectory(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, errors.New("two-phase directory path invalid")
	}
	handle, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, errors.New("two-phase directory pin unavailable")
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("two-phase directory pin rejected")
	}
	return handle, nil
}

func xRetainedFileID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("fixture file identity unavailable")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !xRetainedSamePath(path, resolved) {
		return "", errors.New("fixture file resolves elsewhere")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var data windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &data); err != nil {
		return "", err
	}
	if data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || data.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return "", errors.New("fixture file identity is not an ordinary file")
	}
	return fmt.Sprintf("%08x:%08x%08x:%08x%08x", data.VolumeSerialNumber, data.FileIndexHigh, data.FileIndexLow, data.CreationTime.HighDateTime, data.CreationTime.LowDateTime), nil
}

// Existing-only application selection resolves the fixed manifest under its legacy lock.
func acquireXApplicationRetainedProfile(project string) (*xRetainedQAProfile, error) {
	canonical, err := xRetainedQAProjectRoot()
	if err != nil || project != canonical {
		return nil, errors.New("retained QA project rejected")
	}
	return acquireXRetainedProfileAtParent(filepath.Join(canonical, "runtime", "qa", "auth-session-sidecar"), true)
}
func xRetainedSamePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
func (p *xRetainedQAProfile) Validate() error {
	if p == nil {
		return errors.New("retained QA lease unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released {
		return errors.New("retained QA lease released")
	}
	return p.xRetainedValidateHeldIDs()
}
func (p *xRetainedQAProfile) Matches(root, profile string) bool {
	return p.Validate() == nil && xRetainedSamePath(root, p.Root) && xRetainedSamePath(profile, filepath.Join(p.Root, "shared"))
}

// Close is invoked only before any process exists or after verified complete owner cleanup.
func (p *xRetainedQAProfile) Close() error { return p.Release(true) }
func (p *xRetainedQAProfile) descriptor() ownedprofile.Descriptor {
	return ownedprofile.Descriptor{Root: p.Root, Directory: filepath.Join(p.Root, "shared"), RootID: p.RootID, DirectoryID: p.ProfileID}
}
