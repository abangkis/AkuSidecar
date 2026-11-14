package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func recordingRuntime(t *testing.T) (*Runtime, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})
	runtime, err := newWithProviders(tp, mp)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, recorder, reader
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func checkAttributes(t *testing.T, attrs []attribute.KeyValue, operation, outcome string) {
	t.Helper()
	if len(attrs) != 2 {
		t.Fatalf("unexpected attributes: %v", attrs)
	}
	want := map[string]string{"operation": operation, "outcome": outcome}
	for _, attr := range attrs {
		value, ok := want[string(attr.Key)]
		if !ok || attr.Value.Type() != attribute.STRING || attr.Value.AsString() != value {
			t.Fatalf("unexpected attribute: %v", attr)
		}
		delete(want, string(attr.Key))
	}
	if len(want) != 0 {
		t.Fatalf("missing attributes: %v", want)
	}
}

func TestBoundedSignalsPreserveParentAndExcludeAmbientData(t *testing.T) {
	runtime, recorder, reader := recordingRuntime(t)
	traceID, _ := trace.TraceIDFromHex("1234567890abcdef1234567890abcdef")
	spanID, _ := trace.SpanIDFromHex("1234567890abcdef")
	parent := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	member, err := baggage.NewMember("private-post-id", "token-and-url-secret")
	if err != nil {
		t.Fatal(err)
	}
	bag, err := baggage.New(member)
	if err != nil {
		t.Fatal(err)
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)
	child, finish := runtime.Begin(ctx, UITimelineWrite)
	if got := trace.SpanContextFromContext(child); got.TraceID() != traceID || got.SpanID() == spanID {
		t.Fatalf("child does not preserve trace parent: %v", got)
	}
	if baggage.FromContext(child).Member("private-post-id").Value() != "token-and-url-secret" {
		t.Fatal("request context was not preserved")
	}
	var wait sync.WaitGroup
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			finish(Rejected)
		}()
	}
	wait.Wait()
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one completed request, got %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "akubrowser.diagnostics.ui_timeline_write" || span.InstrumentationScope().Name != instrumentationScope || !span.Parent().Equal(parent) {
		t.Fatalf("unexpected request span: %q, parent %v", span.Name(), span.Parent())
	}
	if len(span.Events()) != 0 || len(span.Links()) != 0 || span.Status().Code != codes.Unset || span.Status().Description != "" {
		t.Fatal("request added unexpected event, link, or status content")
	}
	checkAttributes(t, span.Attributes(), "ui_timeline_write", "rejected")
	data := collect(t, reader)
	if len(data.ScopeMetrics) != 1 || data.ScopeMetrics[0].Scope.Name != instrumentationScope || len(data.ScopeMetrics[0].Metrics) != 2 {
		t.Fatalf("unexpected metric scope: %+v", data.ScopeMetrics)
	}
	for _, signal := range data.ScopeMetrics[0].Metrics {
		switch signal.Name {
		case requestsName:
			sum, ok := signal.Data.(metricdata.Sum[int64])
			if !ok || signal.Unit != "{request}" || !sum.IsMonotonic || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
				t.Fatalf("unexpected request counter: %+v", signal)
			}
			checkAttributes(t, sum.DataPoints[0].Attributes.ToSlice(), "ui_timeline_write", "rejected")
		case durationName:
			histogram, ok := signal.Data.(metricdata.Histogram[float64])
			if !ok || signal.Unit != "ms" || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 || histogram.DataPoints[0].Sum < 0 {
				t.Fatalf("unexpected duration histogram: %+v", signal)
			}
			checkAttributes(t, histogram.DataPoints[0].Attributes.ToSlice(), "ui_timeline_write", "rejected")
		default:
			t.Fatalf("unexpected signal %q", signal.Name)
		}
	}
}

