//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestNativePilotXCaptureDiagnosticClosedAndFirstCause(t *testing.T) {
	first := nativePilotCaptureFail(errors.New("PRIVATE_COOKIE_BODY"), "navigation", "unavailable")
	if d := nativePilotCaptureDetail(nativePilotCaptureFail(first, "cleanup", "close_failed"), "transport", "deadline"); d.stage != "navigation" || d.reason != "unavailable" {
		t.Fatal("secondary failure replaced first capture cause")
	}
	for _, d := range []nativePilotCaptureDiagnostic{{"PRIVATE_URL", "PRIVATE_TOKEN", "PRIVATE_ID"}, {"worker", "empty_unverified", "failed"}} {
		if strings.Contains(fmt.Sprintf("%s %+v %#v", d, d, d), "PRIVATE") {
			t.Fatal("diagnostic leaked private fields")
		}
	}
	for _, code := range []string{"manual_capture_detail_worker__PRIVATE__unknown", "manual_capture_detail_worker__empty_unverified", "manual_capture_detail_worker__empty_unverified__completed__extra", strings.Repeat("x", 200), "manual_capture_detail_worker__empty_unverified__PRIVATE"} {
		if _, ok := nativePilotCaptureRPCDetail(code); ok {
			t.Fatal("invalid capture code accepted")
		}
		if text := nativePilotCaptureTransportError(context.Background(), errors.New(code)).Error(); strings.Contains(text, "PRIVATE") {
			t.Fatal("untrusted transport error leaked")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if d := nativePilotCaptureDetail(nativePilotCaptureTransportError(ctx, errors.New("PRIVATE")), "worker", "unavailable"); d.reason != "canceled" {
		t.Fatal("transport cancellation lost")
	}
	known := "manual_capture_detail_coverage__coverage_extra_pending__failed"
	if d := nativePilotCaptureDetail(nativePilotCaptureTransportError(ctx, errors.New(known)), "transport", "canceled"); d.reason != "coverage_extra_pending" || d.cleanup != "failed" {
		t.Fatal("known first cause lost during cancellation")
	}
	var challenge *headless.CaptureError
	if err := nativePilotCaptureTransportError(ctx, errors.New("manual_capture_challenge_cloudflare")); !errors.As(err, &challenge) || challenge.ChallengeCategory != "cloudflare" {
		t.Fatal("challenge semantics changed")
	}
}

func TestNativePilotXCaptureDiagnosticCatalogMatchesNode(t *testing.T) {
	project, node := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if project == "" || node == "" {
		t.Skip("offline canonical helper required")
	}
	cmd := exec.Command(node, "--input-type=module", "-e", `import {CAPTURE_STAGES,CAPTURE_REASONS,CAPTURE_CLEANUP} from './scripts/auth-session-fixture/x-native-manual-capture.mjs'; process.stdout.write(JSON.stringify([CAPTURE_STAGES,CAPTURE_REASONS,CAPTURE_CLEANUP]));`)
	cmd.Dir = project
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("offline Node diagnostic catalog unavailable")
	}
	var catalogs [][]string
	if json.Unmarshal(raw, &catalogs) != nil || !reflect.DeepEqual(catalogs, [][]string{nativePilotCaptureStages, nativePilotCaptureReasons, nativePilotCaptureCleanup}) {
		t.Fatal("Go/Node capture diagnostic catalogs diverged")
	}
}

func TestNativePilotXCaptureInspectionClassification(t *testing.T) {
	for _, stage := range []string{"owner_before", "owner_after"} {
		for _, tc := range []struct {
			name     string
			err      error
			mismatch bool
			reason   string
		}{
			{"matching", nil, false, ""},
			{"unavailable", errors.New("PRIVATE inspection failure"), true, "unavailable"},
			{"changed", nil, true, "owner_changed"},
			{"canceled", context.Canceled, true, "canceled"},
			{"deadline", context.DeadlineExceeded, true, "deadline"},
		} {
			t.Run(stage+"/"+tc.name, func(t *testing.T) {
				err := nativePilotCaptureInspectError(context.Background(), stage, tc.err, tc.mismatch)
				if tc.reason == "" {
					if err != nil {
						t.Fatal("successful matching inspection rejected")
					}
					return
				}
				d := nativePilotCaptureDetail(err, "unknown", "unknown")
				if d.stage != stage || d.reason != tc.reason {
					t.Fatalf("inspection classification: %s", d)
				}
				if strings.Contains(err.Error(), "PRIVATE") {
					t.Fatal("inspection error text leaked")
				}
			})
		}
	}
}

