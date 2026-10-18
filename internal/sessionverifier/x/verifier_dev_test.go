//go:build auth_session_dev && auth_session_fixture

package x

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

var testStart = time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

func newVerifier(t *testing.T) (*Verifier, *time.Time) {
	t.Helper()
	now := testStart
	verifier, err := NewVerifier(Pin{
		ProfileID: "fixture-profile", OwnerID: "fixture-owner", OwnerEpoch: "fixture-epoch",
		OwnerGeneration: 7, AccountID: "1234567890123456789", LineageGeneration: 3,
	}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return verifier, &now
}

func pairAttempts(t *testing.T, verifier *Verifier, firstBody, secondBody []byte, firstStatus, secondStatus int) (Attempt, Attempt) {
	t.Helper()
	now := verifier.now()
	first, err := fixtureAttempt(verifier, PhaseInitial, now, firstStatus, firstBody)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixtureAttempt(verifier, PhaseRecheck, now, secondStatus, secondBody)
	if err != nil {
		t.Fatal(err)
	}
	first.Lineage = fixtureLineage(first, verifier.pin.LineageGeneration)
	second.Lineage = fixtureLineage(second, verifier.pin.LineageGeneration)
	return first, second
}

func fixtureLineage(attempt Attempt, generation uint64) *SyntheticLineageObservation {
	return &SyntheticLineageObservation{
		Provenance: "synthetic-fixture", Request: attempt.Request, Generation: generation,
		CoverageComplete: true, ObservedAt: attempt.Envelope.ObservedAt,
	}
}

func TestStableIDSurvivesHandleAndDisplayNameChanges(t *testing.T) {
	verifier, _ := newVerifier(t)
	first, second := pairAttempts(t, verifier,
		authenticatedFixtureBody("1234567890123456789", "old_handle", "Old Name"),
		authenticatedFixtureBody("1234567890123456789", "new_handle", "New Name"), 200, 200)
	result := verifier.VerifyPair(first, second)
	if result.State != Authenticated || result.AccountID != "1234567890123456789" || !result.IdentityMatch || result.Continuity != ContinuitySame {
		t.Fatalf("stable ID was not retained across rename: %+v", result)
	}
	assertNonAuthorizing(t, result)
}

func TestPublicUserAndContentAuthorIDsCannotStandInForSelf(t *testing.T) {
	for name, body := range map[string][]byte{
		"public user":    []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","user":{"id_str":"1234567890123456789"}}`),
		"content author": []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","author":{"id_str":"1234567890123456789"}}`),
		"nested user":    []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","data":{"user":{"id_str":"1234567890123456789"}}}`),
		"OAuth users me": []byte(`{"data":{"id":"1234567890123456789","name":"Name","username":"handle"}}`),
	} {
		t.Run(name, func(t *testing.T) {
			verifier, _ := newVerifier(t)
			request, err := verifier.Issue(PhaseInitial)
			if err != nil {
				t.Fatal(err)
			}
			observation := verifier.Observe(Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, 200), Body: body})
			if observation.State == Authenticated && observation.AccountID != "" {
				t.Fatalf("non-self identity was accepted: %+v", observation)
			}
		})
	}
}

func TestMalformedOversizedAndDuplicateIdentityBodiesFailClosed(t *testing.T) {
	cases := map[string][]byte{
		"malformed":       []byte(`{"schema":`),
		"oversized":       bytes.Repeat([]byte("x"), MaxBodyBytes+1),
		"duplicate id":    []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","data":{"id_str":"1234567890123456789","id_str":"9999999999999999999"}}`),
		"duplicate state": []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","state":"logged_out","data":{"id_str":"1234567890123456789"}}`),
		"ambiguous data":  []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","data":{"id_str":"1234567890123456789"},"revision":"arbitrary"}`),
		"session field":   []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","data":{"id_str":"1234567890123456789","session_id":"arbitrary"}}`),
		"root revision":   []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","data":{"id_str":"1234567890123456789"},"revision":"same"}`),
		"lineage claim":   []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","data":{"id_str":"1234567890123456789"},"continuity":"same"}`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			verifier, _ := newVerifier(t)
			request, err := verifier.Issue(PhaseInitial)
			if err != nil {
				t.Fatal(err)
			}
			observation := verifier.Observe(Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, 200), Body: body})
			if observation.State != Unknown || observation.AccountID != "" {
				t.Fatalf("bad body was not closed: %+v", observation)
			}
			if strings.Contains(string(body), string(observation.Reason)) {
				t.Fatalf("reason leaked response content: %q", observation.Reason)
			}
		})
	}
}

