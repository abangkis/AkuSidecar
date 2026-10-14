//go:build windows && auth_session_dev && auth_session_fixture

package appshell

import (
	"context"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type DevelopmentReaderHelperProof struct {
	PID, Parent     uint32
	Image, Created  string
	JobMember       bool
	AgeMS           int64
	TopmostReleased bool
}

func (s *Session) ServeDevelopmentReaderBrokerFixture(ctx context.Context, pipe string, handle func(context.Context, readerbroker.Request, func(readerbroker.Target) (readerbroker.Reply, error)) error, observe func(DevelopmentReaderHelperProof)) error {
	return readerbroker.ServeFixture(ctx, pipe, func(pid uint32) error {
		if err := s.authorizeReaderHelper(pid); err != nil {
			return err
		}
		parent, err := readerbroker.ParentPID(pid)
		if err != nil {
			return err
		}
		image, err := readerbroker.ProcessImage(pid)
		if err != nil {
			return err
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(h)
		var created, exit, kernel, user windows.Filetime
		if err = windows.GetProcessTimes(h, &created, &exit, &kernel, &user); err != nil {
			return err
		}
		proof := DevelopmentReaderHelperProof{PID: pid, Parent: parent, Image: image, Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), AgeMS: time.Since(time.Unix(0, created.Nanoseconds())).Milliseconds(), JobMember: true}
		if observe != nil {
			observe(proof)
		}
		// A real, authorized Chrome child consumes the QA presentation. Publish
		// that proof before the serialized drop so presentation cannot re-raise
		// TOPMOST after this intent begins. No reader runs before the drop returns.
		if err := s.DropDevelopmentFrontendTopmost("AkuBrowser QA - Synthetic native reader"); err != nil {
			return err
		}
		proof.TopmostReleased = true
		if observe != nil {
			observe(proof)
		}
		return nil
	}, handle)
}

type DevelopmentFrontendWindowState struct {
	HWND                                                                    uint64
	Visible, Minimized                                                      bool
	PID                                                                     uint32
	JobMember, TitleMatched, PresentedWithoutActivation                     bool
	Class                                                                   string
	OwnedCandidates                                                         int
	Rect, WorkArea                                                          DevelopmentFrontendRect
	MonitorKnown, WorkAreaIntersection, WithinWorkArea, Topmost             bool
	CloakKnown, Cloaked, SessionKnown, InputDesktopKnown, InputDesktopMatch bool
	WindowSession, ProcessSession                                           uint32
	AuthorizedHelperBegan                                                   bool
}

type DevelopmentFrontendRect struct{ Left, Top, Right, Bottom int32 }

func developmentFrontendFit(work DevelopmentFrontendRect) (DevelopmentFrontendRect, bool) {
	w, h := work.Right-work.Left-40, work.Bottom-work.Top-40
	if w > 1440 {
		w = 1440
	}
	if h > 1000 {
		h = 1000
	}
	if w < 360 || h < 240 {
		return DevelopmentFrontendRect{}, false
	}
	x := work.Left + (work.Right-work.Left-w)/2
	y := work.Top + (work.Bottom-work.Top-h)/2
	return DevelopmentFrontendRect{x, y, x + w, y + h}, true
}
func developmentFrontendPresented(v DevelopmentFrontendWindowState) bool {
	return v.Visible && !v.Minimized && v.MonitorKnown && v.WithinWorkArea && v.WorkAreaIntersection && (v.Topmost || v.AuthorizedHelperBegan) && v.CloakKnown && !v.Cloaked && v.SessionKnown && v.WindowSession == v.ProcessSession && v.InputDesktopKnown && v.InputDesktopMatch
}

func developmentFrontendDesktopName(handle uintptr) (string, bool) {
	if handle == 0 {
		return "", false
	}
	name := make([]uint16, 256)
	var needed uint32
	ok, _, _ := user32.NewProc("GetUserObjectInformationW").Call(handle, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 || needed > uint32(len(name)*2) {
		return "", false
	}
	return windows.UTF16ToString(name), true
}

func developmentFrontendPhysicalState(v *DevelopmentFrontendWindowState) {
	hwnd := uintptr(v.HWND)
	if hwnd == 0 {
		return
	}
	user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&v.Rect)))
	ex, _, _ := user32.NewProc("GetWindowLongPtrW").Call(hwnd, ^uintptr(19))
	v.Topmost = ex&8 != 0 // GWL_EXSTYLE / WS_EX_TOPMOST
	var point struct{ X, Y int32 }
	ok, _, _ := user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	if ok != 0 {
		packed := uint64(uint32(point.X)) | uint64(uint32(point.Y))<<32
		monitor, _, _ := user32.NewProc("MonitorFromPoint").Call(uintptr(packed), 2)
		info := struct {
			Size          uint32
			Monitor, Work DevelopmentFrontendRect
			Flags         uint32
		}{Size: 40}
		valid, _, _ := user32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
		if monitor != 0 && valid != 0 {
			v.MonitorKnown = true
			v.WorkArea = info.Work
			v.WorkAreaIntersection = v.Rect.Right > info.Work.Left && v.Rect.Left < info.Work.Right && v.Rect.Bottom > info.Work.Top && v.Rect.Top < info.Work.Bottom
			v.WithinWorkArea = v.Rect.Right > v.Rect.Left && v.Rect.Bottom > v.Rect.Top && v.Rect.Left >= info.Work.Left && v.Rect.Top >= info.Work.Top && v.Rect.Right <= info.Work.Right && v.Rect.Bottom <= info.Work.Bottom
		}
	}
	var cloaked uint32
	hr, _, _ := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute").Call(hwnd, 14, uintptr(unsafe.Pointer(&cloaked)), 4)
	v.CloakKnown = int32(hr) == 0
	v.Cloaked = cloaked != 0
	a, _, _ := kernel32.NewProc("ProcessIdToSessionId").Call(uintptr(v.PID), uintptr(unsafe.Pointer(&v.WindowSession)))
	b, _, _ := kernel32.NewProc("ProcessIdToSessionId").Call(uintptr(windows.GetCurrentProcessId()), uintptr(unsafe.Pointer(&v.ProcessSession)))
	v.SessionKnown = a != 0 && b != 0
	input, _, _ := user32.NewProc("OpenInputDesktop").Call(0, 0, 1)
	if input != 0 {
		defer user32.NewProc("CloseDesktop").Call(input)
	}
	thread, _, _ := procGetWindowThreadProcess.Call(hwnd, 0)
	desktop, _, _ := user32.NewProc("GetThreadDesktop").Call(thread)
	i, ik := developmentFrontendDesktopName(input)
	d, dk := developmentFrontendDesktopName(desktop)
	v.InputDesktopKnown = ik && dk
	v.InputDesktopMatch = v.InputDesktopKnown && i == d
}

