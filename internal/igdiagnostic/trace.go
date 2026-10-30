// Package igdiagnostic owns the closed IG diagnostic boundary, not execution.
package igdiagnostic

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/diagnostictrace"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
	"io"
	"regexp"
	"sync"
	"time"
	"unicode/utf8"
)

//go:embed catalog.json
var catalogJSON []byte
var catalog = func() map[string][]string {
	var v map[string][]string
	if json.Unmarshal(catalogJSON, &v) != nil {
		panic("invalid IG catalog")
	}
	return v
}()

func Allowed(kind, value string) bool {
	for _, v := range catalog[kind] {
		if v == value {
			return true
		}
	}
	return false
}

const Schema = "aku.ig-trace.v1"

type Cause struct {
	Stage     string `json:"stage"`
	Condition string `json:"condition"`
}
type Settlement struct {
	State     string `json:"state"`
	Condition string `json:"condition"`
}
type Wire struct {
	BudgetMS    *int64     `json:"budgetMS"`
	RemainingMS *int64     `json:"remainingMS"`
	Schema      string     `json:"schema"`
	Stage       string     `json:"stage"`
	Completed   string     `json:"completed"`
	Disposition string     `json:"disposition"`
	Sequence    uint32     `json:"sequence"`
	Cause       *Cause     `json:"cause"`
	Cleanup     Settlement `json:"cleanup"`
	Complete    bool       `json:"complete"`
	ElapsedMS   *int64     `json:"elapsedMS"`
}
type Envelope struct {
	Wire
	NodeElapsedMS *int64            `json:"nodeElapsedMS"`
	NodeSequence  uint32            `json:"nodeSequence"`
	Attempt       string            `json:"attempt"`
	Operation     string            `json:"operation"`
	Revision      string            `json:"revision"`
	Delivery      Settlement        `json:"delivery"`
	HTTPDelivery  Settlement        `json:"httpDelivery"`
	Evidence      map[string]string `json:"evidence"`
}
type Attempt struct {
	httpDelivery              Settlement
	nodeBudget, nodeRemaining *int64
	recorder                  *diagnostictrace.Recorder
	id, operation             string
	start                     time.Time
	mu                        sync.Mutex
	nodeSequence              uint32
	nodeElapsed               *int64
	evidence                  map[string]string
}

var idPattern = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

func NewAttempt(id, operation string) *Attempt {
	if !idPattern.MatchString(id) || !Allowed("operations", operation) {
		return nil
	}
	e := map[string]string{}
	for _, k := range catalog["checks"] {
		e[k] = "not_observed"
	}
	a := &Attempt{recorder: diagnostictrace.New(d.Operation(operation)), id: id, operation: operation, start: time.Now(), evidence: e, httpDelivery: Settlement{"pending", "none"}}
	a.Note("none")
	return a
}
func Ensure(ctx context.Context, operation string) (context.Context, *Attempt) {
	if ctx == nil {
		ctx = context.Background()
	}
	if a := FromContext(ctx); a != nil {
		if a.operation != operation {
			return ctx, nil
		}
		return ctx, a
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return ctx, nil
	}
	a := NewAttempt(hex.EncodeToString(id[:]), operation)
	if a == nil {
		return ctx, nil
	}
	return WithAttempt(ctx, a), a
}
func (a *Attempt) ID() string            { return a.id }
func MarkIncomplete(ctx context.Context) { Incomplete(ctx) }
func FailActive(ctx context.Context, condition string) {
	if a := FromContext(ctx); a != nil {
		a.Fail(string(a.recorder.Snapshot().Location.Phase), condition)
	}
}

type contextKey struct{}

