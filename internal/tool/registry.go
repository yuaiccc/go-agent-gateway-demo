package tool

import (
	"context"
	"fmt"
	"sync"
)

type Call struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Result struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Content any    `json:"content"`
}

type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	Handler     Handler        `json:"-"`
}

type Handler func(ctx context.Context, args map[string]any) (any, error)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Definition
}

func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]Definition)}
	grammar, err := LoadGrammarIndex("")
	if err != nil {
		grammar = &GrammarIndex{}
	}
	r.Register(Definition{
		Name:        "search_grammar",
		Description: "Search local Japanese grammar markdown chunks.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"top_k": map[string]any{"type": "integer"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return grammar.Search(ctx, stringArg(args, "query"), intArg(args, "top_k", 3)), nil
		},
	})
	r.RegisterMemory(nil)
	return r
}

func NewRegistryWithStores(grammar *GrammarIndex, memory *MemoryStore) *Registry {
	if grammar == nil {
		grammar = &GrammarIndex{}
	}
	r := &Registry{tools: make(map[string]Definition)}
	r.Register(Definition{
		Name:        "search_grammar",
		Description: "Search local Japanese grammar markdown chunks.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"top_k": map[string]any{"type": "integer"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return grammar.Search(ctx, stringArg(args, "query"), intArg(args, "top_k", 3)), nil
		},
	})
	r.RegisterMemory(memory)
	return r
}

func (r *Registry) RegisterMemory(memory *MemoryStore) {
	r.Register(Definition{
		Name:        "search_memory",
		Description: "Search learner memory stored in SQLite.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"query", "tenant_id", "user_id"},
			"properties": map[string]any{
				"query":     map[string]any{"type": "string"},
				"tenant_id": map[string]any{"type": "string"},
				"user_id":   map[string]any{"type": "string"},
				"top_k":     map[string]any{"type": "integer"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			if memory == nil {
				return map[string]any{
					"results": []map[string]any{
						{
							"memory_id": "mem-demo",
							"content":   "未接入 SQLite，返回 Registry 默认演示记忆。",
							"score":     0.1,
						},
					},
				}, nil
			}
			tenantID, userID, err := ownerArgs(args)
			if err != nil {
				return nil, err
			}
			return memory.Search(ctx, tenantID, userID, stringArg(args, "query"), intArg(args, "top_k", 3))
		},
	})
}

func (r *Registry) Register(def Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Name] = def
}

func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.tools[name]
	return def, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.tools))
	for _, def := range r.tools {
		out = append(out, def)
	}
	return out
}

func (r *Registry) Call(ctx context.Context, call Call) (Result, error) {
	def, ok := r.Get(call.Name)
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", call.Name)
	}
	content, err := def.Handler(ctx, call.Arguments)
	if err != nil {
		return Result{}, err
	}
	return Result{CallID: call.ID, Name: call.Name, Content: content}, nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}
