//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const frontendNativeDenial = `await new Promise(resolve => setTimeout(() => resolve({ok:false,message:"QA frontend: native activation is disabled."}),1500))`

type frontendStageProof struct {
	ContentSHA, OriginalWorkerSHA, StagedWorkerSHA string
	NativePermission, NativeCalls                  bool
	Origin, ExtensionID                            string
	QAHostName                                     string
}

func frontendHash(raw []byte) string { hash := sha256.Sum256(raw); return hex.EncodeToString(hash[:]) }

func stageDevelopmentFrontendExtension(project, root, origin string) (string, frontendStageProof, error) {
	return stageDevelopmentFrontendExtensionMode(project, root, origin, "")
}

func stageDevelopmentFrontendNativeExtension(project, root, origin, host string) (string, frontendStageProof, error) {
	if err := readerbroker.ValidateFixtureQAHostName(host); err != nil {
		return "", frontendStageProof{}, err
	}
	return stageDevelopmentFrontendExtensionMode(project, root, origin, host)
}

func stageDevelopmentFrontendExtensionMode(project, root, origin, host string) (string, frontendStageProof, error) {
	var proof frontendStageProof
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", proof, errors.New("frontend QA origin rejected")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(project) {
		return "", proof, errors.New("frontend staging paths must be absolute")
	}
	source := filepath.Join(project, "ui-reader-broker")
	manifestRaw, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		return "", proof, err
	}
	var manifest map[string]any
	if json.Unmarshal(manifestRaw, &manifest) != nil {
		return "", proof, errors.New("source manifest invalid")
	}
	content, err := os.ReadFile(filepath.Join(source, "content.js"))
	if err != nil {
		return "", proof, err
	}
	worker, err := os.ReadFile(filepath.Join(source, "service-worker.js"))
	if err != nil {
		return "", proof, err
	}
	originalCall := `await chrome.runtime.sendNativeMessage("com.akubrowser.reader_activation", {
        requestId: message.requestId, source: message.source, url: message.url,
      })`
	text := strings.ReplaceAll(string(worker), "\r\n", "\n")
	if strings.Count(text, originalCall) != 1 {
		return "", proof, errors.New("source native-call shape changed; staging refused")
	}
	if host == "" {
		text = strings.Replace(text, originalCall, frontendNativeDenial, 1)
		text = strings.Replace(text, "// sendNativeMessage launches a fresh executable for each explicit click.", "// QA-only denial substitutes the native transport; no process is launched.", 1)
		text = strings.ReplaceAll(text, `phase: "helper_start"`, `phase: "qa_native_disabled"`)
		text = strings.ReplaceAll(text, `phase: "helper_result"`, `phase: "qa_native_disabled_result"`)
	} else {
		text = strings.Replace(text, originalCall, strings.Replace(originalCall, readerbroker.HostName, host, 1), 1)
	}
	origins := `const uiOrigins = new Set(["http://127.0.0.1:11122", "http://localhost:11122"]);`
	if strings.Count(text, origins) != 1 {
		return "", proof, errors.New("source origin shape changed; staging refused")
	}
	text = strings.Replace(text, origins, `const uiOrigins = new Set([`+strconvJSON(origin)+`]);`, 1)
	if host == "" {
		manifest["permissions"] = []string{}
	} else {
		manifest["permissions"] = []string{"nativeMessaging"}
	}
	manifest["host_permissions"] = []string{origin + "/*"}
	scripts, ok := manifest["content_scripts"].([]any)
	if !ok || len(scripts) != 1 {
		return "", proof, errors.New("source content registration changed")
	}
	script, ok := scripts[0].(map[string]any)
	if !ok {
		return "", proof, errors.New("source content registration invalid")
	}
	script["matches"] = []string{origin + "/*"}
	stagedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", proof, err
	}
	if host == "" && (strings.Contains(string(stagedManifest), "nativeMessaging") || strings.Contains(text+string(content), "sendNativeMessage") || strings.Contains(text+string(content), "connectNative")) {
		return "", proof, errors.New("frontend native activation was not fully removed")
	}
	destination := filepath.Join(root, "ui-reader-broker")
	if err = os.Mkdir(destination, 0700); err != nil {
		return "", proof, err
	}
	for name, raw := range map[string][]byte{"manifest.json": stagedManifest, "content.js": content, "service-worker.js": []byte(text)} {
		if err = os.WriteFile(filepath.Join(destination, name), raw, 0600); err != nil {
			return "", proof, err
		}
	}
	proof = frontendStageProof{ContentSHA: frontendHash(content), OriginalWorkerSHA: frontendHash(worker), StagedWorkerSHA: frontendHash([]byte(text)), Origin: origin, ExtensionID: "dlibmmlopdahibfniinemhnghlifiple", QAHostName: host, NativeCalls: host != "", NativePermission: host != ""}
	if err = verifyDevelopmentFrontendStage(project, destination, proof); err != nil {
		return "", proof, err
	}
	return destination, proof, nil
}
func strconvJSON(value string) string { raw, _ := json.Marshal(value); return string(raw) }