func WithAttempt(ctx context.Context, a *Attempt) context.Context {
	return context.WithValue(ctx, contextKey{}, a)
}
func FromContext(ctx context.Context) *Attempt {
	if ctx == nil {
		return nil
	}
	a, _ := ctx.Value(contextKey{}).(*Attempt)
	return a
}
func Note(ctx context.Context, stage string) {
	if a := FromContext(ctx); a != nil {
		a.Note(stage)
	}
}
func (a *Attempt) Note(stage string) {
	if !Allowed("stages", stage) {
		a.recorder.Incomplete()
		return
	}
	a.recorder.Advance(d.Phase(stage), d.Observed[time.Duration]{Value: time.Since(a.start), Known: true})
}
func Fail(ctx context.Context, stage, condition string) {
	if a := FromContext(ctx); a != nil {
		a.Fail(stage, condition)
	}
}
func (a *Attempt) Fail(stage, condition string) {
	if !Allowed("stages", stage) || !Allowed("conditions", condition) || condition == "none" {
		a.recorder.Incomplete()
		return
	}
	a.recorder.Fail(d.Location{Operation: d.Operation(a.operation), Phase: d.Phase(stage)}, condition)
}
func SetCleanup(ctx context.Context, state, condition string) {
	if a := FromContext(ctx); a != nil {
		a.SetCleanup(state, condition)
	}
}
func (a *Attempt) SetCleanup(state, condition string) {
	if Allowed("cleanup", state) && Allowed("conditions", condition) {
		a.recorder.Cleanup(state, condition)
	} else {
		a.recorder.Incomplete()
	}
}
func SetDelivery(ctx context.Context, state, condition string) {
	if a := FromContext(ctx); a != nil {
		if Allowed("delivery", state) && Allowed("conditions", condition) {
			a.recorder.Delivery(state, condition)
		} else {
			a.recorder.Incomplete()
		}
	}
}
func SetHTTPDelivery(ctx context.Context, state, condition string) {
	if a := FromContext(ctx); a != nil {
		if !Allowed("delivery", state) || !Allowed("conditions", condition) {
			a.recorder.Incomplete()
			return
		}
		a.mu.Lock()
		a.httpDelivery = Settlement{state, condition}
		a.mu.Unlock()
	}
}
func SetDisposition(ctx context.Context, state string) {
	if a := FromContext(ctx); a != nil {
		if Allowed("dispositions", state) {
			a.recorder.Disposition(state)
		} else {
			a.recorder.Incomplete()
		}
	}
}
func Incomplete(ctx context.Context) {
	if a := FromContext(ctx); a != nil {
		a.recorder.Incomplete()
	}
}
func Projection(ctx context.Context) json.RawMessage {
	if a := FromContext(ctx); a != nil {
		return a.SnapshotJSON()
	}
	return nil
}
func (a *Attempt) SnapshotJSON() json.RawMessage {
	s := a.recorder.Snapshot()
	min, max := time.Duration(0), time.Minute
	policy, _ := d.NewDurationPolicy(&min, &max, d.Ceiling)
	ms := d.Milliseconds(s.Elapsed, policy)
	lo, hi := int64(0), int64(10000)
	cp, _ := d.NewCountPolicy(&lo, &hi)
	count := d.BoundedCount(d.Observed[int64]{Value: int64(s.Sequence), Known: true}, cp)
	w := Wire{Schema: Schema, Stage: string(s.Location.Phase), Completed: string(s.Completed), Disposition: s.Disposition, Sequence: uint32(count.Value), Cleanup: Settlement{s.Cleanup.State, s.Cleanup.Condition}, Complete: s.Complete && count.Known}
	if ms.Known {
		w.ElapsedMS = &ms.Value
	}
	if s.Cause != nil {
		w.Cause = &Cause{string(s.Cause.Location.Phase), s.Cause.Condition}
	}
	a.mu.Lock()
	e := map[string]string{}
	for k, v := range a.evidence {
		e[k] = v
	}
	nodeMS, nodeSeq := a.nodeElapsed, a.nodeSequence
	httpDelivery := a.httpDelivery
	w.BudgetMS, w.RemainingMS = a.nodeBudget, a.nodeRemaining
	a.mu.Unlock()
	raw, err := json.Marshal(Envelope{HTTPDelivery: httpDelivery, NodeElapsedMS: nodeMS, NodeSequence: nodeSeq, Wire: w, Attempt: a.id, Operation: a.operation, Revision: "ig1", Delivery: Settlement{s.Delivery.State, s.Delivery.Condition}, Evidence: e})
	if err != nil || len(raw) > 2048 {
		return nil
	}
	return raw
}

