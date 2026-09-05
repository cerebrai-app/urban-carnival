package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/cerebrai-app/urban-carnival/internal/app"
	"github.com/cerebrai-app/urban-carnival/internal/chat"
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

// SQLite is an app.Client backed by the persistent SQLite database this
// package opens (see Open), used in place of a real background-worker IPC
// client until that transport exists (DESIGN.md §3, §9).
//
// It is safe for concurrent use: database/sql pools connections internally,
// and every query here is a single statement.
type SQLite struct {
	db *sql.DB
	// reply turns a session's model plus its message history into an
	// assistant reply (DESIGN.md §5.2). Defaults to the chat package's
	// provider-backed implementation; tests inject a stub via WithReplier so
	// they never shell out to a real model.
	reply Replier
}

var _ app.Client = (*SQLite)(nil)

// Replier generates an assistant reply for a session from its model ID and
// full message history (oldest first, including the just-sent user message).
type Replier func(ctx context.Context, model string, history []app.Message) (string, error)

// Option configures a SQLite at construction time.
type Option func(*SQLite)

// WithReplier overrides how SendMessage generates assistant replies. Mainly
// for tests: the default reaches a real chat model provider.
func WithReplier(r Replier) Option {
	return func(s *SQLite) { s.reply = r }
}

// defaultReplier resolves the session's model to a chat provider and asks it
// for a single reply (DESIGN.md §5.2). In dev builds the provider is the
// local Claude Code CLI wired to the in-process MCP server (see
// internal/devmode/devmcp); otherwise it's chat.Unconfigured and this
// returns chat.ErrNotConfigured.
func defaultReplier(ctx context.Context, model string, history []app.Message) (string, error) {
	return chat.Reply(ctx, chat.ProviderFor(model), history)
}

