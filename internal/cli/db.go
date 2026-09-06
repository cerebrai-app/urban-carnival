package cli

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/cerebrai-app/urban-carnival/internal/storage"
)

func newDBMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-migrate",
		Short: "Bring cerebrai's database schema up to date",
		Long: "db-migrate applies every embedded migration not yet recorded in " +
			"cerebrai's SQLite database, in filename order. In a developer build " +
			"(CEREBRAI_DEV_MODE) it also applies the seed data. This is the same " +
			"step the app runs on every launch; run it directly after editing a " +
			"migration or seed file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, span := tracer.Start(cmd.Context(), "db-migrate")
			defer span.End()

			path, err := storage.Migrate(ctx)
			if err != nil {
				return err
			}

			slog.InfoContext(ctx, "database schema up to date", "path", path)
			cmd.Printf("database schema up to date at %s\n", path)
			return nil
		},
	}
}
