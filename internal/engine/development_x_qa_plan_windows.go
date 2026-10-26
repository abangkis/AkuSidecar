//go:build windows && auth_session_dev

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

type XApplicationQAAsset struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type XApplicationQAProfile struct {
	Root        string `json:"root"`
	Directory   string `json:"directory"`
	RootID      string `json:"rootId"`
	DirectoryID string `json:"directoryId"`
}
type XApplicationQAPlan struct {
	Schema     string                `json:"schema"`
	Instance   string                `json:"instance"`
	Root       string                `json:"root"`
	RootID     string                `json:"rootId"`
	StateID    string                `json:"stateId"`
	Port       int                   `json:"port"`
	Database   string                `json:"database"`
	DatabaseID string                `json:"databaseId"`
	Reader     XApplicationQAProfile `json:"reader"`
	Collector  XApplicationQAProfile `json:"collector"`
	Node       XApplicationQAAsset   `json:"node"`
	Chrome     XApplicationQAAsset   `json:"chrome"`
	Driver     XApplicationQAAsset   `json:"driver"`
}

func (XApplicationQAPlan) String() string     { return "private-isolated-X-QA-plan" }
func (v XApplicationQAPlan) GoString() string { return v.String() }

type xQAMarker struct {
	Schema   string `json:"schema"`
	Instance string `json:"instance"`
	RootID   string `json:"rootId"`
}
type xQASelection struct {
	plan          XApplicationQAPlan
	lease         *ownedprofile.Lease
	pins          []*os.File
	owner         xApplicationOwnerConfig
	fresh         bool
	versionMarker *os.File
}

func (s *xQASelection) close() {
	for _, f := range s.pins {
		_ = f.Close()
	}
	if s.lease != nil {
		_ = s.lease.Close()
	}
}

