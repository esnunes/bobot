# Web Search Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `web_search` tool that calls the Brave Search API so the LLM can search the web for current information.

**Architecture:** Single-file tool in `tools/websearch/websearch.go` with an `httptest`-based test file. Config adds one env var. Registration in `main.go` follows the ThinQ conditional pattern.

**Tech Stack:** Go stdlib (`net/http`, `encoding/json`, `fmt`, `strings`, `context`), `net/http/httptest` for tests.

---

### Task 1: Add config field

**Files:**
- Modify: `config/config.go`

**Step 1: Add `BraveSearchAPIKey` to Config struct**

In `config/config.go`, add the field to the `Config` struct:

```go
type Config struct {
	Server   ServerConfig
	LLM      LLMConfig
	JWT      JWTConfig
	Session  SessionConfig
	Context  ContextConfig
	History  HistoryConfig
	Sync     SyncConfig
	DataDir  string
	BaseURL  string
	BraveSearchAPIKey string
}
```

**Step 2: Load the env var in `Load()`**

Inside the `Load()` function, after the `BaseURL` line (line 89), add:

```go
BraveSearchAPIKey: os.Getenv("BRAVE_SEARCH_API_KEY"),
```

**Step 3: Verify it compiles**

Run: `go build ./...`
Expected: success, no errors.

**Step 4: Commit**

```
feat(config): add BraveSearchAPIKey config field
```

---

### Task 2: Create websearch tool with tests (TDD)

**Files:**
- Create: `tools/websearch/websearch.go`
- Create: `tools/websearch/websearch_test.go`

**Step 1: Write the test file**

Create `tools/websearch/websearch_test.go`. Use `httptest.NewServer` to mock the Brave API, same pattern as `tools/thinq/client_test.go`.

```go
// tools/websearch/websearch_test.go
package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (*Tool, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and headers
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("x-subscription-token") != "test-api-key" {
			t.Errorf("unexpected token: %s", r.Header.Get("x-subscription-token"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		q, _ := body["q"].(string)

		if q == "no results query" {
			json.NewEncoder(w).Encode(map[string]any{
				"web": map[string]any{
					"results": []any{},
				},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{
						"title":       "First Result",
						"url":         "https://example.com/first",
						"description": "This is the first result.",
					},
					{
						"title":       "Second Result",
						"url":         "https://example.com/second",
						"description": "This is the second result.",
					},
				},
			},
		})
	}))

	tool := NewTool("test-api-key")
	tool.baseURL = srv.URL
	t.Cleanup(func() { srv.Close() })

	return tool, srv
}

func TestTool_Name(t *testing.T) {
	tool := NewTool("key")
	if tool.Name() != "web_search" {
		t.Errorf("expected 'web_search', got %q", tool.Name())
	}
}

func TestTool_AdminOnly(t *testing.T) {
	tool := NewTool("key")
	if tool.AdminOnly() {
		t.Error("expected AdminOnly to be false")
	}
}

func TestTool_ParseArgs(t *testing.T) {
	tool := NewTool("key")

	tests := []struct {
		name    string
		raw     string
		wantQ   string
		wantErr bool
	}{
		{
			name:    "empty input",
			raw:     "",
			wantErr: true,
		},
		{
			name:  "simple query",
			raw:   "golang error handling",
			wantQ: "golang error handling",
		},
		{
			name:  "whitespace trimmed",
			raw:   "  some query  ",
			wantQ: "some query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got["query"] != tt.wantQ {
				t.Errorf("got query %q, want %q", got["query"], tt.wantQ)
			}
		})
	}
}

func TestTool_Execute(t *testing.T) {
	tool, _ := setupTestServer(t)

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "test search",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "First Result") {
		t.Errorf("expected 'First Result' in output, got: %s", result)
	}
	if !strings.Contains(result, "https://example.com/first") {
		t.Errorf("expected URL in output, got: %s", result)
	}
	if !strings.Contains(result, "Second Result") {
		t.Errorf("expected 'Second Result' in output, got: %s", result)
	}
}

func TestTool_Execute_NoResults(t *testing.T) {
	tool, _ := setupTestServer(t)

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "no results query",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No results found") {
		t.Errorf("expected 'No results found' message, got: %s", result)
	}
}

func TestTool_Execute_MissingQuery(t *testing.T) {
	tool := NewTool("key")

	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing query")
	}
}

func TestTool_Execute_CountDefault(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{"results": []any{}},
		})
	}))
	defer srv.Close()

	tool := NewTool("key")
	tool.baseURL = srv.URL

	tool.Execute(context.Background(), map[string]any{"query": "test"})

	count, _ := capturedBody["count"].(float64)
	if count != 5 {
		t.Errorf("expected default count 5, got %v", count)
	}
}

func TestTool_Execute_CountCustom(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{"results": []any{}},
		})
	}))
	defer srv.Close()

	tool := NewTool("key")
	tool.baseURL = srv.URL

	tool.Execute(context.Background(), map[string]any{"query": "test", "count": float64(10)})

	count, _ := capturedBody["count"].(float64)
	if count != 10 {
		t.Errorf("expected count 10, got %v", count)
	}
}

func TestTool_Execute_OptionalParams(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{"results": []any{}},
		})
	}))
	defer srv.Close()

	tool := NewTool("key")
	tool.baseURL = srv.URL

	tool.Execute(context.Background(), map[string]any{
		"query":     "test",
		"freshness": "pw",
		"country":   "BR",
	})

	if capturedBody["freshness"] != "pw" {
		t.Errorf("expected freshness 'pw', got %v", capturedBody["freshness"])
	}
	if capturedBody["country"] != "BR" {
		t.Errorf("expected country 'BR', got %v", capturedBody["country"])
	}
}

func TestTool_Execute_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	tool := NewTool("key")
	tool.baseURL = srv.URL

	_, err := tool.Execute(context.Background(), map[string]any{"query": "test"})
	if err == nil {
		t.Error("expected error for API error response")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./tools/websearch/ -v`
Expected: compilation failure (package doesn't exist yet).

**Step 3: Write the implementation**

Create `tools/websearch/websearch.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./tools/websearch/ -v`
Expected: all tests PASS.

**Step 5: Commit**

```
feat(tools): add web_search tool using Brave Search API
```

---

### Task 3: Register tool in main.go

**Files:**
- Modify: `main.go`

**Step 1: Add import**

Add to the import block in `main.go`:

```go
"github.com/esnunes/bobot/tools/websearch"
```

**Step 2: Register the tool**

After the ThinQ registration block (after line 94), add:

```go
// Initialize web search tool (optional, only if configured)
if cfg.BraveSearchAPIKey != "" {
	registry.Register(websearch.NewTool(cfg.BraveSearchAPIKey))
}
```

**Step 3: Verify it compiles**

Run: `go build ./...`
Expected: success.

**Step 4: Run all tests**

Run: `go test ./...`
Expected: all pass (except the pre-existing `TestCreateTopic` failure in `server/`).

**Step 5: Commit**

```
feat(main): register web_search tool when BRAVE_SEARCH_API_KEY is set
```
