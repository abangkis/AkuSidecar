//go:build windows && auth_session_dev

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
)

func xDailyPathsOverlap(left, right string) bool {
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil || rel == "." || !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Configuration prepares only a private process factory. It does not open the
// reader profile lease, Chrome, an account or a session-derivation capability.
func ConfigureXDailyNativeRuntime(parent context.Context, e *Engine, ordinaryProfile, chrome string) error {
	if !e.XDailyConfigured() {
		return nil
	}
	readerPath := e.config.Capture.XReaderProfile
	if actual, err := filepath.EvalSymlinks(ordinaryProfile); err == nil {
		ordinaryProfile = actual
	}
	if !filepath.IsAbs(readerPath) || filepath.Clean(readerPath) != readerPath || !filepath.IsAbs(ordinaryProfile) || xDailyPathsOverlap(readerPath, ordinaryProfile) {
		return errors.New("daily X requires an explicit reader profile separate from ordinary capture")
	}
	if real, err := filepath.EvalSymlinks(readerPath); err != nil || !strings.EqualFold(real, readerPath) {
		return errors.New("daily X reader profile must be an existing ordinary directory")
	}
	root := filepath.Dir(readerPath)
	readerRoot, err := ownedprocess.DirectoryID(root)
	if err != nil {
		return errors.New("daily X reader root unavailable")
	}
	readerID, err := ownedprocess.DirectoryID(readerPath)
	if err != nil {
		return errors.New("daily X reader profile unavailable")
	}
	image, err := os.Executable()
	if err != nil {
		return err
	}
	bin := filepath.Dir(image)
	nodeRoot := filepath.Join(bin, "node")
	if _, err = os.Stat(filepath.Join(nodeRoot, "node.exe")); err != nil {
		nodeRoot = filepath.Join(bin, "headless-worker")
	}
	readAsset := func(path string) (xApplicationOwnerAsset, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return xApplicationOwnerAsset{}, errors.New("daily X packaged asset unavailable")
		}
		sum := sha256.Sum256(raw)
		return xApplicationOwnerAsset{path: path, digest: hex.EncodeToString(sum[:])}, nil
	}
	node, err := readAsset(filepath.Join(nodeRoot, "node.exe"))
	if err != nil {
		return err
	}
	var nodePin struct {
		NodeSHA256 string `json:"nodeSha256"`
	}
	raw, err := os.ReadFile(filepath.Join(nodeRoot, "node.pin.json"))
	if err != nil || json.Unmarshal(raw, &nodePin) != nil || nodePin.NodeSHA256 != node.digest {
		return errors.New("daily X packaged Node pin rejected")
	}
	var manifest struct {
		DailyLifecycle bool `json:"dailyLifecycle"`
		Driver         struct {
			File   string `json:"file"`
			SHA256 string `json:"sha256"`
		} `json:"driver"`
	}
	driverRoot := filepath.Join(bin, "x-daily-owner")
	raw, err = os.ReadFile(filepath.Join(driverRoot, "manifest.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || !manifest.DailyLifecycle || manifest.Driver.File != "x-daily-owner.cjs" {
		return errors.New("daily X owner package required")
	}
	driver, err := readAsset(filepath.Join(driverRoot, manifest.Driver.File))
	if err != nil || driver.digest != manifest.Driver.SHA256 {
		return errors.New("daily X driver pin rejected")
	}
	if chrome == "" {
		chrome = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	}
	chromeAsset, err := readAsset(chrome)
	if err != nil {
		return err
	}
	return e.configureXDailyFactory(parent, func(startup, process context.Context) (xDailyNativeOwner, error) {
		collectorParent := filepath.Join(e.config.Root, "runtime", "x-daily-collectors")
		if err := os.MkdirAll(collectorParent, 0700); err != nil {
			return nil, errors.New("daily X collector namespace unavailable")
		}
		collectorRoot, err := os.MkdirTemp(collectorParent, "process-")
		if err != nil {
			return nil, err
		}
		id, err := ownedprocess.DirectoryID(collectorRoot)
		if err != nil {
			return nil, err
		}
		cfg := xApplicationOwnerConfig{enabled: true, node: node, chrome: chromeAsset, driver: driver,
			reader:    ownedprofile.Descriptor{Root: root, Directory: readerPath, RootID: readerRoot, DirectoryID: readerID},
			collector: ownedprofile.Descriptor{Root: collectorRoot, Directory: filepath.Join(collectorRoot, "unopened"), RootID: id}}
		return newXDailyNativeOwner(process, startup, cfg, e, nil)
	})
}
