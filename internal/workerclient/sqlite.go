package workerclient

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLite is a Client backed by a persistent SQLite database (see
// internal/storage), used in place of a real IPC client until the
// background worker's local API exists (DESIGN.md §3, §9). It has no reply
// generation of its own yet, so SendMessage just echoes.
//
// It is safe for concurrent use: database/sql pools connections internally,
// and every query here is a single statement.
type SQLite struct {
	db *sql.DB
}

// NewSQLite wraps db as a Client. The first time it finds the automations
// table empty (a fresh database), it seeds it with a couple of illustrative
// automations so a new install isn't blank.
func NewSQLite(ctx context.Context, db *sql.DB) (*SQLite, error) {
	s := &SQLite{db: db}
	if err := s.seedIfEmpty(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// exampleAutomations returns a couple of illustrative automations
// timestamped at now, used to seed a fresh database.
func exampleAutomations(now time.Time) []Automation {
	return []Automation{
		{
			ID:          "water-plants",
			Name:        "Water the plants",
			Description: "Remind me to water plants every Tuesday morning.",
			Trigger:     "schedule: 0 9 * * 2",
			Enabled:     true,
			UpdatedAt:   now,
		},
		{
			ID:          "inbox-summary",
			Name:        "Inbox summary",
			Description: "Summarize my inbox every morning.",
			Trigger:     "schedule: 0 8 * * *",
			Enabled:     false,
			UpdatedAt:   now,
		},
	}
}

func (s *SQLite) seedIfEmpty(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM automations`).Scan(&count); err != nil {
		return fmt.Errorf("count automations: %w", err)
	}
	if count > 0 {
		return nil
	}

	// OR IGNORE, not INSERT: this count-then-insert isn't atomic across
	// separate database connections, so two processes opening the same
	// fresh database at once (e.g. the app launched twice before its first
	// run has seeded anything) can both reach here. Without OR IGNORE the
	// loser's insert would fail on the automations primary key and the
	// second launch would crash instead of just opening the existing data.
	for _, a := range exampleAutomations(time.Now()) {
		_, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO automations (id, name, description, trigger, enabled, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Description, a.Trigger, a.Enabled, formatTime(a.UpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("seed automation %q: %w", a.ID, err)
		}
	}
	return nil
}

// SendMessage echoes a placeholder assistant reply.
func (s *SQLite) SendMessage(_ context.Context, content string) (Message, error) {
	return Message{
		ID:        "echo",
		Role:      "assistant",
		Content:   fmt.Sprintf("(mock worker) you said: %q", content),
		CreatedAt: time.Now(),
	}, nil
}

// ListAutomations returns every automation in the database.
func (s *SQLite) ListAutomations(ctx context.Context) ([]Automation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, trigger, enabled, updated_at FROM automations ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("list automations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Automation
	for rows.Next() {
		var a Automation
		var updatedAt string
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.Trigger, &a.Enabled, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan automation: %w", err)
		}
		a.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse updated_at for automation %q: %w", a.ID, err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAutomationEnabled updates the enabled flag on the matching automation.
func (s *SQLite) SetAutomationEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE automations SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, formatTime(time.Now()), id,
	)
	if err != nil {
		return fmt.Errorf("update automation %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update automation %q: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("automation %q not found", id)
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}
