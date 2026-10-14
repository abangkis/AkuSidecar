//go:build windows && auth_session_dev && auth_session_fixture

package appshell

import (
	"context"
	"testing"
)

func TestDevelopmentFrontendPresentationRefusesUnownedOrWrongWindow(t *testing.T) {
	valid := DevelopmentFrontendWindowState{PID: 42, HWND: 7, Class: "Chrome_WidgetWin_1", JobMember: true, TitleMatched: true}
	if !developmentFrontendWindowEligible(valid, 42) {
		t.Fatal("owned hidden QA HWND not eligible")
	}
	for _, mutate := range []func(*DevelopmentFrontendWindowState){func(v *DevelopmentFrontendWindowState) { v.PID = 43 }, func(v *DevelopmentFrontendWindowState) { v.HWND = 0 }, func(v *DevelopmentFrontendWindowState) { v.JobMember = false }, func(v *DevelopmentFrontendWindowState) { v.TitleMatched = false }, func(v *DevelopmentFrontendWindowState) { v.Class = "Chrome_WidgetWin_0" }} {
		v := valid
		mutate(&v)
		if developmentFrontendWindowEligible(v, 42) {
			t.Fatal("unowned/wrong HWND could be shown")
		}
	}
	s := &Session{}
	if _, err := s.PresentDevelopmentFrontend(context.Background(), "ordinary title", nil); err == nil {
		t.Fatal("non QA title accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, err := s.PresentDevelopmentFrontend(ctx, "AkuBrowser QA - Synthetic native reader", nil)
	if err == nil || state.HWND != 0 || state.PresentedWithoutActivation {
		t.Fatal("missing actual owned Window could be presented")
	}
}

func TestDevelopmentFrontendPresentationMonitorAndDesktopProof(t *testing.T) {
	for _, work := range []DevelopmentFrontendRect{{0, 0, 1920, 1040}, {-1920, 0, 0, 1080}, {100, -900, 1700, 0}} {
		fit, ok := developmentFrontendFit(work)
		if !ok || fit.Left < work.Left || fit.Top < work.Top || fit.Right > work.Right || fit.Bottom > work.Bottom || fit.Right-fit.Left > 1440 || fit.Bottom-fit.Top > 1000 {
			t.Fatal("QA viewport did not fit selected work area", work, fit)
		}
	}
	if _, ok := developmentFrontendFit(DevelopmentFrontendRect{0, 0, 300, 200}); ok {
		t.Fatal("undersized monitor accepted")
	}
	valid := DevelopmentFrontendWindowState{Visible: true, MonitorKnown: true, WithinWorkArea: true, WorkAreaIntersection: true, Topmost: true, CloakKnown: true, SessionKnown: true, InputDesktopKnown: true, InputDesktopMatch: true, WindowSession: 1, ProcessSession: 1}
	if !developmentFrontendPresented(valid) {
		t.Fatal("complete physical readiness proof denied")
	}
	for _, mutate := range []func(*DevelopmentFrontendWindowState){func(v *DevelopmentFrontendWindowState) { v.Minimized = true }, func(v *DevelopmentFrontendWindowState) { v.WithinWorkArea = false }, func(v *DevelopmentFrontendWindowState) { v.Topmost = false }, func(v *DevelopmentFrontendWindowState) { v.CloakKnown = false }, func(v *DevelopmentFrontendWindowState) { v.Cloaked = true }, func(v *DevelopmentFrontendWindowState) { v.WindowSession = 2 }, func(v *DevelopmentFrontendWindowState) { v.InputDesktopKnown = false }, func(v *DevelopmentFrontendWindowState) { v.InputDesktopMatch = false }} {
		v := valid
		mutate(&v)
		if developmentFrontendPresented(v) {
			t.Fatal("unknown/offscreen/other desktop readiness accepted")
		}
	}
	valid.Topmost = false
	valid.AuthorizedHelperBegan = true
	if !developmentFrontendPresented(valid) {
		t.Fatal("real early helper drop was mistaken for invisible UI")
	}
}
