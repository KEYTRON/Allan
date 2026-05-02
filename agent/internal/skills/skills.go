package skills

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/keytron/allan/agent/internal/vector"
)

type Skill struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Trigger        string    `json:"trigger"`
	Solution       string    `json:"solution"`
	ToolSequence   []string  `json:"tool_sequence"`
	CreatedAt      time.Time `json:"created_at"`
	UsedCount      int       `json:"used_count"`
	AvgTokensSaved int       `json:"avg_tokens_saved"`
}

type Engine struct {
	db    *sql.DB
	vec   vector.Store
	colName string
}

func NewEngine(db *sql.DB, vec vector.Store) *Engine {
	return &Engine{db: db, vec: vec, colName: "skills"}
}

func (e *Engine) Save(ctx context.Context, s *Skill) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	seq, _ := json.Marshal(s.ToolSequence)
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO skills (id, name, description, trigger, solution, tool_sequence, created_at, used_count, avg_tokens_saved) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Description, s.Trigger, s.Solution, string(seq), s.CreatedAt, s.UsedCount, s.AvgTokensSaved)
	if err != nil {
		return err
	}
	if e.vec != nil && e.vec.Healthy(ctx) {
		text := s.Name + "\n" + s.Description + "\n" + s.Trigger + "\n" + s.Solution
		_ = e.vec.Add(ctx, e.colName, s.ID, text, map[string]any{
			"name":        s.Name,
			"description": s.Description,
		})
	}
	return nil
}

func (e *Engine) List(ctx context.Context) ([]*Skill, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT id, name, description, trigger, solution, tool_sequence, created_at, used_count, avg_tokens_saved FROM skills ORDER BY used_count DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Skill
	for rows.Next() {
		s := &Skill{}
		var seq string
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Trigger, &s.Solution, &seq, &s.CreatedAt, &s.UsedCount, &s.AvgTokensSaved); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(seq), &s.ToolSequence)
		out = append(out, s)
	}
	return out, nil
}

func (e *Engine) Get(ctx context.Context, id string) (*Skill, error) {
	row := e.db.QueryRowContext(ctx,
		`SELECT id, name, description, trigger, solution, tool_sequence, created_at, used_count, avg_tokens_saved FROM skills WHERE id = ?`, id)
	s := &Skill{}
	var seq string
	if err := row.Scan(&s.ID, &s.Name, &s.Description, &s.Trigger, &s.Solution, &seq, &s.CreatedAt, &s.UsedCount, &s.AvgTokensSaved); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(seq), &s.ToolSequence)
	return s, nil
}

func (e *Engine) Delete(ctx context.Context, id string) error {
	_, err := e.db.ExecContext(ctx, `DELETE FROM skills WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if e.vec != nil && e.vec.Healthy(ctx) {
		_ = e.vec.Delete(ctx, e.colName, id)
	}
	return nil
}

func (e *Engine) Search(ctx context.Context, query string, topK int, threshold float32) ([]*Skill, error) {
	if e.vec != nil && e.vec.Healthy(ctx) {
		results, err := e.vec.Query(ctx, e.colName, query, topK)
		if err == nil {
			var out []*Skill
			for _, r := range results {
				if r.Score < threshold {
					continue
				}
				s, err := e.Get(ctx, r.ID)
				if err != nil {
					continue
				}
				out = append(out, s)
			}
			return out, nil
		}
	}
	// Fallback: keyword match against trigger/description
	return e.keywordSearch(ctx, query, topK)
}

func (e *Engine) keywordSearch(ctx context.Context, query string, limit int) ([]*Skill, error) {
	q := "%" + strings.ToLower(query) + "%"
	rows, err := e.db.QueryContext(ctx,
		`SELECT id FROM skills WHERE LOWER(trigger) LIKE ? OR LOWER(description) LIKE ? ORDER BY used_count DESC LIMIT ?`,
		q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	var out []*Skill
	for _, id := range ids {
		s, err := e.Get(ctx, id)
		if err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (e *Engine) IncUsed(ctx context.Context, id string, tokensSaved int) error {
	_, err := e.db.ExecContext(ctx,
		`UPDATE skills SET used_count = used_count + 1, avg_tokens_saved = (avg_tokens_saved * used_count + ?) / (used_count + 1) WHERE id = ?`,
		tokensSaved, id)
	return err
}

func (e *Engine) Export(ctx context.Context) (string, error) {
	skills, err := e.List(ctx)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(skills, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
