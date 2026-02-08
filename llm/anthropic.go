// llm/anthropic.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type AnthropicClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string) *AnthropicClient {
	return &AnthropicClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []anthropicMsg  `json:"messages"`
	Tools     []anthropicTool `json:"tools,omitempty"`
}

type anthropicMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
}

type anthropicContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

func (c *AnthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	msgs := make([]anthropicMsg, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = anthropicMsg{Role: m.Role, Content: m.Content}
	}

	tools := make([]anthropicTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	apiReq := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    req.SystemPrompt,
		Messages:  msgs,
	}
	if len(tools) > 0 {
		apiReq.Tools = tools
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, err
	}

	slog.Debug("llm request", "body", string(body))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	slog.Debug("llm response", "body", string(respBody))

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, err
	}

	result := &ChatResponse{
		StopType: apiResp.StopReason,
	}

	for _, content := range apiResp.Content {
		switch content.Type {
		case "text":
			result.Content += content.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:    content.ID,
				Name:  content.Name,
				Input: content.Input,
			})
		}
	}

	return result, nil
}
