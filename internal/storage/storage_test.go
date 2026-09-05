package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/config"
)

func TestPathDevIsRepoRelative(t *testing.T) {
	t.Setenv(config.EnvDevSettings, "1")

	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got != fileName {
		t.Errorf("Path() = %q, want %q", got, fileName)
	}
}

func TestPathReleaseIsUnderAppDataDir(t *testing.T) {
	// Explicitly empty rather than merely unset, so the test is deterministic
	// even if EnvDevSettings happens to be set to true in the ambient environment.
	t.Setenv(config.EnvDevSettings, "")

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
	t.Setenv(config.EnvDevSettings, "1")
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
