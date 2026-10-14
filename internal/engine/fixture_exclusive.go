//go:build auth_session_fixture

package engine

import (
	"context"
	"errors"
)

// SyntheticSessionEvidence is fixture-only evidence, never real account proof.
type SyntheticSessionEvidence = sessionEvidence

// BindSyntheticExclusive is only compiled with auth_session_fixture. It must be
// called before coordinator/collector startup, on an isolated development Store.
// No production config, environment variable or HTTP endpoint enables this path.
func (e *Engine) BindSyntheticExclusive(expectedAccount string, probe func(context.Context) (SyntheticSessionEvidence, error)) error {
	return e.bindExclusiveSession(expectedAccount, probe, nil, false)
}

// BindSyntheticExclusiveWithBrowserProbe adds an actual browser-session check
// after a fixture owner launches. The authority probe remains available while
// no browser process exists and must never cause an automatic login.
func (e *Engine) BindSyntheticExclusiveWithBrowserProbe(expectedAccount string,
	probe func(context.Context) (SyntheticSessionEvidence, error),
	browserProbe func(context.Context) (SyntheticSessionEvidence, error)) error {
	if browserProbe == nil {
		return errors.New("synthetic browser probe is required")
	}
	return e.bindExclusiveSession(expectedAccount, probe, browserProbe, false)
}