func (s *Session) DropDevelopmentFrontendTopmost(title string) error {
	v := s.developmentFrontendWindowState(title)
	if !developmentFrontendWindowEligible(v, uint32(s.PID())) {
		return errors.New("QA topmost release HWND identity unavailable")
	}
	s.mu.Lock()
	ok, _, _ := user32.NewProc("SetWindowPos").Call(uintptr(v.HWND), ^uintptr(1), 0, 0, 0, 0, 0x13) // NOTOPMOST, NOMOVE|NOSIZE|NOACTIVATE
	s.mu.Unlock()
	if ok == 0 {
		return errors.New("QA topmost release rejected")
	}
	v = s.developmentFrontendWindowState(title)
	if v.HWND == 0 || v.Topmost {
		return errors.New("QA topmost release readback failed")
	}
	return nil
}

func developmentFrontendWindowEligible(v DevelopmentFrontendWindowState, pid uint32) bool {
	return pid != 0 && v.PID == pid && v.HWND != 0 && v.JobMember && v.TitleMatched && v.Class == "Chrome_WidgetWin_1"
}

// Private CDP deliberately uses HideWindow at process launch. Only this tagged
// QA composition may present its already loaded, exact owned QA-title HWND,
// using SW_SHOWNOACTIVATE. It never changes foreground or sends UI input.
func (s *Session) PresentDevelopmentFrontend(ctx context.Context, title string, authorizedHelper func() bool) (DevelopmentFrontendWindowState, error) {
	if title != "AkuBrowser QA - Synthetic native reader" && title != "AkuBrowser QA - Native activation DISABLED" {
		return DevelopmentFrontendWindowState{}, errors.New("development frontend presentation title rejected")
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var state DevelopmentFrontendWindowState
	shown := false
	for {
		state = s.developmentFrontendWindowState(title)
		state.AuthorizedHelperBegan = authorizedHelper != nil && authorizedHelper()
		if developmentFrontendWindowEligible(state, uint32(s.PID())) {
			if (shown || state.AuthorizedHelperBegan) && developmentFrontendPresented(state) {
				state.PresentedWithoutActivation = shown
				return state, nil
			}
			if !shown {
				if !state.MonitorKnown || !state.CloakKnown || state.Cloaked || !state.SessionKnown || state.WindowSession != state.ProcessSession || !state.InputDesktopKnown || !state.InputDesktopMatch {
					return state, errors.New("QA monitor/cloak/session/input-desktop proof unavailable or mismatched")
				}
				fit, ok := developmentFrontendFit(state.WorkArea)
				if !ok {
					return state, errors.New("QA active monitor work area cannot fit frontend")
				}
				s.mu.Lock()
				if authorizedHelper != nil && authorizedHelper() {
					s.mu.Unlock()
					continue
				}
				before, _, _ := user32.NewProc("GetForegroundWindow").Call()
				user32.NewProc("ShowWindowAsync").Call(uintptr(state.HWND), 4)                                                                                                                                           // SW_SHOWNOACTIVATE
				positioned, _, _ := user32.NewProc("SetWindowPos").Call(uintptr(state.HWND), ^uintptr(0), uintptr(int(fit.Left)), uintptr(int(fit.Top)), uintptr(fit.Right-fit.Left), uintptr(fit.Bottom-fit.Top), 0x50) // TOPMOST, SHOWWINDOW|NOACTIVATE
				s.mu.Unlock()
				if positioned == 0 {
					return state, errors.New("QA active monitor presentation rejected")
				}
				shown = true
				after, _, _ := user32.NewProc("GetForegroundWindow").Call()
				if after != before {
					return state, errors.New("development frontend presentation foreground changed")
				}
			}
		}
		select {
		case <-deadline.Done():
			return state, fmt.Errorf("development frontend presentation unavailable: %w", deadline.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (s *Session) developmentFrontendWindowState(title string) DevelopmentFrontendWindowState {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.window.(*Window)
	if !ok || w == nil || w.owner.job == 0 {
		return DevelopmentFrontendWindowState{}
	}
	pid := uint32(w.PID())
	state := DevelopmentFrontendWindowState{PID: pid}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return state
	}
	defer windows.CloseHandle(process)
	state.JobMember = processBelongsToJob(process, w.owner.job)
	if !state.JobMember {
		return state
	}
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var actual uint32
		procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&actual)))
		if actual != pid {
			return 1
		}
		state.OwnedCandidates++
		class := make([]uint16, 128)
		user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		name := windows.UTF16ToString(class)
		if name != "Chrome_WidgetWin_1" {
			return 1
		}
		text := make([]uint16, 512)
		user32.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
		value := windows.UTF16ToString(text)
		matched := title != "" && (value == title || strings.HasPrefix(value, title+" - "))
		if !matched {
			return 1
		}
		state.HWND = uint64(hwnd)
		state.Class = name
		state.TitleMatched = true
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		minimized, _, _ := isCaptureIconic.Call(hwnd)
		state.Visible = visible != 0
		state.Minimized = minimized != 0
		developmentFrontendPhysicalState(&state)
		return 0
	})
	procEnumWindows.Call(callback, 0)
	return state
}

