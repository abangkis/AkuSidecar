//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentFrontendNativeStagingIsConfiguredOriginalSource(t *testing.T) {
	project, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	host := "com.akubrowser.qa_reader_" + strings.Repeat("a", 32)
	destination, proof, err := stageDevelopmentFrontendNativeExtension(project, t.TempDir(), "http://127.0.0.1:43219", host)
	if err != nil {
		t.Fatal(err)
	}
	if !proof.NativeCalls || !proof.NativePermission || proof.QAHostName != host || proof.ExtensionID != readerbroker.ExtensionID {
		t.Fatal("positive staging capability/provenance missing")
	}
	worker := filepath.Join(destination, "service-worker.js")
	raw, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `chrome.runtime.sendNativeMessage("`+host+`",`) || strings.Contains(string(raw), readerbroker.HostName) || strings.Contains(string(raw), frontendNativeDenial) {
		t.Fatal("positive transport substituted or points at original host")
	}
	if err = os.WriteFile(worker, append(raw, []byte(`chrome.runtime.sendNativeMessage("`+readerbroker.HostName+`", {});`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyDevelopmentFrontendStage(project, destination, proof) == nil {
		t.Fatal("production host fallthrough passed preflight")
	}
	for _, bad := range []string{readerbroker.HostName, "com.akubrowser.qa_reader_1", host + ".other"} {
		if _, _, err := stageDevelopmentFrontendNativeExtension(project, t.TempDir(), "http://127.0.0.1:43219", bad); err == nil {
			t.Fatal("non QA host accepted")
		}
	}
}

func TestDevelopmentFrontendNativeCLIRequiresExplicitScope(t *testing.T) {
	root := os.Getenv("AKU_DEV_SESSION_TEST_ROOT")
	if root == "" {
		t.Skip("requires native canonical preparation")
	}
	for _, v := range []struct {
		image  string
		args   []string
		reason string
	}{
		{"aku-sidecar-default-check.exe", []string{"--dev", "--dev-session-reuse=synthetic-frontend-native-fixture"}, "requires an auth_session_dev build"},
		{"aku-sidecar-dev-unavailable.exe", []string{"--dev", "--dev-session-reuse=synthetic-frontend-native-fixture"}, "provider unavailable"},
		{"akusidecar.exe", []string{"--dev-session-reuse=synthetic-frontend-native-fixture"}, "requires --dev"},
		{"akusidecar.exe", []string{"--dev", "--dev-session-reuse=synthetic-frontend-native-fixture"}, "startup grant missing"},
	} {
		t.Run(v.image+v.reason, func(t *testing.T) {
			db := filepath.Join(root, "native-ordinary-state-forbidden", "state.sqlite")
			args := append(append([]string{}, v.args...), "--database="+db, "--config="+filepath.Join(root, "native-user-config-forbidden.json"))
			cmd := exec.Command(filepath.Join(root, v.image), args...)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), v.reason) || strings.Contains(string(output), "native-user-config-forbidden") {
				t.Fatalf("native early rejection incorrect: %v %s", err, output)
			}
			if _, err = os.Stat(filepath.Dir(db)); !os.IsNotExist(err) {
				t.Fatal("native rejected startup touched ordinary database")
			}
			t.Logf("native variant=%s earlyRejection=%s ordinaryStateAbsent=true", v.image, v.reason)
		})
	}
}

func TestDevelopmentFrontendStagingNativeClosedAndExactProvenance(t *testing.T) {
	project, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	destination, proof, err := stageDevelopmentFrontendExtension(project, root, "http://127.0.0.1:43219")
	if err != nil {
		t.Fatal(err)
	}
	if proof.NativePermission || proof.NativeCalls || proof.ContentSHA == "" || proof.OriginalWorkerSHA == proof.StagedWorkerSHA {
		t.Fatal("staging proof invalid")
	}
	for _, origin := range []string{"http://localhost:43219", "https://127.0.0.1:43219", "http://127.0.0.1:43219/path", "http://127.0.0.1", "http://127.0.0.1:43219#secret"} {
		if _, _, err := stageDevelopmentFrontendExtension(project, t.TempDir(), origin); err == nil {
			t.Fatal("invalid QA origin accepted")
		}
	}
	worker := filepath.Join(destination, "service-worker.js")
	raw, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "11122") || !strings.Contains(string(raw), frontendNativeDenial) {
		t.Fatal("staged origin/substitution invalid")
	}
	if err = os.WriteFile(worker, append(raw, []byte("chrome.runtime.connectNative('unexpected');")...), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyDevelopmentFrontendStage(project, destination, proof) == nil {
		t.Fatal("native-capable tampering passed preflight")
	}
}

func TestDevelopmentFrontendCLIFailsBeforeOrdinaryStartup(t *testing.T) {
	root := os.Getenv("AKU_DEV_SESSION_TEST_ROOT")
	if root == "" {
		t.Skip("requires canonical frontend preparation")
	}
	for _, v := range []struct {
		image  string
		args   []string
		reason string
	}{
		{"aku-sidecar-default-check.exe", []string{"--dev", "--dev-session-reuse=synthetic-frontend-fixture"}, "requires an auth_session_dev build"},
		{"aku-sidecar-dev-unavailable.exe", []string{"--dev", "--dev-session-reuse=synthetic-frontend-fixture"}, "provider unavailable"},
		{"akusidecar.exe", []string{"--dev-session-reuse=synthetic-frontend-fixture"}, "requires --dev"},
		{"akusidecar.exe", []string{"--dev", "--dev-session-reuse=synthetic-frontend-fixture"}, "startup grant missing"},
	} {
		t.Run(v.image+v.reason, func(t *testing.T) {
			db := filepath.Join(root, "ordinary-state-forbidden", "state.sqlite")
			args := append(append([]string{}, v.args...), "--config="+filepath.Join(root, "missing-user-config.json"), "--database="+db)
			cmd := exec.Command(filepath.Join(root, v.image), args...)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), v.reason) || strings.Contains(string(output), "missing-user-config") {
				t.Fatalf("wrong early frontend CLI rejection: %v %s", err, output)
			}
			if _, err = os.Stat(filepath.Dir(db)); !os.IsNotExist(err) {
				t.Fatal("rejected frontend startup touched ordinary state")
			}
			t.Logf("variant=%s ordinaryStateAbsent=true earlyRejection=%s", v.image, v.reason)
		})
	}
}