func TestExplicitLoginStatesAreDistinct(t *testing.T) {
	cases := []struct {
		name   string
		state  string
		status int
		want   AuthState
	}{
		{name: "authenticated", state: "authenticated", status: 200, want: Unknown}, // no exact self ID
		{name: "logged out", state: "logged_out", status: 401, want: LoggedOut},
		{name: "challenge", state: "challenge", status: 403, want: Challenge},
		{name: "unknown", state: "unknown", status: 200, want: Unknown},
		{name: "empty is not logged out", state: "", status: 200, want: Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier, _ := newVerifier(t)
			body := fixtureBody(tc.state)
			if tc.name == "empty is not logged out" {
				body = nil
			}
			request, err := verifier.Issue(PhaseInitial)
			if err != nil {
				t.Fatal(err)
			}
			got := verifier.Observe(Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, tc.status), Body: body})
			if got.State != tc.want {
				t.Fatalf("state = %q, want %q (%+v)", got.State, tc.want, got)
			}
		})
	}
}

func TestWrongOriginRedirectStatusAndBindingFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Attempt)
	}{
		{name: "wrong origin", mutate: func(a *Attempt) { a.Envelope.FinalURL = "https://api.x.com" + FixturePath }},
		{name: "wrong path", mutate: func(a *Attempt) { a.Envelope.FinalURL = FixtureOrigin + "/i/users/1" }},
		{name: "redirect", mutate: func(a *Attempt) { a.Envelope.Redirected = true }},
		{name: "profile mismatch", mutate: func(a *Attempt) { a.Envelope.ProfileID = "other-profile" }},
		{name: "owner epoch mismatch", mutate: func(a *Attempt) { a.Envelope.OwnerEpoch = "other-epoch" }},
		{name: "owner generation mismatch", mutate: func(a *Attempt) { a.Envelope.OwnerGeneration++ }},
		{name: "request challenge mismatch", mutate: func(a *Attempt) { a.Envelope.Challenge = "copied-from-body-is-not-binding" }},
		{name: "bad status", mutate: func(a *Attempt) { a.Envelope.StatusCode = 500 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier, _ := newVerifier(t)
			request, err := verifier.Issue(PhaseInitial)
			if err != nil {
				t.Fatal(err)
			}
			attempt := Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")}
			tc.mutate(&attempt)
			got := verifier.Observe(attempt)
			if got.State == Authenticated && got.AccountID != "1234567890123456789" {
				t.Fatalf("invalid envelope produced identity: %+v", got)
			}
			if got.AccountID != "" || got.Reason == ReasonNone {
				t.Fatalf("invalid envelope was not rejected: %+v", got)
			}
		})
	}
}

func TestEvidenceMustBeFreshAndNotFutureDated(t *testing.T) {
	for name, observed := range map[string]time.Time{
		"stale":  testStart.Add(time.Second),
		"future": testStart.Add(time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			verifier, now := newVerifier(t)
			request, err := verifier.Issue(PhaseInitial)
			if err != nil {
				t.Fatal(err)
			}
			if name == "stale" {
				*now = testStart.Add(10 * time.Second)
			}
			got := verifier.Observe(Attempt{Request: request, Envelope: fixtureEnvelope(request, observed, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")})
			if got.State != Unknown || got.Reason != ReasonStaleEvidence {
				t.Fatalf("invalid time was not rejected: %+v", got)
			}
		})
	}
}

func TestAccountMismatchAndConsumedNegativeProof(t *testing.T) {
	verifier, _ := newVerifier(t)
	request, err := verifier.Issue(PhaseInitial)
	if err != nil {
		t.Fatal(err)
	}
	bad := Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, 200), Body: authenticatedFixtureBody("9999999999999999999", "other", "Other")}
	first := verifier.Observe(bad)
	if first.State != Authenticated || first.Reason != ReasonAccountMismatch || first.AccountID != "" {
		t.Fatalf("mismatched account was not kept private and closed: %+v", first)
	}
	goodRetry := Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "expected", "Expected")}
	second := verifier.Observe(goodRetry)
	if second.State != Unknown || second.Reason != ReasonChallengeUnknown {
		t.Fatalf("consumed challenge accepted retry: %+v", second)
	}
}