func (s *Session) DevelopmentFrontendWindowReadback() DevelopmentFrontendWindowState {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.window.(*Window)
	if !ok || w == nil {
		return DevelopmentFrontendWindowState{}
	}
	hwnd := visibleTopLevelWindow(uint32(w.PID()))
	if hwnd == 0 {
		return DevelopmentFrontendWindowState{}
	}
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	minimized, _, _ := isCaptureIconic.Call(hwnd)
	var windowPID uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
	return DevelopmentFrontendWindowState{HWND: uint64(hwnd), PID: windowPID, Visible: visible != 0, Minimized: minimized != 0}
}

// Read the exact owned QA window even while private CDP keeps it hidden. This
// lifetime proof never presents a window or grants foreground capability.
func (s *Session) DevelopmentFrontendLifetimeWindowReadback() DevelopmentFrontendWindowState {
	return s.developmentFrontendWindowState("AkuBrowser QA - Native activation DISABLED")
}

// DevelopmentFrontendProtocol borrows the existing private pipe. The fixture
// uses it for fixed DOM/network readback; it owns no transport or browser.
func (s *Session) DevelopmentFrontendProtocol() (CaptureProtocol, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.window.(*Window)
	if !ok || w == nil || w.CaptureProtocol() == nil {
		return nil, errors.New("development frontend private pipe unavailable")
	}
	return w.CaptureProtocol(), nil
}
