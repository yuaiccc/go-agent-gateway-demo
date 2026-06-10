package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yuaiccc/go-agent-gateway-demo/internal/model"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tenant"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tool"
)

type ChatRequest struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type Event struct {
	Type       string         `json:"type"`
	TenantID   string         `json:"tenant_id,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	Status     string         `json:"status,omitempty"`
	Delta      string         `json:"delta,omitempty"`
	Data       any            `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

type Service struct {
	Tools  *tool.Registry
	Models *model.Client
}

func NewService(tools *tool.Registry) *Service {
	return &Service{
		Tools:  tools,
		Models: model.NewClient(),
	}
}

func (s *Service) Run(ctx context.Context, cfg tenant.Config, req ChatRequest, emit func(Event) error) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	base := Event{
		TenantID:  req.TenantID,
		UserID:    req.UserID,
		SessionID: req.SessionID,
		RunID:     runID,
		Timestamp: time.Now(),
	}

	if err := emit(with(base, "run_start", map[string]any{
		"model": cfg.Model,
		"tools": cfg.Tools,
	})); err != nil {
		return err
	}

	calls := planToolCalls(req.Message)
	var observations []tool.Result
	for _, call := range calls {
		if !cfg.AllowsTool(call.Name) {
			return emit(errorEvent(base, fmt.Sprintf("tenant %s cannot use tool %s", cfg.ID, call.Name)))
		}

		start := base
		start.ToolCallID = call.ID
		start.ToolName = call.Name
		start.Status = "running"
		start.Timestamp = time.Now()
		start.Type = "tool_call_start"
		start.Data = map[string]any{"arguments": call.Arguments}
		if err := emit(start); err != nil {
			return err
		}

		result, err := s.Tools.Call(ctx, call)
		end := base
		end.ToolCallID = call.ID
		end.ToolName = call.Name
		end.Timestamp = time.Now()
		if err != nil {
			end.Type = "tool_call_result"
			end.Status = "failed"
			end.Error = err.Error()
			if emitErr := emit(end); emitErr != nil {
				return emitErr
			}
			continue
		}

		observations = append(observations, result)
		end.Type = "tool_call_result"
		end.Status = "success"
		end.Data = result.Content
		if err := emit(end); err != nil {
			return err
		}
	}

	answer, err := s.Models.Generate(ctx, cfg.Model, req.Message, observations)
	if err != nil {
		return emit(errorEvent(base, err.Error()))
	}
	for _, token := range strings.Split(answer, "") {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		ev := base
		ev.Type = "message_delta"
		ev.Delta = token
		ev.Timestamp = time.Now()
		if err := emit(ev); err != nil {
			return err
		}
		time.Sleep(8 * time.Millisecond)
	}

	done := base
	done.Type = "done"
	done.Status = "completed"
	done.Data = map[string]any{"answer": answer}
	done.Timestamp = time.Now()
	return emit(done)
}

func planToolCalls(message string) []tool.Call {
	lower := strings.ToLower(message)
	calls := []tool.Call{}
	if strings.Contains(message, "て形") ||
		strings.Contains(message, "敬语") ||
		strings.Contains(message, "尊敬语") ||
		strings.Contains(lower, "grammar") ||
		strings.Contains(lower, "食べる") ||
		strings.Contains(lower, "召し上がる") {
		calls = append(calls, tool.Call{
			ID:   "call-grammar-1",
			Name: "search_grammar",
			Arguments: map[string]any{
				"query": message,
				"top_k": 3,
			},
		})
	}
	if strings.Contains(message, "复习") ||
		strings.Contains(message, "记忆") ||
		strings.Contains(lower, "memory") {
		calls = append(calls, tool.Call{
			ID:   "call-memory-1",
			Name: "search_memory",
			Arguments: map[string]any{
				"query": message,
			},
		})
	}
	if len(calls) == 0 {
		calls = append(calls, tool.Call{
			ID:   "call-memory-1",
			Name: "search_memory",
			Arguments: map[string]any{
				"query": message,
			},
		})
	}
	return calls
}

func with(base Event, eventType string, data any) Event {
	base.Type = eventType
	base.Data = data
	base.Timestamp = time.Now()
	return base
}

func errorEvent(base Event, msg string) Event {
	base.Type = "error"
	base.Status = "failed"
	base.Error = msg
	base.Timestamp = time.Now()
	return base
}
