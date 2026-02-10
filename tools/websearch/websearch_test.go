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
