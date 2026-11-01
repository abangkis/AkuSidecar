package engine

import (
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type BridgeStatus struct {
	State              string                     `json:"state"`
	Compatible         bool                       `json:"compatible"`
	Reasons            []string                   `json:"reasons"`
	Warnings           []string                   `json:"warnings"`
	Expected           map[string]any             `json:"expected"`
	Actual             *domain.BridgeHeartbeat    `json:"actual"`
	NegotiatedProtocol *BridgeProtocolNegotiation `json:"negotiatedProtocol,omitempty"`
}

type BridgeProtocolNegotiation struct {
	Major           int  `json:"major"`
	Minor           int  `json:"minor"`
	LegacyHeartbeat bool `json:"legacyHeartbeat"`
}

func (e *Engine) BridgeStatus() BridgeStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	expectedSources := domain.SourceIDs()
	expectedAdapters := domain.ExpectedAdapterVersions()
	expectedMediaAdapters := domain.ExpectedMediaEvidenceAdapterVersions()
	requiredCapabilities := append([]string(nil), expectedBridgeActions...)
	requiredCapabilities = append(requiredCapabilities, "authority.read_only_bounded", "capture.bounded")
	status := BridgeStatus{State: "reconnecting", Expected: map[string]any{"bridgeId": ExpectedBridgeID, "extensionVersion": ExpectedBridgeVersion, "runtimeRevision": ExpectedBridgeRevision, "buildId": ExpectedBridgeBuildID, "focusPolicyRevision": ExpectedBridgeFocusPolicyRevision, "protocolMajor": BridgeProtocolMajor, "minimumProtocolMinor": MinimumBridgeProtocolMinor, "supportedProtocolMinor": BridgeProtocolMinor, "adapterVersions": expectedAdapters, "mediaEvidenceAdapterVersions": expectedMediaAdapters, "contract": domain.BridgeContractVersion, "manifestVersion": 3, "sources": expectedSources, "actions": expectedBridgeActions, "requiredCapabilities": requiredCapabilities, "authority": "read_only_bounded", "captureLimits": domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}}}
	status.Expected["captureSurfacePolicy"] = expectedBridgeSurfacePolicy()
	if e.heartbeat == nil {
		return status
	}
	copy := *e.heartbeat
	status.Actual = &copy
	status.State = "healthy"
	protocolMajor, protocolMinor, legacyHeartbeat := bridgeProtocol(copy)
	negotiatedMinor := protocolMinor
	if negotiatedMinor > BridgeProtocolMinor {
		negotiatedMinor = BridgeProtocolMinor
	}
	status.NegotiatedProtocol = &BridgeProtocolNegotiation{Major: protocolMajor, Minor: negotiatedMinor, LegacyHeartbeat: legacyHeartbeat}
	if copy.BridgeID != ExpectedBridgeID {
		status.Reasons = append(status.Reasons, "bridge id mismatch")
	}
	if protocolMajor != BridgeProtocolMajor {
		status.Reasons = append(status.Reasons, "bridge protocol major mismatch")
	}
	if protocolMinor < MinimumBridgeProtocolMinor {
		status.Reasons = append(status.Reasons, "bridge protocol minor is too old")
	}
	if protocolMinor > MaximumBridgeProtocolMinor {
		status.Reasons = append(status.Reasons, "bridge protocol minor is out of bounds")
	}
	if !validBoundedBridgeCapabilities(copy.UpdateCapabilities) {
		status.Reasons = append(status.Reasons, "bridge update capabilities are malformed or out of bounds")
	}
	if legacyHeartbeat {
		status.Warnings = append(status.Warnings, "legacy Bridge v2 heartbeat inferred as protocol 2.0")
	} else if protocolMinor > BridgeProtocolMinor {
		status.Warnings = append(status.Warnings, "bridge protocol minor is newer than this Sidecar")
	}
	if copy.ExtensionVersion != ExpectedBridgeVersion {
		status.Warnings = append(status.Warnings, "extension version differs from the Sidecar release reference")
	}
	if copy.RuntimeRevision != ExpectedBridgeRevision {
		status.Warnings = append(status.Warnings, "runtime revision differs from the Sidecar release reference")
	}
	if copy.ContractVersion != domain.BridgeContractVersion {
		status.Reasons = append(status.Reasons, "bridge contract mismatch")
	}
	if copy.BuildID != ExpectedBridgeBuildID {
		status.Warnings = append(status.Warnings, "bridge build differs from the Sidecar release reference")
	}
	if copy.FocusPolicyRevision != ExpectedBridgeFocusPolicyRevision {
		status.Reasons = append(status.Reasons, "bridge focus policy revision mismatch")
	}
	if !bridgeSurfacePolicyMatches(copy.CaptureSurfacePolicy) {
		status.Reasons = append(status.Reasons, "bridge capture surface policy mismatch; reload required")
	}
	if !hasRequiredStringMapKeys(copy.AdapterVersions, expectedAdapters) {
		status.Reasons = append(status.Reasons, "required source adapter capability missing")
	} else if !sameStringMap(copy.AdapterVersions, expectedAdapters) {
		status.Warnings = append(status.Warnings, "source adapter versions differ from the Sidecar release reference")
	}
	if !hasRequiredStringMapKeys(copy.MediaEvidenceAdapterVersions, expectedMediaAdapters) {
		status.Reasons = append(status.Reasons, "required media evidence capability missing")
	} else if !sameStringMap(copy.MediaEvidenceAdapterVersions, expectedMediaAdapters) {
		status.Warnings = append(status.Warnings, "media evidence adapter versions differ from the Sidecar release reference")
	}
	if copy.ManifestVersion != 3 {
		status.Reasons = append(status.Reasons, "manifest version mismatch")
	}
	if !containsRequiredStrings(copy.Sources, expectedSources) {
		status.Reasons = append(status.Reasons, "required bridge source capability missing")
	}
	if !containsRequiredStrings(copy.Actions, expectedBridgeActions) {
		status.Reasons = append(status.Reasons, "required bridge action capability missing")
	}
	if copy.Authority != "read_only_bounded" {
		status.Reasons = append(status.Reasons, "bridge authority mismatch")
	}
	if copy.CaptureLimits != (domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}) {
		status.Reasons = append(status.Reasons, "capture limits mismatch")
	}
	if !validBridgeSourceReadiness(copy.SourceAccess.Sources, copy.Sources) {
		status.Reasons = append(status.Reasons, "source readiness contract mismatch")
	}
	if len(e.config.Bridge.TrustedExtensionOrigins) > 0 && (copy.ExtensionOrigin == "" || !e.trustedBridgeExtensionOrigin(copy.ExtensionOrigin)) {
		status.Reasons = append(status.Reasons, "AkuBridge extension origin is not trusted; keep the Chrome Web Store extension or configure one exact development origin")
	}
	if len(e.bridgeOrigins) > 1 {
		status.Reasons = append(status.Reasons, "multiple AkuBridge extension origins are active; disable the legacy unpacked extension and keep only the intended AkuBridge installation")
	}
	status.Compatible = len(status.Reasons) == 0
	if !status.Compatible {
		status.State = "incompatible"
	} else if len(status.Warnings) > 0 {
		status.State = "degraded"
	}
	return status
}

