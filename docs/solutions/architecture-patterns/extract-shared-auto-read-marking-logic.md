---
title: "Extract Shared Auto-Read Marking Logic"
date: 2026-02-23
module: github.com/esnunes/bobot/server
component: read_status, chat, pipeline
problem_type: architecture-pattern
severity: medium
tags: [refactoring, side-effects, code-reuse, single-responsibility, redundancy, client-server-sync]
---

# Extract Shared Auto-Read Marking Logic

## Problem

When implementing a per-topic auto-read feature (automatically mark messages as read for members with `AutoRead=true`), the marking logic needed to run after every topic broadcast. Two parallel types — `*Server` (WebSocket handlers) and `*ChatPipeline` (scheduler) — both needed this logic. Over several iterations, the code accumulated dual divergent implementations with hidden side effects.

## Root Cause

The auto-read logic was tightly coupled to the broadcast operation. Each execution path (`broadcastToTopic` on `*Server` and `*ChatPipeline`) fetched topic members independently, then the auto-read logic either fetched them again or was inlined, leading to:

1. **Redundant DB queries**: `broadcastToTopic` fetched members, then `autoMarkReadForTopic` fetched them again.
2. **SRP violation**: Folding auto-read into `broadcastToTopic` gave it hidden side effects (DB writes + extra WebSocket frames) beyond its name.
3. **Implementation divergence**: Server used a `broadcastReadEvent()` helper; Pipeline manually inlined JSON marshal. If the read event shape changed, Pipeline would silently diverge.
4. **JSON marshaling inefficiency**: Read event JSON was marshaled per-member inside the loop instead of once before it.

## Investigation Steps

**Attempt 1 — Standalone method per type**: Each type got its own `autoMarkReadForTopic()` method. Problem: both `broadcastToTopic` and `autoMarkReadForTopic` fetched members independently, doubling DB queries per broadcast.

**Attempt 2 — Folded into broadcastToTopic**: Auto-read logic was inlined into `broadcastToTopic` on both types. Eliminated the redundant member fetch but introduced SRP violation, dual divergent implementations, and per-member JSON marshaling.

**Attempt 3 (Final) — Shared function + return members**: Extracted a package-level function and changed `broadcastToTopic` to return the members list.

## Solution

### Shared function in `server/read_status.go`

```go
// autoMarkReadForTopic marks the topic as read for all members with auto-read enabled.
// Shared by both *Server and *ChatPipeline to avoid divergent implementations.
func autoMarkReadForTopic(coreDB *db.CoreDB, connections *ConnectionRegistry, topicID int64, members []db.TopicMember) {
    latestID, err := coreDB.GetLatestTopicMessageID(topicID)
    if err != nil || latestID == 0 {
        return
    }
    readEvent, _ := json.Marshal(map[string]any{
        "type":     "read",
        "topic_id": topicID,
    })
    for _, member := range members {
        if member.AutoRead {
            coreDB.MarkChatRead(member.UserID, topicID, latestID)
            connections.Broadcast(member.UserID, readEvent)
        }
    }
}
```

### Modified `broadcastToTopic` (both types)

```go
func (s *Server) broadcastToTopic(topicID int64, data []byte) []db.TopicMember {
    members, err := s.db.GetTopicMembers(topicID)
    if err != nil {
        log.Printf("failed to get topic members: %v", err)
        return nil
    }
    for _, member := range members {
        s.connections.Broadcast(member.UserID, data)
    }
    return members
}
```

### Call sites — explicit invocation

```go
members := s.broadcastToTopic(topicID, userMsgJSON)
autoMarkReadForTopic(s.db, s.connections, topicID, members)
```

Both `*Server` and `*ChatPipeline` use the same shared function with the same call pattern.

### Client-side instant feedback

When enabling auto-read, dispatch `bobot:chat-read` locally so the unread dot clears immediately without waiting for the WebSocket round-trip:

```javascript
if (!isAutoRead) {
    document.dispatchEvent(new CustomEvent('bobot:chat-read', {
        detail: { topic_id: parseInt(topicId, 10) }
    }));
}
```

## Key Insight

**Share by returning data, not by merging implementations.** When two types have parallel execution paths that need shared logic:

- Make the common method return the data it already fetches (`[]db.TopicMember`)
- Extract shared logic as a package-level function taking explicit dependencies (`*db.CoreDB`, `*ConnectionRegistry`)
- Require callers to invoke it explicitly, making side effects visible

This preserves Single Responsibility, eliminates code duplication, and creates a clear contract: `broadcastToTopic` broadcasts; `autoMarkReadForTopic` marks as read.

## Prevention Strategies

1. **Return intermediate results explicitly** — Instead of `broadcastToTopic(topicID)`, use `broadcastToTopic(topicID) []db.TopicMember`. Let callers reuse the data without re-fetching.

2. **Separate data retrieval from side effects** — Functions should either fetch/transform data OR perform side effects, not both. This prevents hidden coupling.

3. **Extract shared domain operations as package-level functions** — When two types need identical logic, use a package-level function with explicit dependencies rather than duplicating methods on each type.

4. **Marshal once, broadcast many** — When sending identical payloads to multiple recipients, marshal the JSON once before the loop.

## When This Pattern Applies

- Two or more types have methods with similar names and purposes
- JSON marshaling or DB queries are duplicated across file boundaries
- A helper method in one type does something similar to inlined logic in another
- New features require changes in multiple places because shared logic is duplicated

## References

- PR: #23 (feat: per-topic auto-read toggle)
- Related: `docs/solutions/ui-bugs/invisible-unread-indicator-websocket-sync.md`
- Related: `docs/solutions/architecture-patterns/inconsistent-unread-indicator-rendering-ssr-vs-js.md`
- Key files: `server/read_status.go`, `server/chat.go:247`, `server/pipeline.go:122`
