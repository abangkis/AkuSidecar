package appshell

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/diagnostictrace"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

const mainWorkerSchema = "aku.main-transport-worker.v1"

type mainWorkerCause struct {
	Schema string `json:"schema"`
	Reason string `json:"reason"`
	Domain string `json:"domain"`
	Method string `json:"method"`
	Role   string `json:"role"`
	Stage  string `json:"stage"`
}

type mainWorkerSnapshot struct {
	mainWorkerCause
	Reported    bool   `json:"reported"`
	ControlRead string `json:"controlRead"`
	Exited      bool   `json:"exited"`
	ExitCode    *int64 `json:"exitCode"`
	Intentional bool   `json:"intentional"`
	Cleanup     string `json:"cleanup"`
	Complete    bool   `json:"complete"`
}

// The application vocabulary is closed here. The generic recorder owns first
// cause and independent cleanup; it neither grants authority nor retains bytes.
type mainWorkerDiagnostic struct {
	mu      sync.Mutex
	trace   *diagnostictrace.Recorder
	value   mainWorkerSnapshot
	observe func(json.RawMessage)
}

func newMainWorkerDiagnostic(observe func(json.RawMessage)) *mainWorkerDiagnostic {
	return &mainWorkerDiagnostic{trace: diagnostictrace.New("main_transport_worker"), observe: observe,
		value: mainWorkerSnapshot{mainWorkerCause: mainWorkerCause{mainWorkerSchema, "none", "unknown", "none", "unknown", "unknown"}, ControlRead: "none", Cleanup: "not_started", Complete: true}}
}

func member(value, values string) bool {
	for _, item := range bytes.Split([]byte(values), []byte("|")) {
		if value == string(item) {
			return true
		}
	}
	return false
}
func validMainWorkerCause(c mainWorkerCause, allowNone bool) bool {
	return (allowNone && c.Reason == "none" || member(c.Reason, "explicit_close|command_timeout|input_end|input_error|output_error|invalid_frame|inbound_limit|reply_scope|listener_failure|sequence_limit|control_end|control_error|control_input_limit|control_output_limit|uncaught_exception|unhandled_rejection|entry_failed|unknown")) &&
		member(c.Domain, "cdp|control|process|owner|unknown") && member(c.Role, "ig_reader|ig_collector|diagnostic|ordinary|unknown") &&
		member(c.Stage, "command|framing|stream|shutdown|exception|unknown") && member(c.Method, "Browser.getVersion|Target.createTarget|Target.attachToTarget|Target.closeTarget|Target.getTargets|Page.enable|Runtime.enable|Network.enable|Page.getFrameTree|Page.navigate|Network.setCacheDisabled|Network.setBypassServiceWorker|Fetch.enable|Fetch.takeResponseBodyAsStream|IO.read|IO.close|Fetch.fulfillRequest|Fetch.disable|Runtime.evaluate|Network.getCookies|Network.setCookies|none|other")
}

// Decode exact envelopes, rejecting duplicate keys before unmarshalling.
func mainWorkerObject(raw []byte, keys string) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		t, err = d.Token()
		key, ok := t.(string)
		if err != nil || !ok || seen[key] || !member(key, keys) {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	if t, err = d.Token(); err != nil || t != json.Delim('}') {
		return false
	}
	if _, err = d.Token(); err != io.EOF {
		return false
	}
	return len(seen) == len(bytes.Split([]byte(keys), []byte("|")))
}