func verifyDevelopmentFrontendStage(project, destination string, proof frontendStageProof) error {
	var actual, original map[string]any
	manifest, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	if err != nil {
		return err
	}
	if json.Unmarshal(manifest, &actual) != nil {
		return errors.New("staged manifest invalid")
	}
	source, err := os.ReadFile(filepath.Join(project, "ui-reader-broker", "manifest.json"))
	if err != nil {
		return err
	}
	if json.Unmarshal(source, &original) != nil || actual["key"] != original["key"] {
		return errors.New("staged extension identity changed")
	}
	worker, err := os.ReadFile(filepath.Join(destination, "service-worker.js"))
	if err != nil {
		return err
	}
	content, err := os.ReadFile(filepath.Join(destination, "content.js"))
	if err != nil {
		return err
	}
	originalContent, err := os.ReadFile(filepath.Join(project, "ui-reader-broker", "content.js"))
	if err != nil {
		return err
	}
	if frontendHash(worker) != proof.StagedWorkerSHA || frontendHash(content) != proof.ContentSHA || string(content) != string(originalContent) {
		return errors.New("frontend native activation/provenance preflight failed")
	}
	permissions, ok := actual["permissions"].([]any)
	if !ok || (proof.QAHostName == "" && len(permissions) != 0) || (proof.QAHostName != "" && (len(permissions) != 1 || permissions[0] != "nativeMessaging")) {
		return errors.New("frontend staged permissions changed")
	}
	if proof.QAHostName == "" {
		if strings.Contains(string(worker)+string(content), "sendNativeMessage") || strings.Contains(string(worker)+string(content), "connectNative") || strings.Contains(string(manifest), "nativeMessaging") {
			return errors.New("closed frontend native capability appeared")
		}
	} else {
		if err := readerbroker.ValidateFixtureQAHostName(proof.QAHostName); err != nil {
			return err
		}
		originalWorker, err := os.ReadFile(filepath.Join(project, "ui-reader-broker", "service-worker.js"))
		if err != nil {
			return err
		}
		configured := strings.ReplaceAll(string(originalWorker), "\r\n", "\n")
		configured = strings.Replace(configured, `const uiOrigins = new Set(["http://127.0.0.1:11122", "http://localhost:11122"]);`, `const uiOrigins = new Set([`+strconvJSON(proof.Origin)+`]);`, 1)
		configured = strings.Replace(configured, `chrome.runtime.sendNativeMessage("`+readerbroker.HostName+`",`, `chrome.runtime.sendNativeMessage("`+proof.QAHostName+`",`, 1)
		if string(worker) != configured || strings.Contains(string(worker), readerbroker.HostName) || strings.Contains(string(worker), frontendNativeDenial) || strings.Contains(string(worker), "connectNative") || strings.Count(string(worker), "chrome.runtime.sendNativeMessage(") != 1 {
			return errors.New("native frontend worker is not exact configured source")
		}
	}
	hosts, ok := actual["host_permissions"].([]any)
	if !ok || len(hosts) != 1 || hosts[0] != proof.Origin+"/*" {
		return errors.New("frontend staged origin changed")
	}
	scripts, ok := actual["content_scripts"].([]any)
	if !ok || len(scripts) != 1 {
		return errors.New("frontend content registration changed")
	}
	script, ok := scripts[0].(map[string]any)
	if !ok {
		return errors.New("frontend content registration invalid")
	}
	matches, ok := script["matches"].([]any)
	if !ok || len(matches) != 1 || matches[0] != proof.Origin+"/*" {
		return errors.New("frontend content origin changed")
	}
	if !strings.Contains(string(worker), `const uiOrigins = new Set([`+strconvJSON(proof.Origin)+`]);`) {
		return errors.New("frontend worker exact port gate changed")
	}
	return nil
}
