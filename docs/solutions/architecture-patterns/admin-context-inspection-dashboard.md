---
title: "Admin Context Inspection Dashboard"
type: architecture-pattern
date: 2026-02-16
component: server, assistant, admin
tags: [admin, context-inspection, llm-debugging, htmx, go-templates]
severity: feature
related_plans:
  - docs/plans/2026-02-15-feat-admin-context-inspector-plan.md
related_brainstorms:
  - docs/brainstorms/2026-02-15-admin-context-inspector-brainstorm.md
pr: https://github.com/esnunes/bobot/pull/18
---

# Admin Context Inspection Dashboard

## Problem

When debugging LLM behavior or investigating user-reported issues, admins had no way to see what the LLM actually receives as its context window. This meant blind troubleshooting -- admins couldn't verify whether the system prompt, tool definitions, skills, or message history were constructed correctly for a given user or topic.

### Symptoms

- Unable to verify system prompt construction for specific users
- No visibility into tool_use/tool_result message blocks
- No way to check token usage or context window fullness
- Debugging LLM behavior required reading code and guessing state

## Root Cause

The engine methods that build context (`BuildSystemPrompt`, `GetContextMessages`, `ToLLMToolsForRole`) were only called during actual LLM requests, with no read-only inspection path.

## Solution

### Architecture

Added a read-only admin dashboard using the existing server-rendered HTMX pattern:

```
/admin                          → Dashboard (users + topics list)
/admin/users/{id}/context       → Private chat context inspection
/admin/topics/{id}/context      → Topic chat context inspection
```

### Key Files

| File | Purpose |
|------|---------|
| `server/admin.go` | Admin page handlers + `buildContextPageData` helper |
| `assistant/engine.go` | `InspectPrivateContext`, `InspectTopicContext`, `BuildRawJSON` methods |
| `server/pages.go` | View structs (`ContextMessageView`, `ToolBlockView`, etc.) + `IsAdmin` field |
| `server/server.go` | `adminMiddleware` + route registration |
| `web/templates/admin.html` | Dashboard template |
| `web/templates/admin_context.html` | Context detail template with structured/raw toggle |
| `db/core.go` | `ListAllTopics()` method |

### Key Patterns

**1. Engine Inspection Methods**

The `InspectPrivateContext` and `InspectTopicContext` methods reconstruct the full LLM context using the same code paths as actual requests, but without calling the LLM:

```go
type ContextInspection struct {
    SystemPrompt string
    Messages     []ContextMessage
    Tools        []llm.Tool
    TotalTokens  int
    MaxTokens    int
}

func (e *Engine) InspectPrivateContext(userID int64, role string) (*ContextInspection, error)
func (e *Engine) InspectTopicContext(topicID int64) (*ContextInspection, error)
```

Critical: `InspectPrivateContext` uses the **inspected user's role** (not the admin's) for `ToLLMToolsForRole`, ensuring the admin sees exactly what that user's LLM sees.

**2. Tool Block Parsing**

Tool messages have empty `Content` fields -- data lives in `RawContent` as Anthropic API JSON arrays. The `buildContextPageData` helper parses these:

```go
if len(rawContent) > 0 && rawContent[0] == '[' {
    var blocks []map[string]any
    if err := json.Unmarshal([]byte(rawContent), &blocks); err == nil {
        for _, b := range blocks {
            // Parse tool_use (name, id, input) and tool_result (tool_use_id, content)
        }
    }
}
```

Template renders Content and ToolBlocks as **independent conditions** (not mutually exclusive), since assistant messages can have both text and tool calls:

```html
{{if .Content}}
<div class="context-message-content" data-role="{{.Role}}">{{.Content}}</div>
{{end}}
{{if .ToolBlocks}}
<div class="context-tool-blocks">...</div>
{{end}}
```

**3. Admin Middleware Chain**

Admin routes use `sessionMiddleware` wrapping `adminMiddleware`:

```go
s.router.HandleFunc("GET /admin", s.sessionMiddleware(s.adminMiddleware(s.handleAdminPage)))
```

**4. Sticky Toolbar Pattern**

The structured/raw toggle and summary bar are placed outside the scrollable `<main>` area to remain visible:

```html
<div class="context-toolbar">  <!-- flex-shrink: 0, always visible -->
    <div class="context-summary">...</div>
    <div class="context-toggle">...</div>
</div>
<main class="admin-list">  <!-- overflow-y: auto, scrollable -->
    ...
</main>
```

**5. Markdown Rendering Reuse**

The context page reuses the chat's markdown pipeline (`marked` + `DOMPurify` + `highlight.js` + `message-renderer.js`) for rendering assistant/system messages:

```javascript
document.querySelectorAll('.context-message-content[data-role]').forEach(function(el) {
    var rendered = window.MessageRenderer.renderMessageContent(el.textContent, el.dataset.role);
    if (rendered) {
        el.innerHTML = rendered;
        window.MessageRenderer.highlightCodeBlocks(el);
        window.MessageRenderer.processBobotTags(el, function() {}, true);
    }
});
```

**6. Raw JSON View**

`BuildRawJSON` constructs the full Anthropic API request payload for debugging:

```go
func (ci *ContextInspection) BuildRawJSON(model string, maxTokens int) (string, error)
```

This includes model, max_tokens, system prompt, messages, and tool definitions -- exactly what the LLM provider would send.

## Gotchas and Lessons Learned

1. **Template condition ordering matters**: Using `{{if .ToolBlocks}}...{{else if .Content}}` makes Content and ToolBlocks mutually exclusive. Assistant messages can have both text AND tool calls. Always use independent `{{if}}` blocks when fields aren't mutually exclusive.

2. **UI controls in scrollable containers disappear**: Placing toggle buttons inside a scrollable `<main>` means they scroll out of view. Move persistent controls to a fixed/sticky container outside the scrollable area.

3. **Tool messages have empty Content**: In the Anthropic API format, `tool_use` and `tool_result` blocks live in `raw_content` as JSON arrays, not in the `content` field. Don't assume `Content` is always populated.

4. **Role matters for tool inspection**: Using the admin's role instead of the inspected user's role would show wrong tools. Always pass the target user's role to `ToLLMToolsForRole`.

5. **`interface{}` vs `any` in Go**: Modern Go linters flag `interface{}` -- use `any` instead.

## Prevention Strategies

- When adding template conditions for multiple optional fields, test with messages that have various combinations (content only, tool blocks only, both, neither)
- When placing UI controls, consider whether the parent container scrolls
- When building inspection/debugging tools, always use the same code path as production to ensure accuracy

## Testing Notes

- Verify admin middleware returns 403 for non-admin users
- Test context pages with users that have different roles (admin vs user tools differ)
- Test with messages containing: plain text, markdown, tool_use blocks, tool_result blocks, and mixed content
- Verify raw JSON matches what the LLM provider would actually send

## Cross-References

- [Plan: Admin Context Inspector](../plans/2026-02-15-feat-admin-context-inspector-plan.md)
- [Brainstorm: Admin Context Inspector](../brainstorms/2026-02-15-admin-context-inspector-brainstorm.md)
- PR: https://github.com/esnunes/bobot/pull/18