func TestAccountMismatchAcrossBothPhasesStaysClosed(t *testing.T) {
	verifier, _ := newVerifier(t)
	first, second := pairAttempts(t, verifier,
		authenticatedFixtureBody("9999999999999999999", "other", "Other"),
		authenticatedFixtureBody("9999999999999999999", "other", "Other"), 200, 200)
	result := verifier.VerifyPair(first, second)
	if result.State != Authenticated || result.IdentityMatch || result.AccountID != "" ||
		result.Continuity != ContinuityChanged || result.Reason != ReasonAccountMismatch || result.CanAuthorize() {
		t.Fatalf("mismatched identity pair was not closed: %+v", result)
	}
}

func TestReplayAndPendingChallengeLimit(t *testing.T) {
	verifier, _ := newVerifier(t)
	request, err := verifier.Issue(PhaseInitial)
	if err != nil {
		t.Fatal(err)
	}
	valid := Attempt{Request: request, Envelope: fixtureEnvelope(request, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")}
	if got := verifier.Observe(valid); got.State != Authenticated {
		t.Fatalf("first use failed: %+v", got)
	}
	if got := verifier.Observe(valid); got.State != Unknown || got.Reason != ReasonChallengeUnknown {
		t.Fatalf("replay was not rejected: %+v", got)
	}
	for i := 0; i < MaxPendingChallenges; i++ {
		if _, err := verifier.Issue(PhaseInitial); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := verifier.Issue(PhaseInitial); err == nil {
		t.Fatal("pending challenge set was not bounded")
	}
}

func TestChallengeNamespacePreventsCrossVerifierReplay(t *testing.T) {
	firstVerifier, _ := newVerifier(t)
	secondVerifier, _ := newVerifier(t)
	firstRequest, err := firstVerifier.Issue(PhaseInitial)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest, err := secondVerifier.Issue(PhaseInitial)
	if err != nil {
		t.Fatal(err)
	}
	if firstRequest.Challenge == secondRequest.Challenge || firstRequest.RequestID == secondRequest.RequestID {
		t.Fatal("independent verifier instances reused request bindings")
	}
	oldAttempt := Attempt{Request: firstRequest, Envelope: fixtureEnvelope(firstRequest, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")}
	if got := secondVerifier.Observe(oldAttempt); got.State != Unknown || got.Reason != ReasonChallengeUnknown {
		t.Fatalf("prior instance challenge replayed: %+v", got)
	}
	validAttempt := Attempt{Request: secondRequest, Envelope: fixtureEnvelope(secondRequest, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")}
	if got := secondVerifier.Observe(validAttempt); got.State != Authenticated {
		t.Fatalf("current instance challenge failed after replay attempt: %+v", got)
	}
}

func TestFreshResponseCannotReuseExpiredIssuedChallenge(t *testing.T) {
	verifier, now := newVerifier(t)
	request, err := verifier.Issue(PhaseInitial)
	if err != nil {
		t.Fatal(err)
	}
	*now = testStart.Add(10 * time.Second)
	got := verifier.Observe(Attempt{
		Request: request, Envelope: fixtureEnvelope(request, *now, 200),
		Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
	})
	if got.State != Unknown || got.Reason != ReasonStaleEvidence || got.AccountID != "" {
		t.Fatalf("fresh response revived expired challenge: %+v", got)
	}
}

func TestIncompleteCoverageAndIdentityOnlyCannotQualify(t *testing.T) {
	t.Run("incomplete coverage", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		second.Envelope.CoverageComplete = false
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuityUnavailable || result.IdentityMatch || result.CanAuthorize() {
			t.Fatalf("incomplete coverage was usable: %+v", result)
		}
	})
	t.Run("matching identity without lineage witness", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		first.Lineage, second.Lineage = nil, nil
		result := verifier.VerifyPair(first, second)
		if result.State != Authenticated || !result.IdentityMatch || result.AccountID == "" || result.Continuity != ContinuityUnsupported || result.CanAuthorize() {
			t.Fatalf("identity alone qualified: %+v", result)
		}
		assertNonAuthorizing(t, result)
	})
	t.Run("lineage coverage gap", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		second.Lineage.CoverageComplete = false
		result := verifier.VerifyPair(first, second)
		if result.State != Authenticated || !result.IdentityMatch || result.Continuity != ContinuityUnavailable || result.Reason != ReasonLineageUnavailable || result.CanAuthorize() {
			t.Fatalf("lineage coverage gap was usable: %+v", result)
		}
	})
}

func TestSyntheticRotationAndSameAccountReloginClassifySeparately(t *testing.T) {
	t.Run("rotation fixture keeps the accepted synthetic generation", func(t *testing.T) {
		// Rotation is represented only by a stable fixture generation. No cookie
		// values are captured, parsed, hashed, or used as continuity evidence.
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "before_rotation", "Before"),
			authenticatedFixtureBody("1234567890123456789", "after_rotation", "After"), 200, 200)
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuitySame || !result.IdentityMatch || result.AccountID == "" || result.CanAuthorize() {
			t.Fatalf("synthetic lineage comparison failed: %+v", result)
		}
	})
	t.Run("logout then same-account relogin is changed", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		logoutRequest, err := verifier.Issue(PhaseInitial)
		if err != nil {
			t.Fatal(err)
		}
		logout := verifier.Observe(Attempt{Request: logoutRequest, Envelope: fixtureEnvelope(logoutRequest, testStart, 401), Body: fixtureBody("logged_out")})
		if logout.State != LoggedOut {
			t.Fatalf("logout fixture was not observed: %+v", logout)
		}

		reloginVerifier, _ := newVerifier(t)
		first, second := pairAttempts(t, reloginVerifier,
			authenticatedFixtureBody("1234567890123456789", "new_handle", "New Name"),
			authenticatedFixtureBody("1234567890123456789", "new_handle", "New Name"), 200, 200)
		generation := reloginVerifier.pin.LineageGeneration + 1
		first.Lineage = fixtureLineage(first, generation)
		second.Lineage = fixtureLineage(second, generation)
		result := reloginVerifier.VerifyPair(first, second)
		if result.Continuity != ContinuityChanged || !result.IdentityMatch || result.CanAuthorize() {
			t.Fatalf("same-account relogin was not classified as changed and closed: %+v", result)
		}
		assertNonAuthorizing(t, result)
	})
	t.Run("lineage changes between fresh phases", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		second.Lineage = fixtureLineage(second, verifier.pin.LineageGeneration+1)
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuityChanged || !result.IdentityMatch || result.CanAuthorize() {
			t.Fatalf("lineage change during verification was missed: %+v", result)
		}
	})
}