// NewSQLite wraps db as an app.Client. In a developer's checkout the
// database's migrations (see Open) also load a couple of illustrative
// automations so a fresh install isn't blank; a real build's schema starts
// empty.
func NewSQLite(db *sql.DB, opts ...Option) *SQLite {
	s := &SQLite{db: db, reply: defaultReplier}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// newID returns a random identifier suitable for a session or message
// primary key.
func newID() string {
	return rand.Text()
}

// CreateSession starts a new, empty conversation thread. An empty title
// falls back to a placeholder that SendMessage replaces once the session's
// first message arrives. The session's model is initialized to
// chat.DefaultModel; SetSessionModel changes it afterward.
func (s *SQLite) CreateSession(ctx context.Context, title string) (app.Session, error) {
	if title == "" {
		title = newSessionTitle
	}
	now := time.Now()
	session := app.Session{ID: newID(), Title: title, Model: chat.DefaultModel(), CreatedAt: now, UpdatedAt: now}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, title, model, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.Title, session.Model, formatTime(session.CreatedAt), formatTime(session.UpdatedAt),
	)
	if err != nil {
		return app.Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// ListSessions returns every conversation thread, most recently active
// first, so the session list surfaces whatever the user was just doing.
func (s *SQLite) ListSessions(ctx context.Context) ([]app.Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, model, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []app.Session
	for rows.Next() {
		var sess app.Session
		var createdAt, updatedAt string
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.Model, &createdAt, &updatedAt); err != nil {
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

// SetSessionModel updates which model a session's future replies should be
// generated with. It does not bump updated_at: switching models is not
// "activity" that should reorder the session list.
func (s *SQLite) SetSessionModel(ctx context.Context, sessionID, model string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET model = ? WHERE id = ?`, model, sessionID,
	)
	if err != nil {
		return fmt.Errorf("set model for session %q: %w", sessionID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set model for session %q: %w", sessionID, err)
	}
	if n == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// ListMessages returns every message in the given session, oldest first.
func (s *SQLite) ListMessages(ctx context.Context, sessionID string) ([]app.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, created_at FROM messages WHERE session_id = ? ORDER BY created_at, rowid`,
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []app.Message
	for rows.Next() {
		var m app.Message
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
func (s *SQLite) insertMessage(ctx context.Context, m app.Message) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.Role, m.Content, formatTime(m.CreatedAt),
	)
	return err
}

// sessionModel returns the chat model ID a session's replies should be
// generated with. An unknown session ID is an error.
func (s *SQLite) sessionModel(ctx context.Context, sessionID string) (string, error) {
	var model string
	err := s.db.QueryRowContext(ctx, `SELECT model FROM sessions WHERE id = ?`, sessionID).Scan(&model)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	if err != nil {
		return "", fmt.Errorf("look up model for session %q: %w", sessionID, err)
	}
	return model, nil
}

// SendMessage persists the user's message, generates an assistant reply via
// the session's chat model (DESIGN.md §5.2), persists that too, and bumps the
// session's updated_at so it sorts to the top of ListSessions.
func (s *SQLite) SendMessage(ctx context.Context, sessionID, content string) (app.Message, error) {
	model, err := s.sessionModel(ctx, sessionID)
	if err != nil {
		return app.Message{}, err
	}

	// The reply is generated from the whole conversation, so load the prior
	// turns before appending this one.
	history, err := s.ListMessages(ctx, sessionID)
	if err != nil {
		return app.Message{}, fmt.Errorf("load session history: %w", err)
	}

	now := time.Now()
	userMessage := app.Message{ID: newID(), SessionID: sessionID, Role: "user", Content: content, CreatedAt: now}
	if err := s.insertMessage(ctx, userMessage); err != nil {
		return app.Message{}, fmt.Errorf("save user message: %w", err)
	}
	history = append(history, userMessage)

	replyText, err := s.reply(ctx, model, history)
	if err != nil {
		return app.Message{}, fmt.Errorf("generate reply: %w", err)
	}

	reply := app.Message{
		ID:        newID(),
		SessionID: sessionID,
		Role:      "assistant",
		Content:   replyText,
		CreatedAt: time.Now(),
	}
	if err := s.insertMessage(ctx, reply); err != nil {
		return app.Message{}, fmt.Errorf("save assistant reply: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET updated_at = ? WHERE id = ?`, formatTime(reply.CreatedAt), sessionID,
	); err != nil {
		return app.Message{}, fmt.Errorf("touch session %q: %w", sessionID, err)
	}

	// A session starts with a placeholder title; once its first message
	// arrives, retitle it from that message so the session list is
	// meaningful instead of a wall of identical placeholders.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET title = ? WHERE id = ? AND title = ?`,
		titleFromContent(content), sessionID, newSessionTitle,
	); err != nil {
		return app.Message{}, fmt.Errorf("retitle session %q: %w", sessionID, err)
	}

	return reply, nil
}

// automationColumns is the column list every automation read selects, in the
// order scanAutomation expects.
const automationColumns = `id, name, description, trigger, enabled, source, updated_at`

// scanAutomation reads one automation row (automationColumns order) from src,
// which is a *sql.Row or *sql.Rows.
func scanAutomation(src interface{ Scan(...any) error }) (app.Automation, error) {
	var a app.Automation
	var updatedAt string
	if err := src.Scan(&a.ID, &a.Name, &a.Description, &a.Trigger, &a.Enabled, &a.Source, &updatedAt); err != nil {
		return app.Automation{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return app.Automation{}, fmt.Errorf("parse updated_at for automation %q: %w", a.ID, err)
	}
	a.UpdatedAt = parsed
	return a, nil
}

// ListAutomations returns every automation in the database.
func (s *SQLite) ListAutomations(ctx context.Context) ([]app.Automation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+automationColumns+` FROM automations ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("list automations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []app.Automation
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan automation: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAutomation returns a single automation, including its authored source.
// An unknown ID is an error.
func (s *SQLite) GetAutomation(ctx context.Context, id string) (app.Automation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+automationColumns+` FROM automations WHERE id = ?`, id)
	a, err := scanAutomation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return app.Automation{}, fmt.Errorf("automation %q not found", id)
	}
	if err != nil {
		return app.Automation{}, fmt.Errorf("get automation %q: %w", id, err)
	}
	return a, nil
}

// CreateAutomation persists a freshly authored automation draft (DESIGN.md
// §5.5). The store assigns the ID, forces it disabled (it's a draft awaiting
// §4's review flow), and stamps updated_at.
func (s *SQLite) CreateAutomation(ctx context.Context, d app.AutomationDraft) (app.Automation, error) {
	a := app.Automation{
		ID:          newID(),
		Name:        d.Name,
		Description: d.Description,
		Trigger:     d.Trigger,
		Enabled:     false,
		Source:      d.Source,
		UpdatedAt:   time.Now(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO automations (id, name, description, trigger, enabled, source, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Description, a.Trigger, a.Enabled, a.Source, formatTime(a.UpdatedAt),
	); err != nil {
		return app.Automation{}, fmt.Errorf("create automation: %w", err)
	}
	return a, nil
}

// UpdateAutomation overwrites an existing automation's metadata and source
// (DESIGN.md §5.5's edit_automation) and stamps updated_at. It does not touch
// the enabled flag. An unknown ID is an error.
func (s *SQLite) UpdateAutomation(ctx context.Context, a app.Automation) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE automations SET name = ?, description = ?, trigger = ?, source = ?, updated_at = ? WHERE id = ?`,
		a.Name, a.Description, a.Trigger, a.Source, formatTime(time.Now()), a.ID,
	)
	if err != nil {
		return fmt.Errorf("update automation %q: %w", a.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update automation %q: %w", a.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("automation %q not found", a.ID)
	}
	return nil
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
