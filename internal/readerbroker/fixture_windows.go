//go:build windows && auth_session_fixture

package readerbroker

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var fixturePipePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Fixture transport changes only the pipe address; every OS identity, ticket,
// request and activation check remains the production implementation.
func ValidateFixturePipe(pipe string) error {
	const prefix = `\\.\pipe\AkuBrowser.fixture-reader.`
	if !strings.HasPrefix(pipe, prefix) || !fixturePipePattern.MatchString(strings.TrimPrefix(pipe, prefix)) {
		return errors.New("invalid isolated fixture pipe")
	}
	return nil
}
func ServeFixture(ctx context.Context, pipe string, authorize func(uint32) error, handle func(context.Context, Request, func(Target) (Reply, error)) error) error {
	if err := ValidateFixturePipe(pipe); err != nil {
		return err
	}
	return servePipe(ctx, pipe, authorize, handle)
}
func RunFixtureClient(ctx context.Context, req Request, pipe string) Reply {
	if err := ValidateFixturePipe(pipe); err != nil {
		return Reply{Message: err.Error()}
	}
	return runClientPipe(ctx, req, pipe)
}
