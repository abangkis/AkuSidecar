//go:build windows && auth_session_dev

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func dispatchXApplicationQA(options config.Options, input io.Reader, output io.Writer) (bool, error) {
	if options.DevelopmentQAPilotPrepare != "" {
		if !options.Dev || options.DevelopmentSessionReuse != "" || options.DevelopmentQAPilotPlan != "" || options.AppShell || options.WindowsCaptureSplit || options.BrowserProfilePath != "" || options.DatabasePath != "" || options.DatabaseInspect || options.DatabaseAction != "" {
			return true, errors.New("QA preparation requires its isolated development-only entry")
		}
		path, err := engine.MaterializeXApplicationQAPlanFile(options.DevelopmentQAPilotPrepare)
		if err != nil {
			return true, err
		}
		hash, err := engine.XApplicationQAPlanSHA256(path)
		if err != nil {
			return true, err
		}
		return true, json.NewEncoder(output).Encode(map[string]any{"event": "qa_plan_prepared", "plan": path, "sha256": hash, "executionApproved": false, "browserStarted": false, "retainedProfileInspected": false})
	}
	selected := options.DevelopmentSessionReuse == "synthetic-x-manual-qa" || options.DevelopmentSessionReuse == "native-x-manual-qa"
	if !selected && options.DevelopmentQAPilotPlan == "" {
		return false, nil
	}
	if !selected || !options.Dev || options.DevelopmentQAPilotPlan == "" || options.AppShell || options.WindowsCaptureSplit || options.BrowserProfilePath != "" || options.DatabasePath != "" || options.DatabaseInspect || options.DatabaseAction != "" {
		return true, errors.New("isolated X QA requires explicit opt-in and no ordinary runtime overrides")
	}
	return true, runXApplicationQA(context.Background(), engine.XApplicationQAOptions{Dev: true, PlanPath: options.DevelopmentQAPilotPlan, Synthetic: options.DevelopmentSessionReuse == "synthetic-x-manual-qa"}, input, output)
}

// This isolated entry bypasses ordinary config, providers, app windows,
// native-host registration and Auto Update. Native code requires its own
// explicit selector and a separate one-use stdin start consent.
// A marker/status listener and bounded stdin controls belong to this process.
func runXApplicationQA(parent context.Context, options engine.XApplicationQAOptions, input io.Reader, output io.Writer) (retErr error) {
	q, err := engine.OpenXApplicationQAPilot(parent, options)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	defer func() {
		closeErr := q.Close()
		if retErr == nil && closeErr != nil {
			retErr = closeErr
		}
		if err := encoder.Encode(map[string]any{"event": "qa_closed", "ok": closeErr == nil, "report": q.Report()}); retErr == nil && err != nil {
			retErr = q.RecordControlFailure("output")
		}
	}()
	address := strings.TrimPrefix(q.Origin(), "http://")
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return q.RecordControlFailure("listener")
	}
	server := &http.Server{Handler: xQAMarkerHandler(q), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = q.RecordControlFailure("listener")
		}
	}()
	defer func() {
		call, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(call)
	}()
	if encoder.Encode(map[string]any{"event": "qa_ready", "report": q.Report()}) != nil {
		return q.RecordControlFailure("output")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 16*1024)
	type line struct {
		raw  []byte
		err  error
		done bool
	}
	lines := make(chan line, 1)
	scanDone := make(chan struct{})
	defer close(scanDone)
	go func() {
		for scanner.Scan() {
			value := line{raw: append([]byte(nil), scanner.Bytes()...)}
			select {
			case lines <- value:
			case <-scanDone:
				return
			}
		}
		select {
		case lines <- line{done: true, err: scanner.Err()}:
		case <-scanDone:
		}
	}()
	count := 0
	for {
		select {
		case <-q.Failed():
			return &qaJourneyFailure{}
		default:
		}
		var next line
		select {
		case <-q.Failed():
			return &qaJourneyFailure{}
		case <-parent.Done():
			return q.RecordControlFailure("canceled")
		case next = <-lines:
		}
		if next.done {
			if next.err != nil {
				return q.RecordControlFailure("input")
			}
			return nil
		}
		count++
		if count > 32 {
			return q.RecordControlFailure("capacity")
		}
		command, decodeErr := engine.DecodeXApplicationQACommand(next.raw)
		clear(next.raw)
		if decodeErr != nil {
			return q.RecordControlFailure("invalid_command")
		}
		call, cancel := context.WithTimeout(parent, 60*time.Second)
		var actionErr error
		switch command.Op {
		case "start":
			actionErr = q.Start(call, command.AcknowledgeLimitations, command.AllowOneSessionDerivation)
		case "open_post":
			actionErr = q.OpenPost(call, *command.Request)
		case "settle":
			actionErr = q.Settle(call)
		case "stop":
			actionErr = q.Stop(call)
		}
		cancel()
		result := map[string]any{"event": "qa_" + command.Op, "ok": actionErr == nil, "report": q.Report()}
		if actionErr != nil {
			result["failure"] = engine.XApplicationQAFailureOf(actionErr)
		}
		if encoder.Encode(result) != nil {
			return q.RecordControlFailure("output")
		}
		if actionErr != nil {
			return actionErr
		}
	}
}

type qaJourneyFailure struct{}

func (*qaJourneyFailure) Error() string {
	return "QA journey stopped after its first failure; see bounded report"
}

func xQAMarkerHandler(q *engine.XApplicationQAPilot) http.Handler {
	marker := regexp.MustCompile(`^split_[a-f0-9]{32}$`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if r.Host != strings.TrimPrefix(q.Origin(), "http://") || r.Method != http.MethodGet {
			http.Error(w, "QA request rejected", 403)
			return
		}
		switch r.URL.Path {
		case "/qa/status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(q.Report())
		case "/split-reader-intent":
			id := r.URL.Query().Get("id")
			if len(r.URL.Query()) != 1 || len(r.URL.Query()["id"]) != 1 || !marker.MatchString(id) {
				http.Error(w, "QA marker rejected", 400)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, "<!doctype html><title>AkuBrowser reader %s</title><p>QA reader: click this window to continue.</p>", id)
		default:
			http.NotFound(w, r)
		}
	})
}
