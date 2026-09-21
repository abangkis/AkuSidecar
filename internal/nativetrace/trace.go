// Package nativetrace provides bounded, passive capture diagnostics. It never
// reads titles, command lines, URLs, content, input events, or screenshots.
package nativetrace

import (
	"context"
	"sync"
	"time"
)

const MaxDuration = 120 * time.Second
const MaxRecords = 256

type Window struct {
	HWND               string `json:"hwnd"`
	Root               string `json:"rootHwnd"`
	PID                uint32 `json:"pid"`
	Class              string `json:"processClass"`
	Visible            bool   `json:"visible"`
	Minimized          bool   `json:"minimized"`
	Topmost            bool   `json:"topmost"`
	TopmostAvailable   bool   `json:"topmostAvailable"`
	Z                  int    `json:"zOrder"`
	AboveForeground    bool   `json:"aboveForeground"`
	OverlapsForeground bool   `json:"overlapsForeground"`
	OverlapAvailable   bool   `json:"overlapAvailable"`
}

type Sample struct {
	At                 string   `json:"at"`
	Trigger            string   `json:"trigger"`
	Status             string   `json:"status"`
	Foreground         string   `json:"foregroundHwnd"`
	PreviousForeground string   `json:"previousForegroundHwnd,omitempty"`
	Focus              string   `json:"keyboardFocusHwnd"`
	FocusAvailable     bool     `json:"keyboardFocusAvailable"`
	EventHWND          string   `json:"eventHwnd,omitempty"`
	EventTick          uint32   `json:"eventTick,omitempty"`
	EventPID           uint32   `json:"eventPid,omitempty"`
	EventClass         string   `json:"eventProcessClass,omitempty"`
	EventRoot          string   `json:"eventRootHwnd,omitempty"`
	ZOrderAvailable    bool     `json:"zOrderAvailable"`
	Windows            []Window `json:"windows"`
	Truncated          bool     `json:"windowsTruncated"`
	Classification     string   `json:"classification"`
}

// classifyCovering identifies geometric covering while the sampled external
// foreground and keyboard-focus handles stay unchanged. It does not assert
// pixel occlusion or rule out transitions between snapshots.
func classifyCovering(current, previous Sample) string {
	if !current.ZOrderAvailable || !current.FocusAvailable || !previous.FocusAvailable || current.Focus == "" || current.Focus == "0x0" {
		return "insufficient_evidence"
	}
	foregroundClass := ""
	for _, w := range current.Windows {
		if w.HWND == current.Foreground {
			foregroundClass = w.Class
			break
		}
	}
	if foregroundClass == "akubrowser_root" {
		return "akubrowser_foreground"
	}
	if foregroundClass != "other" && foregroundClass != "chrome_other" {
		return "insufficient_evidence"
	}
	if current.Foreground != previous.Foreground || current.Focus != previous.Focus || (current.EventClass == "akubrowser_root" && current.Trigger != "reorder_event") {
		return "no_unchanged_focus_covering_signature"
	}
	missingOverlap := false
	for _, w := range current.Windows {
		if w.Class == "akubrowser_root" && w.Visible && !w.Minimized && w.AboveForeground && !w.OverlapAvailable {
			missingOverlap = true
		}
		if w.Class == "akubrowser_root" && w.Visible && !w.Minimized && w.AboveForeground && w.OverlapAvailable && w.OverlapsForeground {
			return "akubrowser_above_external_with_sampled_focus_unchanged"
		}
	}
	if current.Truncated || missingOverlap {
		return "insufficient_evidence"
	}
	return "no_unchanged_focus_covering_signature"
}

type Manager struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	run        string
	generation uint64
}

// Start acknowledges hook setup before command dispatch (bounded to 250ms).
// Only one run trace is active; a different run supersedes the earlier one.
// Emission is buffered and bounded, and cannot block a native callback.
func (m *Manager) Start(run string, pid uint32, emit func(Sample)) {
	m.start(run, pid, emit, observe)
}

func (m *Manager) start(run string, pid uint32, emit func(Sample), observer func(context.Context, uint32, chan struct{}, func(Sample))) {
	m.mu.Lock()
	if m.run == run && m.cancel != nil {
		m.mu.Unlock()
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), MaxDuration)
	m.cancel, m.run = cancel, run
	m.generation++
	generation := m.generation
	m.mu.Unlock()
	ready := make(chan struct{})
	samples := make(chan Sample, MaxRecords)
	go func() {
		defer cancel()
		defer close(samples)
		count := 0
		observer(ctx, pid, ready, func(s Sample) {
			if count >= MaxRecords-1 && s.Trigger != "trace_end" {
				cancel()
				return
			}
			if count >= MaxRecords {
				return
			}
			if count == MaxRecords-1 {
				s.Status = "record_limit_reached"
			}
			select {
			case samples <- s:
				count++
			default:
				cancel()
			}
		})
	}()
	go func() {
		for sample := range samples {
			emit(sample)
		}
		m.mu.Lock()
		if m.generation == generation {
			m.cancel = nil
			m.run = ""
		}
		m.mu.Unlock()
	}()
	select {
	case <-ready:
	case <-time.After(250 * time.Millisecond):
	}
}

// StopRun cannot let an old release receipt cancel a newer capture command.
func (m *Manager) StopRun(run string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run != "" && m.run == run && m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel, m.run = nil, ""
}
