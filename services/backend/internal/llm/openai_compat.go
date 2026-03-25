package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"eva/services/backend/internal/observability"
)

type Client struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Temperature float32   `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Complete(ctx context.Context, messages []Message, tools []ToolDef) (string, []ToolCall, error) {
	start := time.Now()
	defer func() {
		observability.LLMDuration.Observe(time.Since(start).Seconds())
	}()
	body := chatRequest{
		Model:       c.Model,
		Messages:    messages,
		Tools:       tools,
		Stream:      false,
		Temperature: 0.3,
	}
	raw, err := c.post(ctx, "/chat/completions", body)
	if err != nil {
		observability.LLMRequests.WithLabelValues("error").Inc()
		return "", nil, err
	}
	var resp chatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		observability.LLMRequests.WithLabelValues("error").Inc()
		return "", nil, err
	}
	if resp.Error != nil {
		observability.LLMRequests.WithLabelValues("error").Inc()
		return "", nil, fmt.Errorf("llm: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		observability.LLMRequests.WithLabelValues("error").Inc()
		return "", nil, fmt.Errorf("llm: empty choices")
	}
	msg := resp.Choices[0].Message
	observability.LLMRequests.WithLabelValues("ok").Inc()
	return msg.Content, msg.ToolCalls, nil
}

// StreamLines reads OpenAI-style SSE and invokes sink for each JSON data line.
func (c *Client) StreamLines(ctx context.Context, messages []Message, sink func(delta string) error) error {
	body := chatRequest{
		Model:       c.Model,
		Messages:    messages,
		Stream:      true,
		Temperature: 0.3,
	}
	url := strings.TrimSuffix(c.BaseURL, "/") + "/chat/completions"
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 300 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		slurp, _ := io.ReadAll(res.Body)
		return fmt.Errorf("llm stream %d: %s", res.StatusCode, string(slurp))
	}
	sc := bufio.NewScanner(res.Body)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return sc.Err()
		}
		var payload struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		if payload.Error != nil {
			return fmt.Errorf("llm: %s", payload.Error.Message)
		}
		for _, ch := range payload.Choices {
			if ch.Delta.Content != "" {
				if err := sink(ch.Delta.Content); err != nil {
					return err
				}
			}
		}
	}
	return sc.Err()
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	url := strings.TrimSuffix(c.BaseURL, "/") + path
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("llm %d: %s", res.StatusCode, string(raw))
	}
	return raw, nil
}

func WebSearchTool() ToolDef {
	var t ToolDef
	t.Type = "function"
	t.Function.Name = "web_search"
	t.Function.Description = "Search the public web for current information. Use for recent events, facts, or anything needing up-to-date data."
	t.Function.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "search query"},
		},
		"required": []string{"query"},
	}
	return t
}
