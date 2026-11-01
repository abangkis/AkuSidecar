package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Status reads need only the existing Engine state. No Store, process, source
// profile or heartbeat side effect is involved in these characterization cases.
func bridgeStatusCharacterizationEngine(value *domain.BridgeHeartbeat) *Engine {
	return &Engine{heartbeat: value, bridgeOrigins: map[string]time.Time{}}
}

func TestBridgeStatusCharacterizationWireStates(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		compatible  bool
		mutate      func(*domain.BridgeHeartbeat)
	}{
		{name: "no-heartbeat", state: "reconnecting"},
		{name: "current", state: "healthy", compatible: true, mutate: func(*domain.BridgeHeartbeat) {}},
		{name: "version-advisory", state: "degraded", compatible: true, mutate: func(value *domain.BridgeHeartbeat) { value.ExtensionVersion = "fixture-old" }},
		{name: "authority-rejection", state: "incompatible", mutate: func(value *domain.BridgeHeartbeat) { value.Authority = "write_anywhere" }},
		{name: "negative-minor", state: "incompatible", mutate: func(value *domain.BridgeHeartbeat) { value.ProtocolMinor = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var heartbeat *domain.BridgeHeartbeat
			if tc.mutate != nil {
				value := ExpectedHeartbeat()
				tc.mutate(&value)
				heartbeat = &value
			}
			status := bridgeStatusCharacterizationEngine(heartbeat).BridgeStatus()
			if status.State != tc.state || status.Compatible != tc.compatible {
				t.Fatalf("state contract: %+v", status)
			}
			wire, err := json.Marshal(status)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(wire, &fields); err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(fields))
			for key := range fields {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			wantKeys := []string{"actual", "compatible", "expected", "reasons", "state", "warnings"}
			if heartbeat != nil {
				wantKeys = append(wantKeys, "negotiatedProtocol")
				slices.Sort(wantKeys)
			}
			if !reflect.DeepEqual(keys, wantKeys) {
				t.Fatalf("wire keys: %s", wire)
			}
			if heartbeat == nil {
				if string(fields["actual"]) != "null" || string(fields["reasons"]) != "null" || string(fields["warnings"]) != "null" || status.NegotiatedProtocol != nil {
					t.Fatalf("empty wire contract: %s", wire)
				}
			} else if !reflect.DeepEqual(status.Actual, heartbeat) || status.NegotiatedProtocol.Major != 2 || status.NegotiatedProtocol.LegacyHeartbeat {
				t.Fatalf("actual/negotiation contract: %+v", status)
			}
			if tc.name == "current" && (string(fields["reasons"]) != "null" || string(fields["warnings"]) != "null" || string(fields["negotiatedProtocol"]) != `{"major":2,"minor":0,"legacyHeartbeat":false}`) {
				t.Fatalf("current protocol JSON: %s", wire)
			}
			expectedKeys := make([]string, 0, len(status.Expected))
			for key := range status.Expected {
				expectedKeys = append(expectedKeys, key)
			}
			slices.Sort(expectedKeys)
			if !reflect.DeepEqual(expectedKeys, []string{"actions", "adapterVersions", "authority", "bridgeId", "buildId", "captureLimits", "captureSurfacePolicy", "contract", "extensionVersion", "focusPolicyRevision", "manifestVersion", "mediaEvidenceAdapterVersions", "minimumProtocolMinor", "protocolMajor", "requiredCapabilities", "runtimeRevision", "sources", "supportedProtocolMinor"}) {
				t.Fatalf("expected projection keys: %v", expectedKeys)
			}
			required := append(slices.Clone(expectedBridgeActions), "authority.read_only_bounded", "capture.bounded")
			if !reflect.DeepEqual(status.Expected["requiredCapabilities"], required) {
				t.Fatalf("required capability order: %+v", status.Expected)
			}
		})
	}
}