func TestUnknownChallengeAndRevokedOrUnsupportedLineageStayClosed(t *testing.T) {
	t.Run("challenge", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier, fixtureBody("challenge"), fixtureBody("challenge"), 403, 403)
		result := verifier.VerifyPair(first, second)
		if result.State != Challenge || result.Continuity != ContinuityUnavailable || result.CanAuthorize() {
			t.Fatalf("challenge did not remain closed: %+v", result)
		}
	})
	t.Run("unknown state", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier, fixtureBody("unknown"), fixtureBody("unknown"), 200, 200)
		result := verifier.VerifyPair(first, second)
		if result.State != Unknown || result.Continuity != ContinuityUnavailable || result.CanAuthorize() {
			t.Fatalf("unknown state did not remain closed: %+v", result)
		}
	})
	t.Run("untrusted witness provenance", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		second.Lineage.Provenance = "native-x"
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuityUnsupported || result.Reason != ReasonInvalidWitness || result.CanAuthorize() {
			t.Fatalf("unqualified lineage was accepted: %+v", result)
		}
	})
	t.Run("lineage bound to the wrong phase", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		second.Lineage.Request.Phase = PhaseInitial
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuityUnsupported || result.Reason != ReasonInvalidWitness || result.CanAuthorize() {
			t.Fatalf("cross-phase lineage was accepted: %+v", result)
		}
	})
	t.Run("revoked lineage coverage", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		second.Lineage.Generation = 0
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuityUnavailable || result.CanAuthorize() {
			t.Fatalf("revoked lineage remained usable: %+v", result)
		}
	})
}

