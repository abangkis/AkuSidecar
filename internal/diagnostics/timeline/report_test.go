package timeline

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestDecodeReportStrictDocument(t *testing.T) {
	raw, err := json.Marshal(testReport())
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeReport(raw)
	if err != nil || !reflect.DeepEqual(got, testReport()) {
		t.Fatalf("valid report changed: %+v, %v", got, err)
	}
	for _, test := range []struct {
		name string
		from string
		to   string
	}{
		{"unknown root field", `"version":1`, `"version":1,"content":"private"`},
		{"unknown support field", `"longTask":false`, `"longTask":false,"url":"private"`},
		{"unknown frame field", `"count":0`, `"count":0,"private":1`},
		{"unknown stage field", `"name":"scroll_frame"`, `"name":"scroll_frame","text":"private"`},
		{"unknown event field", `"kind":"stage"`, `"kind":"stage","scriptAttribution":"private"`},
		{"missing root field", `"version":1,`, ``},
		{"null root field", `"version":1`, `"version":null`},
		{"null support", `"support":{"longAnimationFrame":false,"longTask":false,"layoutShift":false}`, `"support":null`},
		{"missing support field", `"longTask":false,`, ``},
		{"null frame count", `"count":0`, `"count":null`},
		{"null stage count", `"count":1`, `"count":null`},
		{"missing stage field", `"name":"scroll_frame",`, ``},
		{"null event kind", `"kind":"stage"`, `"kind":null`},
		{"missing event time", `"atMs":12,`, ``},
		{"wrong stage shape", `"stages":[`, `"stages":[null,`},
		{"wrong event shape", `"events":[`, `"events":[null,`},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := bytes.Replace(raw, []byte(test.from), []byte(test.to), 1)
			if bytes.Equal(raw, invalid) {
				t.Fatalf("fixture did not match %q", test.from)
			}
			if _, err := DecodeReport(invalid); err == nil {
				t.Fatal("invalid report was accepted")
			}
		})
	}
	for _, invalid := range [][]byte{nil, []byte(`null`), []byte(`[]`), append(append([]byte(nil), raw...), []byte(` {}`)...), append(append([]byte(nil), raw...), 'x')} {
		if _, err := DecodeReport(invalid); err == nil {
			t.Fatalf("invalid complete document accepted: %s", invalid)
		}
	}
}

