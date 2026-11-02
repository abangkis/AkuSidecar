package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const telemetryRotationPath = "/api/bridge/capture-surfaces/events"

type telemetryRotationFixture struct {
	s     *Server
	token string
	event domain.CaptureSurfaceEvent
}

func newTelemetryRotationFixture(t *testing.T) telemetryRotationFixture {
	t.Helper()
	s, token := splitTestServer(t)
	settings, err := s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	session, err := s.store.CreateUpdateSession(context.Background(), "Offline telemetry rotation", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	return telemetryRotationFixture{s, token, domain.CaptureSurfaceEvent{ID: "telemetry-rotation-event", SessionID: session.ID, RunID: run.ID, Source: run.Source, Event: "released"}}
}

func (f telemetryRotationFixture) request(t *testing.T, key, target string, events []domain.CaptureSurfaceEvent) *http.Request {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122"+target, strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", f.token)
	r.Header.Set("X-Aku-Capture-Instance", key)
	return r
}

func (f telemetryRotationFixture) count(t *testing.T) int {
	t.Helper()
	events, err := f.s.store.CaptureSurfaceEvents(context.Background(), f.event.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

type telemetryRotationGate struct {
	reached chan struct{}
	release chan struct{}
	entered sync.Once
	opened  sync.Once
}

func newTelemetryRotationGate() *telemetryRotationGate {
	return &telemetryRotationGate{reached: make(chan struct{}), release: make(chan struct{})}
}
func (g *telemetryRotationGate) wait() {
	g.entered.Do(func() { close(g.reached) })
	<-g.release
}
func (g *telemetryRotationGate) open() { g.opened.Do(func() { close(g.release) }) }

type telemetryRotationBody struct {
	io.ReadCloser
	gate    *telemetryRotationGate
	onClose bool
}

func (b telemetryRotationBody) Read(p []byte) (int, error) {
	if !b.onClose {
		b.gate.wait()
	}
	return b.ReadCloser.Read(p)
}
func (b telemetryRotationBody) Close() error {
	if b.onClose {
		b.gate.wait()
	}
	return b.ReadCloser.Close()
}

func awaitTelemetryRotation(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("telemetry fixture did not reach its synchronization point")
	}
}

func rotateWithBlockedTelemetry(t *testing.T, s *Server, unblock func()) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.RotateSplitCapture() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		unblock()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("rotation did not drain after unblocking HTTP")
		}
		t.Fatal("HTTP body/response pinned rotation")
	}
}

func TestTelemetryRotationRejectsAdmittedOldOwner(t *testing.T) {
	for _, phase := range []string{"reading_body", "decoded_body"} {
		for _, suffix := range []string{"", "/", "%2F"} {
			t.Run(phase+map[string]string{"": "/canonical", "/": "/slash", "%2F": "/encoded_slash"}[suffix], func(t *testing.T) {
				f := newTelemetryRotationFixture(t)
				old := f.s.splitCapture.key
				r := f.request(t, old, telemetryRotationPath+suffix, []domain.CaptureSurfaceEvent{f.event})
				gate := newTelemetryRotationGate()
				r.Body = telemetryRotationBody{r.Body, gate, phase == "decoded_body"}
				w, done := httptest.NewRecorder(), make(chan struct{})
				go func() { defer close(done); f.s.http.Handler.ServeHTTP(w, r) }()
				t.Cleanup(func() { gate.open(); awaitTelemetryRotation(t, done) })
				awaitTelemetryRotation(t, gate.reached) // Authentication already passed.
				rotateWithBlockedTelemetry(t, f.s, gate.open)
				gate.open()
				awaitTelemetryRotation(t, done)
				count := f.count(t)
				if w.Code != 409 || !strings.Contains(w.Body.String(), `"error":"capture_instance_mismatch"`) || count != 0 {
					t.Fatalf("retired request mutated telemetry: status=%d events=%d", w.Code, count)
				}
				fresh := f.request(t, f.s.splitCapture.key, telemetryRotationPath+suffix, []domain.CaptureSurfaceEvent{f.event})
				current := httptest.NewRecorder()
				f.s.http.Handler.ServeHTTP(current, fresh)
				if current.Code != 202 || f.count(t) != 1 {
					t.Fatal("new owner could not record telemetry", current.Code)
				}
			})
		}
	}
}

