package cli

import (
	"log/slog"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/cerebrai-app/urban-carnival/internal/config"
)

var tracer = otel.Tracer("github.com/cerebrai-app/urban-carnival/internal/cli")

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the cerebrai version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, span := tracer.Start(cmd.Context(), "version")
			defer span.End()

			slog.InfoContext(ctx, "resolved cerebrai version", "version", config.Version)

			cmd.Println(config.String())
			return nil
		},
	}
}