func TestRequiredFieldsRejectMissingAndNull(t *testing.T) {
	marshal := func(value any) []byte {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for _, path := range []string{"root", "support", "frameGaps", "stage", "event"} {
		for _, null := range []bool{false, true} {
			var original map[string]any
			if err := json.Unmarshal(marshal(testReport()), &original); err != nil {
				t.Fatal(err)
			}
			selectObject := func(root map[string]any) map[string]any {
				switch path {
				case "support", "frameGaps":
					return root[path].(map[string]any)
				case "stage":
					return root["stages"].([]any)[0].(map[string]any)
				case "event":
					return root["events"].([]any)[0].(map[string]any)
				default:
					return root
				}
			}
			for field := range selectObject(original) {
				if path == "event" && field != "kind" && field != "atMs" {
					continue
				}
				var root map[string]any
				_ = json.Unmarshal(marshal(testReport()), &root)
				object := selectObject(root)
				if null {
					object[field] = nil
				} else {
					delete(object, field)
				}
				if _, err := DecodeReport(marshal(root)); err == nil {
					t.Errorf("accepted %s.%s with null=%v", path, field, null)
				}
			}
		}
	}
}

func TestReportBoundsAndAllowlists(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Report)
	}{
		{"version", func(r *Report) { r.Version = 2 }},
		{"reason", func(r *Report) { r.Reason = "other" }},
		{"duration", func(r *Report) { r.DurationMs = maxTimeMs + 1 }},
		{"negative", func(r *Report) { r.DurationMs = -1 }},
		{"NaN", func(r *Report) { r.DurationMs = math.NaN() }},
		{"infinity", func(r *Report) { r.DurationMs = math.Inf(1) }},
		{"frame count", func(r *Report) { r.FrameGaps.Count = maxCount + 1 }},
		{"frame over32", func(r *Report) { r.FrameGaps.Over32Ms = -1 }},
		{"frame max", func(r *Report) { r.FrameGaps.MaxMs = maxTimeMs + 1 }},
		{"frame p50", func(r *Report) { r.FrameGaps.P50Ms = -1 }},
		{"frame p95", func(r *Report) { r.FrameGaps.P95Ms = maxTimeMs + 1 }},
		{"scroll count", func(r *Report) { r.ScrollEvents = maxCount + 1 }},
		{"dropped events", func(r *Report) { r.DroppedEvents = -1 }},
		{"dropped frames", func(r *Report) { r.DroppedFrameSamples = maxCount + 1 }},
		{"stage limit", func(r *Report) { r.Stages = make([]Stage, 9) }},
		{"event limit", func(r *Report) { r.Events = make([]Event, 501) }},
		{"stage name", func(r *Report) { r.Stages[0].Name = "private" }},
		{"stage count", func(r *Report) { r.Stages[0].Count = -1 }},
		{"stage total", func(r *Report) { r.Stages[0].TotalMs = maxTotalMs + 1 }},
		{"stage max", func(r *Report) { r.Stages[0].MaxMs = maxTimeMs + 1 }},
		{"event kind", func(r *Report) { r.Events[0].Kind = "private" }},
		{"event name", func(r *Report) { *r.Events[0].Name = "private" }},
		{"event time", func(r *Report) { r.Events[0].AtMs = maxTimeMs + 1 }},
		{"event duration", func(r *Report) { *r.Events[0].DurationMs = maxTimeMs + 1 }},
		{"event value", func(r *Report) { *r.Events[0].Value = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := testReport()
			test.mutate(&report)
			if err := report.validate(); err == nil {
				t.Fatal("out-of-policy report accepted")
			}
		})
	}
	for _, reason := range []string{"timeout", "manual", "hidden"} {
		report := testReport()
		report.Reason = reason
		if err := report.validate(); err != nil {
			t.Errorf("reason %q rejected: %v", reason, err)
		}
	}
	for _, kind := range []string{"frame_gap", "loaf", "longtask", "layout_shift", "stage", "media_load"} {
		report := testReport()
		report.Events[0].Kind = kind
		if err := report.validate(); err != nil {
			t.Errorf("event kind %q rejected: %v", kind, err)
		}
	}
	for _, name := range []string{"scroll_frame", "content_context_scroll", "back_to_top", "side_pane", "timeline_render"} {
		report := testReport()
		report.Stages[0].Name = name
		if err := report.validate(); err != nil {
			t.Errorf("stage name %q rejected: %v", name, err)
		}
	}
}

func TestReportInclusiveLimitsAndOptionalNulls(t *testing.T) {
	report := testReport()
	report.DurationMs = maxTimeMs
	report.FrameGaps = FrameGaps{Count: maxCount, Over32Ms: maxCount, MaxMs: maxTimeMs, P50Ms: maxTimeMs, P95Ms: maxTimeMs}
	report.ScrollEvents, report.DroppedEvents, report.DroppedFrameSamples = maxCount, maxCount, maxCount
	stage := Stage{Name: "scroll_frame", Count: maxCount, TotalMs: maxTotalMs, MaxMs: maxTimeMs}
	report.Stages = make([]Stage, 8)
	for index := range report.Stages {
		report.Stages[index] = stage
	}
	report.Events = make([]Event, 500)
	for index := range report.Events {
		report.Events[index] = Event{Kind: "frame_gap", AtMs: maxTimeMs}
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReport(raw); err != nil {
		t.Fatalf("inclusive limits rejected: %v", err)
	}
	raw = bytes.Replace(raw, []byte(`"kind":"frame_gap"`), []byte(`"kind":"frame_gap","name":null,"durationMs":null,"blockingMs":null,"scriptMs":null,"forcedLayoutMs":null,"styleLayoutMs":null,"value":null,"hadRecentInput":null`), 1)
	if _, err := DecodeReport(raw); err != nil {
		t.Fatalf("optional null values rejected: %v", err)
	}
}
