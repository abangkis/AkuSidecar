// Package observability projects selected diagnostic HTTP requests into bounded
// product telemetry. The application owns the runtime; HTTP handlers borrow it.
package observability

import (
	"context"
	"sync"
	"time"

	telemetryruntime "github.com/abangkis/ai4u-diagnostics-go/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	instrumentationScope = "github.com/abangkis/AkuSidecar/internal/diagnostics/observability"
	requestsName         = "akubrowser.diagnostics.requests"
	durationName         = "akubrowser.diagnostics.duration"
)

// Operation is a closed product vocabulary, independent of HTTP paths.
type Operation uint8

const (
	UITimelineRead Operation = iota + 1
	UITimelineWrite
	InstagramReader
	InstagramBootstrap
	InstagramCorroboration
)

func (o Operation) name() string {
	switch o {
	case UITimelineRead:
		return "ui_timeline_read"
	case UITimelineWrite:
		return "ui_timeline_write"
	case InstagramReader:
		return "instagram_reader"
	case InstagramBootstrap:
		return "instagram_bootstrap"
	case InstagramCorroboration:
		return "instagram_corroboration"
	default:
		return ""
	}
}

// Outcome carries no response body, error content, or source identifiers.
type Outcome uint8

const (
	Success Outcome = iota + 1
	Rejected
	Unavailable
)

func (o Outcome) name() string {
	switch o {
	case Success:
		return "success"
	case Rejected:
		return "rejected"
	default:
		return "unavailable"
	}
}

// Config is explicit configuration. An empty endpoint enables no exporter,
// regardless of ambient OpenTelemetry environment variables.
type Config struct {
	OTLPEndpoint  string
	BuildRevision string
	BuildModified *bool
}

// Runtime owns the library runtime and product instruments. It installs no
// global providers. Request code must not call Shutdown.
type Runtime struct {
	owner    *telemetryruntime.Runtime
	tracer   trace.Tracer
	requests metric.Int64Counter
	duration metric.Float64Histogram
}

// New constructs the official providers through the published shared runtime.
// Export failures are best-effort health data and do not affect request results.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	owner, err := telemetryruntime.New(ctx, telemetryruntime.Config{
		ServiceName:   "AkuSidecar",
		OTLPEndpoint:  cfg.OTLPEndpoint,
		BuildRevision: cfg.BuildRevision,
		BuildModified: cfg.BuildModified,
	})
	if err != nil {
		return nil, err
	}
	runtime, err := newWithProviders(owner.TracerProvider(), owner.MeterProvider())
	if err != nil {
		_ = owner.Shutdown(ctx)
		return nil, err
	}
	runtime.owner = owner
	return runtime, nil
}

// newWithProviders keeps instrument construction testable using official SDK
// providers. Provider ownership remains with its caller.
func newWithProviders(tp trace.TracerProvider, mp metric.MeterProvider) (*Runtime, error) {
	meter := mp.Meter(instrumentationScope)
	requests, err := meter.Int64Counter(requestsName, metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram(durationName, metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}
	return &Runtime{tracer: tp.Tracer(instrumentationScope), requests: requests, duration: duration}, nil
}

// Begin starts one request span and timer. Unknown operations and nil runtimes
// are safe no-ops. The returned completion function records at most once, even
// when called concurrently. Unknown outcomes use the bounded unavailable value.
// No URL, error, request body, or per-scroll observation can enter this API.
func (r *Runtime) Begin(ctx context.Context, operation Operation) (context.Context, func(Outcome)) {
	if ctx == nil {
		ctx = context.Background()
	}
	name := operation.name()
	if r == nil || r.tracer == nil || r.requests == nil || r.duration == nil || name == "" {
		return ctx, func(Outcome) {}
	}
	started := time.Now()
	ctx, span := r.tracer.Start(ctx, "akubrowser.diagnostics."+name,
		trace.WithAttributes(attribute.String("operation", name)))
	var once sync.Once
	return ctx, func(outcome Outcome) {
		once.Do(func() {
			outcomeName := outcome.name()
			span.SetAttributes(attribute.String("outcome", outcomeName))
			attrs := metric.WithAttributes(attribute.String("operation", name), attribute.String("outcome", outcomeName))
			r.requests.Add(ctx, 1, attrs)
			r.duration.Record(ctx, float64(time.Since(started))/float64(time.Millisecond), attrs)
			span.End()
		})
	}
}

// Diagnostics returns only the shared runtime's bounded exporter health data.
func (r *Runtime) Diagnostics() telemetryruntime.Diagnostics {
	if r == nil {
		return telemetryruntime.Diagnostics{}
	}
	return r.owner.Diagnostics()
}

// Shutdown is reserved for the composition-root owner. The shared runtime owns
// bounded, idempotent provider shutdown and suppresses exporter error contents.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return r.owner.Shutdown(ctx)
}
