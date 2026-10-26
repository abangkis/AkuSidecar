//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func xQAEntryTestPlan(t *testing.T) string {
	t.Helper()
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("QA entry helper required")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if port == 11122 {
		t.Fatal("ephemeral port conflicted with main port")
	}
	path, err := engine.WriteXApplicationQASyntheticPlan(t.TempDir(), node, port)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestXQAActualMainSyntheticJourney(t *testing.T) {
	image := os.Getenv("AKU_X_QA_ENTRY_IMAGE")
	if image == "" {
		t.Skip("actual main image required")
	}
	path := xQAEntryTestPlan(t)
	const commands = `{"op":"status"}
{"op":"start","acknowledgeLimitations":true}
{"op":"open_post","request":{"requestId":"broker_0123456789abcdef0123456789abcdef","source":"x","url":"https://x.com/aku_fixture/status/1"}}
{"op":"settle"}
{"op":"status"}
{"op":"stop"}
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, image, "--dev", "--dev-session-reuse=synthetic-x-manual-qa", "--dev-qa-pilot-plan="+path)
	command.Dir = filepath.Dir(path)
	command.Stdin = strings.NewReader(commands)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		t.Fatalf("synthetic main failed %v output=%s stderr=%s", err, out, stderr.String())
	}
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	if len(lines) != 7 {
		t.Fatalf("events=%d output=%s", len(lines), out)
	}
	var observationCount int
	for i, line := range lines {
		var event struct {
			Event  string                      `json:"event"`
			OK     *bool                       `json:"ok"`
			Report engine.XApplicationQAReport `json:"report"`
		}
		if json.Unmarshal(line, &event) != nil || !event.Report.Synthetic || event.Report.NativeQualified {
			t.Fatal("synthetic report claimed native authority")
		}
		if i > 0 && (event.OK == nil || !*event.OK) {
			t.Fatalf("action rejected %s", line)
		}
		if i == 0 && event.Report.Status.State != "disabled" {
			t.Fatal("entry implicitly started pilot")
		}
		observationCount = event.Report.Observations
	}
	if observationCount != 1 || bytes.Contains(out, []byte(filepath.Dir(path))) || bytes.Contains(out, []byte("accountId")) {
		t.Fatal("missing capture or private-data leakage")
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(path), "config", "sidecar.json")); !os.IsNotExist(err) {
		t.Fatal("ordinary config created")
	}
}

func TestXQAUnsupportedBuildsRejectBeforeDatabase(t *testing.T) {
	path := xQAEntryTestPlan(t)
	for _, name := range []string{"AKU_X_QA_ENTRY_DEFAULT", "AKU_X_QA_ENTRY_DEV", "AKU_X_QA_ENTRY_FIXTURE"} {
		t.Run(name, func(t *testing.T) {
			image := os.Getenv(name)
			if image == "" {
				t.Skip("build image required")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, image, "--dev", "--dev-session-reuse=synthetic-x-manual-qa", "--dev-qa-pilot-plan="+path)
			command.Dir = filepath.Dir(path)
			out, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "QA") {
				t.Fatalf("unsupported build accepted %s", out)
			}
			if _, err = os.Stat(filepath.Join(filepath.Dir(path), "state", "pilot.db")); !os.IsNotExist(err) {
				t.Fatal("unsupported build reached database")
			}
		})
	}
}

func TestXQADispatchOptInAndMarkerSurface(t *testing.T) {
	if handled, err := dispatchXApplicationQA(config.Options{}, strings.NewReader(""), &bytes.Buffer{}); handled || err != nil {
		t.Fatal("default entry intercepted ordinary startup")
	}
	for _, options := range []config.Options{{DevelopmentQAPilotPlan: "absent"}, {Dev: true, DevelopmentSessionReuse: "synthetic-x-manual-qa"}, {DevelopmentSessionReuse: "synthetic-x-manual-qa", DevelopmentQAPilotPlan: "absent"}} {
		if handled, err := dispatchXApplicationQA(options, strings.NewReader(""), &bytes.Buffer{}); !handled || err == nil {
			t.Fatal("missing opt-in accepted")
		}
	}
	path := xQAEntryTestPlan(t)
	q, err := engine.OpenXApplicationQAPilot(context.Background(), engine.XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	handler := xQAMarkerHandler(q)
	for _, tc := range []struct {
		path, host, method string
		status             int
	}{
		{"/qa/status", "", http.MethodGet, 200},
		{"/split-reader-intent?id=split_" + strings.Repeat("a", 32), "", http.MethodGet, 200},
		{"/split-reader-intent?id=split_" + strings.Repeat("a", 32) + "&id=duplicate", "", http.MethodGet, 400},
		{"/qa/status", "attacker.invalid", http.MethodGet, 403},
		{"/qa/status", "", http.MethodPost, 403},
		{"/api/settings", "", http.MethodGet, 404},
	} {
		t.Run(tc.method+tc.path+strconv.Itoa(tc.status), func(t *testing.T) {
			r := httptest.NewRequest(tc.method, q.Origin()+tc.path, nil)
			if tc.host != "" {
				r.Host = tc.host
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
}
