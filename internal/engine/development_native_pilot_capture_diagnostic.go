//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Diagnostic-only closed catalogs mirror the private Node capture transport.
// No exception text, page content, paths, URLs, request IDs or owner pins enter
// these fields. They never grant admission, refresh proof or select a retry.
var nativePilotCaptureStages = []string{"unknown", "admission", "behavior", "coordinator", "owner_before", "payload", "transport", "assets", "target_create", "target_attach", "observer", "navigation", "evaluation", "worker", "coverage", "serialization", "wire", "observation", "owner_after", "cleanup"}
var nativePilotCaptureReasons = []string{"unknown", "unavailable", "invalid_request", "deadline", "canceled", "owner_changed", "scope_lost", "request_budget", "header_guard", "coverage_pending", "coverage_shape", "extra_mismatch", "network_failed", "target_lost", "method_rejected", "evaluation_exception", "invalid_payload", "navigation_failed", "empty_unverified", "invalid_observation", "worker_not_ready", "target_unavailable", "worker_other", "envelope_invalid", "digest_mismatch", "observation_invalid", "close_failed", "source_paused", "challenge_cloudflare", "challenge_x_human_verification", "challenge_unknown", "replay", "invalidated", "coverage_response_pending", "coverage_completion_pending", "coverage_extra_pending", "cookie_candidate", "cookie_ambiguous", "login_required", "source_unavailable"}
var nativePilotCaptureCleanup = []string{"unknown", "completed", "failed"}

type nativePilotCaptureDiagnostic struct{ stage, reason, cleanup string }

func nativePilotCaptureMember(value string, catalog []string) string {
	for _, allowed := range catalog {
		if value == allowed {
			return value
		}
	}
	return "unknown"
}
func (d nativePilotCaptureDiagnostic) closed() nativePilotCaptureDiagnostic {
	return nativePilotCaptureDiagnostic{nativePilotCaptureMember(d.stage, nativePilotCaptureStages), nativePilotCaptureMember(d.reason, nativePilotCaptureReasons), nativePilotCaptureMember(d.cleanup, nativePilotCaptureCleanup)}
}
func (d nativePilotCaptureDiagnostic) String() string {
	d = d.closed()
	return fmt.Sprintf("stage=%s reason=%s cleanup=%s", d.stage, d.reason, d.cleanup)
}
func (d nativePilotCaptureDiagnostic) GoString() string { return d.String() }

type nativePilotCaptureError struct {
	detail nativePilotCaptureDiagnostic
	// A domain-owned operational error remains independently discoverable.
	// Diagnostic metadata alone must never manufacture this cause.
	cause error
}

func (e *nativePilotCaptureError) Error() string {
	return "native X capture rejected (" + e.detail.String() + ")"
}
func (e *nativePilotCaptureError) Unwrap() error { return e.cause }

func nativePilotCaptureDetail(err error, stage, reason string) nativePilotCaptureDiagnostic {
	var known *nativePilotCaptureError
	if errors.As(err, &known) {
		return known.detail.closed()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "deadline"
	} else if errors.Is(err, context.Canceled) {
		reason = "canceled"
	}
	var challenge *headless.CaptureError
	if errors.As(err, &challenge) && challenge.Code == "challenge_required" {
		stage, reason = "worker", "challenge_unknown"
		switch challenge.ChallengeCategory {
		case "cloudflare":
			reason = "challenge_cloudflare"
		case "x_human_verification":
			reason = "challenge_x_human_verification"
		}
	}
	return (nativePilotCaptureDiagnostic{stage, reason, "unknown"}).closed()
}
func nativePilotCaptureFail(err error, stage, reason string) error {
	var known *nativePilotCaptureError
	if errors.As(err, &known) {
		return known
	}
	return &nativePilotCaptureError{detail: nativePilotCaptureDetail(err, stage, reason)}
}

