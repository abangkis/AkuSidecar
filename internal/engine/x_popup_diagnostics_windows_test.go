//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func decodeXTwoPhaseOutcome(raw []byte) (string, string, error) {
	invalid := errors.New("two-phase manual observation rejected")
	if len(raw) == 0 || len(raw) > 128 {
		return "", "", invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return "", "", invalid
	}
	seen := map[string]bool{}
	outcome, observation := "", "not_observed"
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] || name != "outcome" && name != "popupObservation" {
			return "", "", invalid
		}
		value, err := decoder.Token()
		text, ok := value.(string)
		if err != nil || !ok {
			return "", "", invalid
		}
		seen[name] = true
		if name == "outcome" {
			outcome = text
		} else {
			observation = text
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || !seen["outcome"] || decoder.Decode(new(any)) != io.EOF {
		return "", "", invalid
	}
	fixed, _ := json.Marshal(map[string]string{"outcome": outcome})
	if _, err := decodeXStandardOutcome(fixed); err != nil {
		return "", "", invalid
	}
	switch observation {
	case "not_observed", "no_visible_change", "blocked_icon_visible", "popup_hidden_or_blank":
		return outcome, observation, nil
	default:
		return "", "", invalid
	}
}

type xPopupWindowState struct {
	visible, minimized, foreground bool
}

type xPopupWindowSummary struct {
	Coverage              string `json:"coverage"`
	Classification        string `json:"classification"`
	BaselineWindows       int    `json:"baselineWindows"`
	AdditionalWindows     int    `json:"additionalWindows"`
	AdditionalVisible     int    `json:"additionalVisible"`
	AdditionalMinimized   int    `json:"additionalMinimized"`
	AdditionalForeground  int    `json:"additionalForeground"`
	AdditionalBackground  int    `json:"additionalBackground"`
	AdditionalNotVisible  int    `json:"additionalNotVisible"`
	PopupBlockingEvidence string `json:"popupBlockingEvidence"`
}

// HWNDs remain private. Samples cover only top-level Chrome windows of the
// pinned QA root; neither titles nor URLs nor other applications are inspected.
type xPopupWindowTracker struct {
	baseline map[uintptr]xPopupWindowState
	observed map[uintptr]xPopupWindowState
	failed   bool
}

func newXPopupWindowTracker(baseline map[uintptr]xPopupWindowState) *xPopupWindowTracker {
	return &xPopupWindowTracker{baseline: baseline, observed: make(map[uintptr]xPopupWindowState)}
}

func (tracker *xPopupWindowTracker) sample(snapshot map[uintptr]xPopupWindowState, err error) {
	if err != nil || len(snapshot) > 32 {
		tracker.failed = true
		return
	}
	for hwnd, state := range snapshot {
		if _, initial := tracker.baseline[hwnd]; initial {
			continue
		}
		if _, exists := tracker.observed[hwnd]; !exists && len(tracker.observed) >= 32 {
			tracker.failed = true
			return
		}
		old := tracker.observed[hwnd]
		tracker.observed[hwnd] = xPopupWindowState{visible: old.visible || state.visible,
			minimized: old.minimized || state.minimized, foreground: old.foreground || state.foreground}
	}
}

func (tracker *xPopupWindowTracker) summary() xPopupWindowSummary {
	result := xPopupWindowSummary{Coverage: "sampled_owned_root_windows", Classification: "no_additional_window_observed",
		BaselineWindows: len(tracker.baseline), AdditionalWindows: len(tracker.observed), PopupBlockingEvidence: "unobserved"}
	for _, state := range tracker.observed {
		if state.visible {
			result.AdditionalVisible++
		} else {
			result.AdditionalNotVisible++
		}
		if state.minimized {
			result.AdditionalMinimized++
		}
		if state.foreground {
			result.AdditionalForeground++
		} else if state.visible {
			result.AdditionalBackground++
		}
	}
	switch {
	case tracker.failed:
		result.Coverage, result.Classification = "partial_or_unavailable", "window_inventory_incomplete"
	case result.AdditionalForeground > 0:
		result.Classification = "additional_window_foreground_observed"
	case result.AdditionalVisible > 0:
		result.Classification = "additional_window_visible_without_foreground_observed"
	case result.AdditionalNotVisible > 0:
		result.Classification = "additional_window_not_visible_observed"
	}
	return result
}

type xPopupInventory struct {
	pid        uint32
	foreground uintptr
	windows    map[uintptr]xPopupWindowState
	failed     bool
}

// Go's Windows callback slots are permanent. Reuse one callback across the
// bounded sampling period instead of allocating a slot for every sample.
var xPopupInventoryMu sync.Mutex
var xPopupCurrentInventory *xPopupInventory
var xPopupInventoryCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
	inventory := xPopupCurrentInventory
	if inventory == nil || processFixtureWindowPID(hwnd) != inventory.pid {
		return 1
	}
	var class [64]uint16
	length, _, _ := processFixtureUser.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	if length == 0 {
		inventory.failed = true
		return 0
	}
	if windows.UTF16ToString(class[:]) != "Chrome_WidgetWin_1" {
		return 1
	}
	if len(inventory.windows) >= 32 {
		inventory.failed = true
		return 0
	}
	visible, _, _ := processFixtureUser.NewProc("IsWindowVisible").Call(hwnd)
	minimized, _, _ := processFixtureUser.NewProc("IsIconic").Call(hwnd)
	inventory.windows[hwnd] = xPopupWindowState{visible: visible != 0, minimized: minimized != 0, foreground: inventory.foreground == hwnd}
	return 1
})

