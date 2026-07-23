package mediaprovenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	ProviderName    = "c2patool"
	VerifierVersion = "c2pa-image-v1"
	maxImageBytes   = 20 << 20
)

type Result struct {
	ManifestState string
	TrustState    string
	AIOrigin      string
	EvidenceCodes []string
	AssetSHA256   string
	Rationale     string
	DurationMS    int64
}

type Inspector interface {
	Name() string
	Version() string
	Available() bool
	Inspect(context.Context, domain.MediaProvenanceAssessment, []string) (Result, error)
}

type C2PAToolInspector struct {
	executable string
	client     *http.Client
}

func NewC2PAToolInspector() *C2PAToolInspector {
	path := discoverExecutable()
	return &C2PAToolInspector{
		executable: path,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (i *C2PAToolInspector) Name() string    { return ProviderName }
func (i *C2PAToolInspector) Version() string { return VerifierVersion }
func (i *C2PAToolInspector) Available() bool { return i != nil && i.executable != "" }
func (i *C2PAToolInspector) Executable() string {
	if i == nil {
		return ""
	}
	return i.executable
}

func (i *C2PAToolInspector) Inspect(ctx context.Context, assessment domain.MediaProvenanceAssessment, trustedHostSuffixes []string) (Result, error) {
	started := time.Now()
	result := Result{ManifestState: "pending", TrustState: "pending", AIOrigin: "unknown"}
	if !i.Available() {
		result.ManifestState = "unavailable"
		result.TrustState = "not_applicable"
		result.DurationMS = time.Since(started).Milliseconds()
		return result, errors.New("c2patool is not available")
	}
	parsed, err := url.Parse(assessment.TargetURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return result, errors.New("media provenance requires an HTTPS image URL")
	}
	if !trustedHost(parsed.Hostname(), trustedHostSuffixes) {
		return result, fmt.Errorf("media host %q is outside the source allowlist", parsed.Hostname())
	}

	tempDir, err := os.MkdirTemp("", "aku-c2pa-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tempDir)
	imagePath, hash, err := i.download(ctx, assessment.TargetURL, tempDir, trustedHostSuffixes)
	if err != nil {
		return result, err
	}
	result.AssetSHA256 = hash
	settingsPath := filepath.Join(tempDir, "settings.json")
	settings := []byte(`{"version":1,"verify":{"remote_manifest_fetch":false,"ocsp_fetch":false}}`)
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		return result, err
	}
	command := exec.CommandContext(ctx, i.executable, imagePath, "--settings", settingsPath)
	output, commandErr := command.CombinedOutput()
	result.DurationMS = time.Since(started).Milliseconds()
	parsedResult, parseErr := ParseC2PAToolOutput(output)
	parsedResult.AssetSHA256 = result.AssetSHA256
	parsedResult.DurationMS = result.DurationMS
	if parseErr == nil {
		return parsedResult, nil
	}
	if commandErr != nil {
		return parsedResult, fmt.Errorf("c2patool failed: %w: %s", commandErr, boundedMessage(output))
	}
	return parsedResult, parseErr
}

func (i *C2PAToolInspector) download(ctx context.Context, target, tempDir string, trustedHostSuffixes []string) (string, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", err
	}
	client := *i.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 2 {
			return errors.New("media download exceeded redirect limit")
		}
		if req.URL == nil || req.URL.Scheme != "https" || !trustedHost(req.URL.Hostname(), trustedHostSuffixes) {
			return errors.New("media redirect left the source allowlist")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil || !trustedHost(response.Request.URL.Hostname(), trustedHostSuffixes) {
		return "", "", errors.New("media redirect left the source allowlist")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("media download returned HTTP %d", response.StatusCode)
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if contentType != "" && !strings.HasPrefix(contentType, "image/") && !strings.HasPrefix(contentType, "application/octet-stream") {
		return "", "", fmt.Errorf("media response is not an image: %s", contentType)
	}
	path := filepath.Join(tempDir, "asset"+imageExtension(response.Request.URL.Path, contentType))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", "", err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxImageBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", "", copyErr
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	if written > maxImageBytes {
		return "", "", fmt.Errorf("image exceeds %d MiB provenance limit", maxImageBytes>>20)
	}
	if written == 0 {
		return "", "", errors.New("image response was empty")
	}
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}

func imageExtension(path, contentType string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".tif", ".tiff", ".heic", ".heif", ".avif":
		return strings.ToLower(filepath.Ext(path))
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/tiff":
		return ".tiff"
	case "image/heic":
		return ".heic"
	case "image/heif":
		return ".heif"
	case "image/avif":
		return ".avif"
	default:
		return ".jpg"
	}
}

func ParseC2PAToolOutput(output []byte) (Result, error) {
	result := Result{ManifestState: "no_manifest", TrustState: "not_applicable", AIOrigin: "none", Rationale: "No embedded C2PA manifest was found; absence is neutral evidence."}
	var root any
	if err := json.Unmarshal(output, &root); err != nil {
		message := strings.ToLower(string(output))
		if strings.Contains(message, "no manifest") || strings.Contains(message, "manifest not found") || strings.Contains(message, "no claim found") {
			return result, nil
		}
		return result, fmt.Errorf("parse c2patool output: %w", err)
	}
	codes := collectStrings(root, func(key, value string) bool {
		key = strings.ToLower(key)
		return strings.Contains(key, "digitalsourcetype") || strings.Contains(key, "digital_source_type")
	})
	origin := classifyOrigin(codes)
	hasManifest := hasNonEmptyValue(root, "active_manifest") || hasNonEmptyValue(root, "activeManifest") || hasNonEmptyValue(root, "manifests")
	if !hasManifest && origin == "none" {
		return result, nil
	}
	result.ManifestState = "valid"
	result.TrustState = classifyTrust(root)
	result.AIOrigin = origin
	if origin == "generated" {
		result.EvidenceCodes = []string{"c2pa_trained_algorithmic_media"}
		result.Rationale = "The attached image declares C2PA trained-algorithmic provenance."
	} else if origin == "edited" {
		result.EvidenceCodes = []string{"c2pa_composite_with_trained_algorithmic_media"}
		result.Rationale = "The attached image declares C2PA AI-assisted or composite provenance."
	} else {
		result.Rationale = "A C2PA manifest was verified, but it did not declare an AI-origin digital source type."
	}
	if containsValidationFailure(root) {
		result.ManifestState = "invalid"
		result.TrustState = "not_evaluated"
		result.AIOrigin = "unknown"
		result.EvidenceCodes = nil
		result.Rationale = "C2PA metadata was present but did not pass integrity validation."
	}
	return result, nil
}

func discoverExecutable() string {
	name := "c2patool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if explicit := strings.TrimSpace(os.Getenv("AKU_C2PATOOL_PATH")); explicit != "" {
		if info, err := os.Stat(explicit); err == nil && !info.IsDir() {
			return explicit
		}
	}
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

func trustedHost(host string, suffixes []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range suffixes {
		suffix = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(suffix), "."))
		if suffix != "" && (host == suffix || strings.HasSuffix(host, "."+suffix)) {
			return true
		}
	}
	return false
}

