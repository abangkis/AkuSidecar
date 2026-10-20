//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

const (
	xQASynchronizedConcurrencySchema = "aku.x.qa-synchronized-concurrency.v1"
	xQACheckpointRendezvousSchema    = "aku.x.qa-checkpoint-rendezvous.v1"
	xQASynchronizedReportMaxBytes    = 64 * 1024
)

type xQACheckpointRendezvousReport struct {
	Schema       string `json:"schema"`
	State        string `json:"state"`
	Arrivals     int    `json:"arrivals"`
	WaitLimitMS  int    `json:"waitLimitMS"`
	CanAuthorize *bool  `json:"canAuthorize"`
}

type xQASynchronizedConcurrencyReport struct {
	Schema            string                        `json:"schema"`
	Experiment        xQAShortConcurrencyReport     `json:"experiment"`
	Rendezvous        xQACheckpointRendezvousReport `json:"rendezvous"`
	ReaderWindow      string                        `json:"readerWindow"`
	AccountComparison string                        `json:"accountComparison"`
	CanAuthorize      *bool                         `json:"canAuthorize"`
}

type xQASynchronizedConcurrencyInput struct {
	Schema            string          `json:"schema"`
	Experiment        json.RawMessage `json:"experiment"`
	Rendezvous        json.RawMessage `json:"rendezvous"`
	ReaderWindow      string          `json:"readerWindow"`
	AccountComparison string          `json:"accountComparison"`
	CanAuthorize      *bool           `json:"canAuthorize"`
}

// decodeXQASynchronizedConcurrency validates the wrapper and delegates its V1 experiment unchanged.
func decodeXQASynchronizedConcurrency(raw []byte) (xQASynchronizedConcurrencyReport, error) {
	var report xQASynchronizedConcurrencyReport
	var input xQASynchronizedConcurrencyInput
	bad := errors.New("QA synchronized concurrency report rejected")
	fields, ok := xQAShortDecodeExact(raw, xQASynchronizedReportMaxBytes,
		[]string{"schema", "experiment", "rendezvous", "readerWindow", "accountComparison", "canAuthorize"}, &input)
	if !ok || input.Schema != xQASynchronizedConcurrencySchema || !xQAShortJSONBool(fields["canAuthorize"]) ||
		!xQAFalse(input.CanAuthorize) || (input.ReaderWindow != "preserved" && input.ReaderWindow != "changed" && input.ReaderWindow != "not_checked") ||
		(input.AccountComparison != "match" && input.AccountComparison != "unknown") {
		return report, bad
	}

	experiment, err := decodeXQAShortConcurrency(input.Experiment)
	if err != nil {
		return report, bad
	}

	var rendezvous xQACheckpointRendezvousReport
	rendezvousFields, ok := xQAShortDecodeExact(input.Rendezvous, xQASynchronizedReportMaxBytes,
		[]string{"schema", "state", "arrivals", "waitLimitMS", "canAuthorize"}, &rendezvous)
	if !ok || rendezvous.Schema != xQACheckpointRendezvousSchema || !xQAShortJSONBool(rendezvousFields["canAuthorize"]) ||
		!xQAFalse(rendezvous.CanAuthorize) {
		return report, bad
	}
	arrivals, arrivalsOK := xQAShortJSONInteger(rendezvousFields["arrivals"])
	waitLimitMS, waitLimitOK := xQAShortJSONInteger(rendezvousFields["waitLimitMS"])
	if !arrivalsOK || arrivals != rendezvous.Arrivals || arrivals < 0 || arrivals > 2 ||
		!waitLimitOK || waitLimitMS != rendezvous.WaitLimitMS || waitLimitMS != 1000 {
		return report, bad
	}
	switch rendezvous.State {
	case "not_started":
		if arrivals != 0 {
			return report, bad
		}
	case "released":
		if arrivals != 2 {
			return report, bad
		}
	case "timed_out":
		if arrivals != 1 {
			return report, bad
		}
	case "aborted":
		if arrivals > 1 {
			return report, bad
		}
	default: // A final report cannot leave an active wait or an unknown state.
		return report, bad
	}

	wantComparison := "unknown"
	if experiment.CompletedCheckpoints == 4 {
		wantComparison = "match"
	}
	if input.AccountComparison != wantComparison {
		return report, bad
	}
	if experiment.CompletedCheckpoints >= 3 && rendezvous.State != "released" {
		return report, bad
	}
	switch experiment.Outcome {
	case "overlap_not_observed", "short_overlap_observed":
		if input.ReaderWindow != "preserved" {
			return report, bad
		}
	case "reader_window_changed":
		if input.ReaderWindow != "changed" {
			return report, bad
		}
	default:
		if input.ReaderWindow == "changed" || input.ReaderWindow == "preserved" && experiment.CompletedCheckpoints != 4 {
			return report, bad
		}
	}
	if experiment.Outcome == "short_overlap_observed" && rendezvous.State != "released" {
		return report, bad
	}

	report = xQASynchronizedConcurrencyReport{Schema: input.Schema, Experiment: experiment, Rendezvous: rendezvous,
		ReaderWindow: input.ReaderWindow, AccountComparison: input.AccountComparison, CanAuthorize: input.CanAuthorize}
	return report, nil
}

