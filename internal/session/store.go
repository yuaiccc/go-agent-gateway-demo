package session

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) GetOrCreate(ctx context.Context, tenantID, userID, sessionID string) (Session, error) {
	if existing, ok, err := s.get(ctx, sessionID); err != nil || ok {
		return existing, err
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions(id, tenant_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		sessionID,
		tenantID,
		userID,
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID:        sessionID,
		TenantID:  tenantID,
		UserID:    userID,
		Messages:  []Message{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Store) Append(ctx context.Context, sessionID, role, content string) (Session, error) {
	existing, ok, err := s.get(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, fmt.Errorf("session %q not found", sessionID)
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO messages(session_id, role, content, created_at) VALUES (?, ?, ?, ?)`,
		sessionID,
		role,
		content,
		now.Format(time.RFC3339Nano),
	); err != nil {
		return Session{}, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		now.Format(time.RFC3339Nano),
		sessionID,
	); err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}

	existing.UpdatedAt = now
	existing.Messages = append(existing.Messages, Message{
		Role:      role,
		Content:   content,
		CreatedAt: now,
	})
	return existing, nil
}

func (s *Store) ValidateOwner(ctx context.Context, sessionID, tenantID, userID string) error {
	existing, ok, err := s.get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if existing.TenantID != tenantID || existing.UserID != userID {
		return fmt.Errorf("session %q does not belong to tenant/user", sessionID)
	}
	return nil
}

func (s *Store) get(ctx context.Context, sessionID string) (Session, bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, tenant_id, user_id, created_at, updated_at FROM sessions WHERE id = ?`,
		sessionID,
	)
	var sess Session
	var createdAt, updatedAt string
	if err := row.Scan(&sess.ID, &sess.TenantID, &sess.UserID, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Session{}, false, nil
		}
		return Session{}, false, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT role, content, created_at FROM messages WHERE session_id = ? ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return Session{}, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var msg Message
		var msgCreatedAt string
		if err := rows.Scan(&msg.Role, &msg.Content, &msgCreatedAt); err != nil {
			return Session{}, false, err
		}
		msg.CreatedAt, _ = time.Parse(time.RFC3339Nano, msgCreatedAt)
		sess.Messages = append(sess.Messages, msg)
	}
	return sess, true, rows.Err()
}
