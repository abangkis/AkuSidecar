package timeline

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func testReport() Report {
	name, duration, input := "scroll_frame", 12.0, false
	return Report{
		Version: 1, DurationMs: 1500, Reason: "timeout",
		Stages: []Stage{{Name: "scroll_frame", Count: 1, TotalMs: 12, MaxMs: 12}},
		Events: []Event{{Kind: "stage", AtMs: 12, Name: &name, DurationMs: &duration,
			BlockingMs: &duration, ScriptMs: &duration, ForcedLayoutMs: &duration,
			StyleLayoutMs: &duration, Value: &duration, HadRecentInput: &input}},
	}
}

func mutate(report *Report) {
	report.Stages[0].Name = "mutated"
	report.Events[0].Kind = "mutated"
	*report.Events[0].Name = "mutated"
	for _, pointer := range []*float64{report.Events[0].DurationMs, report.Events[0].BlockingMs,
		report.Events[0].ScriptMs, report.Events[0].ForcedLayoutMs, report.Events[0].StyleLayoutMs, report.Events[0].Value} {
		*pointer = 99
	}
	*report.Events[0].HadRecentInput = true
}

func TestStoreCloneBoundariesAndTimestamp(t *testing.T) {
	var store Store
	if store.Latest() != nil {
		t.Fatal("empty store must return nil")
	}
	input, expected := testReport(), testReport()
	store.Save(input)
	mutate(&input)
	got := store.Latest()
	if !reflect.DeepEqual(got.Report, expected) {
		t.Fatalf("incoming mutation escaped: %+v", got.Report)
	}
	if timestamp, err := time.Parse(time.RFC3339Nano, got.RecordedAt); err != nil || timestamp.Location() != time.UTC || !strings.HasSuffix(got.RecordedAt, "Z") {
		t.Fatalf("expected UTC timestamp, got %q, %v", got.RecordedAt, err)
	}
	mutate(&got.Report)
	if !reflect.DeepEqual(store.Latest().Report, expected) {
		t.Fatal("outgoing mutation escaped")
	}
	store.Save(Report{Version: 1})
	raw, err := json.Marshal(store.Latest().Report)
	if err != nil || !strings.Contains(string(raw), `"stages":[]`) || !strings.Contains(string(raw), `"events":[]`) {
		t.Fatalf("snapshot must retain empty-array normalization: %s, %v", raw, err)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	var store Store
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				input := testReport()
				store.Save(input)
				mutate(&input)
				got := store.Latest()
				if !reflect.DeepEqual(got.Report, testReport()) {
					t.Error("concurrent write/read lost isolation")
				}
				mutate(&got.Report)
			}
		}()
	}
	workers.Wait()
}
