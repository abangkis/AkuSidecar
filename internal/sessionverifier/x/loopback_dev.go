//go:build auth_session_dev && auth_session_fixture

package x

import (
	"errors"
	"net/url"
	"strconv"
	"time"
)

const (
	LoopbackFixtureOperation = "x.loopback-self.fixture/v1"
	LoopbackFixturePath      = "/x-fixture-operation"
)

// NewLoopbackFixtureVerifier explicitly binds invented self-response semantics
// to a loopback fixture origin. The observed origin is never rewritten to X.
// This does not qualify a native source or grant Store/capture authorization.
func NewLoopbackFixtureVerifier(pin Pin, origin string, now func() time.Time) (*Verifier, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("loopback fixture origin is invalid")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || origin != "http://127.0.0.1:"+strconv.Itoa(port) {
		return nil, errors.New("loopback fixture origin is invalid")
	}
	v, err := NewVerifier(pin, now)
	if err != nil {
		return nil, err
	}
	v.loopbackOrigin = origin
	return v, nil
}

func (v *Verifier) fixtureTransport() (string, string, string) {
	if v.loopbackOrigin != "" {
		return v.loopbackOrigin, LoopbackFixturePath, LoopbackFixtureOperation
	}
	return FixtureOrigin, FixturePath, FixtureOperation
}

func (v *Verifier) validFixtureFinalURL(raw string) bool {
	if v.loopbackOrigin == "" {
		return validFinalURL(raw)
	}
	return raw == v.loopbackOrigin+LoopbackFixturePath
}
