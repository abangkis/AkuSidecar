package appshell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DiscoverSplitUI deliberately does not share capture/system-browser discovery.
// Branded Chrome 137+ ignores --load-extension, so the UI broker requires the
// separately pinned Chrome for Testing artifact. No profile is inspected.
func DiscoverSplitUI(ctx context.Context, sidecarExecutable, explicit string) (Result, error) {
	if runtime.GOOS != "windows" {
		return Result{}, fmt.Errorf("split UI Chromium is Windows-only")
	}
	executable := splitUIExecutable(sidecarExecutable, explicit)
	return validateSplitUI(ctx, executable, func(path string) (string, string, error) {
		version, err := platformVersion(path)
		if err != nil {
			return "", "", err
		}
		product, err := platformProductName(path)
		return version, product, err
	})
}

func splitUIExecutable(sidecarExecutable, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Clean(explicit)
	}
	return filepath.Join(filepath.Dir(sidecarExecutable), "chromium", "bin", "chrome.exe")
}

func validateSplitUI(ctx context.Context, executable string, probe func(string) (string, string, error)) (Result, error) {
	fail := func(reason string) (Result, error) {
		return Result{}, fmt.Errorf("split UI requires pinned Chrome for Testing: %s", reason)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !filepath.IsAbs(executable) || !strings.EqualFold(filepath.Base(executable), "chrome.exe") || !strings.EqualFold(filepath.Base(filepath.Dir(executable)), "bin") {
		return fail("expected an absolute <chromium>/bin/chrome.exe path")
	}
	pinFile, err := os.Open(filepath.Join(filepath.Dir(filepath.Dir(executable)), "pin.json"))
	if err != nil {
		return fail("pin.json is unavailable")
	}
	defer pinFile.Close()
	var pin struct {
		SchemaVersion int    `json:"schemaVersion"`
		Component     string `json:"component"`
		Channel       string `json:"channel"`
		Platform      string `json:"platform"`
		Version       string `json:"version"`
		SourceURL     string `json:"sourceUrl"`
		Executable    string `json:"executable"`
		SHA           string `json:"executableSha256"`
	}
	if err = json.NewDecoder(io.LimitReader(pinFile, 16384)).Decode(&pin); err != nil {
		return fail("pin.json is invalid")
	}
	expectedSource := "https://storage.googleapis.com/chrome-for-testing-public/" + pin.Version + "/win64/chrome-win64.zip"
	if pin.SchemaVersion != 1 || pin.Component != "AkuBrowserAppShellChromium" || pin.Channel != "stable" || pin.Platform != "win64" || strings.ReplaceAll(pin.Executable, `\`, "/") != "bin/chrome.exe" || !versionPattern.MatchString(pin.Version) || pin.SourceURL != expectedSource {
		return fail("unsupported pin metadata or non-CfT source")
	}
	expectedHash, err := hex.DecodeString(pin.SHA)
	if err != nil || len(expectedHash) != sha256.Size {
		return fail("invalid executable checksum")
	}
	version, product, err := probe(executable)
	if err != nil {
		return fail("executable version resource is unavailable")
	}
	if product != "Google Chrome for Testing" || version != pin.Version {
		return fail("executable is not the pinned Chrome for Testing version")
	}
	f, err := os.Open(executable)
	if err != nil {
		return fail("pinned executable unavailable")
	}
	defer f.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, f); err != nil {
		return fail("executable checksum read failed")
	}
	if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), pin.SHA) {
		return fail("executable checksum mismatch")
	}
	if err = ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{Status: "ready", Executable: executable, Version: version, Source: "split-ui-pinned-cft", Message: "Pinned Chrome for Testing supports the UI broker extension."}, nil
}
