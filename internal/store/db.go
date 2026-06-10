package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	SQL *sql.DB
}

func Open(path string) (*DB, error) {
	if path == "" {
		path = os.Getenv("GATEWAY_DB_PATH")
	}
	if path == "" {
		path = filepath.Join(findProjectRoot(), "data", "gateway.sqlite")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	db := &DB{SQL: sqlDB}
	if err := db.Migrate(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := db.Seed(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func findProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "."
		}
		cwd = parent
	}
}

func (db *DB) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, id)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			content TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_owner ON memories(tenant_id, user_id, created_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.SQL.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) Seed(ctx context.Context) error {
	var count int
	if err := db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().Format(time.RFC3339Nano)
	memories := []struct {
		id       string
		tenantID string
		userID   string
		content  string
		tags     string
	}{
		{
			id:       "mem-001",
			tenantID: "tenant-jp",
			userID:   "user-001",
			content:  "学习者最近在练习 食べる、飲む、行く 的活用，て形和敬语是薄弱点。",
			tags:     "日语,て形,敬语",
		},
		{
			id:       "mem-002",
			tenantID: "tenant-jp",
			userID:   "user-001",
			content:  "学习者容易把自己的动作误用尊敬语；回答时要提醒 尊敬语描述对方或上级。",
			tags:     "日语,尊敬语,误区",
		},
		{
			id:       "mem-003",
			tenantID: "tenant-code",
			userID:   "user-001",
			content:  "学习者正在准备 agent 后端面试，重点关注 SSE、MCP、session、tool registry。",
			tags:     "agent,backend,interview",
		},
	}
	for _, item := range memories {
		if _, err := db.SQL.ExecContext(
			ctx,
			`INSERT INTO memories(id, tenant_id, user_id, content, tags, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			item.id,
			item.tenantID,
			item.userID,
			item.content,
			item.tags,
			now,
		); err != nil {
			return fmt.Errorf("seed memory %s: %w", item.id, err)
		}
	}
	return nil
}
