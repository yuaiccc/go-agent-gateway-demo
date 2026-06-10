package tool

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type MemoryStore struct {
	db *sql.DB
}

func NewMemoryStore(db *sql.DB) *MemoryStore {
	return &MemoryStore{db: db}
}

func (m *MemoryStore) Search(ctx context.Context, tenantID, userID, query string, topK int) (map[string]any, error) {
	if topK <= 0 {
		topK = 3
	}
	like := "%" + query + "%"
	rows, err := m.db.QueryContext(
		ctx,
		`SELECT id, content, tags, created_at
		 FROM memories
		 WHERE tenant_id = ? AND user_id = ? AND (content LIKE ? OR tags LIKE ?)
		 ORDER BY created_at DESC
		 LIMIT ?`,
		tenantID,
		userID,
		like,
		like,
		topK,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []map[string]any{}
	for rows.Next() {
		var id, content, tags, createdAt string
		if err := rows.Scan(&id, &content, &tags, &createdAt); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"memory_id":  id,
			"content":    content,
			"tags":       splitTags(tags),
			"created_at": createdAt,
			"score":      scoreMemory(content+" "+tags, query),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(results) > 0 {
		return map[string]any{"results": results}, nil
	}

	fallback, err := m.latest(ctx, tenantID, userID, topK)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"results": fallback,
		"hint":    "没有精确命中，返回最近记忆作为上下文。",
	}, nil
}

func (m *MemoryStore) latest(ctx context.Context, tenantID, userID string, topK int) ([]map[string]any, error) {
	rows, err := m.db.QueryContext(
		ctx,
		`SELECT id, content, tags, created_at
		 FROM memories
		 WHERE tenant_id = ? AND user_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		tenantID,
		userID,
		topK,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []map[string]any{}
	for rows.Next() {
		var id, content, tags, createdAt string
		if err := rows.Scan(&id, &content, &tags, &createdAt); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"memory_id":  id,
			"content":    content,
			"tags":       splitTags(tags),
			"created_at": createdAt,
			"score":      0.2,
		})
	}
	return results, rows.Err()
}

func scoreMemory(blob, query string) float64 {
	blob = strings.ToLower(blob)
	query = strings.ToLower(query)
	if query == "" {
		return 0.2
	}
	score := 0.1
	for _, part := range strings.Fields(query) {
		if strings.Contains(blob, part) {
			score += 0.25
		}
	}
	if score > 0.95 {
		return 0.95
	}
	return score
}

func splitTags(tags string) []string {
	if tags == "" {
		return []string{}
	}
	parts := strings.Split(tags, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func intArg(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case jsonNumber:
		i, err := value.Int64()
		if err == nil {
			return int(i)
		}
	}
	return fallback
}

type jsonNumber interface {
	Int64() (int64, error)
}

func ownerArgs(args map[string]any) (string, string, error) {
	tenantID := stringArg(args, "tenant_id")
	userID := stringArg(args, "user_id")
	if tenantID == "" || userID == "" {
		return "", "", fmt.Errorf("tenant_id and user_id are required")
	}
	return tenantID, userID, nil
}
