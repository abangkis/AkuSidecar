//go:build windows && auth_session_fixture

package main

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"os"
)

func run(ctx context.Context, r readerbroker.Request) readerbroker.Reply {
	return readerbroker.RunFixtureClient(ctx, r, os.Getenv("AKU_BROKER_FIXTURE_PIPE"))
}
