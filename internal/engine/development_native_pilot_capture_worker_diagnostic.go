//go:build auth_session_dev

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Optional structural telemetry. Unknown values stay null and can never grant
// login, account, ownership, admission, retry or persistence authority.
type nativePilotCaptureWorkerSample struct {
	CandidateCount          *int  `json:"candidateCount"`
	Rejected                *int  `json:"rejected"`
	DocumentReady           *bool `json:"documentReady"`
	AuthenticatedUIObserved *bool `json:"authenticatedUiObserved"`
	LoginRequired           *bool `json:"loginRequired"`
	ChallengeDetected       *bool `json:"challengeDetected"`
	Rejections              struct {
		MissingIdentity   *int `json:"missing_identity"`
		AmbiguousIdentity *int `json:"ambiguous_identity"`
		MissingAuthor     *int `json:"missing_author"`
		EmptyEvidence     *int `json:"empty_evidence"`
	} `json:"rejections"`
}
type nativePilotCaptureWorkerDiagnostic struct {
	Phase       string                           `json:"phase"`
	HydrationMS int                              `json:"hydrationMS"`
	ElapsedMS   int                              `json:"elapsedMS"`
	SampleCount int                              `json:"sampleCount"`
	Samples     []nativePilotCaptureWorkerSample `json:"samples"`
}

func decodeNativePilotCaptureWorker(raw []byte) *nativePilotCaptureWorkerDiagnostic {
	if len(raw) == 0 || len(raw) > 1536 || !xNativeCaptureJSON(raw) {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var v nativePilotCaptureWorkerDiagnostic
	if d.Decode(&v) != nil || v.HydrationMS < 1000 || v.HydrationMS > 45000 || v.ElapsedMS < 0 || v.ElapsedMS > 60000 || v.SampleCount < 0 || v.SampleCount > 1000000 || v.Samples == nil || len(v.Samples) > 2 || len(v.Samples) > v.SampleCount {
		return nil
	}
	if v.Phase != "snapshot_unavailable" && v.Phase != "no_posts" && v.Phase != "observation_invalid" {
		return nil
	}
	if v.Phase == "snapshot_unavailable" && v.SampleCount != 0 || v.Phase == "no_posts" && v.SampleCount == 0 {
		return nil
	}
	// Require the timing/count fields so missing telemetry cannot masquerade as zero.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 5 {
		return nil
	}
	for _, key := range []string{"hydrationMS", "elapsedMS", "sampleCount"} {
		if len(fields[key]) == 0 || bytes.Equal(fields[key], []byte("null")) {
			return nil
		}
	}
	for _, s := range v.Samples {
		for _, n := range []*int{s.CandidateCount, s.Rejected, s.Rejections.MissingIdentity, s.Rejections.AmbiguousIdentity, s.Rejections.MissingAuthor, s.Rejections.EmptyEvidence} {
			if n != nil && (*n < 0 || *n > 1000000) {
				return nil
			}
		}
	}
	return &v
}

// Retain canonical projected JSON rather than references into a mutable report.
func nativePilotCaptureWorkerJSON(raw []byte, detail nativePilotCaptureDiagnostic) string {
	if detail.stage != "worker" || detail.reason != "empty_unverified" && detail.reason != "invalid_observation" {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	v := decodeNativePilotCaptureWorker(fields["worker"])
	if v == nil || (detail.reason == "invalid_observation") != (v.Phase == "observation_invalid") {
		return ""
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(encoded)
}
func nativePilotCaptureWorkerDetail(err error) string {
	var known *nativePilotCaptureError
	if errors.As(err, &known) {
		return known.worker
	}
	return ""
}
func cloneNativePilotCaptureWorker(v *nativePilotCaptureWorkerDiagnostic) *nativePilotCaptureWorkerDiagnostic {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return decodeNativePilotCaptureWorker(raw)
}
