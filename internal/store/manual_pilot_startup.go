//go:build auth_session_dev

package store

import (
	"context"
	"encoding/hex"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"time"
)

// DevelopmentManualPilotBinding is returned only by a trusted in-process
// startup validator. The digest covers the complete private owner/account/pair
// binding. It is not a report format or a deserializable authority token.
type DevelopmentManualPilotBinding struct {
	Digest, Attempt     string
	OwnerGeneration     uint64
	StartedAt, Deadline time.Time
}

func (DevelopmentManualPilotBinding) String() string               { return "[development startup binding redacted]" }
func (v DevelopmentManualPilotBinding) GoString() string           { return v.String() }
func (DevelopmentManualPilotBinding) MarshalJSON() ([]byte, error) { return nil, errManualPilot }
func (*DevelopmentManualPilotBinding) UnmarshalJSON([]byte) error  { return errManualPilot }

// NewDevelopmentManualPilotIssuer is a trusted Go composition boundary, not a
// native provider registration. Its validator must inspect the private validated
// startup vector and provenance; public reports and caller-supplied flags are
// not validators. No callback runs inside a Store transaction or mutex.
func (s *Store) NewDevelopmentManualPilotIssuer(validate func(context.Context) (DevelopmentManualPilotBinding, error)) func(context.Context, string, string) (*ManualPilotPermit, error) {
	return s.newDevelopmentPilotIssuer(validate, false)
}

// Daily permits retain the same account/owner/deadline checks, while fencing
// X rather than unrelated source runs in the ordinary mixed-source session.
func (s *Store) NewDevelopmentDailyPilotIssuer(validate func(context.Context) (DevelopmentManualPilotBinding, error)) func(context.Context, string, string) (*ManualPilotPermit, error) {
	return s.NewDevelopmentSourceDailyPilotIssuer(domain.SourceX, validate)
}

func (s *Store) NewDevelopmentSourceDailyPilotIssuer(source domain.Source, validate func(context.Context) (DevelopmentManualPilotBinding, error)) func(context.Context, string, string) (*ManualPilotPermit, error) {
	return s.newSourceDevelopmentPilotIssuer(source, validate, true)
}
func (s *Store) newDevelopmentPilotIssuer(validate func(context.Context) (DevelopmentManualPilotBinding, error), runScoped bool) func(context.Context, string, string) (*ManualPilotPermit, error) {
	return s.newSourceDevelopmentPilotIssuer(domain.SourceX, validate, runScoped)
}
func (s *Store) newSourceDevelopmentPilotIssuer(source domain.Source, validate func(context.Context) (DevelopmentManualPilotBinding, error), runScoped bool) func(context.Context, string, string) (*ManualPilotPermit, error) {
	return func(ctx context.Context, session, run string) (*ManualPilotPermit, error) {
		if validate == nil || session == "" || run == "" {
			return nil, errManualPilot
		}
		v, err := validate(ctx)
		if err != nil {
			return nil, err
		}
		digest, err := hex.DecodeString(v.Digest)
		now := s.Now()
		if err != nil || len(digest) != 32 || v.Attempt == "" || v.OwnerGeneration == 0 || v.StartedAt.IsZero() || v.StartedAt.After(now) || !v.Deadline.After(now) || v.Deadline.After(v.StartedAt.Add(60*time.Second)) {
			return nil, errManualPilot
		}
		p, err := s.issueSourceDevelopmentPilot(ctx, source, session, run, "startup-sha256:"+v.Digest, "startup-sha256:"+v.Digest, v.Attempt, v.OwnerGeneration, v.Deadline, runScoped)
		if err != nil {
			return nil, err
		}
		// An invalidation while SQL was waiting closes the new permit before it
		// can escape. The Engine also latches invalidation during publication.
		after, err := validate(ctx)
		if err != nil || after != v {
			revokeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = s.RevokeManualPilot(revokeCtx, p, "startup-rejected:"+v.Attempt)
			return nil, errManualPilot
		}
		return p, nil
	}
}
