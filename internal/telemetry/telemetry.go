// Package telemetry wires up OpenTelemetry tracing and metrics.
//
// By default it uses the stdout exporters, printing spans and metrics to
// stderr with no network calls — no collector required for local runs. When
// OTLP mode is requested, it instead exports via OTLP/gRPC, configured
// through the standard OTEL_EXPORTER_OTLP_* environment variables; when
// OTEL_EXPORTER_OTLP_ENDPOINT is unset, it defaults to a local collector at
// localhost:4317 over an insecure connection.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// otlpShutdownTimeout bounds how long a CLI invocation will wait to flush
// telemetry on exit when exporting via OTLP, so an unreachable collector
// adds a bounded delay rather than hanging on the default 10s OTLP export
// timeout. The stdout exporters never make network calls, so they need no
// such bound.
const otlpShutdownTimeout = 3 * time.Second

// Shutdown flushes and stops the telemetry providers. Call it before the
// process exits, e.g. via defer in main.
type Shutdown func(context.Context) error

// Options configures Setup.
type Options struct {
	// OTLP selects the OTLP/gRPC exporters. When false (the default), spans
	// and metrics are printed via the stdout exporters instead.
	OTLP bool
}

// Setup configures global trace and meter providers for serviceName/version
// and returns a Shutdown func to flush and release them.
func Setup(ctx context.Context, serviceName, serviceVersion string, opts Options) (Shutdown, error) {
	// Schemaless so it merges cleanly with resource.Default() regardless of
	// which semconv schema version the SDK's default detectors use.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	var (
		spanExporter   sdktrace.SpanExporter
		metricExporter metric.Exporter
	)
	if opts.OTLP {
		spanExporter, metricExporter, err = newOTLPExporters(ctx)
	} else {
		spanExporter, metricExporter, err = newStdoutExporters()
	}
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
	)
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		if opts.OTLP {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, otlpShutdownTimeout)
			defer cancel()
		}
		return errors.Join(
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		)
	}
	return shutdown, nil
}

func newStdoutExporters() (sdktrace.SpanExporter, metric.Exporter, error) {
	spanExporter, err := stdouttrace.New(
		stdouttrace.WithWriter(os.Stderr),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: create stdout trace exporter: %w", err)
	}

	metricExporter, err := stdoutmetric.New(
		stdoutmetric.WithWriter(os.Stderr),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: create stdout metric exporter: %w", err)
	}

	return spanExporter, metricExporter, nil
}

func newOTLPExporters(ctx context.Context) (sdktrace.SpanExporter, metric.Exporter, error) {
	var traceOpts []otlptracegrpc.Option
	var metricOpts []otlpmetricgrpc.Option
	if _, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT"); !ok {
		traceOpts = append(traceOpts,
			otlptracegrpc.WithEndpoint("localhost:4317"),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithTimeout(otlpShutdownTimeout),
		)
		metricOpts = append(metricOpts,
			otlpmetricgrpc.WithEndpoint("localhost:4317"),
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithTimeout(otlpShutdownTimeout),
		)
	}

	spanExporter, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: create OTLP trace exporter: %w", err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: create OTLP metric exporter: %w", err)
	}

	return spanExporter, metricExporter, nil
}
