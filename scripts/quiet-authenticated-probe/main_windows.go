//go:build windows

// Operator-only QA adapter. It borrows production Quiet collection code and
// uses a unique local static host, without loading or registering an extension.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/collection/quiet"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type request struct {
	ID               json.RawMessage `json:"id"`
	Type             string          `json:"type"`
	Chrome           string          `json:"chrome"`
	Profile          string          `json:"profile"`
	ProfileDirectory string          `json:"profileDirectory"`
	BridgePath       string          `json:"bridgePath"`
	Source           domain.Source   `json:"source"`
	Payload          map[string]any  `json:"payload"`
}

type probe struct {
	window  *appshell.Window
	host    *httptest.Server
	hostID  string
	targets *quiet.Targets
	worker  *quiet.Worker
}

var profileDirectoryPattern = regexp.MustCompile(`^(Default|Profile [0-9]+)$`)

func (p *probe) init(ctx context.Context, req request) (any, error) {
	if p.window != nil || !profileDirectoryPattern.MatchString(req.ProfileDirectory) {
		return nil, errors.New("invalid_init")
	}
	root := os.Getenv("AKU_PARITY_WORKER_ROOT")
	options := headless.Options{Node: filepath.Join(root, "node.exe"), Worker: filepath.Join(root, "worker.mjs"),
		Pin: filepath.Join(root, "node.pin.json"), Chrome: req.Chrome, Profile: req.Profile, BridgePath: req.BridgePath}
	if err := headless.Validate(options); err != nil {
		return nil, errors.New("runtime_validation_failed")
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, errors.New("fixture_identity_failed")
	}
	path := "/quiet-qa-" + hex.EncodeToString(token[:])
	p.host = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprint(w, "<!doctype html><title>Aku Quiet QA host</title><p>Read-only collection QA</p>")
	}))
	url := p.host.URL + path
	var err error
	p.window, err = appshell.Launch(ctx, appshell.LaunchOptions{Executable: req.Chrome, UserDataDir: req.Profile,
		URL: url, StartMinimized: true, PrivateCDP: true, SuppressStartupWindow: true,
		ExtraArgs: []string{"--start-minimized", "--profile-directory=" + req.ProfileDirectory}})
	if err != nil {
		return nil, errors.New("quiet_host_launch_failed")
	}
	protocol := p.window.CaptureProtocol()
	// Bind only the fresh random URL passed to this launch, never restored blanks.
	p.hostID, err = findHost(ctx, protocol, url)
	if err != nil {
		return nil, err
	}
	p.targets, err = quiet.NewTargets(ctx, protocol)
	if err != nil {
		return nil, errors.New("quiet_targets_init_failed")
	}
	p.worker = quiet.NewWorker(p.targets, options)
	return map[string]any{"pid": p.window.PID(), "chromeVersion": p.targets.Version(),
		"workerDriver": map[string]any{"name": "aku-quiet-qa-adapter", "protocolVersion": 1},
		"backend":      "browser_quiet_hidden", "extensionRegistrationChanged": false}, nil
}

func findHost(ctx context.Context, protocol appshell.CaptureProtocol, url string) (string, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err := protocol.Call(ctx, "Target.getTargets", nil, "")
		if err != nil {
			return "", errors.New("quiet_host_lookup_failed")
		}
		id, err := hostFromListing(raw, url)
		if err != nil || id != "" {
			return id, err
		}
		select {
		case <-ctx.Done():
			return "", errors.New("quiet_host_missing")
		case <-ticker.C:
		}
	}
}

func hostFromListing(raw json.RawMessage, url string) (string, error) {
	var listing struct {
		TargetInfos []struct {
			ID   string `json:"targetId"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"targetInfos"`
	}
	if json.Unmarshal(raw, &listing) != nil {
		return "", errors.New("quiet_host_listing_invalid")
	}
	id := ""
	for _, target := range listing.TargetInfos {
		if target.Type != "page" || target.URL != url {
			continue
		}
		if id != "" || target.ID == "" {
			return "", errors.New("quiet_host_ambiguous")
		}
		id = target.ID
	}
	return id, nil
}

// Retire Node/hidden targets before the exact host. Never Browser.close,
// Terminate, or close a root Job while unrelated windows may still be alive.
func (p *probe) shutdown(ctx context.Context) error {
	if p.worker != nil {
		if err := p.worker.Retire(ctx); err != nil {
			return errors.New("quiet_worker_cleanup_unverified")
		}
	} else if p.targets != nil {
		if err := p.targets.Close(ctx); err != nil {
			return errors.New("quiet_target_cleanup_unverified")
		}
	}
	if p.window != nil {
		select {
		case <-p.window.Done():
			return nil
		default:
		}
		if p.hostID == "" {
			return errors.New("quiet_host_ownership_unverified")
		}
		_, _ = p.window.CaptureProtocol().Call(ctx, "Target.closeTarget", map[string]string{"targetId": p.hostID}, "")
		// Natural Done includes verified zero Job members and pipe release.
		select {
		case <-p.window.Done():
			return nil
		case <-ctx.Done():
			return errors.New("quiet_profile_owner_retained")
		}
	}
	return nil
}

func reply(id json.RawMessage, result any, err error) {
	message := map[string]any{"id": id, "ok": err == nil}
	if err == nil {
		message["result"] = result
	} else {
		code := err.Error()
		var capture *headless.CaptureError
		if errors.As(err, &capture) {
			message["error"] = map[string]any{"code": capture.Code, "message": capture.Message}
		} else {
			message["error"] = map[string]any{"code": code}
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(message)
}

func main() {
	p := &probe{}
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 4096), 2<<20)
	for input.Scan() {
		var req request
		if json.Unmarshal(input.Bytes(), &req) != nil || len(req.ID) == 0 {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		var result any
		var err error
		switch req.Type {
		case "init":
			result, err = p.init(ctx, req)
		case "capture":
			if p.worker == nil || (req.Source != domain.SourceX && req.Source != domain.SourceFacebook) {
				err = errors.New("quiet_capture_unavailable")
			} else {
				result, err = p.worker.Capture(ctx, req.Source, req.Payload)
			}
		case "shutdown":
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 12*time.Second)
			err = p.shutdown(ctx)
			result = map[string]any{"stopped": err == nil}
		default:
			err = errors.New("unsupported_request")
		}
		cancel()
		reply(req.ID, result, err)
		if req.Type == "shutdown" && err == nil {
			if p.host != nil {
				p.host.Close()
			}
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err := p.shutdown(ctx)
	cancel()
	if err != nil && p.window != nil {
		// Keep the owner and private pipe alive after EOF. The outer harness must
		// report blocked restoration instead of killing unknown restored windows.
		<-p.window.Done()
	}
	if p.host != nil {
		p.host.Close()
	}
}
