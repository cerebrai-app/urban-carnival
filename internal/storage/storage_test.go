package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/config"
)

func TestPathDevIsRepoRelative(t *testing.T) {
	t.Setenv(config.EnvDevMode, "1")
	t.Setenv(config.EnvDBPath, "")

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != fileName {
		t.Errorf("Path() = %q, want %q", got, fileName)
	}
}

func TestPathHonorsExplicitOverride(t *testing.T) {
	// The override wins even with dev settings on, which would otherwise
	// resolve to a bare repo-relative fileName.
	t.Setenv(config.EnvDevMode, "1")
	want := filepath.Join(t.TempDir(), "pinned.db")
	t.Setenv(config.EnvDBPath, want)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPathReleaseIsUnderAppDataDir(t *testing.T) {
	// Explicitly empty rather than merely unset, so the test is deterministic
	// even if EnvDevMode happens to be set to true in the ambient environment.
	t.Setenv(config.EnvDevMode, "")
	t.Setenv(config.EnvDBPath, "")

	wantDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir on this platform: %v", err)
	}

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(wantDir, "cerebrai", fileName)
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// TestOpenAppliesMigrationsAndIsIdempotent runs Open twice against the same
// on-disk file (as a restarted app would) and checks the schema it left
// behind, guarding against a re-applied migration erroring on the second run.
func TestOpenAppliesMigrationsAndIsIdempotent(t *testing.T) {
	t.Setenv(config.EnvDevMode, "1")
	t.Setenv(config.EnvDBPath, "")
	t.Chdir(t.TempDir())

	ctx := context.Background()

	db, err := Open(ctx)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// The 0001 migration seeds two example automations on a fresh database.
	var seeded int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM automations`).Scan(&seeded); err != nil {
		t.Fatalf("count seeded automations: %v", err)
	}
	if seeded != 2 {
		t.Errorf("seeded automations = %d, want 2", seeded)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO automations (id, name, description, trigger, enabled, updated_at)
		VALUES ('a', 'A', 'desc', 'manual', 1, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert into migrated table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := Open(ctx)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() {
		if err := db2.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	var count int
	if err := db2.QueryRowContext(ctx, `SELECT COUNT(*) FROM automations`).Scan(&count); err != nil {
		t.Fatalf("query after reopen: %v", err)
	}
	if count != 3 {
		t.Errorf("automations count after reopen = %d, want 3 (2 seeded + 1 inserted; data should persist across Open calls)", count)
	}
}

// TestApplyMigrationSkipsVersionClaimedConcurrently reproduces the race
// between two processes opening the same fresh database: both pass the outer
// migrationApplied check, then one commits the migration while the other is
// still inside applyMigration. The straggler must notice the version is now
// taken and skip its DDL rather than re-run it (and fail on, e.g., a
// duplicate CREATE TABLE).
func TestApplyMigrationSkipsVersionClaimedConcurrently(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	const version = "0001_test.sql"
	const ddl = `CREATE TABLE widgets (id TEXT PRIMARY KEY)`

	// The winning process: schema_migrations + a fully applied migration.
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if err := applyMigration(ctx, db, version, ddl); err != nil {
		t.Fatalf("first applyMigration: %v", err)
	}

	// The straggler, running the same migration it wrongly believes is
	// pending: it must return nil (claim already taken), not an error.
	if err := applyMigration(ctx, db, version, ddl); err != nil {
		t.Errorf("second applyMigration on already-claimed version: %v", err)
	}

	var tables int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'widgets'`).Scan(&tables); err != nil {
		t.Fatalf("count widgets table: %v", err)
	}
	if tables != 1 {
		t.Errorf("widgets table count = %d, want 1", tables)
	}
}
