package workerclient

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// newSessionTitle is the placeholder title a freshly created session holds
// until its first message retitles it (see SendMessage).
const newSessionTitle = "New chat"

// titleFromContent derives a session's display title from its first
// message: a single line, truncated so it reads well in a narrow list.
func titleFromContent(content string) string {
	const maxRunes = 60
	for i, r := range content {
		if r == '\n' {
			content = content[:i]
			break
		}
	}
	runes := []rune(content)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return content
}

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

// NewSQLite wraps db as a Client. The database's schema migrations
// (internal/storage) also seed a couple of illustrative automations so a
// fresh install isn't blank.
func NewSQLite(db *sql.DB) *SQLite {
	return &SQLite{db: db}
}

// newID returns a random identifier suitable for a session or message
// primary key.
func newID() string {
	return rand.Text()
}

// CreateSession starts a new, empty conversation thread. An empty title
// falls back to a placeholder that SendMessage replaces once the session's
// first message arrives.
func (s *SQLite) CreateSession(ctx context.Context, title string) (Session, error) {
	if title == "" {
		title = newSessionTitle
	}
	now := time.Now()
	session := Session{ID: newID(), Title: title, CreatedAt: now, UpdatedAt: now}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		session.ID, session.Title, formatTime(session.CreatedAt), formatTime(session.UpdatedAt),
	)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// ListSessions returns every conversation thread, most recently active
// first, so the session list surfaces whatever the user was just doing.
func (s *SQLite) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Session
	for rows.Next() {
		var sess Session
		var createdAt, updatedAt string
		if err := rows.Scan(&sess.ID, &sess.Title, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		if sess.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at for session %q: %w", sess.ID, err)
		}
		if sess.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at for session %q: %w", sess.ID, err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// ListMessages returns every message in the given session, oldest first.
func (s *SQLite) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, created_at FROM messages WHERE session_id = ? ORDER BY created_at, rowid`,
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Message
	for rows.Next() {
		var m Message
		var createdAt string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &createdAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		if m.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at for message %q: %w", m.ID, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// insertMessage persists one message row.
func (s *SQLite) insertMessage(ctx context.Context, m Message) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.Role, m.Content, formatTime(m.CreatedAt),
	)
	return err
}

// SendMessage persists the user's message, generates a placeholder
// assistant reply (echo), persists that too, and bumps the session's
// updated_at so it sorts to the top of ListSessions.
func (s *SQLite) SendMessage(ctx context.Context, sessionID, content string) (Message, error) {
	now := time.Now()
	userMessage := Message{ID: newID(), SessionID: sessionID, Role: "user", Content: content, CreatedAt: now}
	if err := s.insertMessage(ctx, userMessage); err != nil {
		return Message{}, fmt.Errorf("save user message: %w", err)
	}

	reply := Message{
		ID:        newID(),
		SessionID: sessionID,
		Role:      "assistant",
		Content:   fmt.Sprintf("(mock worker) you said: %q", content),
		CreatedAt: time.Now(),
	}
	if err := s.insertMessage(ctx, reply); err != nil {
		return Message{}, fmt.Errorf("save assistant reply: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET updated_at = ? WHERE id = ?`, formatTime(reply.CreatedAt), sessionID,
	); err != nil {
		return Message{}, fmt.Errorf("touch session %q: %w", sessionID, err)
	}

	// A session starts with a placeholder title; once its first message
	// arrives, retitle it from that message so the session list is
	// meaningful instead of a wall of identical placeholders.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET title = ? WHERE id = ? AND title = ?`,
		titleFromContent(content), sessionID, newSessionTitle,
	); err != nil {
		return Message{}, fmt.Errorf("retitle session %q: %w", sessionID, err)
	}

	return reply, nil
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
