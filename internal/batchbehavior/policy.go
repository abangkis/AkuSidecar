// Package batchbehavior defines load pacing, not browser identity or authority.
package batchbehavior

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"time"
)

type Policy struct {
	Enabled                                   bool
	Interval, Jitter, BackoffBase, BackoffMax time.Duration
}

type Completion struct {
	ID        string
	At        time.Time
	Code      string
	Retryable bool
}

type Decision struct {
	Allowed   bool
	NotBefore time.Time
	Reason    string
}

// Decide has no clock, I/O, or random state. Completions are newest first.
// Stable per-completion sampling prevents polls or restarts from redrawing delay.
func (p Policy) Decide(now time.Time, active bool, recent []Completion, sample func(string) float64) Decision {
	if !p.Enabled {
		return Decision{Allowed: true}
	}
	if active {
		return Decision{Reason: "capture_active"}
	}
	if len(recent) == 0 {
		return Decision{Allowed: true}
	}
	if sample == nil {
		sample = StableSample
	}
	fraction := sample(recent[0].ID)
	if fraction < 0 || fraction != fraction {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	delay := p.Interval
	reason := "capture_spacing"
	backoff := time.Duration(0)
	for _, completion := range recent {
		if !Temporary(completion.Code, completion.Retryable) {
			break
		}
		if backoff == 0 {
			backoff = p.BackoffBase
		} else if backoff < p.BackoffMax/2 {
			backoff *= 2
		} else {
			backoff = p.BackoffMax
		}
		if backoff > p.BackoffMax {
			backoff = p.BackoffMax
		}
	}
	if backoff > delay {
		delay = backoff
		reason = "temporary_backoff"
	}
	delay += time.Duration(float64(p.Jitter) * fraction)
	until := recent[0].At.Add(delay)
	if now.Before(until) {
		return Decision{NotBefore: until, Reason: reason}
	}
	return Decision{Allowed: true}
}

func StableSample(id string) float64 {
	hash := sha256.Sum256([]byte(id))
	return float64(binary.BigEndian.Uint64(hash[:8])>>11) / float64(uint64(1)<<53)
}

// Only explicit transport/time-budget errors qualify. Login and challenges
// are never converted into a timed recovery or retried by this policy.
func Temporary(code string, retryable bool) bool {
	if !retryable {
		return false
	}
	code = strings.TrimPrefix(strings.TrimPrefix(code, "headless_"), "quiet_")
	switch code {
	case "capture_timeout", "navigation_timeout", "worker_timeout", "transport_unavailable", "capture_lease_expired":
		return true
	}
	return false
}
