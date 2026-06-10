package session

import (
	"fmt"
	"sync"
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
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

func (s *Store) GetOrCreate(tenantID, userID, sessionID string) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.sessions[sessionID]; ok {
		return existing
	}

	now := time.Now()
	created := Session{
		ID:        sessionID,
		TenantID:  tenantID,
		UserID:    userID,
		Messages:  []Message{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.sessions[sessionID] = created
	return created
}

func (s *Store) Append(sessionID, role, content string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, fmt.Errorf("session %q not found", sessionID)
	}

	existing.Messages = append(existing.Messages, Message{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	})
	existing.UpdatedAt = time.Now()
	s.sessions[sessionID] = existing
	return existing, nil
}

func (s *Store) ValidateOwner(sessionID, tenantID, userID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	existing, ok := s.sessions[sessionID]
	if !ok {
		return nil
	}
	if existing.TenantID != tenantID || existing.UserID != userID {
		return fmt.Errorf("session %q does not belong to tenant/user", sessionID)
	}
	return nil
}