// Decode checks duplicate keys recursively before typed decoding. No raw data is retained.
func Decode(raw []byte, limit int, out any) bool {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !unique(d, 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out) == nil
}
func unique(d *json.Decoder, depth int) bool {
	if depth > 8 {
		return false
	}
	t, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			s, ok := k.(string)
			if e != nil || !ok || seen[s] {
				return false
			}
			seen[s] = true
			if !unique(d, depth+1) {
				return false
			}
		}
		t, err = d.Token()
		return err == nil && t == json.Delim('}')
	case '[':
		for d.More() {
			if !unique(d, depth+1) {
				return false
			}
		}
		t, err = d.Token()
		return err == nil && t == json.Delim(']')
	}
	return false
}
func validWire(w Wire) bool {
	return validBudget(w) && w.Schema == Schema && Allowed("stages", w.Stage) && Allowed("stages", w.Completed) && Allowed("dispositions", w.Disposition) && w.Sequence > 0 && w.Sequence <= 10000 && Allowed("cleanup", w.Cleanup.State) && Allowed("conditions", w.Cleanup.Condition) && (w.Cleanup.State == "uncertain" || w.Cleanup.Condition == "none") && (w.Cause == nil || Allowed("stages", w.Cause.Stage) && Allowed("conditions", w.Cause.Condition) && w.Cause.Condition != "none") && (w.ElapsedMS == nil || *w.ElapsedMS >= 0 && *w.ElapsedMS <= 60000)
}
func (a *Attempt) Observe(raw []byte) bool {
	var w Wire
	if !exactKeys(raw, 11) || !Decode(raw, 512, &w) || !validWire(w) {
		a.recorder.Incomplete()
		return false
	}
	a.mu.Lock()
	if w.Sequence <= a.nodeSequence {
		a.mu.Unlock()
		return false
	}
	a.nodeSequence = w.Sequence
	a.nodeBudget, a.nodeRemaining = w.BudgetMS, w.RemainingMS
	if w.ElapsedMS != nil {
		v := *w.ElapsedMS
		a.nodeElapsed = &v
	}
	a.mu.Unlock()
	a.Note(w.Stage)
	a.recorder.Completed(d.Phase(w.Completed))
	a.recorder.Disposition(w.Disposition)
	a.SetCleanup(w.Cleanup.State, w.Cleanup.Condition)
	if w.Cause != nil {
		a.Fail(w.Cause.Stage, w.Cause.Condition)
	}
	if !w.Complete {
		a.recorder.Incomplete()
	}
	return true
}
func exactKeys(raw []byte, n int) bool {
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil || len(v) != n {
		return false
	}
	keys := []string{"schema", "stage", "completed", "disposition", "sequence", "cause", "cleanup", "complete", "elapsedMS", "budgetMS", "remainingMS"}
	if n == 19 {
		keys = append(keys, "attempt", "operation", "revision", "delivery", "evidence", "nodeElapsedMS", "nodeSequence", "httpDelivery")
	}
	for _, k := range keys {
		if _, ok := v[k]; !ok {
			return false
		}
	}
	if string(v["complete"]) != "true" && string(v["complete"]) != "false" {
		return false
	}
	if !pairKeys(v["cleanup"], "state", "condition") {
		return false
	}
	if string(v["cause"]) != "null" && !pairKeys(v["cause"], "stage", "condition") {
		return false
	}
	if n == 19 && (!pairKeys(v["delivery"], "state", "condition") || !pairKeys(v["httpDelivery"], "state", "condition") || string(v["nodeSequence"]) == "null") {
		return false
	}
	return true
}
func pairKeys(raw []byte, a, b string) bool {
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil || len(v) != 2 {
		return false
	}
	_, x := v[a]
	_, y := v[b]
	return x && y
}
func ValidateJSON(raw []byte) (json.RawMessage, bool) {
	var e Envelope
	if !exactKeys(raw, 19) || !Decode(raw, 2048, &e) || !validWire(e.Wire) || !idPattern.MatchString(e.Attempt) || !Allowed("operations", e.Operation) || e.NodeSequence > 10000 || e.NodeElapsedMS != nil && (*e.NodeElapsedMS < 0 || *e.NodeElapsedMS > 60000) || e.Revision != "ig1" || !Allowed("delivery", e.Delivery.State) || !Allowed("conditions", e.Delivery.Condition) || !Allowed("delivery", e.HTTPDelivery.State) || !Allowed("conditions", e.HTTPDelivery.Condition) || len(e.Evidence) != len(catalog["checks"]) {
		return nil, false
	}
	for _, k := range catalog["checks"] {
		if !Allowed("evidence", e.Evidence[k]) {
			return nil, false
		}
	}
	b, err := json.Marshal(e)
	return b, err == nil && len(b) <= 2048
}
func (a *Attempt) Evidence(key, state string) {
	if !Allowed("checks", key) || !Allowed("evidence", state) {
		a.recorder.Incomplete()
		return
	}
	a.mu.Lock()
	a.evidence[key] = state
	a.mu.Unlock()
}

type Error struct {
	cause      error
	diagnostic json.RawMessage
}

func (e *Error) Error() string          { return "Instagram diagnostic failed" }
func (e *Error) String() string         { return e.Error() }
func (e *Error) GoString() string       { return e.Error() }
func (e *Error) Unwrap() error          { return e.cause }
func (e *Error) DiagnosticJSON() []byte { return append([]byte(nil), e.diagnostic...) }
func Wrap(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	return &Error{cause: err, diagnostic: Projection(ctx)}
}

func validBudget(w Wire) bool {
	return w.BudgetMS == nil && w.RemainingMS == nil || w.BudgetMS != nil && w.RemainingMS != nil && *w.BudgetMS >= 1 && *w.BudgetMS <= 30000 && *w.RemainingMS >= 0 && *w.RemainingMS <= *w.BudgetMS
}

func (a *Attempt) Disposition() string { return a.recorder.Snapshot().Disposition }
