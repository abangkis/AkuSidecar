package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// NavigateCaptureHost starts only the uniquely identified, caller-owned static
// host after server bindings exist. It retains the root pipe and detaches only
// the temporary page session; command acceptance is not Bridge readiness.
func NavigateCaptureHost(ctx context.Context, protocol CaptureProtocol, launchURL string) error {
	if protocol == nil || launchURL == "" {
		return errors.New("capture host protocol unavailable")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	targetID := ""
	for targetID == "" {
		raw, err := protocol.Call(lookupCtx, "Target.getTargets", nil, "")
		if err != nil {
			return errors.New("capture host target lookup failed")
		}
		var result struct {
			TargetInfos []struct {
				ID   string `json:"targetId"`
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"targetInfos"`
		}
		if json.Unmarshal(raw, &result) != nil {
			return errors.New("capture host target response invalid")
		}
		for _, target := range result.TargetInfos {
			if target.Type != "page" || target.URL != launchURL {
				continue
			}
			if targetID != "" {
				return errors.New("capture host target identity ambiguous")
			}
			targetID = target.ID
		}
		if targetID == "" {
			select {
			case <-lookupCtx.Done():
				return errors.New("capture host target unavailable")
			case <-ticker.C:
			}
		}
	}
	raw, err := protocol.Call(ctx, "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if err != nil {
		return errors.New("capture host session attach failed")
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(raw, &attached) != nil || attached.SessionID == "" {
		return errors.New("capture host session response invalid")
	}
	defer func() {
		detachCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = protocol.Call(detachCtx, "Target.detachFromTarget", map[string]string{"sessionId": attached.SessionID}, "")
	}()
	raw, err = protocol.Call(ctx, "Page.navigate", map[string]string{"url": launchURL}, attached.SessionID)
	if err != nil {
		return errors.New("capture host navigation command failed")
	}
	var navigation struct {
		ErrorText string `json:"errorText"`
	}
	if json.Unmarshal(raw, &navigation) != nil || navigation.ErrorText != "" {
		return errors.New("capture host navigation rejected")
	}
	return nil
}
