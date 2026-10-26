//go:build windows && auth_session_dev

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
)

// This preparation DTO contains no account fields or execution consent. The
// fixed retained reader is deliberately unresolved until explicit native start.
type XApplicationQAMaterialization struct {
	Schema string              `json:"schema"`
	Parent string              `json:"parent"`
	Port   int                 `json:"port"`
	Node   XApplicationQAAsset `json:"node"`
	Chrome XApplicationQAAsset `json:"chrome"`
	Driver XApplicationQAAsset `json:"driver"`
}

func MaterializeXApplicationQAPlanFile(path string) (string, error) {
	project, err := xRetainedQAProjectRoot()
	parent := filepath.Dir(path)
	if err != nil || filepath.Base(path) != "materialize-request.json" || filepath.Dir(parent) != filepath.Join(project, "build", "x-application-qa-prepared") || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(filepath.Base(parent)) {
		return "", errors.New("QA materialization request path rejected")
	}
	var request XApplicationQAMaterialization
	f, err := xQAReadPinned(path, &request)
	if err != nil {
		return "", errors.New("QA materialization request unavailable")
	}
	defer f.Close()
	if request.Parent != parent {
		return "", errors.New("QA materialization request scope rejected")
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return "", errors.New("QA materialization request rejected")
	}
	return MaterializeXApplicationQAPlan(raw)
}

// MaterializeXApplicationQAPlan writes only a fresh isolated layout and plan.
// It never opens a database/profile/lease or constructs an application owner.
func MaterializeXApplicationQAPlan(raw []byte) (string, error) {
	var request XApplicationQAMaterialization
	if xManualPilotDecode(raw, &request) != nil || request.Schema != "aku.x.application-qa-materialize.v1" || request.Port < 1024 || request.Port > 65535 || request.Port == 11122 {
		return "", errors.New("QA materialization request rejected")
	}
	project, err := xRetainedQAProjectRoot()
	if err != nil || xQAOrdinary(request.Parent, true) != nil || filepath.Dir(request.Parent) != filepath.Join(project, "build", "x-application-qa-prepared") || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(filepath.Base(request.Parent)) {
		return "", errors.New("QA materialization parent rejected")
	}
	for _, asset := range []XApplicationQAAsset{request.Node, request.Chrome, request.Driver} {
		pin, e := pinXApplicationAsset(xApplicationOwnerAsset{asset.Path, asset.SHA256})
		if e != nil {
			return "", errors.New("QA materialization asset rejected")
		}
		defer pin.Close()
	}
	instance := strings.TrimPrefix(domain.NewID("qa"), "qa_")
	root := filepath.Join(request.Parent, "qa-x-pilot_"+instance)
	if os.Mkdir(root, 0700) != nil {
		return "", errors.New("fresh QA instance unavailable")
	}
	for _, relative := range []string{"state", "profiles/collector/profile"} {
		if os.MkdirAll(filepath.Join(root, relative), 0700) != nil {
			return "", errors.New("QA materialization layout unavailable")
		}
	}
	id := func(path string) (string, error) { return ownedprocess.DirectoryID(path) }
	rootID, err := id(root)
	if err != nil {
		return "", errors.New("QA instance identity unavailable")
	}
	stateID, err := id(filepath.Join(root, "state"))
	if err != nil {
		return "", errors.New("QA state identity unavailable")
	}
	cRoot := filepath.Join(root, "profiles", "collector")
	cProfile := filepath.Join(cRoot, "profile")
	cRootID, err := id(cRoot)
	if err != nil {
		return "", errors.New("QA collector identity unavailable")
	}
	cProfileID, err := id(cProfile)
	if err != nil {
		return "", errors.New("QA collector identity unavailable")
	}
	plan := XApplicationQAPlan{Schema: "aku.x.application-qa.v1", Instance: instance, Root: root, RootID: rootID, StateID: stateID, Port: request.Port,
		Database: filepath.Join(root, "state", "pilot.db"), ReaderSource: "retained-qa", ProjectRoot: project,
		Collector: XApplicationQAProfile{cRoot, cProfile, cRootID, cProfileID}, Node: request.Node, Chrome: request.Chrome, Driver: request.Driver}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", errors.New("QA plan encoding unavailable")
	}
	marker, _ := json.Marshal(xQAMarker{Schema: "aku.x.qa-isolation.v1", Instance: instance, RootID: rootID})
	write := func(path string, value []byte) error {
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(value)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		return closeErr
	}
	path := filepath.Join(root, "plan.json")
	if write(filepath.Join(root, "instance.json"), marker) != nil || write(path, encoded) != nil {
		return "", errors.New("QA materialization publication unavailable")
	}
	return path, nil
}

// Fingerprinting a completed plan grants no execution or session authority.
func XApplicationQAPlanSHA256(path string) (string, error) {
	if filepath.Base(path) != "plan.json" || xQAOrdinary(path, false) != nil {
		return "", errors.New("QA plan unavailable")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("QA plan unavailable")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