func TestBridgeStatusCharacterizationReasonWarningOrder(t *testing.T) {
	value := ExpectedHeartbeat()
	value.BridgeID, value.ProtocolMajor, value.ProtocolMinor = "wrong", 3, 65536
	value.UpdateCapabilities = []string{"bad-format"}
	value.ExtensionVersion, value.RuntimeRevision, value.BuildID = "old-version", "old-revision", "old-build"
	value.ContractVersion, value.FocusPolicyRevision = "wrong", "wrong"
	value.CaptureSurfacePolicy = domain.BridgeCaptureSurfacePolicy{}
	value.AdapterVersions, value.MediaEvidenceAdapterVersions = nil, nil
	value.ManifestVersion, value.Sources, value.Actions = 2, []string{"x"}, nil
	value.Authority = "write_anywhere"
	value.CaptureLimits.MaxSnapshots++
	value.SourceAccess.Sources = nil
	value.ExtensionOrigin = "chrome-extension://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runtime := bridgeStatusCharacterizationEngine(&value)
	runtime.config.Bridge.TrustedExtensionOrigins = []string{"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	runtime.bridgeOrigins["first"], runtime.bridgeOrigins["second"] = time.Unix(1, 0), time.Unix(2, 0)
	status := runtime.BridgeStatus()
	wantReasons := []string{
		"bridge id mismatch", "bridge protocol major mismatch", "bridge protocol minor is out of bounds",
		"bridge update capabilities are malformed or out of bounds", "bridge contract mismatch", "bridge focus policy revision mismatch",
		"bridge capture surface policy mismatch; reload required", "required source adapter capability missing", "required media evidence capability missing",
		"manifest version mismatch", "required bridge source capability missing", "required bridge action capability missing",
		"bridge authority mismatch", "capture limits mismatch", "source readiness contract mismatch",
		"AkuBridge extension origin is not trusted; keep the Chrome Web Store extension or configure one exact development origin",
		"multiple AkuBridge extension origins are active; disable the legacy unpacked extension and keep only the intended AkuBridge installation",
	}
	wantWarnings := []string{"bridge protocol minor is newer than this Sidecar", "extension version differs from the Sidecar release reference", "runtime revision differs from the Sidecar release reference", "bridge build differs from the Sidecar release reference"}
	if status.Compatible || status.State != "incompatible" || !reflect.DeepEqual(status.Reasons, wantReasons) || !reflect.DeepEqual(status.Warnings, wantWarnings) || status.NegotiatedProtocol.Minor != 0 {
		t.Fatalf("reason/warning order or rejection precedence changed: %+v", status)
	}
}

func TestBridgeStatusCharacterizationCapabilityBounds(t *testing.T) {
	names := func(count int) []string {
		result := make([]string, count)
		for i := range result {
			result[i] = fmt.Sprintf("future_%d", i)
		}
		return result
	}
	for _, tc := range []struct {
		name   string
		values []string
		valid  bool
	}{
		{"nil", nil, true}, {"unknown-valid", []string{"future_capability_v2"}, true},
		{"count-32", names(32), true}, {"count-33", names(33), false},
		{"length-64", []string{strings.Repeat("a", 64)}, true}, {"length-65", []string{strings.Repeat("a", 65)}, false},
		{"empty", []string{""}, false}, {"duplicate", []string{"same", "same"}, false},
		{"digit-first", []string{"1future"}, false}, {"non-ascii", []string{"future_é"}, false},
		{"whitespace", []string{" future "}, false}, {"dash", []string{"future-capability"}, false}, {"uppercase", []string{"Future"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := ExpectedHeartbeat()
			value.UpdateCapabilities = tc.values
			status := bridgeStatusCharacterizationEngine(&value).BridgeStatus()
			if status.Compatible != tc.valid || len(status.Warnings) != 0 {
				t.Fatalf("capability acceptance changed: %+v", status)
			}
			if !tc.valid && !reflect.DeepEqual(status.Reasons, []string{"bridge update capabilities are malformed or out of bounds"}) {
				t.Fatalf("capability rejection changed: %+v", status)
			}
		})
	}
}

func TestBridgeStatusCharacterizationSourceReadiness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		valid  bool
		mutate func(*domain.BridgeHeartbeat)
	}{
		{"missing", false, func(value *domain.BridgeHeartbeat) { value.SourceAccess.Sources = value.SourceAccess.Sources[1:] }},
		{"duplicate", false, func(value *domain.BridgeHeartbeat) { value.SourceAccess.Sources[1] = value.SourceAccess.Sources[0] }},
		{"extra", false, func(value *domain.BridgeHeartbeat) {
			value.SourceAccess.Sources = append(value.SourceAccess.Sources, domain.BridgeSourceReadiness{Source: "extra"})
		}},
		{"unknown-replacement", false, func(value *domain.BridgeHeartbeat) { value.SourceAccess.Sources[0].Source = "unknown" }},
		{"inconsistent-ready", false, func(value *domain.BridgeHeartbeat) { value.SourceAccess.Sources[0].PermissionGranted = false }},
		{"valid-unready", true, func(value *domain.BridgeHeartbeat) {
			value.SourceAccess.Sources[0].ScriptRegistered = false
			value.SourceAccess.Sources[0].Ready = false
		}},
		{"reordered", true, func(value *domain.BridgeHeartbeat) { slices.Reverse(value.SourceAccess.Sources) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := ExpectedHeartbeat()
			tc.mutate(&value)
			status := bridgeStatusCharacterizationEngine(&value).BridgeStatus()
			if status.Compatible != tc.valid || (!tc.valid && !reflect.DeepEqual(status.Reasons, []string{"source readiness contract mismatch"})) {
				t.Fatalf("source-readiness contract changed: %+v", status)
			}
		})
	}
}

