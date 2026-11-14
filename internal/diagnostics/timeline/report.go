// Package timeline owns the bounded UI performance report schema and latest snapshot.
package timeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
)

const (
	maxCount   = 10_000_000
	maxTimeMs  = 60_000
	maxTotalMs = 1_000_000_000
)

type Report struct {
	Version             int       `json:"version"`
	DurationMs          float64   `json:"durationMs"`
	Reason              string    `json:"reason"`
	Support             Support   `json:"support"`
	FrameGaps           FrameGaps `json:"frameGaps"`
	ScrollEvents        int       `json:"scrollEvents"`
	DroppedEvents       int       `json:"droppedEvents"`
	DroppedFrameSamples int       `json:"droppedFrameSamples"`
	Stages              []Stage   `json:"stages"`
	Events              []Event   `json:"events"`
}

type Support struct {
	LongAnimationFrame bool `json:"longAnimationFrame"`
	LongTask           bool `json:"longTask"`
	LayoutShift        bool `json:"layoutShift"`
}

type FrameGaps struct {
	Count    int     `json:"count"`
	MaxMs    float64 `json:"maxMs"`
	P50Ms    float64 `json:"p50Ms"`
	P95Ms    float64 `json:"p95Ms"`
	Over32Ms int     `json:"over32Ms"`
}

type Stage struct {
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	TotalMs float64 `json:"totalMs"`
	MaxMs   float64 `json:"maxMs"`
}

type Event struct {
	Kind           string   `json:"kind"`
	Name           *string  `json:"name,omitempty"`
	AtMs           float64  `json:"atMs"`
	DurationMs     *float64 `json:"durationMs,omitempty"`
	BlockingMs     *float64 `json:"blockingMs,omitempty"`
	ScriptMs       *float64 `json:"scriptMs,omitempty"`
	ForcedLayoutMs *float64 `json:"forcedLayoutMs,omitempty"`
	StyleLayoutMs  *float64 `json:"styleLayoutMs,omitempty"`
	Value          *float64 `json:"value,omitempty"`
	HadRecentInput *bool    `json:"hadRecentInput,omitempty"`
}

type Snapshot struct {
	RecordedAt string `json:"recordedAt"`
	Report     Report `json:"report"`
}

// DecodeReport decodes the complete privacy-bounded report document.
// Required values must be present and non-null, and unknown fields are rejected.
func DecodeReport(raw []byte) (Report, error) {
	var report Report
	if err := validateUIPerformanceJSONShape(raw); err != nil {
		return report, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	if err := report.validate(); err != nil {
		return report, err
	}
	return report, nil
}

func validateUIPerformanceJSONShape(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}

	root, err := uiPerformanceJSONObject(document)
	if err != nil {
		return err
	}
	if err := requireUIPerformanceFields(root, "version", "durationMs", "reason", "support", "frameGaps", "scrollEvents", "droppedEvents", "droppedFrameSamples", "stages", "events"); err != nil {
		return err
	}

	support, err := uiPerformanceJSONObject(root["support"])
	if err != nil {
		return err
	}
	if err := requireUIPerformanceFields(support, "longAnimationFrame", "longTask", "layoutShift"); err != nil {
		return err
	}

	frameGaps, err := uiPerformanceJSONObject(root["frameGaps"])
	if err != nil {
		return err
	}
	if err := requireUIPerformanceFields(frameGaps, "count", "maxMs", "p50Ms", "p95Ms", "over32Ms"); err != nil {
		return err
	}

	if err := validateUIPerformanceArray(root["stages"], []string{"name", "count", "totalMs", "maxMs"}); err != nil {
		return err
	}
	if err := validateUIPerformanceArray(root["events"], []string{"kind", "atMs"}); err != nil {
		return err
	}
	return nil
}

func uiPerformanceJSONObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("expected JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil || fields == nil {
		return nil, errors.New("expected JSON object")
	}
	return fields, nil
}

func requireUIPerformanceFields(fields map[string]json.RawMessage, required ...string) error {
	for _, name := range required {
		raw, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("missing required field")
		}
	}
	return nil
}

