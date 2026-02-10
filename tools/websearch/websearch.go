// tools/websearch/websearch.go
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBaseURL = "https://api.search.brave.com/res/v1/web/search"

type Tool struct {
	apiKey  string
	baseURL string
}

func NewTool(apiKey string) *Tool {
	return &Tool{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
	}
}

func (t *Tool) Name() string        { return "web_search" }
func (t *Tool) AdminOnly() bool     { return false }

func (t *Tool) Description() string {
	return "Search the web for current information using Brave Search."
}

func (t *Tool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to look up on the web.",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "Number of results to return (1-20, default 5). Use the default unless the user explicitly needs more results. Higher values consume more tokens.",
			},
			"freshness": map[string]any{
				"type":        "string",
				"description": "Filter results by age. Use only when recency matters.",
				"enum":        []string{"pd", "pw", "pm", "py"},
			},
			"country": map[string]any{
				"type":        "string",
				"description": "Country code for search context (e.g. 'US', 'BR'). Only set when location relevance matters.",
			},
		},
		"required": []string{"query"},
	}
}

func (t *Tool) ParseArgs(raw string) (map[string]any, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return nil, fmt.Errorf("missing query. Usage: /web_search <query>")
	}
	return map[string]any{"query": query}, nil
}

func (t *Tool) Execute(ctx context.Context, input map[string]any) (string, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return "", fmt.Errorf("missing query")
	}

	// Build request body
	body := map[string]any{
		"q":     query,
		"count": 5,
	}

	if count, ok := input["count"]; ok {
		switch v := count.(type) {
		case float64:
			body["count"] = int(v)
		case json.Number:
			if n, err := v.Int64(); err == nil {
				body["count"] = int(n)
			}
		}
	}

	if freshness, ok := input["freshness"].(string); ok && freshness != "" {
		body["freshness"] = freshness
	}
	if country, ok := input["country"].(string); ok && country != "" {
		body["country"] = country
	}

	// Make HTTP request
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("x-subscription-token", t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Brave Search API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Web.Results) == 0 {
		return fmt.Sprintf("No results found for: %s", query), nil
	}

	// Format output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Web search results for %q:\n", query))
	for i, r := range result.Web.Results {
		sb.WriteString(fmt.Sprintf("\n%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Description))
	}

	return sb.String(), nil
}
