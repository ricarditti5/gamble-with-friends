// Package otel initializes OpenTelemetry tracing (OTLP/HTTP) and structured
// logging (slog). Tracing is opt-in: set OTEL_EXPORTER_OTLP_ENDPOINT to enable.
package otel

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const ServiceName = "gamblefriends-backend"

// Setup configures the default slog logger (JSON when GWF_LOG_FORMAT=json)
// and, when an OTLP endpoint is configured, a tracer provider exporting via
// OTLP/HTTP. Returns a shutdown function (may be a no-op when tracing is off).
func Setup(ctx context.Context) (func(context.Context) error, error) {
	format := strings.ToLower(os.Getenv("GWF_LOG_FORMAT"))
	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, nil)
	if format == "" || format == "text" {
		handler = slog.NewTextHandler(os.Stdout, nil)
	}
	slog.SetDefault(slog.New(handler))

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if traces := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); traces != "" {
		endpoint = traces
	}
	if endpoint == "" || os.Getenv("OTEL_TRACES_EXPORTER") == "none" {
		slog.Info("tracing: disabled (set OTEL_EXPORTER_OTLP_ENDPOINT to enable OTLP export)")
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracehttp.Option{}
	scheme, host := splitEndpoint(endpoint)
	if scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	opts = append(opts, otlptracehttp.WithEndpoint(host))

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		slog.Error("tracing: failed to create OTLP exporter", "error", err)
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(ServiceName)))
	if err != nil {
		slog.Warn("tracing: failed to merge resource", "error", err)
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	slog.Info("tracing: OTLP exporter enabled", "endpoint", endpoint)
	return tp.Shutdown, nil
}

func splitEndpoint(e string) (scheme, host string) {
	switch {
	case strings.HasPrefix(e, "http://"):
		return "http", strings.TrimPrefix(e, "http://")
	case strings.HasPrefix(e, "https://"):
		return "https", strings.TrimPrefix(e, "https://")
	}
	return "http", e
}