func TestTelemetryRotationSerializesBatchMutation(t *testing.T) {
	f := newTelemetryRotationFixture(t)
	second := f.event
	second.ID += "-second"
	r := f.request(t, f.s.splitCapture.key, telemetryRotationPath, []domain.CaptureSurfaceEvent{f.event, second})
	gate := newTelemetryRotationGate()
	r.Body = telemetryRotationBody{r.Body, gate, true}
	w, done := httptest.NewRecorder(), make(chan struct{})
	go func() { defer close(done); f.s.http.Handler.ServeHTTP(w, r) }()
	t.Cleanup(func() { gate.open(); awaitTelemetryRotation(t, done) })
	awaitTelemetryRotation(t, gate.reached)
	db, err := sql.Open("sqlite", f.s.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	releaseDB := func() { once.Do(func() { _, _ = db.Exec(`ROLLBACK`) }) }
	defer releaseDB()
	gate.open()
	// Keep the real SQLite writer pending, then observe the specified shared
	// mutex as a barrier before attempting the competing rotation.
	locked := false
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for !locked {
		if f.s.splitCapture.mu.TryLock() {
			f.s.splitCapture.mu.Unlock()
		} else {
			locked = true
			break
		}
		select {
		case <-done:
			t.Fatal("request finished before the pending writer was released")
		case <-deadline.C:
			t.Fatal("mutation did not hold the rotation mutex")
		case <-time.After(time.Millisecond):
		}
	}
	rotated := make(chan error, 1)
	go func() { rotated <- f.s.RotateSplitCapture() }()
	select {
	case err := <-rotated:
		t.Fatal("rotation completed while the old batch writer was pending", err)
	case <-time.After(25 * time.Millisecond):
	}
	releaseDB()
	awaitTelemetryRotation(t, done)
	select {
	case err := <-rotated:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("rotation did not resume after the batch")
	}
	if w.Code != 202 || f.count(t) != 2 {
		t.Fatal("pre-rotation batch lost events", w.Code)
	}
}

type telemetryRetiringProcess struct {
	*splitLeaseProcess
	gate *telemetryRotationGate
}

func (p *telemetryRetiringProcess) CloseForRetry(ctx context.Context) error {
	p.gate.entered.Do(func() { close(p.gate.reached) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.gate.release:
		return p.splitLeaseProcess.CloseForRetry(ctx)
	}
}

func TestTelemetryRotationAllowsCleanupWhileReplacing(t *testing.T) {
	f := newTelemetryRotationFixture(t)
	gate := newTelemetryRotationGate()
	p := &telemetryRetiringProcess{&splitLeaseProcess{done: make(chan error, 1)}, gate}
	m, err := captureruntime.New(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetSplitCaptureRuntime(m); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	replaced := make(chan struct{})
	var replacementError error
	go func() {
		defer close(replaced)
		replacementError = m.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
			if err := f.s.RotateSplitCapture(); err != nil {
				return nil, err
			}
			return &splitLeaseProcess{done: make(chan error, 1)}, nil
		})
	}()
	t.Cleanup(func() { gate.open(); cancel(); awaitTelemetryRotation(t, replaced); m.Terminate() })
	awaitTelemetryRotation(t, gate.reached)
	if snapshot := m.Snapshot(); snapshot.State != captureruntime.Replacing || snapshot.ActiveLeases != 0 {
		t.Fatal("fixture is not draining its old owner", snapshot)
	}
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, f.request(t, f.s.splitCapture.key, telemetryRotationPath, []domain.CaptureSurfaceEvent{f.event}))
	if w.Code != 202 || f.count(t) != 1 || m.Snapshot().ActiveLeases != 0 {
		t.Fatal("cleanup was rejected or acquired a replacement-blocking lease", w.Code)
	}
	gate.open()
	awaitTelemetryRotation(t, replaced)
	if replacementError != nil {
		t.Fatal(replacementError)
	}
}

type telemetryRotationWriter struct {
	*httptest.ResponseRecorder
	gate *telemetryRotationGate
}

func (w telemetryRotationWriter) Write(p []byte) (int, error) {
	w.gate.wait()
	return w.ResponseRecorder.Write(p)
}

