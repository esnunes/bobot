# User Profiles: Extraction & Personalization

## Summary

A CLI subcommand (`bobot update-profiles`) that scans user messages since the last run, uses the LLM to extract personal details and preferences into a free-form text profile, and stores it per-user. The assistant then injects this profile into the system prompt for personalized interactions.

## Database Schema

New `user_profiles` table in `core.db`:

```sql
CREATE TABLE IF NOT EXISTS user_profiles (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    content TEXT NOT NULL DEFAULT '',
    last_message_id INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

- `user_id` - One profile per user, primary key.
- `content` - Free-form text produced by the LLM.
- `last_message_id` - Highest `messages.id` processed. Next run fetches `id > last_message_id`.
- `updated_at` - When the profile was last refreshed.

## New CoreDB Methods

- `GetUserProfile(userID int64) (content string, lastMessageID int64, err error)` - Returns empty content and 0 if no row exists.
- `UpsertUserProfile(userID int64, content string, lastMessageID int64) error` - INSERT OR REPLACE with `updated_at = CURRENT_TIMESTAMP`.
- `GetUserMessagesSince(userID int64, sinceMessageID int64) ([]Message, error)` - Messages where `sender_id = userID`, `role = 'user'`, `id > sinceMessageID`, ordered by id. Only private messages (topic_id IS NULL).
- `ListActiveUsers() ([]User, error)` - Non-blocked users excluding the system user (id = 0).

## CLI Subcommand

Add `os.Args` routing in `main.go`:

```go
if len(os.Args) > 1 {
    switch os.Args[1] {
    case "update-profiles":
        runUpdateProfiles(cfg, coreDB)
        return
    default:
        log.Fatalf("Unknown command: %s", os.Args[1])
    }
}
```

The `runUpdateProfiles` function lives in a new file `profiles.go` (package `main`):

1. Initialize the LLM provider from config.
2. Fetch all active users via `ListActiveUsers()`.
3. For each user:
   a. Get current profile and `last_message_id` via `GetUserProfile()`.
   b. Fetch user messages since `last_message_id` via `GetUserMessagesSince()`.
   c. If no new messages, skip.
   d. Call the LLM with current profile + new messages.
   e. Upsert updated profile with new `last_message_id` (max id from fetched messages).
   f. Log progress.
4. Exit when done.

Processing is sequential, one user at a time.

## LLM Prompt

System prompt:

```
You are a profile extraction assistant. Given a user's current profile
(which may be empty) and their recent messages, produce an updated
profile summary.

Extract and maintain:
- Personal details: name, location, timezone, language, job/role, company
- Preferences: communication style, response format preferences,
  interests, hobbies, topics they care about

Rules:
- Write in third person, concise natural language
- Preserve existing information unless explicitly contradicted
- Only add information the user has clearly stated or implied
- If the current profile is empty, create one from scratch
- Do not invent or assume information
- Output ONLY the updated profile text, nothing else
```

User message:

```
Current profile:
<profile>
{existing profile content or "No profile yet."}
</profile>

New messages:
<messages>
{messages formatted as one per line}
</messages>
```

Single-turn chat, no tools. Response content is stored as the new profile.

## System Prompt Injection

The assistant engine injects the user profile into the system prompt for private chats.

New interface in `assistant` package:

```go
type ProfileProvider interface {
    GetUserProfile(userID int64) (string, int64, error)
}
```

`Engine` gains a `ProfileProvider` field, set via `NewEngine`. When building the system prompt for a private chat, if a non-empty profile exists, append:

```
## User Profile
The following is known about the user you are chatting with:
<user-profile>
{profile content}
</user-profile>
```

No profile injection for topic chats (multi-user context).

## Files Changed

| File | Change |
|------|--------|
| `db/core.go` | Add migration, add `GetUserProfile`, `UpsertUserProfile`, `GetUserMessagesSince`, `ListActiveUsers` methods |
| `main.go` | Add `os.Args` routing, pass profile provider to engine |
| `profiles.go` (new) | `runUpdateProfiles` orchestration function |
| `assistant/engine.go` | Add `ProfileProvider` interface, inject profile into system prompt |
| `context/adapter.go` | Implement `ProfileProvider` or wire `CoreDB` directly |

## Out of Scope

- Topic message scanning (only private 1:1 messages)
- Concurrency (sequential user processing)
- Rate limiting or retry logic
- Profile editing UI
