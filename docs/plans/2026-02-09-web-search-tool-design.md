# Web Search Tool Design

## Overview

Add a `web_search` AI tool that allows the LLM to search the web via the Brave Search API. This gives the assistant access to current information beyond its training data.

## API

**Brave Web Search API**
- Endpoint: `POST https://api.search.brave.com/res/v1/web/search`
- Auth: `x-subscription-token` header with API key
- Reference: https://api-dashboard.search.brave.com/api-reference/web/search/post

## Package Structure

Single file: `tools/websearch/websearch.go`

No database, no separate client file. The HTTP call is simple enough to live inline.

## Configuration

- Env var: `BRAVE_SEARCH_API_KEY`
- Added to `config.Config` struct as `BraveSearchAPIKey`
- Tool only registered when env var is set (same pattern as ThinQ)

## Tool Interface

- `Name()` -> `"web_search"`
- `Description()` -> "Search the web for current information using Brave Search"
- `AdminOnly()` -> `false`
- `ParseArgs()` -> raw string becomes the `query`, all other params use defaults (supports slash commands for free)

## Schema

```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "The search query to look up on the web."
    },
    "count": {
      "type": "integer",
      "description": "Number of results to return (1-20, default 5). Use the default unless the user explicitly needs more results. Higher values consume more tokens."
    },
    "freshness": {
      "type": "string",
      "description": "Filter results by age. Use only when recency matters.",
      "enum": ["pd", "pw", "pm", "py"]
    },
    "country": {
      "type": "string",
      "description": "Country code for search context (e.g. 'US', 'BR'). Only set when location relevance matters."
    }
  },
  "required": ["query"]
}
```

## HTTP Request

- Method: POST
- URL: `https://api.search.brave.com/res/v1/web/search`
- Headers:
  - `x-subscription-token: <api_key>`
  - `Content-Type: application/json`
- Body: `{ "q": query, "count": count }` plus optional `freshness` and `country` if provided

## Response Parsing

Extract the `results` array from the Brave API JSON response. For each result, extract `title`, `url`, and `description`.

Format returned to the LLM:

```
Web search results for "golang error handling":

1. Error handling in Go - The Go Blog
   https://go.dev/blog/error-handling
   Go uses explicit error values to handle errors. Functions return an error...

2. Effective Error Handling in Go
   https://example.com/article
   This article covers best practices for...
```

## Error Handling

- Non-2xx HTTP status -> return descriptive error (e.g., "Brave Search API error: rate limited")
- No results -> return "No results found for: {query}"

## Registration

In `main.go`, following the ThinQ pattern:

```go
if cfg.BraveSearchAPIKey != "" {
    registry.Register(websearch.NewTool(cfg.BraveSearchAPIKey))
}
```

## Design Decisions

- **No caching**: Web search results are time-sensitive. Brave caches server-side. Avoids unnecessary complexity.
- **Minimal response**: Only title + URL + description per result. Saves tokens, sufficient for the LLM to answer questions.
- **Default count of 5**: Balances usefulness vs token usage. LLM can request more when needed.
- **Slash command support**: Comes for free since `ParseArgs()` is required by the interface.
