package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yuaiccc/go-agent-gateway-demo/internal/tenant"
	"github.com/yuaiccc/go-agent-gateway-demo/internal/tool"
)

type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) Generate(ctx context.Context, cfg tenant.ModelConfig, userMessage string, observations []tool.Result) (string, error) {
	var answer strings.Builder
	err := c.Stream(ctx, cfg, userMessage, observations, func(delta string) error {
		answer.WriteString(delta)
		return nil
	})
	return answer.String(), err
}

func (c *Client) Stream(ctx context.Context, cfg tenant.ModelConfig, userMessage string, observations []tool.Result, emit func(string) error) error {
	switch strings.ToLower(cfg.Provider) {
	case "", "mock":
		answer := mockAnswer(userMessage, observations)
		for _, token := range strings.Split(answer, "") {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := emit(token); err != nil {
				return err
			}
			time.Sleep(8 * time.Millisecond)
		}
		return nil
	case "deepseek":
		return c.streamOpenAICompatible(ctx, cfg, providerConfig{
			apiKeyEnv:  "DEEPSEEK_API_KEY",
			baseURLEnv: "DEEPSEEK_BASE_URL",
			defaultURL: "https://api.deepseek.com/chat/completions",
		}, userMessage, observations, emit)
	default:
		return fmt.Errorf("unsupported model provider %q", cfg.Provider)
	}
}

type providerConfig struct {
	apiKeyEnv  string
	baseURLEnv string
	defaultURL string
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) openAICompatible(ctx context.Context, cfg tenant.ModelConfig, provider providerConfig, userMessage string, observations []tool.Result) (string, error) {
	apiKey := os.Getenv(provider.apiKeyEnv)
	if apiKey == "" {
		return "", fmt.Errorf("%s is not set", provider.apiKeyEnv)
	}
	baseURL := os.Getenv(provider.baseURLEnv)
	if baseURL == "" {
		baseURL = provider.defaultURL
	}

	obs, err := json.MarshalIndent(observations, "", "  ")
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "你是一个日语学习 Agent。你会基于工具结果回答，尽量给出简洁解释、例句和来源提示。不要编造工具结果里没有的来源。",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("用户问题：%s\n\n工具结果 JSON：\n%s", userMessage, string(obs)),
			},
		},
		Temperature: cfg.Temperature,
		Stream:      false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil {
			return "", fmt.Errorf("model request failed: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("model request failed with status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("model returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

type streamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason any `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) streamOpenAICompatible(ctx context.Context, cfg tenant.ModelConfig, provider providerConfig, userMessage string, observations []tool.Result, emit func(string) error) error {
	apiKey := os.Getenv(provider.apiKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("%s is not set", provider.apiKeyEnv)
	}
	baseURL := os.Getenv(provider.baseURLEnv)
	if baseURL == "" {
		baseURL = provider.defaultURL
	}

	body, err := buildChatBody(cfg, userMessage, observations, true)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var parsed chatResponse
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != nil {
			return fmt.Errorf("model request failed: %s", parsed.Error.Message)
		}
		return fmt.Errorf("model request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return nil
		}

		var chunk streamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return err
		}
		if chunk.Error != nil {
			return fmt.Errorf("model stream failed: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := emit(choice.Delta.Content); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func buildChatBody(cfg tenant.ModelConfig, userMessage string, observations []tool.Result, stream bool) ([]byte, error) {
	obs, err := json.MarshalIndent(observations, "", "  ")
	if err != nil {
		return nil, err
	}

	return json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "你是一个日语学习 Agent。你会基于工具结果回答，尽量给出简洁解释、例句和来源提示。不要编造工具结果里没有的来源。",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("用户问题：%s\n\n工具结果 JSON：\n%s", userMessage, string(obs)),
			},
		},
		Temperature: cfg.Temperature,
		Stream:      stream,
	})
}

func mockAnswer(message string, observations []tool.Result) string {
	if len(observations) == 0 {
		return "我没有拿到工具结果，因此只能给出保守回答。建议补充知识库或重试。"
	}

	var b strings.Builder
	b.WriteString("这是一个 demo agent 的回答：\n\n")
	b.WriteString("我根据问题「")
	b.WriteString(message)
	b.WriteString("」调用了 ")
	b.WriteString(fmt.Sprintf("%d", len(observations)))
	b.WriteString(" 个工具。\n\n")
	for _, obs := range observations {
		b.WriteString("- 工具 `")
		b.WriteString(obs.Name)
		b.WriteString("` 返回了可用上下文。\n")
	}
	b.WriteString("\n在真实系统里，这一步会把 tool results 回填给模型，由模型生成带引用的最终答案。")
	return b.String()
}
