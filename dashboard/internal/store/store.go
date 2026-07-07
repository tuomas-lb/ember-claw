// Package store persists dashboard chat history in PostgreSQL.
//
// It is optional: when DATABASE_URL is not configured the dashboard runs with
// a nil *Store and all methods are safe no-ops via the guards in the callers.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Message is a single persisted chat message.
type Message struct {
	ID        int64     `json:"id"`
	Instance  string    `json:"instance"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // "user" | "agent"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Session summarizes a conversation thread for an instance.
type Session struct {
	SessionID    string    `json:"session_id"`
	MessageCount int       `json:"message_count"`
	LastActivity time.Time `json:"last_activity"`
}

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New connects to PostgreSQL using the given URL and ensures the schema exists.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	// Verify connectivity early with a short timeout.
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) migrate(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS chat_messages (
    id         BIGSERIAL PRIMARY KEY,
    instance   TEXT        NOT NULL,
    session_id TEXT        NOT NULL,
    role       TEXT        NOT NULL,
    content    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chat_instance_session_time
    ON chat_messages (instance, session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_instance_time
    ON chat_messages (instance, created_at);
`
	if _, err := s.pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// AddMessage persists a single chat message and returns its stored row.
func (s *Store) AddMessage(ctx context.Context, instance, sessionID, role, content string) (*Message, error) {
	m := &Message{Instance: instance, SessionID: sessionID, Role: role, Content: content}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO chat_messages (instance, session_id, role, content)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		instance, sessionID, role, content,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	return m, nil
}

// ListMessages returns messages for an instance/session ordered oldest-first.
// If sessionID is empty, messages across all sessions for the instance are returned.
// limit caps the number of most-recent messages (0 or negative → default 500).
func (s *Store) ListMessages(ctx context.Context, instance, sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 500
	}

	// Fetch the newest `limit` rows, then return them oldest-first so the UI
	// renders in chronological order.
	query := `SELECT id, instance, session_id, role, content, created_at FROM (
	              SELECT * FROM chat_messages
	              WHERE instance = $1` + `
	              %s
	              ORDER BY created_at DESC, id DESC
	              LIMIT %s
	          ) t ORDER BY created_at ASC, id ASC`
	args := []any{instance}
	var sessionClause, limitPlaceholder string
	if sessionID == "" {
		sessionClause = ""
		args = append(args, limit)
		limitPlaceholder = "$2"
	} else {
		sessionClause = "AND session_id = $2"
		args = append(args, sessionID, limit)
		limitPlaceholder = "$3"
	}
	query = fmt.Sprintf(query, sessionClause, limitPlaceholder)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	out := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Instance, &m.SessionID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListSessions returns the distinct sessions for an instance, most-recent first.
func (s *Store) ListSessions(ctx context.Context, instance string) ([]Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT session_id, COUNT(*), MAX(created_at)
		 FROM chat_messages
		 WHERE instance = $1
		 GROUP BY session_id
		 ORDER BY MAX(created_at) DESC`,
		instance)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	out := make([]Session, 0)
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.SessionID, &sess.MessageCount, &sess.LastActivity); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}