func TestBridgeStatusCharacterizationTrustedOrigin(t *testing.T) {
	const allowed = "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, tc := range []struct {
		name, actual string
		configured   bool
		valid        bool
	}{
		{"exact", allowed, true, true}, {"normalized", " CHROME-EXTENSION://AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/ ", true, true},
		{"different", "chrome-extension://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true, false}, {"missing", "", true, false},
		{"empty-config-with-origin", allowed, false, true}, {"empty-config-without-origin", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := ExpectedHeartbeat()
			value.ExtensionOrigin = tc.actual
			runtime := bridgeStatusCharacterizationEngine(&value)
			if tc.configured {
				runtime.config.Bridge.TrustedExtensionOrigins = []string{allowed}
			}
			status := runtime.BridgeStatus()
			if status.Compatible != tc.valid || status.Actual.ExtensionOrigin != tc.actual || (!tc.valid && !reflect.DeepEqual(status.Reasons, []string{"AkuBridge extension origin is not trusted; keep the Chrome Web Store extension or configure one exact development origin"})) {
				t.Fatalf("origin gate or input normalization changed: %+v", status)
			}
		})
	}
}

func TestBridgeStatusCharacterizationReadHasNoLifecycleEffects(t *testing.T) {
	value := ExpectedHeartbeat()
	value.ReceivedAt, value.SourceAccess.ObservedAt = "2020-01-01T00:00:00Z", "2020-01-01T00:00:00Z"
	runtime := bridgeStatusCharacterizationEngine(&value)
	runtime.headlessAccess = []domain.Source{domain.SourceX}
	runtime.bridgeOrigins["old-first"], runtime.bridgeOrigins["old-second"] = time.Unix(1, 0), time.Unix(2, 0)
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	first, second := runtime.BridgeStatus(), runtime.BridgeStatus()
	after, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || string(before) != string(after) || runtime.heartbeat != &value || first.Actual == &value || !reflect.DeepEqual(runtime.headlessAccess, []domain.Source{domain.SourceX}) || len(runtime.bridgeOrigins) != 2 || runtime.bridgeOrigins["old-first"] != time.Unix(1, 0) || runtime.bridgeOrigins["old-second"] != time.Unix(2, 0) {
		t.Fatal("status read changed heartbeat, retained authority, origin registry or snapshot behavior")
	}
	if !reflect.DeepEqual(first.Reasons, []string{"multiple AkuBridge extension origins are active; disable the legacy unpacked extension and keep only the intended AkuBridge installation"}) {
		t.Fatalf("status read added freshness/expiry behavior: %+v", first)
	}
}

func TestBridgeStatusCharacterizationExpectedHeartbeat(t *testing.T) {
	value := ExpectedHeartbeat()
	if value.ReceivedAt != "" || value.ProtocolMajor != 2 || value.ProtocolMinor != 0 || value.BridgeID != ExpectedBridgeID || value.ExtensionVersion != ExpectedBridgeVersion || value.RuntimeRevision != ExpectedBridgeRevision || value.BuildID != ExpectedBridgeBuildID || value.FocusPolicyRevision != ExpectedBridgeFocusPolicyRevision || value.ContractVersion != domain.BridgeContractVersion || value.CaptureSurfacePolicy != expectedBridgeSurfacePolicy() || value.CaptureLimits != (domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}) {
		t.Fatalf("heartbeat blueprint changed: %+v", value)
	}
	if !reflect.DeepEqual(value.Actions, expectedBridgeActions) || !reflect.DeepEqual(value.UpdateCapabilities, []string{"background_check", "staged_apply", "idle_deferral", "rollback_status"}) || !reflect.DeepEqual(value.Sources, domain.SourceIDs()) || !reflect.DeepEqual(value.SourceAccess.GrantedSources, domain.SourceIDs()) {
		t.Fatalf("heartbeat capability/source blueprint changed: %+v", value)
	}
	if _, err := time.Parse(time.RFC3339Nano, value.SourceAccess.ObservedAt); err != nil {
		t.Fatal(err)
	}
	for _, source := range value.SourceAccess.Sources {
		if !source.PermissionGranted || !source.ScriptRegistered || !source.Ready || source.Reason != "ready" {
			t.Fatalf("heartbeat source blueprint changed: %+v", source)
		}
	}
	value.Actions[0] = "fixture-mutation"
	value.AdapterVersions["x"] = "fixture-mutation"
	next := ExpectedHeartbeat()
	if !reflect.DeepEqual(next.Actions, expectedBridgeActions) || next.AdapterVersions["x"] != domain.ExpectedAdapterVersions()["x"] {
		t.Fatal("heartbeat blueprint mutated shared action/adapter state")
	}
}
