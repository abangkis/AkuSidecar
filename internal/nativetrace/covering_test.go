package nativetrace

import "testing"

func TestCoveringSignaturePreservesExternalForegroundAndTypingFocus(t *testing.T) {
	previous := Sample{Foreground: "0xcodex", Focus: "0xeditor", FocusAvailable: true}
	baseline := Sample{Foreground: previous.Foreground, Focus: previous.Focus, FocusAvailable: true, ZOrderAvailable: true, Windows: []Window{
		{HWND: previous.Foreground, Class: "other", Visible: true, Z: 3},
		{HWND: "0xaku", Class: "akubrowser_root", Visible: true, Z: 1, AboveForeground: true, OverlapAvailable: true, OverlapsForeground: true},
	}}
	for _, tc := range []struct {
		name, want string
		change     func(*Sample)
	}{
		{"external_typing_continues", "akubrowser_above_external_with_sampled_focus_unchanged", func(*Sample) {}},
		{"chrome_typing_continues", "akubrowser_above_external_with_sampled_focus_unchanged", func(s *Sample) { s.Windows[0].Class = "chrome_other" }},
		{"not_above", "no_unchanged_focus_covering_signature", func(s *Sample) { s.Windows[1].AboveForeground = false }},
		{"not_overlapping", "no_unchanged_focus_covering_signature", func(s *Sample) { s.Windows[1].OverlapsForeground = false }},
		{"minimized", "no_unchanged_focus_covering_signature", func(s *Sample) { s.Windows[1].Minimized = true }},
		{"hidden", "no_unchanged_focus_covering_signature", func(s *Sample) { s.Windows[1].Visible = false }},
		{"focus_changed", "no_unchanged_focus_covering_signature", func(s *Sample) { s.Focus = "0xother" }},
		{"activation_event", "no_unchanged_focus_covering_signature", func(s *Sample) { s.EventClass = "akubrowser_root" }},
		{"reorder_is_not_activation", "akubrowser_above_external_with_sampled_focus_unchanged", func(s *Sample) {
			s.EventClass = "akubrowser_root"
			s.Trigger = "reorder_event"
		}},
		{"topmost_covering", "akubrowser_above_external_with_sampled_focus_unchanged", func(s *Sample) {
			s.Windows[1].Topmost, s.Windows[1].TopmostAvailable = true, true
		}},
		{"ordinary_z_order_covering", "akubrowser_above_external_with_sampled_focus_unchanged", func(s *Sample) {
			s.Windows[1].TopmostAvailable = true
		}},
		{"aku_foreground", "akubrowser_foreground", func(s *Sample) { s.Foreground = "0xaku" }},
		{"unknown_focus", "insufficient_evidence", func(s *Sample) { s.FocusAvailable = false }},
		{"unknown_overlap", "insufficient_evidence", func(s *Sample) { s.Windows[1].OverlapAvailable = false }},
		{"unknown_foreground_owner", "insufficient_evidence", func(s *Sample) { s.Windows[0].Class = "unavailable" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := baseline
			s.Windows = append([]Window(nil), baseline.Windows...)
			tc.change(&s)
			if got := classifyCovering(s, previous); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	if got := classifyCovering(baseline, Sample{}); got != "insufficient_evidence" {
		t.Fatalf("missing prior focus: %s", got)
	}
}