func TestPairMustUseTwoDistinctFreshPhasesAndRecheckAtCompletion(t *testing.T) {
	t.Run("wrong phase", func(t *testing.T) {
		verifier, _ := newVerifier(t)
		first, err := fixtureAttempt(verifier, PhaseRecheck, testStart, 200, authenticatedFixtureBody("1234567890123456789", "handle", "Name"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := fixtureAttempt(verifier, PhaseInitial, testStart, 200, authenticatedFixtureBody("1234567890123456789", "handle", "Name"))
		if err != nil {
			t.Fatal(err)
		}
		result := verifier.VerifyPair(first, second)
		if result.Continuity != ContinuityUnsupported || result.CanAuthorize() {
			t.Fatalf("wrong phase order qualified: %+v", result)
		}
	})
	t.Run("ages before final pair check", func(t *testing.T) {
		verifier, now := newVerifier(t)
		first, second := pairAttempts(t, verifier,
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"),
			authenticatedFixtureBody("1234567890123456789", "handle", "Name"), 200, 200)
		*now = testStart.Add(10 * time.Second)
		result := verifier.VerifyPair(first, second)
		if result.State != Unknown || result.AccountID != "" || result.IdentityMatch || result.Continuity != ContinuityUnavailable || result.Reason != ReasonStaleEvidence || result.CanAuthorize() {
			t.Fatalf("pair expired before completion but stayed usable: %+v", result)
		}
	})
	t.Run("expires after both observations but before pair completion", func(t *testing.T) {
		clockCalls := 0
		clock := func() time.Time {
			clockCalls++
			if clockCalls <= 4 {
				return testStart
			}
			return testStart.Add(10 * time.Second)
		}
		verifier, err := NewVerifier(Pin{
			ProfileID: "fixture-profile", OwnerID: "fixture-owner", OwnerEpoch: "fixture-epoch",
			OwnerGeneration: 7, AccountID: "1234567890123456789", LineageGeneration: 3,
		}, clock)
		if err != nil {
			t.Fatal(err)
		}
		firstRequest, err := verifier.Issue(PhaseInitial)
		if err != nil {
			t.Fatal(err)
		}
		secondRequest, err := verifier.Issue(PhaseRecheck)
		if err != nil {
			t.Fatal(err)
		}
		first := Attempt{Request: firstRequest, Envelope: fixtureEnvelope(firstRequest, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")}
		second := Attempt{Request: secondRequest, Envelope: fixtureEnvelope(secondRequest, testStart, 200), Body: authenticatedFixtureBody("1234567890123456789", "handle", "Name")}
		first.Lineage = fixtureLineage(first, verifier.pin.LineageGeneration)
		second.Lineage = fixtureLineage(second, verifier.pin.LineageGeneration)
		result := verifier.VerifyPair(first, second)
		if clockCalls != 5 {
			t.Fatalf("expected two issuance times, two fresh observations, and one completion time; got %d clock reads", clockCalls)
		}
		if result.State != Unknown || result.AccountID != "" || result.IdentityMatch || result.Continuity != ContinuityUnavailable || result.Reason != ReasonStaleEvidence || result.CanAuthorize() {
			t.Fatalf("completion-time expiry retained identity flags: %+v", result)
		}
	})
}

func assertNonAuthorizing(t *testing.T, result PairResult) {
	t.Helper()
	if result.CanAuthorize() || result.CaptureAuthorized || result.ProductionQualified {
		t.Fatal("synthetic X verifier result became authorization evidence")
	}
}