func classifyOrigin(values []string) string {
	for _, value := range values {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "trainedalgorithmicmedia") && !strings.Contains(lower, "compositewith") {
			return "generated"
		}
		if strings.Contains(lower, "algorithmicmedia") && !strings.Contains(lower, "composite") {
			return "generated"
		}
	}
	for _, value := range values {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "compositewithtrainedalgorithmicmedia") || strings.Contains(lower, "compositesynthetic") {
			return "edited"
		}
	}
	return "none"
}

func classifyTrust(root any) string {
	text := strings.ToLower(string(mustJSON(root)))
	if strings.Contains(text, "signingcredential.untrusted") || strings.Contains(text, `"untrusted"`) {
		return "untrusted"
	}
	if strings.Contains(text, "signingcredential.trusted") || strings.Contains(text, `"trusted"`) {
		return "trusted"
	}
	return "not_evaluated"
}

func containsValidationFailure(root any) bool {
	text := strings.ToLower(string(mustJSON(root)))
	return strings.Contains(text, "claimsignature.mismatch") ||
		strings.Contains(text, "assertion.hasheduri.mismatch") ||
		strings.Contains(text, `"validation_status":"invalid"`) ||
		strings.Contains(text, `"validationstatus":"invalid"`)
}

func collectStrings(value any, include func(string, string) bool) []string {
	var result []string
	var walk func(any, string)
	walk = func(current any, key string) {
		switch typed := current.(type) {
		case map[string]any:
			for childKey, child := range typed {
				walk(child, childKey)
			}
		case []any:
			for _, child := range typed {
				walk(child, key)
			}
		case string:
			if include(key, typed) {
				result = append(result, typed)
			}
		}
	}
	walk(value, "")
	return result
}

func containsKey(value any, target string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == target || containsKey(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsKey(child, target) {
				return true
			}
		}
	}
	return false
}

func hasNonEmptyValue(value any, target string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == target {
				switch candidate := child.(type) {
				case nil:
					return false
				case string:
					return strings.TrimSpace(candidate) != ""
				case map[string]any:
					return len(candidate) > 0
				case []any:
					return len(candidate) > 0
				default:
					return true
				}
			}
			if hasNonEmptyValue(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasNonEmptyValue(child, target) {
				return true
			}
		}
	}
	return false
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func boundedMessage(value []byte) string {
	message := strings.TrimSpace(string(value))
	if len(message) > 500 {
		return message[:500] + "..."
	}
	return message
}