func TestNativePilotXCaptureDiagnosticSideband(t *testing.T) {
	valid := json.RawMessage(`{"schema":"aku.capture-diagnostic.v1","stage":"worker","reason":"challenge_cloudflare","cleanup":"failed"}`)
	wire := chromeFixtureFailure("manual_capture_challenge_cloudflare", valid)
	err := nativePilotCaptureTransportError(context.Background(), wire)
	var challenge *headless.CaptureError
	if !errors.As(err, &challenge) || challenge.ChallengeCategory != "cloudflare" {
		t.Fatal("operational challenge lost")
	}
	if d := nativePilotCaptureDetail(err, "unknown", "unknown"); d.cleanup != "failed" {
		t.Fatal("challenge cleanup detail lost")
	}
	if next := nativePilotCaptureFail(err, "cleanup", "unavailable"); !errors.As(next, &challenge) || nativePilotCaptureDetail(next, "unknown", "unknown").cleanup != "failed" {
		t.Fatal("wrapping lost independent operational cause/diagnostic")
	}
	// A diagnostic claiming challenge cannot create operational challenge/pause.
	if errors.As(nativePilotCaptureTransportError(context.Background(), chromeFixtureFailure("manual_capture_rejected", valid)), &challenge) {
		t.Fatal("diagnostic created operational challenge")
	}
	for _, raw := range []string{
		`{"schema":"aku.capture-diagnostic.v2","stage":"worker","reason":"challenge_cloudflare","cleanup":"failed"}`,
		`{"schema":"aku.capture-diagnostic.v1","stage":"worker","reason":"challenge_cloudflare","cleanup":"failed","cleanup":"completed"}`,
		`{"schema":"aku.capture-diagnostic.v1","stage":"worker","reason":"challenge_x_human_verification","cleanup":"failed"}`,
		`{"schema":"aku.capture-diagnostic.v1","stage":"PRIVATE","reason":"challenge_cloudflare","cleanup":"failed"}`,
		`{"schema":"aku.capture-diagnostic.v1","stage":"worker","reason":"challenge_cloudflare","cleanup":null}`,
		`{"schema":"aku.capture-diagnostic.v1","stage":"worker","reason":"challenge_cloudflare","cleanup":"failed","token":"PRIVATE"}`,
		strings.Repeat("x", 513),
	} {
		cause := nativePilotCaptureTransportError(context.Background(), chromeFixtureFailure("manual_capture_challenge_cloudflare", json.RawMessage(raw)))
		if !errors.As(cause, &challenge) || challenge.ChallengeCategory != "cloudflare" {
			t.Fatal("bad metadata changed operational handling")
		}
		if d := nativePilotCaptureDetail(cause, "unknown", "unknown"); d.cleanup != "unknown" {
			t.Fatal("bad metadata was promoted")
		}
		if strings.Contains(fmt.Sprintf("%v %#v", cause, wire), "PRIVATE") {
			t.Fatal("opaque metadata leaked")
		}
	}
}

func TestNativePilotXCaptureWorkerDiagnosticValidation(t *testing.T) {
	worker := `{"phase":"no_posts","hydrationMS":3000,"elapsedMS":3010,"sampleCount":1,"samples":[{"candidateCount":2,"rejected":2,"documentReady":true,"authenticatedUiObserved":true,"loginRequired":false,"challengeDetected":false,"rejections":{"missing_identity":1,"ambiguous_identity":0,"missing_author":1,"empty_evidence":0}}]}`
	wrap := func(value string) error {
		raw := json.RawMessage(`{"schema":"aku.capture-diagnostic.v1","stage":"worker","reason":"empty_unverified","cleanup":"failed","worker":` + value + `}`)
		return nativePilotCaptureTransportError(context.Background(), chromeFixtureFailure("manual_capture_rejected", raw))
	}
	err := wrap(worker)
	projected := nativePilotCaptureWorkerDetail(err)
	v := decodeNativePilotCaptureWorker([]byte(projected))
	if v == nil || v.Phase != "no_posts" || *v.Samples[0].Rejections.MissingAuthor != 1 || nativePilotCaptureDetail(err, "unknown", "unknown").cleanup != "failed" {
		t.Fatal("valid bounded structural evidence lost")
	}
	if nativePilotCaptureWorkerDetail(nativePilotCaptureFail(err, "cleanup", "unavailable")) != projected {
		t.Fatal("cleanup replaced first worker telemetry")
	}
	for _, raw := range []string{
		strings.Replace(worker, `"phase":"no_posts"`, `"phase":"PRIVATE"`, 1),
		strings.Replace(worker, `"hydrationMS":3000`, `"hydrationMS":null`, 1),
		strings.Replace(worker, `"elapsedMS":3010`, `"elapsedMS":-1`, 1),
		strings.Replace(worker, `"elapsedMS":3010`, `"elapsedMS":3010,"elapsedMS":1`, 1),
		strings.Replace(worker, `"candidateCount":2`, `"candidateCount":"PRIVATE_TOKEN"`, 1),
		strings.Replace(worker, `"missing_identity":1`, `"missing_identity":1,"PRIVATE_TOKEN":1`, 1),
		strings.Replace(worker, `"sampleCount":1`, `"sampleCount":0`, 1),
		`null`,
	} {
		cause := wrap(raw)
		if nativePilotCaptureWorkerDetail(cause) != "" || nativePilotCaptureDetail(cause, "unknown", "unknown").reason != "empty_unverified" || strings.Contains(fmt.Sprint(cause), "PRIVATE") {
			t.Fatal("invalid optional telemetry leaked or replaced operational cause")
		}
	}
}
