package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// NativeReader owns a normal Chrome process. It has no capture host or Bridge.
// Its only automatically closable page is the untouched initial local placeholder.
type NativeReader struct {
	*Window
	mu          sync.Mutex
	protocol    CaptureProtocol
	containment CaptureContainment
	idleURL     string
	idleTarget  string
}

func NewNativeReader(ctx context.Context, window *Window, idleURL string, logger *log.Logger) (*NativeReader, error) {
	r := &NativeReader{Window: window, protocol: window.CaptureProtocol(), idleURL: idleURL}
	if r.protocol == nil {
		return r, errors.New("native reader private protocol unavailable")
	}
	if err := window.SetCaptureHandoff(r.closeIdle); err != nil {
		return r, err
	}
	containment, err := window.StartCaptureContainment(logger)
	if err != nil {
		return r, err
	}
	r.containment = containment
	// Chrome may not have registered its initial page when the pipe first opens.
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		targets, err := r.targets(ctx)
		if err != nil {
			return r, err
		}
		for _, target := range targets {
			if target.Type == "page" && target.URL == idleURL {
				if r.idleTarget != "" {
					return r, errors.New("native reader placeholder is ambiguous")
				}
				r.idleTarget = target.ID
			}
		}
		if r.idleTarget != "" {
			return r, nil
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-tick.C:
		}
	}
}

type nativeReaderTarget struct {
	ID   string `json:"targetId"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

func (r *NativeReader) targets(ctx context.Context) ([]nativeReaderTarget, error) {
	raw, err := r.protocol.Call(ctx, "Target.getTargets", nil, "")
	if err != nil {
		return nil, err
	}
	var result struct {
		Targets []nativeReaderTarget `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result.Targets, nil
}

func (r *NativeReader) closeIdle(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.idleTarget == "" {
		return nil
	}
	targets, err := r.targets(ctx)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target.ID != r.idleTarget {
			continue
		}
		if target.URL != r.idleURL {
			return errors.New("native reader placeholder was changed; close the window manually")
		}
		raw, err := r.protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": target.ID}, "")
		var reply struct {
			Success bool `json:"success"`
		}
		if err != nil {
			return err
		}
		if json.Unmarshal(raw, &reply) != nil || !reply.Success {
			return errors.New("native reader placeholder close unverified")
		}
		r.idleTarget = ""
		return nil
	}
	r.idleTarget = ""
	return nil
}

// PrepareNativePost is called only after the exact trusted UI broker request
// attaches. Bind a local marker before navigating the same owned page to the URL.
func (r *NativeReader) PrepareNativePost(ctx context.Context, actionID, url, markerURL string) (readerbroker.Target, func(context.Context) error, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.idleTarget
	if id != "" {
		targets, err := r.targets(ctx)
		if err != nil {
			return readerbroker.Target{}, nil, err
		}
		found := false
		for _, target := range targets {
			if target.ID == id && target.URL == r.idleURL {
				found = true
			}
		}
		if !found {
			return readerbroker.Target{}, nil, errors.New("native reader placeholder is no longer owned at its original URL")
		}
		// From this point the page belongs to the user, even if preparation fails.
		r.idleTarget = ""
	} else {
		raw, err := r.protocol.Call(ctx, "Target.createTarget", map[string]any{"url": markerURL, "newWindow": true}, "")
		if err != nil {
			return readerbroker.Target{}, nil, err
		}
		var created struct {
			ID string `json:"targetId"`
		}
		if json.Unmarshal(raw, &created) != nil || created.ID == "" {
			return readerbroker.Target{}, nil, errors.New("native reader page creation unverified")
		}
		id = created.ID
	}
	raw, err := r.protocol.Call(ctx, "Target.attachToTarget", map[string]any{"targetId": id, "flatten": true}, "")
	if err != nil {
		return readerbroker.Target{}, nil, err
	}
	var attached struct {
		ID string `json:"sessionId"`
	}
	if json.Unmarshal(raw, &attached) != nil || attached.ID == "" {
		return readerbroker.Target{}, nil, errors.New("native reader session unavailable")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_, _ = r.protocol.Call(cleanup, "Target.detachFromTarget", map[string]string{"sessionId": attached.ID}, "")
	}()
	if err := r.navigate(ctx, attached.ID, markerURL); err != nil {
		return readerbroker.Target{}, nil, err
	}
	target, verify, err := r.containment.PrepareBrokerReader(ctx, "AkuBrowser reader "+actionID)
	if err != nil {
		return readerbroker.Target{}, nil, err
	}
	if err := r.navigate(ctx, attached.ID, url); err != nil {
		return readerbroker.Target{}, nil, err
	}
	return target, verify, nil
}

func (r *NativeReader) navigate(ctx context.Context, session, url string) error {
	raw, err := r.protocol.Call(ctx, "Page.navigate", map[string]string{"url": url}, session)
	if err != nil {
		return err
	}
	var result struct {
		Error string `json:"errorText"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Error != "" {
		return errors.New("native reader navigation rejected")
	}
	return nil
}
