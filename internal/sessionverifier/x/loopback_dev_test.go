//go:build auth_session_dev && auth_session_fixture

package x

import (
	"testing"
	"time"
)

func TestLoopbackFixtureVerifierPreservesObservedTransport(t *testing.T) {
	legacy, _ := newVerifier(t)
	verifier, err := NewLoopbackFixtureVerifier(legacy.pin, "http://127.0.0.1:43127", func() time.Time { return testStart })
	if err != nil {
		t.Fatal(err)
	}
	first, second := pairAttempts(t, verifier,
		authenticatedFixtureBody(legacy.pin.AccountID, "fixture", "Fixture"),
		authenticatedFixtureBody(legacy.pin.AccountID, "renamed", "Renamed"), 200, 200)
	if first.Request.Origin != "http://127.0.0.1:43127" || first.Request.Path != LoopbackFixturePath ||
		first.Request.Operation != LoopbackFixtureOperation || first.Envelope.FinalURL != first.Request.Origin+first.Request.Path {
		t.Fatal("loopback observation was relabeled as a native transport")
	}
	result := verifier.VerifyPair(first, second)
	if !result.IdentityMatch || result.State != Authenticated {
		t.Fatalf("loopback fixture did not reach the existing verifier: %+v", result)
	}
	assertNonAuthorizing(t, result)
}

func TestLoopbackFixtureVerifierRejectsUnownedOrigins(t *testing.T) {
	legacy, _ := newVerifier(t)
	for _, origin := range []string{
		"https://x.com", "https://127.0.0.1:43127", "http://localhost:43127", "http://127.0.0.2:43127",
		"http://127.0.0.1", "http://127.0.0.1:0", "http://127.0.0.1:65536", "http://127.0.0.1:043127",
		"http://127.0.0.1:43127/", "http://127.0.0.1:43127?", "http://127.0.0.1:43127#secret",
		"http://user:secret@127.0.0.1:43127", "http://127.0.0.1:43127/path",
	} {
		if _, err := NewLoopbackFixtureVerifier(legacy.pin, origin, nil); err == nil || err.Error() != "loopback fixture origin is invalid" {
			t.Fatal("unowned or noncanonical origin was not rejected safely")
		}
	}
}

func TestLoopbackFixtureVerifierRejectsRelabeledOrRedirectedResponse(t *testing.T) {
	for _, finalURL := range []string{FixtureOrigin + FixturePath, "http://127.0.0.1:43127/x-fixture-operation?secret"} {
		legacy, _ := newVerifier(t)
		verifier, err := NewLoopbackFixtureVerifier(legacy.pin, "http://127.0.0.1:43127", func() time.Time { return testStart })
		if err != nil {
			t.Fatal(err)
		}
		attempt, err := fixtureAttempt(verifier, PhaseInitial, testStart, 200,
			authenticatedFixtureBody(legacy.pin.AccountID, "fixture", "Fixture"))
		if err != nil {
			t.Fatal(err)
		}
		attempt.Envelope.FinalURL = finalURL
		if got := verifier.Observe(attempt); got.Reason != ReasonWrongOriginOrPath || got.AccountID != "" {
			t.Fatal("response outside the observed loopback endpoint was accepted")
		}
	}
}
