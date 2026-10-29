package appshell

import (
	"context"
	"encoding/json"
	"time"
)

// MainBrowserTransportOptions selects an explicitly packaged Node transport.
// It owns CDP bytes only. The Window remains the sole Chrome lifecycle owner.
type MainBrowserTransportOptions struct {
	Node, Driver             string
	NodeSHA256, DriverSHA256 string
	StartupTimeout           time.Duration
}

// MainBrowserProtocol adds a private X lane to the ordinary projected calls.
// Neither lane exposes raw network events/cookie responses to Go.
type MainBrowserProtocol interface {
	CaptureProtocol
	CallX(context.Context, string, map[string]any) (json.RawMessage, error)
	TransportWorker() *OwnedCommand
}

// MainBrowserProgressProtocol is optional so existing protocol implementations
// and test doubles remain source-compatible. Progress stays a bounded private
// diagnostic callback on a single X operation.
type MainBrowserProgressProtocol interface {
	CallXWithProgress(context.Context, string, map[string]any, func([]byte)) (json.RawMessage, error)
}

// MainBrowserSourceProtocol is an optional, source-bound private lane. Existing
// X implementations retain their original wire contract and interfaces.
type MainBrowserSourceProtocol interface {
	CallSource(context.Context, string, string, map[string]any) (json.RawMessage, error)
}

// MainBrowserInstagramDiagnosticProtocol is an optional sealed reader diagnostic.
// It accepts only a caller nonce and returns a closed, privacy-safe projection.
type MainBrowserInstagramDiagnosticProtocol interface {
	DiagnoseInstagramReader(context.Context, string) (json.RawMessage, error)
}

// Diagnostics are a closed, Node-normalized projection. Formatting never emits
// payloads; callers explicitly decode the private diagnostic when needed.
type MainBrowserError struct {
	Code       string
	Diagnostic json.RawMessage
}

func (*MainBrowserError) Error() string { return "main browser operation rejected" }
