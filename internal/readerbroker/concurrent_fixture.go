//go:build auth_session_dev && auth_session_fixture

package readerbroker

import (
	"context"
	"errors"
	"io"
)

// RouteConcurrentFixture exercises the existing bounded framing and request
// validation without a listener, activation target, window handle or foreground
// claim. The callback is supplied only by the synthetic fixture owner.
func RouteConcurrentFixture(ctx context.Context, input io.Reader, output io.Writer, open func(context.Context, Request) error) error {
	var request Request
	if err := Read(input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Source != "x" || ctx.Err() != nil || open == nil {
		return errors.New("synthetic concurrent reader unavailable")
	}
	if err := open(ctx, request); err != nil {
		return err
	}
	return Write(output, struct {
		Accepted bool `json:"syntheticAccepted"`
	}{true})
}