// Canonical paths and held file identities precede config/SQLite/browser access.
// The mandatory namespace/layout cannot name an ordinary user browser profile.
func xQAOrdinary(path string, directory bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "?#%") {
		return errors.New("QA path must be explicit and canonical")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(resolved, path) {
		return errors.New("QA path alias rejected")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || !directory && !info.Mode().IsRegular() {
		return errors.New("QA path type rejected")
	}
	return nil
}
func xQAPinFile(path string) (*os.File, string, error) {
	if xQAOrdinary(path, false) != nil {
		return nil, "", errors.New("QA file unavailable")
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, "", errors.New("QA file unavailable")
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, "", errors.New("QA file pin unavailable")
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		windows.CloseHandle(h)
		return nil, "", errors.New("QA file identity rejected")
	}
	id := fmt.Sprintf("%08x:%08x%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime)
	return os.NewFile(uintptr(h), path), id, nil
}
func xQAReadPinned(path string, out any) (*os.File, error) {
	f, _, err := xQAPinFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, nativePilotStartupWireMax+1))
	defer clear(raw)
	if err != nil || xManualPilotDecode(raw, out) != nil {
		f.Close()
		return nil, errors.New("QA plan/marker invalid")
	}
	return f, nil
}
func openXQASelection(ctx context.Context, path string) (selection *xQASelection, err error) {
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(path) || filepath.Base(path) != "plan.json" || !regexp.MustCompile(`^qa-x-pilot_[a-f0-9]{32}$`).MatchString(filepath.Base(filepath.Dir(path))) {
		return nil, errors.New("explicit isolated QA plan required")
	}
	s := &xQASelection{}
	defer func() {
		if err != nil {
			s.close()
		}
	}()
	f, err := xQAReadPinned(path, &s.plan)
	if err != nil {
		return nil, err
	}
	s.pins = append(s.pins, f)
	p := s.plan
	if p.Schema != "aku.x.application-qa.v1" || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(p.Instance) || p.Root != filepath.Dir(path) || filepath.Base(p.Root) != "qa-x-pilot_"+p.Instance || p.Port < 1024 || p.Port > 65535 || p.Port == 11122 || p.Database != filepath.Join(p.Root, "state", "pilot.db") {
		return nil, errors.New("QA plan scope rejected")
	}
	rootID, idErr := ownedprocess.DirectoryID(p.Root)
	stateID, stateErr := ownedprocess.DirectoryID(filepath.Join(p.Root, "state"))
	if idErr != nil || stateErr != nil || rootID != p.RootID || stateID != p.StateID {
		return nil, errors.New("QA root identity rejected")
	}
	var marker xQAMarker
	f, err = xQAReadPinned(filepath.Join(p.Root, "instance.json"), &marker)
	if err != nil {
		return nil, err
	}
	s.pins = append(s.pins, f)
	if marker.Schema != "aku.x.qa-isolation.v1" || marker.Instance != p.Instance || marker.RootID != rootID {
		return nil, errors.New("QA isolation marker rejected")
	}
	profile := func(v XApplicationQAProfile, role string) (ownedprofile.Descriptor, error) {
		if v.Root != filepath.Join(p.Root, "profiles", role) || v.Directory != filepath.Join(v.Root, "profile") {
			return ownedprofile.Descriptor{}, errors.New("QA profile layout rejected")
		}
		r, e := ownedprocess.DirectoryID(v.Root)
		d, de := ownedprocess.DirectoryID(v.Directory)
		if e != nil || de != nil || r != v.RootID || d != v.DirectoryID {
			return ownedprofile.Descriptor{}, errors.New("QA profile identity rejected")
		}
		return ownedprofile.Descriptor{Root: v.Root, Directory: v.Directory, RootID: r, DirectoryID: d}, nil
	}
	r, err := profile(p.Reader, "reader")
	if err != nil {
		return nil, err
	}
	c, err := profile(p.Collector, "collector")
	if err != nil {
		return nil, err
	}
	s.owner = xApplicationOwnerConfig{enabled: true, reader: r, collector: c, node: xApplicationOwnerAsset{p.Node.Path, p.Node.SHA256}, chrome: xApplicationOwnerAsset{p.Chrome.Path, p.Chrome.SHA256}, driver: xApplicationOwnerAsset{p.Driver.Path, p.Driver.SHA256}}
	for _, asset := range []xApplicationOwnerAsset{s.owner.node, s.owner.chrome, s.owner.driver} {
		f, e := pinXApplicationAsset(asset)
		if e != nil {
			return nil, errors.New("QA package asset rejected")
		}
		s.pins = append(s.pins, f)
	}
	s.lease, err = ownedprofile.Acquire(ownedprofile.Descriptor{Root: p.Root, Directory: filepath.Join(p.Root, "state"), RootID: rootID, DirectoryID: stateID})
	if err != nil {
		return nil, errors.New("QA instance already owned or changed")
	}
	// Only cleanly closed QA databases are supported by this entry. Do not
	// inspect/recover a WAL or journal supplied by another process or alias.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, e := os.Lstat(p.Database + suffix); !os.IsNotExist(e) {
			return nil, errors.New("QA database sidecar requires explicit offline recovery")
		}
	}
	versionPath := filepath.Join(filepath.Dir(p.Database), store.RuntimeVersionMarkerName)
	if _, e := os.Lstat(versionPath); os.IsNotExist(e) {
		file, e := os.OpenFile(versionPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if e != nil {
			return nil, errors.New("QA marker creation unavailable")
		}
		defer file.Close()
		if _, e = file.WriteString(domain.ApplicationVersion + "\n"); e != nil {
			return nil, errors.New("QA marker creation unavailable")
		}
	} else if e != nil {
		return nil, errors.New("QA marker unavailable")
	}
	s.versionMarker, _, err = xQAPinFile(versionPath)
	if err != nil {
		return nil, err
	}
	s.pins = append(s.pins, s.versionMarker)
	var version [129]byte
	n, _ := s.versionMarker.ReadAt(version[:], 0)
	if string(version[:n]) != domain.ApplicationVersion+"\n" {
		return nil, errors.New("QA runtime marker must match the current version")
	}
	_, statErr := os.Lstat(p.Database)
	if os.IsNotExist(statErr) {
		if p.DatabaseID != "" {
			return nil, errors.New("expected QA database missing")
		}
		f, err = os.OpenFile(p.Database, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return nil, errors.New("fresh QA database unavailable")
		}
		defer f.Close()
		s.fresh = true
	} else if statErr != nil || p.DatabaseID == "" {
		return nil, errors.New("existing QA database requires explicit identity")
	}
	f, id, err := xQAPinFile(p.Database)
	if err != nil {
		return nil, err
	}
	s.pins = append(s.pins, f)
	if !s.fresh && (id != p.DatabaseID || store.InspectDatabase(p.Database).Status != "current") {
		return nil, errors.New("QA database identity/schema rejected")
	}
	return s, nil
}
