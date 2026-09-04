package cli

import (
	"log/slog"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/cerebrai-app/urban-carnival/internal/telemetry"
	"github.com/cerebrai-app/urban-carnival/internal/version"
)

var tracer = otel.Tracer("github.com/cerebrai-app/urban-carnival/internal/cli")

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the cerebrai version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, span := tracer.Start(cmd.Context(), "version")
			defer span.End()

			slog.LogAttrs(ctx, slog.LevelInfo, "resolved cerebrai version",
				append(telemetry.TraceAttrs(ctx), slog.String("version", version.Version))...,
			)

			cmd.Println(version.String())
			return nil
		},
	}
}