func TestClosedVocabulary(t *testing.T) {
	runtime, recorder, reader := recordingRuntime(t)
	ctx := context.Background()
	for _, operation := range []Operation{0, 6, 255} {
		child, finish := runtime.Begin(ctx, operation)
		if child != ctx {
			t.Fatal("unknown operation changed context")
		}
		finish(Success)
	}
	if len(recorder.Started()) != 0 || len(collect(t, reader).ScopeMetrics) != 0 {
		t.Fatal("unknown operations emitted telemetry")
	}
	operations := []Operation{UITimelineRead, UITimelineWrite, InstagramReader, InstagramBootstrap, InstagramCorroboration}
	names := []string{"ui_timeline_read", "ui_timeline_write", "instagram_reader", "instagram_bootstrap", "instagram_corroboration"}
	for i, operation := range operations {
		_, finish := runtime.Begin(ctx, operation)
		finish(Outcome(255))
		checkAttributes(t, recorder.Ended()[i].Attributes(), names[i], "unavailable")
	}
	for _, item := range []struct {
		outcome Outcome
		name    string
	}{{Success, "success"}, {Rejected, "rejected"}, {Unavailable, "unavailable"}, {0, "unavailable"}} {
		if got := item.outcome.name(); got != item.name {
			t.Fatalf("outcome %d: got %q", item.outcome, got)
		}
	}
}

func TestIndependentRuntimes(t *testing.T) {
	first, firstRecorder, firstReader := recordingRuntime(t)
	_, secondRecorder, secondReader := recordingRuntime(t)
	_, finish := first.Begin(context.Background(), InstagramReader)
	finish(Success)
	if len(firstRecorder.Ended()) != 1 || len(collect(t, firstReader).ScopeMetrics) != 1 {
		t.Fatal("first runtime did not record its request")
	}
	if len(secondRecorder.Started()) != 0 || len(collect(t, secondReader).ScopeMetrics) != 0 {
		t.Fatal("runtime instruments leaked between owners")
	}
}

func TestDefaultOffIgnoresOTelEnvironmentAndDoesNotInstallGlobals(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"} {
		t.Setenv(key, server.URL)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=private-token")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "private-post-id=private-value")
	t.Setenv("OTEL_SERVICE_NAME", "AmbientService")
	globalTracer, globalMeter, globalHandler := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetErrorHandler()
	runtime, err := New(context.Background(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, finish := runtime.Begin(context.Background(), UITimelineRead)
	if !trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("default-off runtime lost local trace identity")
	}
	finish(Success)
	if diagnostics := runtime.Diagnostics(); diagnostics.ExportConfigured || diagnostics.ExportFailures != 0 {
		t.Fatalf("unexpected default-off health: %+v", diagnostics)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("default-off runtime made %d export requests", requests.Load())
	}
	if otel.GetTracerProvider() != globalTracer || otel.GetMeterProvider() != globalMeter || otel.GetErrorHandler() != globalHandler {
		t.Fatal("runtime replaced an OpenTelemetry global")
	}
}

func TestNilRuntimeIsSafe(t *testing.T) {
	var runtime *Runtime
	ctx := context.Background()
	child, finish := runtime.Begin(ctx, UITimelineRead)
	if child != ctx {
		t.Fatal("nil runtime changed context")
	}
	finish(Success)
	child, finish = runtime.Begin(nil, UITimelineRead)
	if child == nil {
		t.Fatal("nil context was not normalized")
	}
	finish(Success)
	if runtime.Diagnostics().ExportConfigured || runtime.Diagnostics().ExportFailures != 0 || runtime.Shutdown(nil) != nil {
		t.Fatal("nil runtime returned nonzero health or shutdown error")
	}
}

func TestExplicitUnavailableExporterDoesNotChangeRequestResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	runtime, err := New(context.Background(), Config{OTLPEndpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, finish := runtime.Begin(context.Background(), InstagramBootstrap)
	finish(Success)
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("export failure changed shutdown result: %v", err)
	}
	// Both collector rejection and unavailable local transport remain best-effort.
	if health := runtime.Diagnostics(); !health.ExportConfigured || health.ExportFailures == 0 {
		t.Fatalf("missing bounded exporter failure health: %+v", health)
	}
}
