// Package cli defines the cerebrai command tree.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cerebrai-app/urban-carnival/internal/config"
	"github.com/cerebrai-app/urban-carnival/internal/telemetry"
)

type shutdownKey struct{}

var (
	logLevel       string
	printTelemetry bool
)

// Execute builds the root command and runs it with ctx, returning any error
// from the invoked subcommand.
func Execute(ctx context.Context) error {
	return newRootCmd().ExecuteContext(ctx)
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "cerebrai",
		Short:         "cerebrai is the cerebrai command-line tool",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Spans and metrics are exported via OTLP when a collector endpoint
			// is configured (OTEL_EXPORTER_OTLP_ENDPOINT), which a developer's
			// checkout sets in .env. Otherwise they are printed to stderr only
			// when --print-telemetry is passed; an installed CLI with neither
			// stays quiet. Logs always go to stderr.
			//
			// Only the general endpoint variable is consulted, not the
			// per-signal OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT
			// overrides; set the general one to opt into OTLP.
			otlp := os.Getenv(telemetry.EnvOTLPEndpoint) != ""
			shutdown, err := telemetry.Setup(cmd.Context(), "cerebrai", config.Version, telemetry.Options{
				OTLP:          otlp,
				PrintToStderr: printTelemetry,
				LogLevel:      logLevel,
			})
			if err != nil {
				return fmt.Errorf("setup telemetry: %w", err)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), shutdownKey{}, shutdown))
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			shutdown, ok := cmd.Context().Value(shutdownKey{}).(telemetry.Shutdown)
			if !ok {
				return nil
			}
			// A telemetry backend being unreachable (e.g. no local collector
			// running) must never fail the command itself. This is printed
			// directly to stderr rather than logged via slog: in OTLP mode
			// slog.Default() ships records through the very backend that may
			// have just failed, so a slog.Warn here could be silently lost.
			if err := shutdown(cmd.Context()); err != nil {
				fmt.Fprintln(os.Stderr, "telemetry shutdown:", err)
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	cmd.PersistentFlags().BoolVar(&printTelemetry, "print-telemetry", false, "print spans and metrics to stderr (ignored when an OTLP endpoint is configured)")

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newDBMigrateCmd())
	return cmd
}