// Missing inspection evidence is not proof of a mismatch. The operational
// rejection remains the same; only a successful comparison can say changed.
func nativePilotCaptureInspectError(ctx context.Context, stage string, inspectionErr error, mismatch bool) error {
	if inspectionErr != nil {
		if ctx != nil && ctx.Err() != nil {
			inspectionErr = ctx.Err()
		}
		return nativePilotCaptureFail(inspectionErr, stage, "unavailable")
	}
	if mismatch {
		return nativePilotCaptureFail(nil, stage, "owner_changed")
	}
	return nil
}

// A versioned, bounded observational envelope. Invalid/unknown metadata is
// ignored without changing the independently decoded operational failure.
func nativePilotCaptureJSONDetail(raw []byte) (nativePilotCaptureDiagnostic, bool) {
	var empty nativePilotCaptureDiagnostic
	if len(raw) == 0 || len(raw) > 512 || !nativePilotStartupUnicode(raw) {
		return empty, false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return empty, false
	}
	fields := map[string]string{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return empty, false
		}
		if _, exists := fields[key]; exists {
			return empty, false
		}
		if key != "schema" && key != "stage" && key != "reason" && key != "cleanup" {
			return empty, false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || len(value) == 0 || value[0] != '"' {
			return empty, false
		}
		var text string
		if json.Unmarshal(value, &text) != nil {
			return empty, false
		}
		fields[key] = text
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return empty, false
	}
	if _, err := d.Token(); err != io.EOF || len(fields) != 4 || fields["schema"] != "aku.capture-diagnostic.v1" {
		return empty, false
	}
	detail := nativePilotCaptureDiagnostic{fields["stage"], fields["reason"], fields["cleanup"]}
	return detail, detail == detail.closed()
}

// Only a complete, bounded code from the private compiled transport is decoded.
// A patterned or arbitrary error never becomes report text or authority.
func nativePilotCaptureRPCDetail(code string) (nativePilotCaptureDiagnostic, bool) {
	if len(code) > 160 {
		return nativePilotCaptureDiagnostic{}, false
	}
	const prefix = "manual_capture_detail_"
	if !strings.HasPrefix(code, prefix) {
		return nativePilotCaptureDiagnostic{}, false
	}
	p := strings.Split(strings.TrimPrefix(code, prefix), "__")
	if len(p) != 3 {
		return nativePilotCaptureDiagnostic{}, false
	}
	d := nativePilotCaptureDiagnostic{p[0], p[1], p[2]}
	return d, d == d.closed()
}

func nativePilotCaptureTransportError(ctx context.Context, err error) error {
	var known *nativePilotCaptureError
	if errors.As(err, &known) {
		return known
	}
	var sideband nativePilotCaptureDiagnostic
	var carrier interface{ diagnosticJSON() []byte }
	valid := false
	if errors.As(err, &carrier) {
		sideband, valid = nativePilotCaptureJSONDetail(carrier.diagnosticJSON())
	}
	if err != nil {
		category, challenge := "", true
		switch err.Error() {
		case "manual_capture_challenge_cloudflare":
			category = "cloudflare"
		case "manual_capture_challenge_x_human_verification":
			category = "x_human_verification"
		case "manual_capture_challenge_unknown":
		default:
			challenge = false
		}
		if challenge {
			cause := &headless.CaptureError{Code: "challenge_required", Message: domain.XChallengeMessage, ChallengeCategory: category}
			detail := nativePilotCaptureDetail(cause, "worker", "challenge_unknown")
			if valid && sideband.stage == detail.stage && sideband.reason == detail.reason {
				detail = sideband
			}
			return &nativePilotCaptureError{detail: detail, cause: cause}
		}
		if d, ok := nativePilotCaptureRPCDetail(err.Error()); ok {
			return &nativePilotCaptureError{detail: d}
		}
		if err.Error() == "manual_capture_rejected" && valid && !strings.HasPrefix(sideband.reason, "challenge_") {
			return &nativePilotCaptureError{detail: sideband}
		}
	}
	var canceled error
	if ctx != nil {
		canceled = ctx.Err()
	}
	return nativePilotCaptureFail(canceled, "transport", "unavailable")
}
