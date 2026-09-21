//go:build windows

package main

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func run(ctx context.Context, r readerbroker.Request) readerbroker.Reply {
	return readerbroker.RunClient(ctx, r)
}
