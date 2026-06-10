package tenant

import (
	"fmt"
	"sync"
)

type ModelConfig struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

type Config struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Model  ModelConfig `json:"model"`
	KBIDs  []string    `json:"kb_ids"`
	Tools  []string    `json:"tools"`
	Active bool        `json:"active"`
}

type Store struct {
	mu      sync.RWMutex
	tenants map[string]Config
}

func NewStore() *Store {
	return &Store{
		tenants: map[string]Config{
			"tenant-jp": {
				ID:   "tenant-jp",
				Name: "Japanese Learning Team",
				Model: ModelConfig{
					Provider:    "deepseek",
					Model:       "deepseek-chat",
					Temperature: 0.3,
				},
				KBIDs:  []string{"grammar", "dictionary"},
				Tools:  []string{"search_grammar", "search_memory"},
				Active: true,
			},
			"tenant-code": {
				ID:   "tenant-code",
				Name: "Coding Agent Team",
				Model: ModelConfig{
					Provider:    "mock",
					Model:       "mock-coding-agent",
					Temperature: 0.1,
				},
				KBIDs:  []string{"engineering"},
				Tools:  []string{"search_memory"},
				Active: true,
			},
		},
	}
}

func (s *Store) Get(id string) (Config, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.tenants[id]
	return cfg, ok
}

func (s *Store) List() []Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Config, 0, len(s.tenants))
	for _, cfg := range s.tenants {
		out = append(out, cfg)
	}
	return out
}

func (s *Store) UpdateModel(id string, model ModelConfig) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.tenants[id]
	if !ok {
		return Config{}, fmt.Errorf("tenant %q not found", id)
	}
	cfg.Model = model
	s.tenants[id] = cfg
	return cfg, nil
}

func (cfg Config) AllowsTool(name string) bool {
	for _, tool := range cfg.Tools {
		if tool == name {
			return true
		}
	}
	return false
}
