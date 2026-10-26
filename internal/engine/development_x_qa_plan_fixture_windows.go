//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
)

// Creates only a fresh accountless QA layout. Neither a native grant nor a
// process is created. Test callers supply their already-owned temporary parent.
func WriteXApplicationQASyntheticPlan(parent, node string, port int) (string, error) {
	if xQAOrdinary(parent, true) != nil {
		return "", errors.New("synthetic QA parent unavailable")
	}
	instance := strings.TrimPrefix(domain.NewID("qa"), "qa_")
	root := filepath.Join(parent, "qa-x-pilot_"+instance)
	if os.Mkdir(root, 0700) != nil {
		return "", errors.New("fresh synthetic QA root unavailable")
	}
	for _, relative := range []string{"state", "profiles/reader/profile", "profiles/collector/profile", "bin"} {
		if os.MkdirAll(filepath.Join(root, relative), 0700) != nil {
			return "", errors.New("synthetic QA layout unavailable")
		}
	}
	rootID, _ := ownedprocess.DirectoryID(root)
	stateID, _ := ownedprocess.DirectoryID(filepath.Join(root, "state"))
	profile := func(role string) XApplicationQAProfile {
		r := filepath.Join(root, "profiles", role)
		d := filepath.Join(r, "profile")
		ri, _ := ownedprocess.DirectoryID(r)
		di, _ := ownedprocess.DirectoryID(d)
		return XApplicationQAProfile{r, d, ri, di}
	}
	asset := func(path string) (XApplicationQAAsset, error) {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return XApplicationQAAsset{}, err
		}
		raw, err := os.ReadFile(resolved)
		if err != nil {
			return XApplicationQAAsset{}, err
		}
		h := sha256.Sum256(raw)
		return XApplicationQAAsset{resolved, hex.EncodeToString(h[:])}, nil
	}
	n, err := asset(node)
	if err != nil {
		return "", errors.New("synthetic node asset unavailable")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", errors.New("synthetic image unavailable")
	}
	c, err := asset(exe)
	if err != nil {
		return "", errors.New("synthetic image unavailable")
	}
	driver := filepath.Join(root, "bin", "synthetic-only.mjs")
	if os.WriteFile(driver, []byte("throw new Error('synthetic plan must never execute a driver');\n"), 0600) != nil {
		return "", errors.New("synthetic driver unavailable")
	}
	d, _ := asset(driver)
	plan := XApplicationQAPlan{Schema: "aku.x.application-qa.v1", Instance: instance, Root: root, RootID: rootID, StateID: stateID, Port: port, Database: filepath.Join(root, "state", "pilot.db"), Reader: profile("reader"), Collector: profile("collector"), Node: n, Chrome: c, Driver: d}
	raw, _ := json.Marshal(plan)
	marker, _ := json.Marshal(xQAMarker{Schema: "aku.x.qa-isolation.v1", Instance: instance, RootID: rootID})
	path := filepath.Join(root, "plan.json")
	if os.WriteFile(path, raw, 0600) != nil || os.WriteFile(filepath.Join(root, "instance.json"), marker, 0600) != nil {
		return "", errors.New("synthetic QA plan unavailable")
	}
	return path, nil
}