func (e *Engine) trustedBridgeExtensionOrigin(actual string) bool {
	actual = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(actual)), "/")
	// Directly constructed engines are used by deterministic unit tests. The
	// executable configuration path always validates a non-empty allowlist.
	if len(e.config.Bridge.TrustedExtensionOrigins) == 0 {
		return actual == ""
	}
	for _, allowed := range e.config.Bridge.TrustedExtensionOrigins {
		if actual == strings.TrimSuffix(strings.ToLower(strings.TrimSpace(allowed)), "/") {
			return true
		}
	}
	return false
}

func ExpectedHeartbeat() domain.BridgeHeartbeat {
	readiness := make([]domain.BridgeSourceReadiness, 0, len(domain.SourceIDs()))
	for _, source := range domain.SourceIDs() {
		readiness = append(readiness, domain.BridgeSourceReadiness{Source: source, PermissionGranted: true, ScriptRegistered: true, Ready: true, Reason: "ready"})
	}
	return domain.BridgeHeartbeat{CaptureSurfacePolicy: expectedBridgeSurfacePolicy(), BridgeID: ExpectedBridgeID, ExtensionVersion: ExpectedBridgeVersion, RuntimeRevision: ExpectedBridgeRevision, BuildID: ExpectedBridgeBuildID, FocusPolicyRevision: ExpectedBridgeFocusPolicyRevision, ProtocolMajor: BridgeProtocolMajor, ProtocolMinor: BridgeProtocolMinor, UpdateCapabilities: []string{"background_check", "staged_apply", "idle_deferral", "rollback_status"}, AdapterVersions: domain.ExpectedAdapterVersions(), MediaEvidenceAdapterVersions: domain.ExpectedMediaEvidenceAdapterVersions(), ContractVersion: domain.BridgeContractVersion, ManifestVersion: 3, Sources: domain.SourceIDs(), Actions: append([]string(nil), expectedBridgeActions...), Authority: "read_only_bounded", CaptureLimits: domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}, SourceAccess: domain.BridgeSourceAccess{GrantedSources: domain.SourceIDs(), Sources: readiness, ObservedAt: domain.Now()}}
}

func bridgeProtocol(value domain.BridgeHeartbeat) (major, minor int, legacy bool) {
	if value.ProtocolMajor == 0 && value.ProtocolMinor == 0 && value.ContractVersion == domain.BridgeContractVersion {
		return BridgeProtocolMajor, 0, true
	}
	return value.ProtocolMajor, value.ProtocolMinor, false
}

func sameStringMap(actual, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func hasRequiredStringMapKeys(actual, required map[string]string) bool {
	for key := range required {
		if strings.TrimSpace(actual[key]) == "" {
			return false
		}
	}
	return true
}

func containsRequiredStrings(actual, required []string) bool {
	values := make(map[string]bool, len(actual))
	for _, value := range actual {
		values[value] = true
	}
	for _, value := range required {
		if !values[value] {
			return false
		}
	}
	return true
}

func validBoundedBridgeCapabilities(values []string) bool {
	if len(values) > MaxBridgeUpdateCapabilities {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) || len(value) > MaxBridgeCapabilityLength || seen[value] {
			return false
		}
		for index := 0; index < len(value); index++ {
			character := value[index]
			if index == 0 && (character < 'a' || character > 'z') {
				return false
			}
			if index > 0 && (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
				return false
			}
		}
		seen[value] = true
	}
	return true
}

func validBridgeSourceReadiness(actual []domain.BridgeSourceReadiness, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	values := make(map[string]domain.BridgeSourceReadiness, len(actual))
	for _, value := range actual {
		if _, duplicate := values[value.Source]; duplicate {
			return false
		}
		if value.Ready != (value.PermissionGranted && value.ScriptRegistered) {
			return false
		}
		values[value.Source] = value
	}
	for _, source := range expected {
		if _, ok := values[source]; !ok {
			return false
		}
	}
	return true
}
