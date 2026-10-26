//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