func xQASynchronizedConcurrencyFixtureJSON(experiment xQAShortConcurrencyReport, state string, arrivals int,
	readerWindow, accountComparison string) []byte {
	experimentJSON, _ := json.Marshal(experiment)
	canAuthorize := false
	rendezvousJSON, _ := json.Marshal(xQACheckpointRendezvousReport{Schema: xQACheckpointRendezvousSchema,
		State: state, Arrivals: arrivals, WaitLimitMS: 1000, CanAuthorize: &canAuthorize})
	encoded, _ := json.Marshal(xQASynchronizedConcurrencyInput{Schema: xQASynchronizedConcurrencySchema,
		Experiment: experimentJSON, Rendezvous: rendezvousJSON, ReaderWindow: readerWindow,
		AccountComparison: accountComparison, CanAuthorize: &canAuthorize})
	return encoded
}

func xQASynchronizedReplaceLast(raw, old, replacement []byte) []byte {
	index := bytes.LastIndex(raw, old)
	if index < 0 {
		return append([]byte(nil), raw...)
	}
	result := make([]byte, 0, len(raw)-len(old)+len(replacement))
	result = append(result, raw[:index]...)
	result = append(result, replacement...)
	result = append(result, raw[index+len(old):]...)
	return result
}

func TestXQASynchronizedConcurrencyOffline(t *testing.T) {
	success := xQAShortConcurrencyFixture()
	successRaw := xQASynchronizedConcurrencyFixtureJSON(success, "released", 2, "preserved", "match")
	if decoded, err := decodeXQASynchronizedConcurrency(successRaw); err != nil || decoded.Rendezvous.State != "released" ||
		decoded.Experiment.Outcome != "short_overlap_observed" {
		t.Fatalf("valid synchronized overlap report rejected: %v", err)
	}

	zeroOverlap := xQAShortConcurrencyFixture()
	zeroOverlap.Outcome = "overlap_not_observed"
	zeroOverlap.OverlapMS = 0
	zeroOverlap.AccountMatch = "unknown"
	zeroRaw := xQASynchronizedConcurrencyFixtureJSON(zeroOverlap, "released", 2, "preserved", "match")
	if decoded, err := decodeXQASynchronizedConcurrency(zeroRaw); err != nil || decoded.AccountComparison != "match" ||
		decoded.Experiment.AccountMatch != "unknown" {
		t.Fatalf("valid four-checkpoint zero-overlap report rejected or changed V1 account match: %v", err)
	}

	point := xQAShortConcurrencyFixture().Checkpoints
	timedOutExperiment := xQAShortConcurrencyReport{Schema: xQAShortConcurrencySchema, Outcome: "deadline", CollectorCreated: true,
		SeedAttempted: true, SessionTransferred: func() *bool { value := true; return &value }(),
		Checkpoints: point[:2], CompletedCheckpoints: 2, OverlapMS: 0, DurationMS: 1800, CleanupVerified: true,
		AccountMatch: "unknown", Continuity: "unknown", MutationGate: "deferred_for_short_qa", Refresh: "not_qualified",
		CanAuthorize: func() *bool { value := false; return &value }(), NativeQualified: false}
	timedOutRaw := xQASynchronizedConcurrencyFixtureJSON(timedOutExperiment, "timed_out", 1, "not_checked", "unknown")
	if decoded, err := decodeXQASynchronizedConcurrency(timedOutRaw); err != nil || decoded.Rendezvous.Arrivals != 1 {
		t.Fatalf("valid one-arrival timeout report rejected: %v", err)
	}
	partialEarly := xQAShortConcurrencyReport{Schema: xQAShortConcurrencySchema, Outcome: "reader_checkpoint_rejected", CollectorCreated: true,
		SeedAttempted: false, SessionTransferred: func() *bool { value := false; return &value }(), Checkpoints: point[:1],
		CompletedCheckpoints: 0, OverlapMS: 0, DurationMS: 250, CleanupVerified: true, AccountMatch: "unknown",
		Continuity: "unknown", MutationGate: "deferred_for_short_qa", Refresh: "not_qualified",
		CanAuthorize: func() *bool { value := false; return &value }(), NativeQualified: false}
	partialEarlyRaw := xQASynchronizedConcurrencyFixtureJSON(partialEarly, "not_started", 0, "not_checked", "unknown")
	if decoded, err := decodeXQASynchronizedConcurrency(partialEarlyRaw); err != nil || decoded.Rendezvous.State != "not_started" {
		t.Fatalf("valid pre-rendezvous partial report rejected: %v", err)
	}
	abortedRaw := xQASynchronizedConcurrencyFixtureJSON(timedOutExperiment, "aborted", 1, "not_checked", "unknown")
	if decoded, err := decodeXQASynchronizedConcurrency(abortedRaw); err != nil || decoded.Rendezvous.State != "aborted" {
		t.Fatalf("valid canceled one-arrival partial report rejected: %v", err)
	}

	readerChanged := xQAShortConcurrencyFixture()
	readerChanged.Outcome = "reader_window_changed"
	readerChanged.AccountMatch = "unknown"
	readerChangedRaw := xQASynchronizedConcurrencyFixtureJSON(readerChanged, "released", 2, "changed", "match")
	if _, err := decodeXQASynchronizedConcurrency(readerChangedRaw); err != nil {
		t.Fatalf("valid changed-reader-window report rejected: %v", err)
	}

	for label, invalid := range map[string][]byte{
		"duplicate wrapper key": bytes.Replace(successRaw, []byte(`"schema":"aku.x.qa-synchronized-concurrency.v1"`),
			[]byte(`"schema":"aku.x.qa-synchronized-concurrency.v1","schema":"forged"`), 1),
		"duplicate rendezvous key": bytes.Replace(successRaw, []byte(`"state":"released"`), []byte(`"state":"released","state":"aborted"`), 1),
		"duplicate experiment key": bytes.Replace(successRaw, []byte(`"outcome":"short_overlap_observed"`),
			[]byte(`"outcome":"short_overlap_observed","outcome":"deadline"`), 1),
		"authority promotion":                  xQASynchronizedReplaceLast(successRaw, []byte(`"canAuthorize":false}`), []byte(`"canAuthorize":true}`)),
		"active wait in final report":          bytes.Replace(successRaw, []byte(`"state":"released"`), []byte(`"state":"waiting"`), 1),
		"released with one arrival":            bytes.Replace(successRaw, []byte(`"arrivals":2`), []byte(`"arrivals":1`), 1),
		"timeout with second arrival":          bytes.Replace(timedOutRaw, []byte(`"arrivals":1`), []byte(`"arrivals":2`), 1),
		"aborted with second arrival":          bytes.Replace(successRaw, []byte(`"state":"released"`), []byte(`"state":"aborted"`), 1),
		"reader window mismatch":               bytes.Replace(successRaw, []byte(`"readerWindow":"preserved"`), []byte(`"readerWindow":"not_checked"`), 1),
		"reader changed under another outcome": bytes.Replace(successRaw, []byte(`"readerWindow":"preserved"`), []byte(`"readerWindow":"changed"`), 1),
		"forged account comparison":            bytes.Replace(successRaw, []byte(`"accountComparison":"match"`), []byte(`"accountComparison":"unknown"`), 1),
		"success without release":              bytes.Replace(successRaw, []byte(`"state":"released","arrivals":2`), []byte(`"state":"aborted","arrivals":1`), 1),
		"match with incomplete experiment":     bytes.Replace(timedOutRaw, []byte(`"accountComparison":"unknown"`), []byte(`"accountComparison":"match"`), 1),
		"reflected account id":                 bytes.Replace(successRaw, []byte(`"waitLimitMS":1000`), []byte(`"waitLimitMS":1000,"accountId":"123"`), 1),
	} {
		if _, err := decodeXQASynchronizedConcurrency(invalid); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
	unknownWrapperField := append(bytes.TrimSuffix(successRaw, []byte(`}`)), []byte(`,"cookieValue":"PRIVATE"}`)...)
	if _, err := decodeXQASynchronizedConcurrency(unknownWrapperField); err == nil {
		t.Fatal("unknown root privacy field accepted")
	}

	for label, invalid := range map[string][]byte{
		"not started with arrival":        bytes.Replace(zeroRaw, []byte(`"state":"released"`), []byte(`"state":"not_started"`), 1),
		"timeout with no arrival":         bytes.Replace(timedOutRaw, []byte(`"arrivals":1`), []byte(`"arrivals":0`), 1),
		"four checkpoints before arrival": bytes.Replace(zeroRaw, []byte(`"state":"released","arrivals":2`), []byte(`"state":"not_started","arrivals":0`), 1),
		"four checkpoints at timeout":     bytes.Replace(zeroRaw, []byte(`"state":"released","arrivals":2`), []byte(`"state":"timed_out","arrivals":1`), 1),
		"invalid wait limit":              bytes.Replace(successRaw, []byte(`"waitLimitMS":1000`), []byte(`"waitLimitMS":1001`), 1),
		"null arrival count":              bytes.Replace(successRaw, []byte(`"arrivals":2`), []byte(`"arrivals":null`), 1),
		"nonboolean wrapper authority":    xQASynchronizedReplaceLast(successRaw, []byte(`"canAuthorize":false}`), []byte(`"canAuthorize":null}`)),
	} {
		if _, err := decodeXQASynchronizedConcurrency(invalid); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
}