func mainWorkerLifecycleFrame(raw []byte) (mainWorkerCause, bool, bool) {
	var frame struct {
		Event  string          `json:"event"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(raw, &frame) != nil || frame.Event != "main_transport_lifecycle" {
		return mainWorkerCause{}, false, false
	}
	var c mainWorkerCause
	valid := len(raw) <= 512 && mainWorkerObject(raw, "event|result") && mainWorkerObject(frame.Result, "schema|reason|domain|method|role|stage") && json.Unmarshal(frame.Result, &c) == nil && c.Schema == "aku.main-transport-lifecycle.v1" && validMainWorkerCause(c, false)
	return c, true, valid
}

// ValidateMainBrowserWorkerDiagnostic rejects private or unrecognized fields.
// Consumers must validate again before attaching a diagnostic to public status.
func ValidateMainBrowserWorkerDiagnostic(raw []byte) bool {
	var v mainWorkerSnapshot
	if len(raw) == 0 || len(raw) > 1024 || !mainWorkerObject(raw, "schema|reason|domain|method|role|stage|reported|controlRead|exited|exitCode|intentional|cleanup|complete") || json.Unmarshal(raw, &v) != nil {
		return false
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, key := range []string{"reported", "exited", "intentional", "complete"} {
		if !bytes.Equal(fields[key], []byte("true")) && !bytes.Equal(fields[key], []byte("false")) {
			return false
		}
	}
	return v.Schema == mainWorkerSchema && validMainWorkerCause(v.mainWorkerCause, true) && member(v.ControlRead, "none|eof|read_error|handshake_rejected|frame_rejected") && member(v.Cleanup, "not_started|pending|verified|uncertain") && (v.ExitCode == nil || v.Exited && *v.ExitCode >= -1 && *v.ExitCode <= 4294967295)
}

func (r *mainWorkerDiagnostic) json() json.RawMessage {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.value
	trace := r.trace.Snapshot()
	v.Complete, v.Cleanup = trace.Complete, trace.Cleanup.State
	if trace.Cause != nil {
		v.Reason, v.Stage = trace.Cause.Condition, string(trace.Cause.Location.Phase)
	}
	raw, _ := json.Marshal(v)
	return raw
}
func (r *mainWorkerDiagnostic) publish() {
	if r != nil && r.observe != nil {
		r.observe(r.json())
	}
}
func (r *mainWorkerDiagnostic) report(c mainWorkerCause) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := false
	if r.trace.Snapshot().Cause == nil {
		c.Schema = mainWorkerSchema
		r.value.mainWorkerCause = c
		r.value.Reported = true
		r.trace.Fail(d.Location{Operation: "main_transport_worker", Phase: d.Phase(c.Stage)}, c.Reason)
		changed = true
	}
	r.mu.Unlock()
	if changed {
		r.publish()
	}
}
func (r *mainWorkerDiagnostic) readEnded(kind string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := false
	if r.value.ControlRead == "none" {
		r.value.ControlRead = kind
		changed = true
	}
	if !r.value.Reported {
		r.value.Reason = "unknown"
		r.trace.Incomplete()
	}
	r.mu.Unlock()
	if changed {
		r.publish()
	}
}
func (r *mainWorkerDiagnostic) intentional() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.value.Intentional {
		r.mu.Unlock()
		return
	}
	r.value.Intentional = true
	if !r.value.Exited {
		r.trace.Cleanup("pending", "none")
	}
	r.mu.Unlock()
	r.publish()
}
func (r *mainWorkerDiagnostic) exited(code int, cleanup error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	exit := int64(code)
	r.value.Exited = true
	r.value.ExitCode = &exit
	r.value.Cleanup = "verified"
	if cleanup != nil {
		r.value.Cleanup = "uncertain"
	}
	r.trace.Cleanup(r.value.Cleanup, "none")
	if !r.value.Reported {
		r.value.Reason = "unknown"
		r.trace.Incomplete()
	}
	r.mu.Unlock()
	r.publish()
}

type mainBrowserSourceError struct {
	cause      error
	diagnostic func() json.RawMessage
}

func (*mainBrowserSourceError) Error() string                  { return "main browser source operation failed" }
func (e *mainBrowserSourceError) String() string               { return e.Error() }
func (e *mainBrowserSourceError) GoString() string             { return e.Error() }
func (e *mainBrowserSourceError) Unwrap() error                { return e.cause }
func (e *mainBrowserSourceError) WorkerDiagnosticJSON() []byte { return e.diagnostic() }

// MainBrowserWorkerFailureDiagnostic follows ordinary error wrapping while
// leaving retry and operational failure classification entirely unchanged.
func MainBrowserWorkerFailureDiagnostic(err error) json.RawMessage {
	var carrier interface{ WorkerDiagnosticJSON() []byte }
	if !errors.As(err, &carrier) {
		return nil
	}
	raw := carrier.WorkerDiagnosticJSON()
	if !ValidateMainBrowserWorkerDiagnostic(raw) {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
