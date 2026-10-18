//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type xLocalBootstrapEntry struct {
	Sequence int    `json:"sequence"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Phase    string `json:"phase"`
}

type xLocalBootstrapRepro struct {
	Schema              string                   `json:"schema"`
	Case                string                   `json:"case"`
	Adapter             string                   `json:"adapter"`
	Mechanism           xLocalBootstrapMechanism `json:"mechanism"`
	Early               json.RawMessage          `json:"early"`
	Late                json.RawMessage          `json:"late"`
	ConstantReads       int                      `json:"constantReads"`
	FirstFailure        *xLocalBootstrapEntry    `json:"firstFailure"`
	Chronology          []xLocalBootstrapEntry   `json:"chronology"`
	ChronologyTruncated bool                     `json:"chronologyTruncated"`
	CanAuthorize        bool                     `json:"canAuthorize"`
}

type xLocalBootstrapMechanism struct {
	PauseSource                 string                  `json:"pauseSource"`
	Evaluation                  string                  `json:"evaluation"`
	PauseAdapter                string                  `json:"pauseAdapter"`
	ActualInstrumentationPauses int                     `json:"actualInstrumentationPauses"`
	ActualOtherPauses           int                     `json:"actualOtherPauses"`
	PausedEvaluationAttempts    int                     `json:"pausedEvaluationAttempts"`
	LateConstantResult          *bool                   `json:"lateConstantResult"`
	Handoff                     *xLocalBootstrapHandoff `json:"handoff"`
}

type xLocalBootstrapHandoff struct {
	InstallAttempts       int    `json:"installAttempts"`
	Installed             int    `json:"installed"`
	Matched               int    `json:"matched"`
	Unexpected            int    `json:"unexpected"`
	Mismatched            int    `json:"mismatched"`
	Stale                 int    `json:"stale"`
	Replayed              int    `json:"replayed"`
	Removed               int    `json:"removed"`
	NormalCleanupVerified bool   `json:"normalCleanupVerified"`
	Outcome               string `json:"outcome"`
}

func xLocalBootstrapMechanismForCase(kind string) (xLocalBootstrapMechanism, bool) {
	value := xLocalBootstrapMechanism{PauseSource: "instrumentation_breakpoint", PauseAdapter: "none"}
	switch kind {
	case "constant":
		value.Evaluation = "constant_callframe_full"
	case "projection":
		value.Evaluation = "projection_callframe_full"
	case "instrumentation_resume_only":
		value.Evaluation = "resume_only"
	case "instrumentation_minimal":
		value.Evaluation = "constant_callframe_minimal"
	case "ordinary_full":
		value.PauseSource, value.Evaluation, value.PauseAdapter = "debugger_statement", "constant_callframe_full", "ordinary_other_as_capture_eligible"
	case "instrumentation_runtime":
		value.Evaluation = "constant_runtime_context"
	case "handoff_constant":
		value.PauseSource, value.Evaluation, value.PauseAdapter = "instrumentation_to_normal_breakpoint", "constant_callframe_full", "verified_normal_breakpoint_as_capture_eligible"
	case "handoff_projection":
		value.PauseSource, value.Evaluation, value.PauseAdapter = "instrumentation_to_normal_breakpoint", "projection_callframe_full", "verified_normal_breakpoint_as_capture_eligible"
	default:
		return value, false
	}
	return value, true
}

func decodeXLocalBootstrapRepro(raw []byte) (xLocalBootstrapRepro, error) {
	var value xLocalBootstrapRepro
	invalid := errors.New("local bootstrap diagnostics rejected")
	var fields map[string]json.RawMessage
	if len(raw) == 0 || len(raw) > 100000 || json.Unmarshal(raw, &fields) != nil || len(fields) != 11 ||
		string(fields["canAuthorize"]) != "false" ||
		(string(fields["chronologyTruncated"]) != "true" && string(fields["chronologyTruncated"]) != "false") {
		return value, invalid
	}
	for _, key := range []string{"schema", "case", "adapter", "mechanism", "early", "late", "constantReads", "firstFailure", "chronology", "chronologyTruncated", "canAuthorize"} {
		if fields[key] == nil || key == "constantReads" && bytes.Equal(fields[key], []byte("null")) {
			return value, invalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Schema != "aku.x.local-bootstrap-repro.v3" ||
		value.Adapter != "loopback_origin_comparisons_only" ||
		value.ConstantReads < 0 || value.ConstantReads > 64 || (value.Case == "projection" || value.Case == "handoff_projection") && value.ConstantReads != 0 ||
		len(value.Chronology) > 512 || value.Chronology == nil || value.CanAuthorize {
		return value, invalid
	}
	expected, knownCase := xLocalBootstrapMechanismForCase(value.Case)
	var mechanismFields map[string]json.RawMessage
	if !knownCase || json.Unmarshal(fields["mechanism"], &mechanismFields) != nil || len(mechanismFields) != 8 ||
		value.Mechanism.PauseSource != expected.PauseSource || value.Mechanism.Evaluation != expected.Evaluation || value.Mechanism.PauseAdapter != expected.PauseAdapter ||
		value.Mechanism.ActualInstrumentationPauses < 0 || value.Mechanism.ActualInstrumentationPauses > 64 ||
		value.Mechanism.ActualOtherPauses < 0 || value.Mechanism.ActualOtherPauses > 64 ||
		value.Mechanism.PausedEvaluationAttempts < 0 || value.Mechanism.PausedEvaluationAttempts > 64 ||
		value.ConstantReads > value.Mechanism.PausedEvaluationAttempts ||
		value.Mechanism.Evaluation == "resume_only" && value.Mechanism.PausedEvaluationAttempts != 0 {
		return value, invalid
	}
	for _, key := range []string{"actualInstrumentationPauses", "actualOtherPauses", "pausedEvaluationAttempts"} {
		if mechanismFields[key] == nil || bytes.Equal(mechanismFields[key], []byte("null")) {
			return value, invalid
		}
	}
	handoff := value.Case == "handoff_constant" || value.Case == "handoff_projection"
	variant := value.Case != "constant" && value.Case != "projection" && !handoff
	if handoff != (value.Mechanism.Handoff != nil) {
		return value, invalid
	}
	if !variant && value.Mechanism.LateConstantResult != nil || variant && !bytes.Equal(value.Late, []byte("null")) {
		return value, invalid
	}
	early, err := decodeXEarlyBootstrap(value.Early)
	if err != nil || value.ConstantReads > early.ReadAttempts || value.Mechanism.PausedEvaluationAttempts > early.ReadAttempts ||
		value.Mechanism.LateConstantResult != nil && !early.CleanupVerified {
		return value, invalid
	}
	value.Early, _ = json.Marshal(early)
	if !bytes.Equal(value.Late, []byte("null")) {
		late, err := decodeXBootstrapCurrentSelf(value.Late)
		if err != nil || !early.CleanupVerified {
			return value, invalid
		}
		value.Late, _ = json.Marshal(late)
	}
	oneOf := func(candidate string, allowed ...string) bool {
		for _, item := range allowed {
			if candidate == item {
				return true
			}
		}
		return false
	}
	if handoff {
		var handoffFields map[string]json.RawMessage
		h := value.Mechanism.Handoff
		if json.Unmarshal(mechanismFields["handoff"], &handoffFields) != nil || len(handoffFields) != 10 ||
			(string(handoffFields["normalCleanupVerified"]) != "true" && string(handoffFields["normalCleanupVerified"]) != "false") ||
			h.InstallAttempts < 0 || h.InstallAttempts > 8 || h.Installed < 0 || h.Installed > h.InstallAttempts ||
			h.Matched < 0 || h.Matched > h.Installed || h.Removed < 0 || h.Removed > h.Installed ||
			h.Unexpected < 0 || h.Unexpected > 64 || h.Mismatched < 0 || h.Mismatched > 64 || h.Stale < 0 || h.Stale > 64 || h.Replayed < 0 || h.Replayed > 64 ||
			h.Installed > value.Mechanism.ActualInstrumentationPauses || h.Matched > value.Mechanism.ActualOtherPauses ||
			value.Mechanism.PausedEvaluationAttempts > h.Matched || h.NormalCleanupVerified && h.Removed != h.Installed ||
			!oneOf(h.Outcome, "not_started", "waiting_for_normal", "normal_hit_verified", "install_failed", "hit_rejected", "cleanup_failed", "no_normal_hit") {
			return value, invalid
		}
		for _, key := range []string{"installAttempts", "installed", "matched", "unexpected", "mismatched", "stale", "replayed", "removed"} {
			if handoffFields[key] == nil || bytes.Equal(handoffFields[key], []byte("null")) {
				return value, invalid
			}
		}
		if !bytes.Equal(value.Late, []byte("null")) && (!h.NormalCleanupVerified || h.Matched == 0 || h.Unexpected+h.Mismatched+h.Stale+h.Replayed != 0) ||
			early.StopReason == "captured" && (h.Matched == 0 || h.Unexpected+h.Mismatched+h.Stale+h.Replayed != 0) {
			return value, invalid
		}
	}
	validEntry := func(entry xLocalBootstrapEntry) bool {
		if entry.Sequence < 1 || entry.Sequence > 1000000 || !oneOf(entry.Phase, "setup", "observation", "handoff_install", "paused_read", "cleanup", "late_read", "complete") {
			return false
		}
		if entry.Kind == "command" {
			return oneOf(entry.Status, "sent", "acknowledged", "error", "timeout", "rejected") && oneOf(entry.Label,
				"targets", "attach", "pageEnable", "frameTree", "runtimeEnable", "runtimeDisable", "debuggerEnable", "debuggerDisable",
				"breakpointSet", "breakpointRemove", "normalBreakpointSet", "normalBreakpointRemove", "reload", "resume", "pausedRead", "pausedReadRuntime", "lateRead", "browserClose", "initialFrame", "lateFrame")
		}
		return entry.Kind == "event" && oneOf(entry.Status, "observed", "error") && oneOf(entry.Label,
			"rootHome", "rootOther", "contextCreated", "contextsCleared", "contextDestroyed", "scriptParsed", "instrumentationPause", "otherPause", "resumed", "pageLoaded", "protocol", "initialHomePending", "initialHomeReady", "initialHomeUnavailable", "instrumentationOmitted", "breakpointRemovalOmitted", "handoffInstalled", "handoffHitMatched", "handoffHitRejected", "handoffHitMissing", "lateHomeUnavailable")
	}
	var first *xLocalBootstrapEntry
	for index, entry := range value.Chronology {
		if !validEntry(entry) || entry.Sequence != index+1 {
			return value, invalid
		}
		if first == nil && (entry.Status == "error" || entry.Status == "timeout") {
			copy := entry
			first = &copy
		}
	}
	if value.FirstFailure != nil {
		if !validEntry(*value.FirstFailure) || !oneOf(value.FirstFailure.Status, "error", "timeout") ||
			first != nil && *first != *value.FirstFailure || first == nil && (!value.ChronologyTruncated || value.FirstFailure.Sequence <= len(value.Chronology)) {
			return value, invalid
		}
	} else if first != nil {
		return value, invalid
	}
	return value, nil
}

func TestXLocalBootstrapOffline(t *testing.T) {
	const missing = `{"schema":"aku.x.bootstrap-current-self.v1","diagnostic":"bootstrap_missing","bootstrapState":"absent","sessionState":"absent","idFields":{"user_id":"absent","user_id_str":"absent"},"stableIdStringCandidates":0,"sessionIdsAgree":null,"authFields":{"loggedIn":"absent","isAuthenticated":"absent","authenticated":"absent","isActive":"absent"},"entityChecks":{"normalizedUsers":"unknown","rootUsers":"unknown"},"canAuthorize":false,"continuityProven":false}`
	early := xEarlyBootstrapCapture{Schema: "aku.x.early-bootstrap-capture.v1", StopReason: "deadline", PauseCount: 1, ReadAttempts: 1,
		Snapshot: json.RawMessage(missing), CleanupVerified: true}
	earlyRaw, _ := json.Marshal(early)
	mechanism, _ := xLocalBootstrapMechanismForCase("constant")
	mechanism.ActualInstrumentationPauses, mechanism.PausedEvaluationAttempts = 1, 1
	base := xLocalBootstrapRepro{Schema: "aku.x.local-bootstrap-repro.v3", Case: "constant", Adapter: "loopback_origin_comparisons_only", Mechanism: mechanism,
		Early: earlyRaw, Late: json.RawMessage(missing), ConstantReads: 1, Chronology: []xLocalBootstrapEntry{
			{Sequence: 1, Kind: "command", Label: "pausedRead", Status: "sent", Phase: "paused_read"},
			{Sequence: 2, Kind: "command", Label: "pausedRead", Status: "acknowledged", Phase: "paused_read"}}}
	raw, _ := json.Marshal(base)
	if _, err := decodeXLocalBootstrapRepro(raw); err != nil {
		t.Fatal("fixed value-free reproduction rejected")
	}
	for _, mutate := range []func(map[string]any){
		func(value map[string]any) { value["private"] = "source-value" },
		func(value map[string]any) { value["adapter"] = "x_origin_spoof" },
		func(value map[string]any) { value["mechanism"].(map[string]any)["pauseAdapter"] = "hidden_pause_spoof" },
		func(value map[string]any) { value["mechanism"].(map[string]any)["pausedEvaluationAttempts"] = nil },
		func(value map[string]any) { value["constantReads"] = nil },
		func(value map[string]any) { value["canAuthorize"] = true },
		func(value map[string]any) {
			value["chronology"].([]any)[0].(map[string]any)["label"] = "private-script-id"
		},
		func(value map[string]any) {
			value["chronology"].([]any)[0].(map[string]any)["rawError"] = "source-value"
		},
		func(value map[string]any) {
			value["firstFailure"] = map[string]any{"sequence": 2, "kind": "command", "label": "pausedRead", "status": "timeout", "phase": "paused_read"}
		},
	} {
		var candidate map[string]any
		_ = json.Unmarshal(raw, &candidate)
		mutate(candidate)
		unsafe, _ := json.Marshal(candidate)
		if _, err := decodeXLocalBootstrapRepro(unsafe); err == nil {
			t.Fatal("unsafe local reproduction diagnostics accepted")
		}
	}
	base.Chronology[1].Status = "timeout"
	base.FirstFailure = &base.Chronology[1]
	base.Late = json.RawMessage("null")
	raw, _ = json.Marshal(base)
	if _, err := decodeXLocalBootstrapRepro(raw); err != nil {
		t.Fatal("bounded first-failure evidence rejected")
	}
	for _, kind := range []string{"instrumentation_resume_only", "instrumentation_minimal", "ordinary_full", "instrumentation_runtime"} {
		base.Case = kind
		base.Mechanism, _ = xLocalBootstrapMechanismForCase(kind)
		base.ConstantReads = 0
		base.Mechanism.PausedEvaluationAttempts = 0
		variantRaw, _ := json.Marshal(base)
		if _, err := decodeXLocalBootstrapRepro(variantRaw); err != nil {
			t.Fatal("closed local mechanism case rejected")
		}
	}
	base.Case = "handoff_constant"
	base.Mechanism, _ = xLocalBootstrapMechanismForCase(base.Case)
	base.Mechanism.ActualInstrumentationPauses, base.Mechanism.ActualOtherPauses, base.Mechanism.PausedEvaluationAttempts = 1, 1, 1
	base.Mechanism.Handoff = &xLocalBootstrapHandoff{InstallAttempts: 1, Installed: 1, Matched: 1, Removed: 1, NormalCleanupVerified: true, Outcome: "normal_hit_verified"}
	base.ConstantReads = 1
	base.FirstFailure = nil
	base.Chronology[1].Status = "acknowledged"
	base.Late = json.RawMessage(missing)
	handoffRaw, _ := json.Marshal(base)
	if _, err := decodeXLocalBootstrapRepro(handoffRaw); err != nil {
		t.Fatal("bounded normal-breakpoint handoff rejected")
	}
	for _, mutate := range []func(map[string]any){
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["privateID"] = "source-value"
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["outcome"] = "private-error"
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["matched"] = 2
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["removed"] = 0
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["replayed"] = nil
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["unexpected"] = 1
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["mismatched"] = 1
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["stale"] = 1
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["replayed"] = 1
		},
		func(value map[string]any) {
			value["mechanism"].(map[string]any)["handoff"].(map[string]any)["normalCleanupVerified"] = false
		},
	} {
		var candidate map[string]any
		_ = json.Unmarshal(handoffRaw, &candidate)
		mutate(candidate)
		unsafe, _ := json.Marshal(candidate)
		if _, err := decodeXLocalBootstrapRepro(unsafe); err == nil {
			t.Fatal("unsafe handoff diagnostics accepted")
		}
	}
}

type xLocalBootstrapCase struct {
	Case                 string                `json:"case"`
	Diagnostic           string                `json:"diagnostic"`
	Result               *xLocalBootstrapRepro `json:"result"`
	OuterCleanupVerified bool                  `json:"outerCleanupVerified"`
}

func TestXLocalBootstrapRepro(t *testing.T) {
	if os.Getenv("AKU_X_LOCAL_BOOTSTRAP_ENABLED") != "1" {
		t.Skip("requires the accountless local bootstrap helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("local bootstrap source unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	reportPath := os.Getenv("AKU_X_LOCAL_BOOTSTRAP_REPORT_PATH")
	if !samePath(filepath.Dir(reportPath), qaParent) || !regexp.MustCompile(`^x-local-bootstrap-[a-f0-9]{32}\.report\.json$`).MatchString(filepath.Base(reportPath)) {
		t.Fatal("local bootstrap report scope rejected")
	}
	report := struct {
		Schema       string                 `json:"schema"`
		Cases        []*xLocalBootstrapCase `json:"cases"`
		CanAuthorize bool                   `json:"canAuthorize"`
	}{Schema: "aku.x.local-bootstrap-native.v1", Cases: []*xLocalBootstrapCase{}}
	defer func() {
		output, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Error("local bootstrap report unavailable")
			return
		}
		encodeErr := json.NewEncoder(output).Encode(report)
		closeErr := output.Close()
		if encodeErr != nil || closeErr != nil {
			t.Error("local bootstrap report failed")
		}
	}()
	cases := []string{"constant", "projection"}
	if os.Getenv("AKU_X_LOCAL_BOOTSTRAP_VARIANTS") == "1" {
		cases = []string{"instrumentation_resume_only", "instrumentation_minimal", "ordinary_full", "instrumentation_runtime"}
	}
	if os.Getenv("AKU_X_LOCAL_BOOTSTRAP_HANDOFF") == "1" {
		cases = []string{"handoff_constant", "handoff_projection"}
	}
	if os.Getenv("AKU_X_LOCAL_BOOTSTRAP_PROJECTION_ONLY") == "1" {
		if os.Getenv("AKU_X_LOCAL_BOOTSTRAP_HANDOFF") != "1" || os.Getenv("AKU_X_LOCAL_BOOTSTRAP_VARIANTS") == "1" {
			t.Fatal("projection-only retry requires the fixed handoff mode")
		}
		cases = []string{"handoff_projection"}
	}
	for _, kind := range cases {
		caseReport := &xLocalBootstrapCase{Case: kind, Diagnostic: "setup_incomplete"}
		report.Cases = append(report.Cases, caseReport)
		t.Run(kind, func(t *testing.T) { runXLocalBootstrapCase(t, project, kind, caseReport) })
	}
}

func runXLocalBootstrapCase(t *testing.T, project, kind string, report *xLocalBootstrapCase) {
	t.Helper()
	var documents struct {
		Home   string `json:"home"`
		Middle string `json:"middle"`
		Late   string `json:"late"`
	}
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-inline'; connect-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if r.Method != http.MethodGet || "http://"+r.Host != origin || r.URL.RawQuery != "" {
			http.Error(w, "denied", 403)
			return
		}
		switch r.URL.Path {
		case "/home":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, documents.Home)
		case "/middle.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, documents.Middle)
		case "/late.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, documents.Late)
		case "/favicon.ico":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	origin = server.URL
	t.Cleanup(server.Close)
	var nonceBytes [16]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		t.Fatal("local bootstrap nonce unavailable")
	}
	caseRoot := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "run-x-local-bootstrap-"+kind+"-"+hex.EncodeToString(nonceBytes[:]))
	if os.Mkdir(caseRoot, 0700) != nil {
		t.Fatal("local bootstrap root unavailable")
	}
	rootPin, err := pinXTwoPhaseDirectory(caseRoot)
	if err != nil {
		t.Fatal("local bootstrap root pin failed")
	}
	t.Cleanup(func() { _ = windows.CloseHandle(rootPin) })
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	config := chromeFixtureConfig{NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"), Root: caseRoot,
		Origin: origin, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-local-bootstrap-driver.mjs")}
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("local bootstrap Job bridge failed")
	}
	registerChromeFixtureBridge(bridge)
	var root windows.Handle
	var profilePin windows.Handle
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		report.OuterCleanupVerified = bridge.cleanupError() == nil && bridge.cleanupVerified()
		if root != 0 {
			state, err := windows.WaitForSingleObject(root, 5000)
			if err != nil || state != windows.WAIT_OBJECT_0 {
				report.OuterCleanupVerified = false
			}
			_ = windows.CloseHandle(root)
		}
		if profilePin != 0 {
			_ = windows.CloseHandle(profilePin)
		}
		if !report.OuterCleanupVerified {
			t.Error("local bootstrap outer Job/root cleanup unverified")
		}
	})
	if bridge.rpc(ctx, "local_bootstrap_documents", nil, &documents) != nil || len(documents.Home) == 0 || len(documents.Home) > 4096 || len(documents.Middle) > 128 || len(documents.Late) > 128 {
		t.Fatal("local bootstrap fixed documents rejected")
	}
	owner, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("local bootstrap launch failed")
	}
	root, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(owner.PID()))
	if err != nil {
		t.Fatal("local bootstrap Chrome pin failed")
	}
	verifyXChromeFixtureImage(t, uint32(owner.PID()), config.ChromePath)
	verifyXChromeFixtureImage(t, bridge.pid, config.NodePath)
	profilePin, err = pinXTwoPhaseDirectory(filepath.Join(caseRoot, "shared"))
	if err != nil {
		t.Fatal("local bootstrap profile pin failed")
	}
	readCtx, readCancel := context.WithTimeout(ctx, 45*time.Second)
	defer readCancel()
	var raw json.RawMessage
	if bridge.rpc(readCtx, "local_bootstrap_capture", map[string]any{"owner": owner.owner}, &raw) != nil {
		report.Diagnostic = "capture_rpc_failed"
		t.Fatal("local bootstrap capture RPC failed")
	}
	result, err := decodeXLocalBootstrapRepro(raw)
	if err != nil || result.Case != kind {
		t.Fatal("local bootstrap projected result rejected")
	}
	report.Result = &result
	report.Diagnostic = "capture_unqualified"
	early, _ := decodeXEarlyBootstrap(result.Early)
	late, lateErr := decodeXBootstrapCurrentSelf(result.Late)
	if result.FirstFailure != nil {
		report.Diagnostic = "command_failure_reproduced"
		if early.ReadAttempts == 0 {
			report.Diagnostic = "pre_read_failure"
		}
		t.Logf("Accountless %s firstFailure label=%s status=%s phase=%s readAttempts=%d; result does not qualify native X capture", kind, result.FirstFailure.Label, result.FirstFailure.Status, result.FirstFailure.Phase, early.ReadAttempts)
	} else if early.CleanupVerified && lateErr == nil && late.Diagnostic == "bootstrap_missing" {
		if kind == "constant" && result.ConstantReads > 0 {
			report.Diagnostic = "constant_control_passed"
		}
		if kind == "projection" && early.StopReason == "captured" {
			report.Diagnostic = "transient_bootstrap_captured"
		}
		if h := result.Mechanism.Handoff; h != nil && h.NormalCleanupVerified && h.Matched > 0 && h.Installed == h.Matched && h.Removed == h.Installed &&
			h.Unexpected+h.Mismatched+h.Stale+h.Replayed == 0 && !result.ChronologyTruncated {
			if kind == "handoff_constant" && result.ConstantReads == result.Mechanism.PausedEvaluationAttempts && result.ConstantReads > 0 {
				report.Diagnostic = "handoff_full_guard_control_passed"
			}
			if kind == "handoff_projection" && early.StopReason == "captured" {
				projection, err := decodeXBootstrapCurrentSelf(early.Snapshot)
				if err == nil && projection.Diagnostic == "projected" && projection.BootstrapState == "object" && projection.SessionState == "object" &&
					projection.IDFields.UserID == "string" && projection.IDFields.UserIDString == "string" && projection.StableIDStringCandidates == 2 &&
					projection.SessionIDsAgree != nil && *projection.SessionIDsAgree && projection.EntityChecks.NormalizedUsers == "match" && projection.EntityChecks.RootUsers == "match" &&
					projection.AuthFields.LoggedIn == "absent" && projection.AuthFields.IsAuthenticated == "absent" && projection.AuthFields.Authenticated == "absent" && projection.AuthFields.IsActive == "absent" {
					report.Diagnostic = "handoff_transient_bootstrap_captured"
				}
			}
		}
	} else if early.CleanupVerified && result.Mechanism.LateConstantResult != nil && *result.Mechanism.LateConstantResult {
		actualExpectedPause := result.Mechanism.ActualInstrumentationPauses > 0 && result.Mechanism.ActualOtherPauses == 0
		if kind == "ordinary_full" {
			actualExpectedPause = result.Mechanism.ActualOtherPauses > 0 && result.Mechanism.ActualInstrumentationPauses == 0
		}
		if actualExpectedPause && kind == "instrumentation_resume_only" && result.Mechanism.PausedEvaluationAttempts == 0 {
			report.Diagnostic = "resume_only_passed"
		}
		if actualExpectedPause && kind != "instrumentation_resume_only" && result.ConstantReads > 0 {
			report.Diagnostic = "constant_control_passed"
		}
	}
	if report.Diagnostic == "capture_unqualified" {
		t.Error("accountless bootstrap did not isolate an outcome; bounded diagnostics retained")
	}
}
