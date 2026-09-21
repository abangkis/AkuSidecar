//go:build !windows

package main

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func run(context.Context, readerbroker.Request) readerbroker.Reply {
	return readerbroker.Reply{Message: "Reader broker is Windows-only"}
}
