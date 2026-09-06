// Package telemetry wires up OpenTelemetry tracing and metrics.
//
// Where spans and metrics go depends on Options. In OTLP mode it exports
// spans, metrics, and logs via OTLP/gRPC, configured through the standard
// OTEL_EXPORTER_OTLP_* environment variables; when OTEL_EXPORTER_OTLP_ENDPOINT
// is unset, it defaults to a local collector at localhost:4317 over an
// insecure connection. With PrintToStderr it uses the stdout exporters
// instead, printing spans and metrics to stderr with no network calls. With
// neither, spans and metrics are dropped (no-op providers) so an installed
// command stays quiet. Logs always go to a plain readable JSON slog handler
// on stderr except in OTLP mode (which ships them through otelslog), since the
// stdout log exporter dumps raw internal record structs rather than a
// readable line.
//
// It also owns LogChatExchange, the one log call that emits conversation
// content: what it records is chosen at compile time by the cerebrai_dev
// build tag (see chatlog.go and chatlog_dev.go), since that content must
// never reach a collector from a build a user might run.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// otlpShutdownTimeout bounds how long a CLI invocation will wait to flush
// telemetry on exit when exporting via OTLP, so an unreachable collector
// adds a bounded delay rather than hanging on the default 10s OTLP export
// timeout. The stdout exporters never make network calls, so they need no
// such bound.
const otlpShutdownTimeout = 3 * time.Second

// EnvOTLPEndpoint is the standard OpenTelemetry variable naming the OTLP
// collector endpoint. The cerebrai CLI treats its presence as the signal to
// export via OTLP (Options.OTLP); with it unset the CLI falls back to
// --print-telemetry or stays silent.
const EnvOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

// Shutdown flushes and stops the telemetry providers. Call it before the
// process exits, e.g. via defer in main.
type Shutdown func(context.Context) error

// Options configures Setup.
type Options struct {
	// OTLP selects the OTLP/gRPC exporters for spans, metrics, and logs,
	// configured through the standard OTEL_EXPORTER_OTLP_* environment
	// variables. It takes precedence over PrintToStderr.
	OTLP bool

	// PrintToStderr prints spans and metrics to stderr via the stdout
	// exporters. Ignored when OTLP is set. When neither is set, spans and
	// metrics are dropped via no-op providers — an installed command should
	// not spew telemetry unless asked. Logs still go to a plain stderr slog
	// handler in that case.
	PrintToStderr bool

	// LogLevel is the minimum slog level (debug, info, warn, error) the
	// default logger emits. Invalid or empty values fall back to info.
	LogLevel string
}

// Setup configures global trace, meter, and logger providers for
// serviceName/version, installs a slog.Default logger (otelslog-backed in
// OTLP mode, a plain stderr JSON handler otherwise), and returns a Shutdown
// func to flush and release them.
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
	var loggerProvider *sdklog.LoggerProvider
	switch {
	case opts.OTLP:
		var logExporter sdklog.Exporter
		spanExporter, metricExporter, logExporter, err = newOTLPExporters(ctx)
		if err != nil {
			return nil, err
		}
		loggerProvider = sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
			sdklog.WithResource(res),
		)
	case opts.PrintToStderr:
		spanExporter, metricExporter, err = newStdoutExporters()
		if err != nil {
			return nil, err
		}
	}

	// A nil exporter leaves the provider with no batcher/reader, so spans and
	// metrics are simply dropped — the quiet default for an installed command.
	traceProviderOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if spanExporter != nil {
		traceProviderOpts = append(traceProviderOpts, sdktrace.WithBatcher(spanExporter))
	}
	tracerProvider := sdktrace.NewTracerProvider(traceProviderOpts...)

	meterProviderOpts := []metric.Option{metric.WithResource(res)}
	if metricExporter != nil {
		meterProviderOpts = append(meterProviderOpts, metric.WithReader(metric.NewPeriodicReader(metricExporter)))
	}
	meterProvider := metric.NewMeterProvider(meterProviderOpts...)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	var level slog.Level
	if err := level.UnmarshalText([]byte(opts.LogLevel)); err != nil {
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if opts.OTLP {
		logglobal.SetLoggerProvider(loggerProvider)
		handler = otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(loggerProvider))
	} else {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(&traceContextHandler{Handler: &levelHandler{level: level, Handler: handler}}))

	shutdown := func(ctx context.Context) error {
		if opts.OTLP {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, otlpShutdownTimeout)
			defer cancel()
		}
		errs := []error{
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		}
		if loggerProvider != nil {
			errs = append(errs, loggerProvider.Shutdown(ctx))
		}
		return errors.Join(errs...)
	}
	return shutdown, nil
}

// levelHandler filters out records below level before delegating, since
// otelslog.Handler itself applies no severity threshold.
type levelHandler struct {
	level slog.Level
	slog.Handler
}

func (h *levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level && h.Handler.Enabled(ctx, level)
}

// traceContextHandler adds trace_id/span_id attributes from the active span
// in ctx, so log lines can be correlated with the trace/span shown by the
// telemetry exporters. The otelslog handler does this on its own, but the
// plain JSON handler used for non-OTLP stdout logging does not.
type traceContextHandler struct {
	slog.Handler
}

func (h *traceContextHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, record)
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

func newOTLPExporters(ctx context.Context) (sdktrace.SpanExporter, metric.Exporter, sdklog.Exporter, error) {
	var traceOpts []otlptracegrpc.Option
	var metricOpts []otlpmetricgrpc.Option
	var logOpts []otlploggrpc.Option
	if _, ok := os.LookupEnv(EnvOTLPEndpoint); !ok {
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
		logOpts = append(logOpts,
			otlploggrpc.WithEndpoint("localhost:4317"),
			otlploggrpc.WithInsecure(),
			otlploggrpc.WithTimeout(otlpShutdownTimeout),
		)
	}

	spanExporter, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("telemetry: create OTLP trace exporter: %w", err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, metricOpts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("telemetry: create OTLP metric exporter: %w", err)
	}

	logExporter, err := otlploggrpc.New(ctx, logOpts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("telemetry: create OTLP log exporter: %w", err)
	}

	return spanExporter, metricExporter, logExporter, nil
}
