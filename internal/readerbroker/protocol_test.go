package readerbroker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestActivationReasonIsAllowlisted(t *testing.T) {
	for message, want := range map[string]string{
		"Reader intent expired or UI foreground changed": "ui_foreground_changed",
		"Reader binding expired or changed":              "binding_expired_or_changed",
		"Windows rejected reader activation":             "windows_activation_rejected",
		"https://private.example/token=secret":           "activation_rejected",
	} {
		if got := ActivationReason(Reply{Message: message}, nil); got != want {
			t.Fatalf("got %s want %s", got, want)
		}
	}
	if ActivationReason(Reply{OK: true, Readback: true}, nil) != "accepted" {
		t.Fatal("success misclassified")
	}
	if ActivationReason(Reply{}, errors.New("secret")) != "exchange_failed" {
		t.Fatal("exchange error leaked")
	}
}

func TestForegroundDiagnosticsUseOnlyFixedCategories(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		hwnd                 uintptr
		foregroundPID, uiPID uint32
		readerHWND           uintptr
		want                 string
	}{
		{name: "no foreground", want: "none"},
		{name: "UI", hwnd: 10, foregroundPID: 20, uiPID: 20, readerHWND: 30, want: "ui"},
		{name: "exact reader", hwnd: 30, foregroundPID: 20, uiPID: 20, readerHWND: 30, want: "exact_reader"},
		{name: "other window", hwnd: 40, foregroundPID: 50, uiPID: 20, readerHWND: 30, want: "other"},
		{name: "unknown process", hwnd: 40, readerHWND: 30, want: "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyForeground(tc.hwnd, tc.foregroundPID, tc.uiPID, tc.readerHWND); got != tc.want {
				t.Fatalf("focus category=%q want %q", got, tc.want)
			}
		})
	}
	for input, want := range map[string]string{
		"none": "none", "exact_reader": "exact_reader", "ui": "ui", "other": "other",
		"https://private.example/window": "unknown", "": "unknown",
	} {
		if got := DiagnosticFocusCategory(Reply{FocusCategory: input}); got != want {
			t.Fatalf("diagnostic category %q became %q want %q", input, got, want)
		}
	}
}

func TestReaderRequestRejectsNonClickIDsAndSourceConfusion(t *testing.T) {
	good := Request{RequestID: "broker_" + strings.Repeat("a", 32), Source: "x", URL: "https://x.com/a/status/1"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Request{{RequestID: "dispatch", Source: "x", URL: good.URL}, {RequestID: good.RequestID, Source: "x", URL: "https://x.com.evil/a"}, {RequestID: good.RequestID, Source: "x", URL: "https://user:pass@x.com/a"}, {RequestID: good.RequestID, Source: "linkedin", URL: good.URL}} {
		if bad.Validate() == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
func TestReaderFramingRejectsOversizedAndTruncatedMessages(t *testing.T) {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(8193))
	var r Request
	if Read(&b, &r) == nil {
		t.Fatal("oversized accepted")
	}
	b.Reset()
	binary.Write(&b, binary.LittleEndian, uint32(5))
	b.WriteString("{}")
	if Read(&b, &r) == nil {
		t.Fatal("truncated accepted")
	}
}
func TestReaderTicketDisclosesTargetOnlyAfterSingleClaimAndRevalidation(t *testing.T) {
	for _, mode := range []string{"success", "wrong_ticket", "identity_changed", "expired"} {
		t.Run(mode, func(t *testing.T) {
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			server.SetDeadline(time.Now().Add(time.Second))
			client.SetDeadline(time.Now().Add(time.Second))
			target := Target{HWND: 123, PID: 456, Value: 1, Property: "AkuBrowser.ExplicitReader.456", Action: "split_one", Expires: time.Now().Add(500 * time.Millisecond)}
			result := make(chan error, 1)
			checks := 0
			go func() {
				_, err := Exchange(context.Background(), server, target, func() error {
					checks++
					if mode == "identity_changed" {
						return errors.New("changed")
					}
					return nil
				})
				result <- err
				server.Close()
			}()
			var issue Reply
			if err := Read(client, &issue); err != nil {
				t.Fatal(err)
			}
			if issue.Target != nil || len(issue.Ticket) != 64 {
				t.Fatal("target leaked before claim")
			}
			ticket := issue.Ticket
			if mode == "wrong_ticket" {
				ticket = strings.Repeat("0", 64)
			}
			if mode == "expired" {
				time.Sleep(550 * time.Millisecond)
			}
			if err := Write(client, Reply{Ticket: ticket}); err != nil {
				t.Fatal(err)
			}
			var binding Reply
			err := Read(client, &binding)
			if mode == "success" {
				if err != nil || binding.Target == nil || binding.Target.HWND != 123 {
					t.Fatalf("binding %v %v", binding, err)
				}
				Write(client, Reply{OK: true, Readback: true})
				if err = <-result; err != nil {
					t.Fatal(err)
				}
				if checks != 1 {
					t.Fatal(checks)
				}
			} else {
				if err == nil || binding.Target != nil {
					t.Fatal("invalid claim disclosed target")
				}
				if <-result == nil {
					t.Fatal("failure accepted")
				}
			}
		})
	}
}

func TestColdPreparationDoesNotExtendActivationCapability(t *testing.T) {
	if PreparationLifetime <= Lifetime || PreparationLifetime > 30*time.Second {
		t.Fatal("cold preparation must be separately bounded")
	}
	var stream bytes.Buffer
	target := Target{HWND: 123, PID: 456, Value: 1, Action: "split_one", Expires: time.Now().Add(Lifetime + time.Second)}
	if _, err := Exchange(context.Background(), &stream, target, func() error { return nil }); err == nil {
		t.Fatal("long preparation accepted an extended activation target")
	}
	if stream.Len() != 0 {
		t.Fatal("invalid target disclosed a ticket")
	}
}
