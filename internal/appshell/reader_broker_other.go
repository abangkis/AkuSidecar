//go:build !windows

package appshell

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func (*Session) ServeReaderBroker(context.Context, func(context.Context, readerbroker.Request, func(readerbroker.Target) (readerbroker.Reply, error)) error) error {
	return errors.New("reader broker is Windows-only")
}
