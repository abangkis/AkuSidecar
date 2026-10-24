package batchbehavior

import (
	"testing"
	"time"
)

func TestPolicyBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := Policy{Enabled: true, Interval: 5 * time.Second, Jitter: 2 * time.Second, BackoffBase: 10 * time.Second, BackoffMax: 40 * time.Second}
	recent := []Completion{{ID: "one", At: now}}
	for _, test := range []struct {
		name   string
		sample float64
		delay  time.Duration
	}{{"zero", 0, 5 * time.Second}, {"max", 1, 7 * time.Second}, {"negative", -1, 5 * time.Second}, {"clamp", 2, 7 * time.Second}} {
		t.Run(test.name, func(t *testing.T) {
			d := p.Decide(now, false, recent, func(string) float64 { return test.sample })
			if d.Allowed || !d.NotBefore.Equal(now.Add(test.delay)) {
				t.Fatalf("%+v", d)
			}
			if !p.Decide(d.NotBefore, false, recent, func(string) float64 { return test.sample }).Allowed {
				t.Fatal("boundary blocked")
			}
		})
	}
	if p.Decide(now, true, nil, nil).Allowed {
		t.Fatal("overlap admitted")
	}
	if !p.Decide(now, false, nil, nil).Allowed {
		t.Fatal("first capture blocked")
	}
	p.Enabled = false
	if !p.Decide(now, true, recent, nil).Allowed {
		t.Fatal("disabled behavior changed legacy")
	}
}

func TestPolicyBackoffIsBoundedAndNotAuthenticationRecovery(t *testing.T) {
	now := time.Now()
	p := Policy{Enabled: true, Interval: time.Second, BackoffBase: 10 * time.Second, BackoffMax: 40 * time.Second}
	var recent []Completion
	for i := 0; i < 5; i++ {
		recent = append(recent, Completion{ID: "failure", At: now, Code: "quiet_capture_timeout", Retryable: true})
	}
	d := p.Decide(now, false, recent, nil)
	if d.Reason != "temporary_backoff" || !d.NotBefore.Equal(now.Add(40*time.Second)) {
		t.Fatalf("%+v", d)
	}
	for _, code := range []string{"login_required", "headless_login_required", "challenge_required", "quiet_challenge_detected", "unexpected"} {
		if Temporary(code, true) {
			t.Fatalf("%s incorrectly transient", code)
		}
	}
	recent[0].Retryable = false
	d = p.Decide(now, false, recent, nil)
	if d.Reason != "capture_spacing" {
		t.Fatalf("%+v", d)
	}
	if StableSample("same") != StableSample("same") || StableSample("same") < 0 || StableSample("same") >= 1 {
		t.Fatal("unstable jitter")
	}
}