func xPopupWindowSnapshot(bridge *chromeFixtureBridge, root windows.Handle, pid uint32) (map[uintptr]xPopupWindowState, error) {
	state, err := windows.WaitForSingleObject(root, 0)
	identity, identityErr := processFixtureIdentityFromHandle(root)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) || identityErr != nil || identity.PID != pid || bridge.containsPID(pid) != nil {
		return nil, errors.New("popup root ownership unavailable")
	}
	xPopupInventoryMu.Lock()
	defer xPopupInventoryMu.Unlock()
	foreground, _, _ := processFixtureUser.NewProc("GetForegroundWindow").Call()
	inventory := &xPopupInventory{pid: pid, foreground: foreground, windows: make(map[uintptr]xPopupWindowState)}
	xPopupCurrentInventory = inventory
	defer func() { xPopupCurrentInventory = nil }()
	enumerated, _, _ := processFixtureUser.NewProc("EnumWindows").Call(xPopupInventoryCallback, 0)
	if enumerated == 0 || inventory.failed {
		return nil, errors.New("popup window inventory unavailable")
	}
	return inventory.windows, nil
}

func TestXPopupDiagnosticsOffline(t *testing.T) {
	for _, observation := range []string{"not_observed", "no_visible_change", "blocked_icon_visible", "popup_hidden_or_blank"} {
		outcome, got, err := decodeXTwoPhaseOutcome([]byte(`{"outcome":"other_error","popupObservation":"` + observation + `"}`))
		if err != nil || outcome != "other_error" || got != observation {
			t.Fatal("fixed manual popup observation rejected")
		}
	}
	if _, observation, err := decodeXTwoPhaseOutcome([]byte(`{"outcome":"home_visible"}`)); err != nil || observation != "not_observed" {
		t.Fatal("legacy outcome lost unknown popup state")
	}
	for _, raw := range []string{`{"outcome":"other_error","popupObservation":"private"}`, `{"outcome":"other_error","popupObservation":"no_visible_change","popupObservation":"blocked_icon_visible"}`, `{"outcome":"other_error","url":"private"}`} {
		if _, _, err := decodeXTwoPhaseOutcome([]byte(raw)); err == nil {
			t.Fatal("unsafe or duplicate observation accepted")
		}
	}
	for _, test := range []struct {
		name           string
		state          xPopupWindowState
		classification string
	}{
		{"foreground", xPopupWindowState{visible: true, foreground: true}, "additional_window_foreground_observed"},
		{"background", xPopupWindowState{visible: true}, "additional_window_visible_without_foreground_observed"},
		{"minimized", xPopupWindowState{visible: true, minimized: true}, "additional_window_visible_without_foreground_observed"},
		{"hidden", xPopupWindowState{}, "additional_window_not_visible_observed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tracker := newXPopupWindowTracker(map[uintptr]xPopupWindowState{1: {visible: true}})
			tracker.sample(map[uintptr]xPopupWindowState{1: {visible: true}, 2: test.state}, nil)
			got := tracker.summary()
			if got.Classification != test.classification || got.AdditionalWindows != 1 || got.PopupBlockingEvidence != "unobserved" {
				t.Fatal("owned-window classification mismatch")
			}
			if test.state.minimized && got.AdditionalMinimized != 1 {
				t.Fatal("minimized window was not distinguished")
			}
		})
	}
	tracker := newXPopupWindowTracker(map[uintptr]xPopupWindowState{1: {visible: true}})
	tracker.sample(map[uintptr]xPopupWindowState{1: {visible: true}}, nil)
	if got := tracker.summary(); got.Classification != "no_additional_window_observed" || got.PopupBlockingEvidence != "unobserved" {
		t.Fatal("no new window was misrepresented as blocked")
	}
	tracker.sample(nil, errors.New("unavailable"))
	if tracker.summary().Classification != "window_inventory_incomplete" {
		t.Fatal("failed sample was treated as absence")
	}
	transient := newXPopupWindowTracker(map[uintptr]xPopupWindowState{1: {visible: true}})
	transient.sample(map[uintptr]xPopupWindowState{2: {visible: true, foreground: true}}, nil)
	transient.sample(map[uintptr]xPopupWindowState{}, nil)
	if transient.summary().AdditionalForeground != 1 {
		t.Fatal("transient window observation was erased")
	}
	bounded := newXPopupWindowTracker(nil)
	for index := uintptr(1); index <= 33; index++ {
		bounded.sample(map[uintptr]xPopupWindowState{index: {}}, nil)
	}
	if got := bounded.summary(); got.Classification != "window_inventory_incomplete" || got.AdditionalWindows != 32 {
		t.Fatal("window accumulator exceeded its bound or reported complete coverage")
	}
}
