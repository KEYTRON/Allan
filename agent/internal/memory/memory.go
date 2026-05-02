package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Memory struct {
	db *sql.DB
}

type Session struct {
	ID         string
	StartedAt  time.Time
	EndedAt    *time.Time
	Backend    string
	Model      string
	ToolCalls  int
	TokensIn   int
	TokensOut  int
}

type ConvMessage struct {
	ID        int64
	SessionID string
	Role      string
	Content   string
	Timestamp time.Time
}

func Open(dbPath string) (*Memory, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	m := &Memory{db: db}
	if err := m.migrate(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Memory) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			started_at DATETIME,
			ended_at DATETIME,
			backend TEXT,
			model TEXT,
			tool_calls INTEGER DEFAULT 0,
			tokens_in INTEGER DEFAULT 0,
			tokens_out INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			role TEXT,
			content TEXT,
			timestamp DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_session ON conversations(session_id)`,
		`CREATE TABLE IF NOT EXISTS skills (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			trigger TEXT,
			solution TEXT,
			tool_sequence TEXT,
			created_at DATETIME,
			used_count INTEGER DEFAULT 0,
			avg_tokens_saved INTEGER DEFAULT 0,
			embedding BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS facts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			content TEXT,
			created_at DATETIME
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5(content, content='facts', content_rowid='id')`,
	}
	for _, s := range stmts {
		if _, err := m.db.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w (stmt: %s)", err, s)
		}
	}
	return nil
}

func (m *Memory) Close() error { return m.db.Close() }

func (m *Memory) DB() *sql.DB { return m.db }

func (m *Memory) StartSession(ctx context.Context, s *Session) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO sessions (id, started_at, backend, model) VALUES (?, ?, ?, ?)`,
		s.ID, s.StartedAt, s.Backend, s.Model)
	return err
}

func (m *Memory) EndSession(ctx context.Context, s *Session) error {
	now := time.Now()
	s.EndedAt = &now
	_, err := m.db.ExecContext(ctx,
		`UPDATE sessions SET ended_at = ?, tool_calls = ?, tokens_in = ?, tokens_out = ? WHERE id = ?`,
		now, s.ToolCalls, s.TokensIn, s.TokensOut, s.ID)
	return err
}

func (m *Memory) AppendMessage(ctx context.Context, sessionID, role, content string) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO conversations (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		sessionID, role, content, time.Now())
	return err
}

func (m *Memory) LastSessionMessages(ctx context.Context, n int) ([]ConvMessage, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT id FROM sessions WHERE ended_at IS NOT NULL ORDER BY started_at DESC LIMIT 1`)
	var sid string
	if err := row.Scan(&sid); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, timestamp FROM conversations WHERE session_id = ? ORDER BY id DESC LIMIT ?`,
		sid, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConvMessage
	for rows.Next() {
		var cm ConvMessage
		if err := rows.Scan(&cm.ID, &cm.SessionID, &cm.Role, &cm.Content, &cm.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, cm)
	}
	// Reverse to chronological order
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (m *Memory) AddFact(ctx context.Context, sessionID, content string) error {
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO facts (session_id, content, created_at) VALUES (?, ?, ?)`,
		sessionID, content, time.Now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO facts_fts (rowid, content) VALUES (?, ?)`, id, content)
	return err
}

func (m *Memory) SearchFacts(ctx context.Context, query string, limit int) ([]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT content FROM facts_fts WHERE facts_fts MATCH ? LIMIT ?`,
		query, limit)
	if err != nil {
		// FTS may fail on bad queries; fallback to LIKE
		rows, err = m.db.QueryContext(ctx,
			`SELECT content FROM facts WHERE content LIKE ? LIMIT ?`,
			"%"+query+"%", limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