func TestTelemetryRotationDoesNotWaitForResponse(t *testing.T) {
	f := newTelemetryRotationFixture(t)
	gate := newTelemetryRotationGate()
	w := telemetryRotationWriter{httptest.NewRecorder(), gate}
	r := f.request(t, f.s.splitCapture.key, telemetryRotationPath, []domain.CaptureSurfaceEvent{f.event})
	done := make(chan struct{})
	go func() { defer close(done); f.s.http.Handler.ServeHTTP(w, r) }()
	t.Cleanup(func() { gate.open(); awaitTelemetryRotation(t, done) })
	awaitTelemetryRotation(t, gate.reached)
	rotateWithBlockedTelemetry(t, f.s, gate.open)
	gate.open()
	awaitTelemetryRotation(t, done)
	if w.Code != 202 || f.count(t) != 1 {
		t.Fatal("response after rotation lost the already-recorded event", w.Code)
	}
}

func TestTelemetryRotationStopHonorsDeadlineWithPendingWriter(t *testing.T) {
	f := newTelemetryRotationFixture(t)
	gate := newTelemetryRotationGate()
	handler := f.s.http.Handler
	fixture := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = telemetryRotationBody{r.Body, gate, true}
		handler.ServeHTTP(w, r)
	}))
	fixture.Start()
	f.s.http, f.s.listener = fixture.Config, fixture.Listener
	t.Cleanup(fixture.Close)
	r := f.request(t, f.s.splitCapture.key, telemetryRotationPath, []domain.CaptureSurfaceEvent{f.event})
	r.URL.Host, r.Host, r.RequestURI = fixture.Listener.Addr().String(), fixture.Listener.Addr().String(), ""
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := fixture.Client().Do(r)
		if err == nil {
			response.Body.Close()
		}
	}()
	t.Cleanup(func() { gate.open(); awaitTelemetryRotation(t, requestDone) })
	awaitTelemetryRotation(t, gate.reached)
	db, err := sql.Open("sqlite", f.s.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	releaseDB := func() { once.Do(func() { _, _ = db.Exec(`ROLLBACK`) }) }
	defer releaseDB()
	gate.open()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		if !f.s.splitCapture.mu.TryLock() {
			break
		}
		f.s.splitCapture.mu.Unlock()
		select {
		case <-deadline.C:
			t.Fatal("fixture did not enter the pending mutation")
		case <-time.After(time.Millisecond):
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stopped := make(chan struct{})
	var stopError error
	go func() { defer close(stopped); stopError = f.s.Stop(ctx) }()
	select {
	case <-stopped:
	case <-time.After(750 * time.Millisecond):
		t.Error("HTTP shutdown exceeded its deadline while waiting for telemetry")
	}
	releaseDB()
	awaitTelemetryRotation(t, stopped)
	awaitTelemetryRotation(t, requestDone)
	awaitTelemetryRotation(t, f.s.splitCapture.done)
	if stopError != nil {
		t.Fatal(stopError)
	}
	if f.count(t) != 0 {
		t.Fatal("shutdown did not cancel the pending telemetry write")
	}
}

func TestTelemetryRotationPreservesBatchErrorsAndNonSplit(t *testing.T) {
	for _, split := range []bool{true, false} {
		for _, scenario := range []string{"valid", "duplicate", "empty", "oversize", "native_trace_first", "native_trace_later", "store_error_later"} {
			t.Run(map[bool]string{true: "split/", false: "non_split/"}[split]+scenario, func(t *testing.T) {
				f := newTelemetryRotationFixture(t)
				key := f.s.splitCapture.key
				if !split {
					f.s.splitCapture = nil
					key = ""
				}
				events, status, count := []domain.CaptureSurfaceEvent{f.event}, 202, 1
				switch scenario {
				case "duplicate":
					events = append(events, f.event)
				case "empty":
					events, status, count = nil, 400, 0
				case "oversize":
					events, status, count = make([]domain.CaptureSurfaceEvent, 33), 400, 0
				case "native_trace_first", "native_trace_later", "store_error_later":
					bad := f.event
					bad.ID += "-bad"
					bad.Event = "native_trace"
					status = 400
					if scenario == "native_trace_first" {
						events, count = []domain.CaptureSurfaceEvent{bad, f.event}, 0
					} else {
						if scenario == "store_error_later" {
							bad.Event, bad.ID = "created", ""
						}
						events = append(events, bad)
					}
				}
				w := httptest.NewRecorder()
				f.s.http.Handler.ServeHTTP(w, f.request(t, key, telemetryRotationPath, events))
				if w.Code != status || f.count(t) != count {
					t.Fatalf("batch contract changed: status=%d want=%d events=%d want=%d", w.Code, status, f.count(t), count)
				}
				if status == 400 {
					bridgeCommandsError(t, w, 400, "invalid_request", "")
				}
			})
		}
	}
}
