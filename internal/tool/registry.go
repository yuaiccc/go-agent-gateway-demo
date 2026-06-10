package tool

import (
	"context"
	"fmt"
	"strings"
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
	r.Register(Definition{
		Name:        "search_grammar",
		Description: "Search a small Japanese grammar knowledge base.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"top_k": map[string]any{"type": "integer"},
			},
		},
		Handler: searchGrammar,
	})
	r.Register(Definition{
		Name:        "search_memory",
		Description: "Search learner memory and review state.",
		Schema: map[string]any{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
		Handler: searchMemory,
	})
	return r
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

func searchGrammar(_ context.Context, args map[string]any) (any, error) {
	query := strings.ToLower(stringArg(args, "query"))
	hits := []map[string]any{}

	knowledge := []map[string]string{
		{
			"source":  "grammar/te-form.md#usage",
			"title":   "て形",
			"content": "一段动词把る去掉后接て，例如 食べる -> 食べて。て形可以连接动作、提出请求、表达状态延续。",
		},
		{
			"source":  "grammar/keigo.md#sonkeigo",
			"title":   "尊敬语",
			"content": "召し上がる 是 食べる/飲む 的尊敬语，用于描述对方或上级的动作，不能用于自己。",
		},
		{
			"source":  "grammar/particles.md#wa-ga",
			"title":   "は 和 が",
			"content": "は 标记话题，が 标记主语或焦点。新信息、存在句和强调主语时常用 が。",
		},
	}

	for _, item := range knowledge {
		blob := strings.ToLower(item["title"] + " " + item["content"])
		if query == "" || strings.Contains(blob, query) || strings.Contains(query, strings.ToLower(item["title"])) {
			hits = append(hits, map[string]any{
				"source":  item["source"],
				"title":   item["title"],
				"content": item["content"],
				"score":   0.88,
			})
		}
	}

	if len(hits) == 0 {
		hits = append(hits, map[string]any{
			"source":  "grammar/index.md",
			"title":   "fallback",
			"content": "未命中精确语法点，可尝试改写 query 或调用 web/search 工具。",
			"score":   0.31,
		})
	}
	return map[string]any{"results": hits}, nil
}

func searchMemory(_ context.Context, args map[string]any) (any, error) {
	query := stringArg(args, "query")
	return map[string]any{
		"results": []map[string]any{
			{
				"memory_id": "mem-001",
				"content":   "学习者最近在练习 食べる、飲む、行く 的活用，て形和敬语是薄弱点。",
				"query":     query,
				"score":     0.76,
			},
		},
	}, nil
}
