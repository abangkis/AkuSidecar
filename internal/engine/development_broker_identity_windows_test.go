//go:build windows && auth_session_fixture && auth_session_dev

package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDevelopmentBrokerStagedMainMatchesUnchangedNativeIdentity(t *testing.T) {
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if root == "" {
		t.Skip("requires broker preparation helper")
	}
	image := filepath.Join(root, "akusidecar.exe")
	helper := filepath.Join(root, "aku-reader-broker.exe")
	for _, path := range []string{image, helper} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatal("staged executable missing", filepath.Base(path), err)
		}
	}
	if !strings.EqualFold(filepath.Dir(image), filepath.Dir(helper)) {
		t.Fatal("unchanged native same-directory gate would reject main")
	}
	source, err := os.ReadFile(filepath.Join("..", "readerbroker", "windows.go"))
	if err != nil {
		t.Fatal(err)
	}
	names := regexp.MustCompile(`base != "([a-z-]+\.exe)"`).FindAllStringSubmatch(string(source), -1)
	accepted := false
	for _, match := range names {
		if match[1] == filepath.Base(image) {
			accepted = true
		}
	}
	if !accepted {
		t.Fatal("staged main name is outside unchanged native client allowlist")
	}
	// Run the actual main's preflight only, proving this is cmd/akusidecar rather
	// than an engine test executable renamed to resemble a production image.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, image, "--dev-session-reuse=synthetic-broker-fixture")
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := child.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "development session requires --dev") {
		t.Fatal("staged actual-main preflight proof failed", err)
	}
	t.Log("actualMainImage=akusidecar.exe unchangedNativeAllowlist=true sameDirectoryAsNativeHelper=true actualMainPreflightRejectedWithoutDev=true UIStarted=false")
}
