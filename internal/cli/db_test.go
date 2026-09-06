package cli

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/cerebrai-app/urban-carnival/internal/config"
	"github.com/cerebrai-app/urban-carnival/internal/devmode"
)

// TestDBMigrateCmdBuildsSchema runs `cerebrai db-migrate` against a fresh
// temp-file database and checks it left the seeded dev schema behind.
func TestDBMigrateCmdBuildsSchema(t *testing.T) {
	t.Setenv(devmode.EnvDevMode, "1")
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	t.Setenv(config.EnvDBPath, dbPath)

	cmd := newDBMigrateCmd()
	cmd.SetContext(context.Background())
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("db-migrate: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Any row proves the DDL created the table and the dev seeds ran; the
	// exact count lives in seeds/0001_dev_data.sql and is not this test's
	// concern.
	var seeded int
	if err := db.QueryRow(`SELECT COUNT(*) FROM automations`).Scan(&seeded); err != nil {
		t.Fatalf("count seeded automations: %v", err)
	}
	if seeded == 0 {
		t.Error("no seeded automations; expected dev seed data")
	}
}
