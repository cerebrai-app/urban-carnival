// Package cli defines the cerebrai command tree.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/cerebrai-app/urban-carnival/internal/telemetry"
	"github.com/cerebrai-app/urban-carnival/internal/version"
)

type shutdownKey struct{}

var (
	logLevel string
	otlp     bool
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
			setupLogger(logLevel)

			shutdown, err := telemetry.Setup(cmd.Context(), "cerebrai", version.Version, telemetry.Options{OTLP: otlp})
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
			// running) must never fail the command itself.
			if err := shutdown(cmd.Context()); err != nil {
				slog.Warn("telemetry shutdown", "error", err)
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	cmd.PersistentFlags().BoolVar(&otlp, "otlp", false, "export telemetry via OTLP/gRPC instead of printing it to stderr")

	cmd.AddCommand(newVersionCmd())
	return cmd
}

func setupLogger(level string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
