# User-Defined Skills

Users can define custom skills scoped to their private chat or to individual topics. Skills are stored in the database and injected into the system prompt alongside built-in skills.

## Data Model

New `skills` table:

```sql
CREATE TABLE skills (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    user_id INTEGER NOT NULL REFERENCES users(id),
    topic_id INTEGER REFERENCES topics(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Scope logic:
- `topic_id IS NULL` — skill is scoped to the user's private chat.
- `topic_id IS NOT NULL` — skill is scoped to that topic.

Each skill has a name, description, and markdown content, matching the existing `Skill` struct.

Uniqueness: one skill per name per scope. Enforced with partial unique indexes:
- `UNIQUE(user_id, LOWER(name)) WHERE topic_id IS NULL` — private chat skills.
- `UNIQUE(topic_id, LOWER(name)) WHERE topic_id IS NOT NULL` — topic skills.

Additional indexes:
- `(user_id) WHERE topic_id IS NULL` — fast lookup for private chat skills.
- `(topic_id) WHERE topic_id IS NOT NULL` — fast lookup for topic skills.

## Skill Tool (`tools/skill/`)

New tool following the existing pattern (`tools.Tool` interface). Requires access to `CoreDB` (for topic ownership checks) and a new `SkillDB`.

### Slash Commands

- `/skill create <name>` — create a skill in the current scope (private chat or current topic).
- `/skill update <name>` — update an existing skill.
- `/skill delete <name>` — delete a skill.
- `/skill list` — list skills for the current scope.

### LLM Tool Call Schema

```json
{
  "command": { "type": "string", "enum": ["create", "update", "delete", "list"] },
  "name": { "type": "string" },
  "description": { "type": "string" },
  "content": { "type": "string" }
}
```

### Permissions

- **Private chat skills:** only the owning user can manage.
- **Topic skills:** topic owner or admin-role users can create/update/delete. Any member can list.

### Scope Resolution

Same pattern as the topic tool:
- In a topic chat, commands operate on that topic's skills (via `ChatData.TopicID`).
- In private chat, commands operate on the user's private chat skills.

## System Prompt Integration

In `assistant/engine.go`, before calling `BuildSystemPrompt`:

1. Fetch user-defined skills from the database based on chat scope:
   - Private chat: `WHERE user_id = ? AND topic_id IS NULL`
   - Topic chat: `WHERE topic_id = ?`
2. Convert DB rows to `assistant.Skill` structs.
3. Append them after built-in skills.
4. Pass the combined slice to `BuildSystemPrompt`.

No changes needed to `BuildSystemPrompt` itself — it already iterates `[]Skill`.

Ordering: built-in skills first, then user-defined skills.

## Web UI

HTMX-based, following existing server patterns.

### Entry Points

- Private chat: a "Skills" link in the chat settings/sidebar.
- Topic chat: a "Skills" link in the topic settings (visible to owner + admins).

### Routes

- `GET /skills?topic_id=X` — list skills (omit `topic_id` for private chat).
- `GET /skills/new?topic_id=X` — create form.
- `POST /skills` — create skill.
- `GET /skills/:id/edit` — edit form.
- `PUT /skills/:id` — update skill.
- `DELETE /skills/:id` — delete skill.

### UI Components

- **List view:** skill cards showing name, description, edit/delete buttons.
- **Create/edit form:** name (text input), description (text input), content (textarea). Save and cancel buttons.
- **Delete:** confirmation before deleting.

## Error Handling

- **Duplicate names:** clear error "a skill named 'X' already exists in this scope."
- **Topic deleted:** skills cascade-deleted via `ON DELETE CASCADE`.
- **Permission denied:** clear error when non-owner/non-admin manages topic skills.

## Monitoring

Log warnings (do not enforce hard limits) when:
- A scope exceeds 10 skills.
- A skill's content exceeds 4KB.

## Testing

- Unit tests for `SkillDB`: CRUD, uniqueness constraints, scope filtering.
- Unit tests for `SkillTool`: permission checks, scope resolution, command parsing.
- Integration test: verify user-defined skills appear in the system prompt.