func validateUIPerformanceArray(raw json.RawMessage, requiredFields []string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return errors.New("expected JSON array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(trimmed, &entries); err != nil {
		return err
	}
	for _, entry := range entries {
		fields, err := uiPerformanceJSONObject(entry)
		if err != nil {
			return err
		}
		if err := requireUIPerformanceFields(fields, requiredFields...); err != nil {
			return err
		}
	}
	return nil
}

func (r Report) validate() error {
	if r.Version != 1 || !validUIPerformanceNumber(r.DurationMs) || r.DurationMs > maxTimeMs {
		return errors.New("invalid report version or duration")
	}
	switch r.Reason {
	case "timeout", "manual", "hidden":
	default:
		return errors.New("invalid report reason")
	}
	if !validUIPerformanceCount(r.FrameGaps.Count) || !validUIPerformanceCount(r.FrameGaps.Over32Ms) ||
		!validUIPerformanceNumber(r.FrameGaps.MaxMs) || r.FrameGaps.MaxMs > maxTimeMs ||
		!validUIPerformanceNumber(r.FrameGaps.P50Ms) || r.FrameGaps.P50Ms > maxTimeMs ||
		!validUIPerformanceNumber(r.FrameGaps.P95Ms) || r.FrameGaps.P95Ms > maxTimeMs {
		return errors.New("invalid frame gap metrics")
	}
	if !validUIPerformanceCount(r.ScrollEvents) || !validUIPerformanceCount(r.DroppedEvents) || !validUIPerformanceCount(r.DroppedFrameSamples) {
		return errors.New("invalid event counts")
	}
	if len(r.Stages) > 8 || len(r.Events) > 500 {
		return errors.New("too many stages or events")
	}
	for _, stage := range r.Stages {
		if !validUIPerformanceStageName(stage.Name) || !validUIPerformanceCount(stage.Count) ||
			!validUIPerformanceNumber(stage.TotalMs) || stage.TotalMs > maxTotalMs ||
			!validUIPerformanceNumber(stage.MaxMs) || stage.MaxMs > maxTimeMs {
			return errors.New("invalid stage metrics")
		}
	}
	for _, event := range r.Events {
		if !validUIPerformanceEventKind(event.Kind) || !validUIPerformanceNumber(event.AtMs) || event.AtMs > maxTimeMs {
			return errors.New("invalid event")
		}
		if event.Name != nil && !validUIPerformanceStageName(*event.Name) {
			return errors.New("invalid event name")
		}
		for _, duration := range []*float64{event.DurationMs, event.BlockingMs, event.ScriptMs, event.ForcedLayoutMs, event.StyleLayoutMs} {
			if duration != nil && (!validUIPerformanceNumber(*duration) || *duration > maxTimeMs) {
				return errors.New("invalid event duration")
			}
		}
		if event.Value != nil && !validUIPerformanceNumber(*event.Value) {
			return errors.New("invalid event value")
		}
	}
	return nil
}

func validUIPerformanceCount(value int) bool {
	return value >= 0 && value <= maxCount
}

func validUIPerformanceNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func validUIPerformanceStageName(value string) bool {
	switch value {
	case "scroll_frame", "content_context_scroll", "back_to_top", "side_pane", "timeline_render":
		return true
	default:
		return false
	}
}

func validUIPerformanceEventKind(value string) bool {
	switch value {
	case "frame_gap", "loaf", "longtask", "layout_shift", "stage", "media_load":
		return true
	default:
		return false
	}
}

func cloneUIPerformanceReport(report Report) Report {
	report.Stages = append([]Stage{}, report.Stages...)
	report.Events = append([]Event{}, report.Events...)
	for index := range report.Events {
		event := &report.Events[index]
		event.Name = cloneStringPointer(event.Name)
		event.DurationMs = cloneFloatPointer(event.DurationMs)
		event.BlockingMs = cloneFloatPointer(event.BlockingMs)
		event.ScriptMs = cloneFloatPointer(event.ScriptMs)
		event.ForcedLayoutMs = cloneFloatPointer(event.ForcedLayoutMs)
		event.StyleLayoutMs = cloneFloatPointer(event.StyleLayoutMs)
		event.Value = cloneFloatPointer(event.Value)
		if event.HadRecentInput != nil {
			value := *event.HadRecentInput
			event.HadRecentInput = &value
		}
	}
	return report
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
