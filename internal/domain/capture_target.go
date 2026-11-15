package domain

import (
	"encoding/json"
	"errors"
	"regexp"
)

// CaptureTarget freezes the primary post scope without changing extraction policy.
type CaptureTarget struct {
	Source    Source `json:"source"`
	Permalink string `json:"permalink"`
}

var captureTargetXPermalink = regexp.MustCompile(`^https://x\.com/[A-Za-z0-9_]{1,15}/status/[0-9]{5,30}$`)

func (target CaptureTarget) Validate() error {
	canonical, ok := CanonicalSourceURL(target.Source, target.Permalink)
	if target.Source != SourceX || !ok || canonical != target.Permalink || !captureTargetXPermalink.MatchString(target.Permalink) {
		return errors.New("captureTarget requires a canonical HTTPS X post permalink")
	}
	return nil
}

// SessionCaptureTarget reads the durable scope; malformed scope fails closed.
func SessionCaptureTarget(session Session) (*CaptureTarget, error) {
	raw, exists := session.Coverage["captureTarget"]
	if !exists {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var target CaptureTarget
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, err
	}
	if err := target.Validate(); err != nil {
		return nil, err
	}
	return &target, nil
}
