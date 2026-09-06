// Package storage owns cerebrai's on-disk persistence: where the SQLite
// database lives (see Path), opening it with its schema up to date (see
// Open), and the SQLite app.Client that reads and writes its tables (see
// SQLite) — a stand-in for a real background-worker IPC client until that
// transport exists (DESIGN.md §3, §9).
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"modernc.org/sqlite"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed seeds/*.sql
var seedsFS embed.FS

// Open opens (creating if necessary) cerebrai's SQLite database at its
// resolved Path, brings its schema up to date via migrate, and returns the
// connection pool. Callers must Close it when done.
func Open(ctx context.Context) (*sql.DB, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	// foreign_keys is off by default in SQLite; every table cerebrai defines
	// is expected to declare its foreign keys correctly, so enforce them.
	// journal_mode(WAL) lets readers and the writer work without blocking
	// each other, and busy_timeout makes SQLite retry for a while instead of
	// immediately failing a query with SQLITE_BUSY when they do collide.
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database at %s: %w", path, err)
	}
	// SQLite only ever allows one writer at a time regardless of connection
	// count; pinning the pool to a single connection makes database/sql
	// queue callers for it rather than handing out a second connection that
	// would just contend with the first.
	db.SetMaxOpenConns(1)

	if err := migrate(ctx, db); err != nil {
		// The migration error is what matters here; a failure closing an
		// already-broken connection would just obscure it.
		_ = db.Close()
		return nil, fmt.Errorf("migrate database at %s: %w", path, err)
	}

	return db, nil
}

// Migrate opens cerebrai's database at its resolved Path, brings its schema
// up to date the same way Open does on every launch, closes it again, and
// returns the path it acted on. It backs the `cerebrai db-migrate` command,
// so a developer can pick up a migration or seed edit without starting the
// app. Like Open, it also applies the embedded seeds under seeds/ when
// devmode.Enabled.
func Migrate(ctx context.Context) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	db, err := Open(ctx)
	if err != nil {
		return "", err
	}
	return path, db.Close()
}

// migrate brings the database's schema up to date. Every embedded migration
// under migrations/ not yet recorded in schema_migrations is applied in
// filename order, each inside its own transaction. It is safe to run
// concurrently against the same database (see applyMigration): the
// migrationApplied check is only a fast path.
//
// A real build stops there, leaving a completely empty schema. A developer's
// checkout (devmode.Enabled) additionally applies the embedded files under
// seeds/, so a fresh dev database carries illustrative rows rather than being
// blank. Seed files are tracked in schema_migrations under a "seed:" version
// prefix so their entries can't collide with a real migration of the same
// filename.
func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	if err := applyEmbedded(ctx, db, migrationsFS, "migrations", ""); err != nil {
		return err
	}

	if devmode.Enabled() {
		if err := applyEmbedded(ctx, db, seedsFS, "seeds", "seed:"); err != nil {
			return err
		}
	}

	return nil
}

// applyEmbedded applies every *.sql file in dir of fsys that is not already
// recorded in schema_migrations, in filename order, recording each under
// versionPrefix + filename.
func applyEmbedded(ctx context.Context, db *sql.DB, fsys fs.FS, dir, versionPrefix string) error {
	// fs.ReadDir (and embed.FS's directory listing) already returns entries
	// sorted by filename, which is the apply order we want.
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	for _, entry := range entries {
		version := versionPrefix + entry.Name()

		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		sqlBytes, err := fs.ReadFile(fsys, dir+"/"+entry.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", version, err)
		}

		if err := applyMigration(ctx, db, version, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
	}

	return nil
}

func migrationApplied(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", name, err)
	}
	return count > 0, nil
}

// applyMigration runs one migration and records it in schema_migrations, all
// in a single transaction. The outer migrationApplied check in migrate is
// only a fast path: two processes opening the same fresh database at once can
// both pass it and reach here. So the transaction records the version first —
// that INSERT takes the database's write lock, and the version PRIMARY KEY
// makes the claim exclusive. The process that loses the race finds its own
// INSERT rejected by the unique constraint and skips the migration instead of
// running its DDL a second time (which would fail on, e.g., CREATE TABLE) and
// crashing the launch.
func applyMigration(ctx context.Context, db *sql.DB, name, sqlText string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
		if isConstraintViolation(err) {
			// Another process applied this migration between migrate's
			// check and now; its transaction owns the schema change.
			return nil
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return err
	}
	return tx.Commit()
}

// isConstraintViolation reports whether err is a SQLite constraint failure
// (primary result code SQLITE_CONSTRAINT), such as the unique-violation an
// INSERT hits when another process has already claimed a migration version.
func isConstraintViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	// modernc.org/sqlite reports extended result codes (primary code in the
	// low byte); SQLITE_CONSTRAINT is 19.
	return sqliteErr.Code()&0xFF == 19
}
