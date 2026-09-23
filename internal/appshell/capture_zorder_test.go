package appshell

import (
	"reflect"
	"testing"
)

func TestCaptureZOrderTargetsOnlyOwnedBackgroundAboveAnchor(t *testing.T) {
	windows := []captureZWindow{
		{hwnd: 1, owned: true, visible: true},
		{hwnd: 2, owned: false, visible: true},
		{hwnd: 3, owned: true, reader: true, visible: true},
		{hwnd: 4, owned: true, visible: false},
		{hwnd: 5, visible: true},
		{hwnd: 6, owned: true, visible: true},
	}
	if got := captureWindowsToLower(5, windows); !reflect.DeepEqual(got, []uintptr{1}) {
		t.Fatalf("targets=%v", got)
	}
	for _, fg := range []uintptr{0, 99, 1, 6} {
		if got := captureWindowsToLower(fg, windows); len(got) != 0 {
			t.Fatalf("unsafe anchor %d targets=%v", fg, got)
		}
	}
	if got := captureWindowsToLower(3, windows); !reflect.DeepEqual(got, []uintptr{1}) {
		t.Fatalf("reader anchor targets=%v", got)
	}
}

func TestCaptureZOrderRevalidationFailsClosedBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		expected, live         uintptr
		owned, reader, success bool
		attempted, applied     bool
	}{
		{"missing anchor", 0, 0, true, false, true, false, false},
		{"foreground changed", 9, 8, true, false, true, false, false},
		{"HWND ownership changed", 9, 9, false, false, true, false, false},
		{"reader exempt", 9, 9, true, true, true, false, false},
		{"native failure", 9, 9, true, false, false, true, false},
		{"accepted", 9, 9, true, false, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			attempted, applied := lowerCaptureWindow(tc.expected, func() uintptr { return tc.live }, func() bool { return tc.owned }, func() bool { return tc.reader }, func() bool { writes++; return tc.success })
			if attempted != tc.attempted || applied != tc.applied || (writes > 0) != tc.attempted {
				t.Fatalf("attempted=%t applied=%t writes=%d", attempted, applied, writes)
			}
		})
	}
}

func TestCaptureReadbackRequiresLiveOwnedWindowBelowKnownAnchor(t *testing.T) {
	anchor := captureZWindow{hwnd: 9, visible: true}
	owned := captureZWindow{hwnd: 1, owned: true, visible: true}
	if !captureReadbackVerified(1, 9, []captureZWindow{anchor, owned}) {
		t.Fatal("below anchor not verified")
	}
	for _, list := range [][]captureZWindow{
		{owned, anchor}, {anchor}, {owned},
		{anchor, {hwnd: 1, visible: true}},
		{anchor, {hwnd: 1, owned: true}},
		{anchor, {hwnd: 1, owned: true, visible: true, reader: true}},
		{{hwnd: 9, owned: true}, owned},
	} {
		if captureReadbackVerified(1, 9, list) {
			t.Fatalf("false readback for %+v", list)
		}
	}
}

func TestCaptureOwnedForegroundIsReportedButReaderAndExternalForegroundAreNot(t *testing.T) {
	windows := []captureZWindow{
		{hwnd: 1, owned: true, visible: true},
		{hwnd: 2, owned: true, reader: true, visible: true},
		{hwnd: 3, owned: false, visible: true},
	}
	for _, tc := range []struct {
		name       string
		foreground uintptr
		want       bool
	}{
		{name: "capture root foreground", foreground: 1, want: true},
		{name: "explicit reader foreground", foreground: 2, want: false},
		{name: "external foreground", foreground: 3, want: false},
		{name: "unknown foreground", foreground: 99, want: false},
		{name: "missing foreground", foreground: 0, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := captureOwnedForeground(tc.foreground, windows); got != tc.want {
				t.Fatalf("captureOwnedForeground(%d)=%t want %t", tc.foreground, got, tc.want)
			}
		})
	}
}
