//go:build windows && auth_session_dev && auth_session_fixture

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
	"testing"

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

// acquireXRetainedQAProfile is the only production entry point. It binds the
// lease to this source checkout and the fixed project QA directory.
func acquireXRetainedQAProfile(project string) (*xRetainedQAProfile, error) {
	canonicalProject, err := xRetainedQAProjectRoot()
	if err != nil {
		return nil, err
	}
	projectPath, err := filepath.Abs(filepath.Clean(project))
	if err != nil || !samePath(projectPath, canonicalProject) {
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
	parentPin, err := pinXTwoPhaseDirectory(parent)
	if err != nil {
		_ = windows.CloseHandle(lock)
		return nil, errors.New("retained QA parent could not be pinned")
	}
	parentIDAfterPin, err := processFixtureDirectoryID(parent)
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
	if manifestErr == nil {
		if manifest.State != "clean" {
			return nil, errors.New("retained QA profile has an active or unknown prior lease")
		}
		root := filepath.Join(parent, manifest.Root)
		if !xRetainedValidRootPath(parent, root) {
			return nil, errors.New("retained QA root scope rejected")
		}
		rootID, idErr := processFixtureDirectoryID(root)
		if idErr != nil || rootID != manifest.RootID {
			return nil, errors.New("retained QA root identity changed")
		}
		rootPin, pinErr := pinXTwoPhaseDirectory(root)
		if pinErr != nil {
			return nil, errors.New("retained QA root could not be pinned")
		}
		rootIDAfterPin, afterPinErr := processFixtureDirectoryID(root)
		if afterPinErr != nil || rootIDAfterPin != manifest.RootID {
			_ = windows.CloseHandle(rootPin)
			return nil, errors.New("retained QA root identity changed while pinning")
		}
		lease.Root = root
		lease.RootID = rootIDAfterPin
		lease.rootPin = rootPin
		if manifest.ProfileID != "" {
			shared := filepath.Join(root, "shared")
			profileID, profileErr := processFixtureDirectoryID(shared)
			if profileErr != nil || profileID != manifest.ProfileID {
				_ = windows.CloseHandle(rootPin)
				lease.rootPin = 0
				return nil, errors.New("retained QA profile identity changed")
			}
			profilePin, profilePinErr := pinXTwoPhaseDirectory(shared)
			if profilePinErr != nil {
				_ = windows.CloseHandle(rootPin)
				lease.rootPin = 0
				return nil, errors.New("retained QA profile could not be pinned")
			}
			profileIDAfterPin, afterPinErr := processFixtureDirectoryID(shared)
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
	parentID, err := processFixtureDirectoryID(p.parent)
	if err != nil || parentID != p.parentID {
		return errors.New("retained QA parent identity changed")
	}
	rootPath, err := filepath.Abs(filepath.Clean(root))
	if err != nil || !samePath(rootPath, p.Root) {
		return errors.New("fixture root does not match retained QA lease")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil || !samePath(resolvedRoot, p.Root) || !xRetainedValidRootPath(p.parent, rootPath) {
		return errors.New("retained QA root path is not canonical")
	}
	rootID, err := processFixtureDirectoryID(rootPath)
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
	profileID, err := processFixtureDirectoryID(shared)
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
	parentID, err := processFixtureDirectoryID(p.parent)
	if err != nil || parentID != p.parentID {
		return errors.New("retained QA parent identity changed")
	}
	rootID, err := processFixtureDirectoryID(p.Root)
	if err != nil || rootID != p.RootID {
		return errors.New("retained QA root identity changed")
	}
	shared := filepath.Join(p.Root, "shared")
	if p.ProfileID != "" {
		profileID, idErr := processFixtureDirectoryID(shared)
		if idErr != nil || profileID != p.ProfileID || p.profilePin == 0 {
			return errors.New("retained QA profile identity changed")
		}
		return nil
	}
	profileID, err := processFixtureDirectoryID(shared)
	if err != nil || !xRetainedValidRootPath(p.parent, p.Root) {
		return errors.New("new retained QA profile directory unavailable")
	}
	profilePin, err := pinXTwoPhaseDirectory(shared)
	if err != nil {
		return errors.New("new retained QA profile could not be pinned")
	}
	profileIDAfterPin, err := processFixtureDirectoryID(shared)
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
	parentID, err := processFixtureDirectoryID(p.parent)
	if err != nil || parentID != p.parentID {
		return errors.New("retained QA parent identity changed")
	}
	rootID, err := processFixtureDirectoryID(p.Root)
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
	profileID, err := processFixtureDirectoryID(shared)
	if err != nil || profileID != p.ProfileID {
		return errors.New("retained QA profile identity changed")
	}
	return nil
}

func (p *xRetainedQAProfile) xRetainedPersist(state string) error {
	parentID, parentErr := processFixtureDirectoryID(p.parent)
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
	if err != nil || !samePath(project, resolved) {
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
	if err != nil || !samePath(abs, resolved) {
		return "", "", errors.New("directory path is not canonical")
	}
	id, err := processFixtureDirectoryID(abs)
	if err != nil || !xRetainedValidID(id) {
		return "", "", errors.New("directory identity unavailable")
	}
	return abs, id, nil
}

func xRetainedValidRootPath(parent, root string) bool {
	if !xRetainedValidBase(filepath.Base(root)) || !samePath(filepath.Dir(filepath.Clean(root)), parent) {
		return false
	}
	if _, _, err := xRetainedOrdinaryDirectory(parent); err != nil {
		return false
	}
	rootPath, _, err := xRetainedOrdinaryDirectory(root)
	return err == nil && samePath(rootPath, root)
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
		pin, err := pinXTwoPhaseDirectory(root)
		if err != nil {
			return "", "", 0, errors.New("new retained QA root could not be pinned")
		}
		currentID, idErr := processFixtureDirectoryID(root)
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
	fileID, idErr := processFixtureFileID(path)
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
	fileID, idErr := processFixtureFileID(path)
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
	fileIDAfterRead, afterReadErr := processFixtureFileID(path)
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
	if !samePath(filepath.Dir(filepath.Clean(path)), parent) || filepath.Base(path) != xRetainedQAProfileManifestName {
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
	tempID, idErr := processFixtureFileID(temp)
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

func TestXRetainedQAProfileOffline(t *testing.T) {
	fixtureRoot := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if fixtureRoot == "" {
		t.Skip("retained QA profile preflight requires the project-local helper")
	}
	fixtureRoot, _, err := xRetainedOrdinaryDirectory(fixtureRoot)
	if err != nil {
		t.Fatal("offline fixture root unavailable")
	}
	project, err := xRetainedQAProjectRoot()
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	if err != nil || !samePath(filepath.Dir(fixtureRoot), qaParent) || !strings.HasPrefix(filepath.Base(fixtureRoot), "run-") {
		t.Fatal("offline fixture root is outside the project QA scope")
	}
	newParent := func(t *testing.T) string {
		t.Helper()
		parent, createErr := os.MkdirTemp(fixtureRoot, "retained-qa-offline-")
		if createErr != nil {
			t.Fatal("offline retained QA parent unavailable")
		}
		resolved, _, resolveErr := xRetainedOrdinaryDirectory(parent)
		if resolveErr != nil || !samePath(filepath.Dir(resolved), fixtureRoot) {
			t.Fatal("offline retained QA parent scope rejected")
		}
		// The canonical fixture helper removes AKU_CHROME_FIXTURE_ROOT after the test.
		return resolved
	}

	t.Run("exclusive lock and clean profile reuse", func(t *testing.T) {
		parent := newParent(t)
		first, acquireErr := acquireXRetainedQAProfileAtParent(parent)
		if acquireErr != nil {
			t.Fatal("offline lease unavailable")
		}
		project, projectErr := xRetainedQAProjectRoot()
		if projectErr != nil {
			_ = first.Release(false)
			t.Fatal("offline project root unavailable")
		}
		fixtureConfig := func(lease *xRetainedQAProfile) chromeFixtureConfig {
			return chromeFixtureConfig{
				NodePath: "C:\\fixture\\node.exe", ChromePath: "C:\\fixture\\chrome.exe", Root: lease.Root,
				Origin: "http://127.0.0.1:12345", DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-account-settings-driver.mjs"),
				qaProfileLease: lease,
			}
		}
		freshConfig := fixtureConfig(first)
		freshBootstrap := freshConfig
		freshBootstrap.DriverPath = filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-bootstrap-driver.mjs")
		if _, configErr := validateChromeFixtureConfig(freshBootstrap); configErr == nil {
			_ = first.Release(false)
			t.Fatal("bootstrap probe accepted a fresh profile instead of the retained profile")
		}
		freshQualification := freshConfig
		freshQualification.DriverPath = filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-source-qualification-driver.mjs")
		if _, configErr := validateChromeFixtureConfig(freshQualification); configErr == nil {
			_ = first.Release(false)
			t.Fatal("source qualification accepted a fresh profile")
		}
		if got, configErr := validateChromeFixtureConfig(freshConfig); configErr != nil || !samePath(got, first.Root) {
			_ = first.Release(false)
			t.Fatal("fresh retained QA fixture config was rejected")
		}
		if second, secondErr := acquireXRetainedQAProfileAtParent(parent); secondErr == nil {
			_ = second.Release(false)
			t.Fatal("exclusive retained QA lease collision was accepted")
		}
		profileDir := filepath.Join(first.Root, "shared")
		if os.Mkdir(profileDir, 0700) != nil || os.WriteFile(filepath.Join(profileDir, "synthetic-marker"), []byte("offline fixture marker"), 0600) != nil {
			_ = first.Release(false)
			t.Fatal("synthetic shared directory unavailable")
		}
		firstNonce, firstReuse := first.fixtureProfileScope()
		if len(firstNonce) != 64 || firstReuse {
			_ = first.Release(false)
			t.Fatal("first lease scope was not fresh")
		}
		if err = first.RememberProfile(); err != nil {
			_ = first.Release(false)
			t.Fatal("synthetic profile identity was not remembered")
		}
		firstRoot, firstRootID, firstProfileID := first.Root, first.RootID, first.ProfileID
		if strings.Contains(string(mustXRetainedManifest(t, first.manifestPath)), first.Nonce) {
			_ = first.Release(false)
			t.Fatal("private nonce was persisted")
		}
		if err = first.Release(true); err != nil {
			t.Fatal("verified lease did not become clean")
		}
		if err = first.Release(true); err != nil {
			t.Fatal("clean release was not idempotent")
		}

		second, err := acquireXRetainedQAProfileAtParent(parent)
		if err != nil {
			t.Fatal("clean retained profile was not reusable")
		}
		defer second.Release(true)
		nonce, reuse := second.fixtureProfileScope()
		if !second.Existing || !reuse || nonce == firstNonce || len(nonce) != 64 || second.Root != firstRoot ||
			second.RootID != firstRootID || second.ProfileID != firstProfileID {
			t.Fatal("clean profile reuse scope or identity was incorrect")
		}
		if err = second.validateFixtureProfile(firstRoot); err != nil {
			t.Fatal("unchanged retained profile was rejected")
		}
		reusedConfig := fixtureConfig(second)
		reusedBootstrap := reusedConfig
		reusedBootstrap.DriverPath = filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-bootstrap-driver.mjs")
		if _, configErr := validateChromeFixtureConfig(reusedBootstrap); configErr != nil {
			t.Fatal("bootstrap probe rejected the owned retained profile")
		}
		reusedQualification := reusedConfig
		reusedQualification.DriverPath = filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-source-qualification-driver.mjs")
		if _, configErr := validateChromeFixtureConfig(reusedQualification); configErr != nil {
			t.Fatal("source qualification rejected the owned retained profile")
		}
		if got, configErr := validateChromeFixtureConfig(reusedConfig); configErr != nil || !samePath(got, firstRoot) {
			t.Fatal("reused retained QA fixture config was rejected")
		}
		for _, driver := range []string{"x-two-phase-metadata-driver.mjs", "x-native-metadata-probe-driver.mjs"} {
			bad := reusedConfig
			bad.DriverPath = filepath.Join(project, "scripts", "auth-session-fixture", driver)
			if _, configErr := validateChromeFixtureConfig(bad); configErr == nil {
				t.Fatal("old or default driver was accepted for retained QA")
			}
		}
		badNative := reusedConfig
		badNative.NativeReaderStartup = true
		if _, configErr := validateChromeFixtureConfig(badNative); configErr == nil {
			t.Fatal("native reader startup was accepted for retained QA")
		}
		badGrant := reusedConfig
		badGrant.processGrant = &processFixtureGrant{}
		if _, configErr := validateChromeFixtureConfig(badGrant); configErr == nil {
			t.Fatal("process grant was mixed into retained QA")
		}
		badStart := reusedConfig
		badStart.processStart = &processFixtureStart{}
		if _, configErr := validateChromeFixtureConfig(badStart); configErr == nil {
			t.Fatal("process start was mixed into retained QA")
		}
		badMixed := reusedConfig
		badMixed.processGrant = &processFixtureGrant{}
		badMixed.processStart = &processFixtureStart{}
		if _, configErr := validateChromeFixtureConfig(badMixed); configErr == nil {
			t.Fatal("mixed process grant and start were accepted for retained QA")
		}
		missingLease := reusedConfig
		missingLease.qaProfileLease = nil
		if _, configErr := validateChromeFixtureConfig(missingLease); configErr == nil {
			t.Fatal("existing shared profile was accepted without its lease")
		}
		if err = second.validateFixtureProfile(filepath.Join(parent, "run-x-two-phase-00000000000000000000000000000000")); err == nil {
			t.Fatal("different canonical fixture root was accepted")
		}
	})

	t.Run("dirty lease denies reuse", func(t *testing.T) {
		parent := newParent(t)
		lease, acquireErr := acquireXRetainedQAProfileAtParent(parent)
		if acquireErr != nil {
			t.Fatal("offline lease unavailable")
		}
		if err = lease.Release(false); err == nil {
			t.Fatal("unverified cleanup was reported clean")
		}
		if _, err = acquireXRetainedQAProfileAtParent(parent); err == nil {
			t.Fatal("dirty prior lease was reused")
		}
	})

	t.Run("profile and root replacement are rejected", func(t *testing.T) {
		profileParent := newParent(t)
		profileLease, acquireErr := acquireXRetainedQAProfileAtParent(profileParent)
		if acquireErr != nil {
			t.Fatal("profile replacement lease unavailable")
		}
		profileDir := filepath.Join(profileLease.Root, "shared")
		if os.Mkdir(profileDir, 0700) != nil || profileLease.RememberProfile() != nil || profileLease.Release(true) != nil {
			t.Fatal("profile replacement baseline unavailable")
		}
		if os.Rename(profileDir, filepath.Join(profileLease.Root, "old-shared")) != nil || os.Mkdir(profileDir, 0700) != nil {
			t.Fatal("offline profile replacement unavailable")
		}
		if _, err = acquireXRetainedQAProfileAtParent(profileParent); err == nil {
			t.Fatal("replacement profile at the same path was adopted")
		}

		rootParent := newParent(t)
		rootLease, acquireErr := acquireXRetainedQAProfileAtParent(rootParent)
		if acquireErr != nil {
			t.Fatal("root replacement lease unavailable")
		}
		rootPath := rootLease.Root
		if os.Mkdir(filepath.Join(rootPath, "shared"), 0700) != nil || rootLease.RememberProfile() != nil || rootLease.Release(true) != nil {
			t.Fatal("root replacement baseline unavailable")
		}
		if os.Rename(rootPath, rootPath+"-old") != nil || os.Mkdir(rootPath, 0700) != nil {
			t.Fatal("offline root replacement unavailable")
		}
		if _, err = acquireXRetainedQAProfileAtParent(rootParent); err == nil {
			t.Fatal("replacement root at the same path was adopted")
		}
	})

	t.Run("invalid and traversal manifests are rejected", func(t *testing.T) {
		for name, body := range map[string]string{
			"traversal":       `{"version":1,"root":"../outside","rootId":"00000000:0000000000000000:0000000000000000","profileId":"","state":"clean"}`,
			"unknown field":   `{"version":1,"root":"run-x-two-phase-00000000000000000000000000000000","rootId":"00000000:0000000000000000:0000000000000000","profileId":"","state":"clean","extra":true}`,
			"duplicate field": `{"version":1,"root":"run-x-two-phase-00000000000000000000000000000000","root":"run-x-two-phase-11111111111111111111111111111111","rootId":"00000000:0000000000000000:0000000000000000","profileId":"","state":"clean"}`,
		} {
			t.Run(name, func(t *testing.T) {
				parent := newParent(t)
				lease, acquireErr := acquireXRetainedQAProfileAtParent(parent)
				if acquireErr != nil {
					t.Fatal("manifest baseline lease unavailable")
				}
				manifestPath := lease.manifestPath
				if err = lease.Release(true); err != nil {
					t.Fatal("manifest baseline did not become clean")
				}
				if err = os.WriteFile(manifestPath, []byte(body), 0600); err != nil {
					t.Fatal("offline malformed manifest setup failed")
				}
				if _, err = acquireXRetainedQAProfileAtParent(parent); err == nil {
					t.Fatal("invalid retained QA manifest was accepted")
				}
			})
		}
	})
}

func mustXRetainedManifest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > 4096 {
		t.Fatal("retained QA manifest unavailable")
	}
	return data
}
