package nativetrace

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestManagerBoundsAndReportsRecordLimit(t *testing.T) {
	var manager Manager
	got := make(chan Sample, MaxRecords+1)
	manager.start("run", 12, func(s Sample) { got <- s }, func(ctx context.Context, pid uint32, ready chan struct{}, emit func(Sample)) {
		if pid != 12 {
			t.Errorf("pid=%d", pid)
		}
		close(ready)
		for i := 0; i < MaxRecords+100; i++ {
			emit(Sample{Trigger: "state_change"})
		}
		<-ctx.Done()
		emit(Sample{Trigger: "trace_end"})
	})
	for i := 0; i < MaxRecords; i++ {
		select {
		case s := <-got:
			if i == MaxRecords-1 && (s.Trigger != "trace_end" || s.Status != "record_limit_reached") {
				t.Fatalf("terminal=%+v", s)
			}
		case <-time.After(time.Second):
			t.Fatal("bounded trace did not terminate")
		}
	}
	select {
	case s := <-got:
		t.Fatalf("extra sample: %+v", s)
	default:
	}
}

func TestOldReleaseDoesNotStopNewRun(t *testing.T) {
	var manager Manager
	done := make(chan struct{})
	manager.start("new", 0, func(Sample) {}, func(ctx context.Context, _ uint32, ready chan struct{}, _ func(Sample)) {
		close(ready)
		<-ctx.Done()
		close(done)
	})
	manager.StopRun("old")
	select {
	case <-done:
		t.Fatal("old release cancelled new trace")
	default:
	}
	manager.StopRun("new")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("release failed to stop matching trace")
	}
}

func TestStartSupersedesAndStopCancels(t *testing.T) {
	var manager Manager
	started := make(chan string, 2)
	ended := make(chan string, 2)
	observer := func(name string) func(context.Context, uint32, chan struct{}, func(Sample)) {
		return func(ctx context.Context, _ uint32, ready chan struct{}, _ func(Sample)) {
			started <- name
			close(ready)
			<-ctx.Done()
			ended <- name
		}
	}
	manager.start("a", 0, func(Sample) {}, observer("a"))
	manager.start("a", 0, func(Sample) {}, observer("duplicate"))
	manager.start("b", 0, func(Sample) {}, observer("b"))
	manager.Stop()
	for i := 0; i < 2; i++ {
		select {
		case <-ended:
		case <-time.After(time.Second):
			t.Fatal("observer leaked")
		}
	}
	if len(started) != 2 {
		t.Fatalf("started=%d", len(started))
	}
}

func TestTelemetrySchemaIsAnExplicitPrivacyAllowlist(t *testing.T) {
	want := []string{"at", "trigger", "status", "foregroundHwnd", "previousForegroundHwnd,omitempty", "keyboardFocusHwnd", "keyboardFocusAvailable", "eventHwnd,omitempty", "eventTick,omitempty", "eventPid,omitempty", "eventProcessClass,omitempty", "eventRootHwnd,omitempty", "zOrderAvailable", "windows", "windowsTruncated", "classification"}
	typ := reflect.TypeOf(Sample{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Tag.Get("json"))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("review privacy schema change: %v", got)
	}
	wantWindow := []string{"hwnd", "rootHwnd", "pid", "processClass", "visible", "minimized", "topmost", "topmostAvailable", "zOrder", "aboveForeground", "overlapsForeground", "overlapAvailable"}
	windowType := reflect.TypeOf(Window{})
	var gotWindow []string
	for i := 0; i < windowType.NumField(); i++ {
		gotWindow = append(gotWindow, windowType.Field(i).Tag.Get("json"))
	}
	if !reflect.DeepEqual(gotWindow, wantWindow) {
		t.Fatalf("review window privacy schema change: %v", gotWindow)
	}
	s := Sample{Windows: make([]Window, 16)}
	for i := range s.Windows {
		s.Windows[i] = Window{HWND: "0xffffffffffffffff", Root: "0xffffffffffffffff", PID: 4294967295, Class: "akubrowser_root", Z: 511}
	}
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > 7000 {
		t.Fatalf("native detail exceeds safe store envelope: %d %v", len(raw), err)
	}
}
