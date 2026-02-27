---
title: "feat: Add Spotify playback control tool"
type: feat
date: 2026-02-27
issue: https://github.com/esnunes/bobot/issues/44
brainstorm: docs/brainstorms/2026-02-27-spotify-control-brainstorm.md
---

# feat: Add Spotify playback control tool

## Overview

Add a Spotify integration tool that lets users control music playback through natural language chat. Users connect their Spotify Premium account via OAuth2 from topic settings, and the LLM invokes tool commands for search, playback control, device selection, playlist browsing, and status queries.

Key architectural difference from Calendar: tokens are stored per `user_id` (not per `topic_id`) and a link table maps topics to the user whose Spotify account they use.

## Problem Statement / Motivation

Users want to control Spotify through the chatbot, similar to how they interact with Google Calendar. This enables hands-free music control via natural language — particularly useful when combined with the scheduled commands and quick actions features already in the codebase.

## Proposed Solution

### Data Model

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ oauth_states │     │    tokens    │     │ topic_links  │
├──────────────┤     ├──────────────┤     ├──────────────┤
│ state (PK)   │     │ user_id (PK) │◄────│ user_id (FK) │
│ user_id      │     │ access_token │     │ topic_id (PK)│
│ topic_id     │     │ refresh_token│     │ created_at   │
│ verifier     │     │ token_type   │     └──────────────┘
│ created_at   │     │ expiry       │
└──────────────┘     │ created_at   │
                     │ updated_at   │
                     └──────────────┘
```

**Token ownership rule:** Each topic has at most one linked `user_id`. When the tool executes in a topic, it always uses the linked user's token regardless of who sent the chat message. Only the token owner can link their Spotify to topics they are a member of.

### Tool Commands

| Command | Parameters | Description |
|---------|-----------|-------------|
| `search` | `query` (string, required) | Search tracks. Returns name, artist, album, Spotify URI |
| `play` | `uri` (string, required), `device_id` (string, optional) | Play a specific track/album/playlist by Spotify URI |
| `pause` | — | Pause current playback |
| `resume` | — | Resume paused playback |
| `next` | — | Skip to next track |
| `previous` | — | Go to previous track |
| `volume` | `level` (int, 0-100, required) | Set playback volume |
| `devices` | — | List available playback devices |
| `transfer` | `device_id` (string, required) | Transfer playback to a device |
| `status` | — | Get current track, artist, device, progress, volume |
| `playlists` | — | List user's playlists (name, track count, URI) |
| `play_playlist` | `uri` (string, required), `device_id` (string, optional) | Play a playlist by URI |

### OAuth Flow

1. User clicks "Connect Spotify" in topic settings
2. Server generates state + PKCE verifier, stores in `oauth_states`, redirects to Spotify
3. User authorizes on Spotify
4. Spotify redirects to `/api/spotify/callback`
5. Server exchanges code for tokens using client_secret + PKCE verifier (confidential client)
6. Server calls `/v1/me` to check `product` field
7. If not `"premium"`: redirect to settings with `?error=spotify_premium_required`, discard tokens
8. If premium: store token keyed by `user_id`, create topic link, redirect to settings

### Settings UI States

| User has token? | Topic linked? | UI shows |
|----------------|--------------|----------|
| No | No | "Connect Spotify" button (starts OAuth) |
| Yes | No | "Link Spotify" button (creates link record) |
| Yes | Yes | "Unlink" button (or "Disconnect" if this is the last linked topic) |
| No | Orphaned link | "Connect Spotify" button (link record ignored) |

### Server Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/spotify/auth` | Initiate OAuth flow (requires `topic_id` query param) |
| GET | `/api/spotify/callback` | OAuth callback from Spotify |
| POST | `/api/spotify/link` | Link existing connection to a topic |
| DELETE | `/api/spotify/link` | Unlink Spotify from a topic |
| DELETE | `/api/spotify` | Full disconnect: revoke token + delete all links |

## Technical Considerations

### SQLite Safety (from institutional learnings)

- Enable WAL mode + `busy_timeout(5000)` on `tool_spotify.db`
- Use status guards on token updates for concurrent access safety
- Foreign keys enabled for link table integrity

### Token Refresh

Follow Calendar's `persistingTokenSource` pattern:
- Wrap `oauth2.TokenSource` to persist refreshed tokens to DB automatically
- On `invalid_grant` (revocation): delete local token + all links, return reconnect message
- On transient errors (network, 5xx): do NOT delete tokens, return temporary error message

### Error Handling

- **No active device:** Return "No active Spotify devices found. Please open Spotify on your phone, computer, or speaker, then try again."
- **Premium downgrade:** Detect 403 on playback endpoints, return "Spotify Premium is required for playback control. Your account may have been downgraded."
- **Rate limiting (429):** Return "Spotify is temporarily rate-limited. Please try again in a moment." (no automatic retry)
- **No active playback (404):** Return "No active playback session. Try playing a song first."

### Disconnect Cascade

`DELETE /api/spotify` deletes the user's token AND all topic link records in a single transaction. This prevents orphaned links.

### Topic Deletion Cleanup

When a topic is deleted, its Spotify link record becomes orphaned. This is non-critical (the tool gracefully handles missing links). A future cleanup job can address this, consistent with how Calendar handles the same gap.

## Acceptance Criteria

### Functional

- [ ] Users can connect Spotify Premium account via OAuth from topic settings
- [ ] Free-tier users see a "Premium required" error at connection time
- [ ] Users can link their existing Spotify connection to additional topics
- [ ] Users can unlink Spotify from a topic (removes link only)
- [ ] Users can fully disconnect Spotify (revokes token + removes all links)
- [ ] All 12 tool commands work when invoked by the LLM
- [ ] Search results include Spotify URIs for chaining with `play`
- [ ] Token refresh happens transparently on expired access tokens
- [ ] Revoked tokens are detected and cleaned up with a reconnect message
- [ ] Settings page shows correct state (Connect/Link/Unlink/Disconnect)
- [ ] `?error=spotify_premium_required` shows an error banner in settings
- [ ] Tool is conditionally registered only when env vars are configured
- [ ] Tool is topic-scoped: only works in topics with a linked Spotify connection

### Non-Functional

- [ ] SQLite database uses WAL mode + busy_timeout
- [ ] OAuth state validated with PKCE + session user verification
- [ ] Tests cover DB layer, API client (httptest), tool commands, and OAuth flow
- [ ] Graceful error messages for all common failure modes

## Implementation Phases

### Phase 1: Database Layer

**Files:**
- Create `tools/spotify/db.go`
- Create `tools/spotify/db_test.go`

**Tables:**
- `oauth_states` (state TEXT PK, user_id INTEGER, topic_id INTEGER, verifier TEXT, created_at DATETIME)
- `tokens` (user_id INTEGER PK, access_token TEXT, refresh_token TEXT, token_type TEXT, expiry DATETIME, created_at DATETIME, updated_at DATETIME)
- `topic_links` (topic_id INTEGER PK, user_id INTEGER NOT NULL REFERENCES tokens(user_id), created_at DATETIME)

**Methods:**
- `NewSpotifyDB(dbPath string) (*SpotifyDB, error)` — opens DB with WAL + busy_timeout, runs migrations
- `SaveOAuthState(state, userID, topicID, verifier)` / `GetOAuthState(state)` / `DeleteOAuthState(state)`
- `SaveToken(userID, token)` / `GetToken(userID)` / `DeleteToken(userID)` — token CRUD
- `LinkTopic(topicID, userID)` / `UnlinkTopic(topicID)` / `GetTopicLink(topicID)` — link CRUD
- `GetLinkedTopics(userID) []int64` — all topics linked to a user
- `Disconnect(userID)` — delete token + all links in transaction
- `HasToken(userID) bool` — check if user has a Spotify connection

**Tests:** All DB methods with `t.TempDir()` for ephemeral SQLite databases.

### Phase 2: OAuth Module

**Files:**
- Create `tools/spotify/oauth.go`

**Implementation:**
- `SpotifyOAuth` struct with `clientID`, `clientSecret`, `baseURL`
- `AuthURL(state, verifier string) string` — generates Spotify auth URL with scopes + PKCE
- `Exchange(ctx, code, verifier string) (*oauth2.Token, error)` — exchanges code for token
- `CheckPremium(ctx, accessToken string) (bool, error)` — calls `/v1/me`, checks `product == "premium"`
- `RevokeToken(ctx, refreshToken string) error` — best-effort token revocation
- `PersistingTokenSource(userID int64, token *oauth2.Token, db *SpotifyDB) oauth2.TokenSource` — wraps token source to persist refreshed tokens

**Spotify OAuth endpoints:**
- Auth: `https://accounts.spotify.com/authorize`
- Token: `https://accounts.spotify.com/api/token`
- Scopes: `user-read-playback-state user-modify-playback-state user-read-currently-playing playlist-read-private playlist-read-collaborative`

### Phase 3: API Client

**Files:**
- Create `tools/spotify/client.go`
- Create `tools/spotify/client_test.go`

**Methods:**
- `NewClient(httpClient *http.Client) *Client`
- `Search(ctx, query string) ([]Track, error)` — `GET /v1/search?type=track`
- `Play(ctx, uri string, deviceID string) error` — `PUT /v1/me/player/play`
- `Pause(ctx) error` — `PUT /v1/me/player/pause`
- `Resume(ctx) error` — `PUT /v1/me/player/play` (no body)
- `Next(ctx) error` — `POST /v1/me/player/next`
- `Previous(ctx) error` — `POST /v1/me/player/previous`
- `SetVolume(ctx, level int) error` — `PUT /v1/me/player/volume?volume_percent=N`
- `GetDevices(ctx) ([]Device, error)` — `GET /v1/me/player/devices`
- `TransferPlayback(ctx, deviceID string) error` — `PUT /v1/me/player`
- `GetPlaybackState(ctx) (*PlaybackState, error)` — `GET /v1/me/player`
- `GetPlaylists(ctx) ([]Playlist, error)` — `GET /v1/me/playlists`
- `PlayPlaylist(ctx, uri string, deviceID string) error` — `PUT /v1/me/player/play` with context_uri
- `GetCurrentUser(ctx) (*User, error)` — `GET /v1/me` (for Premium check)

**Types:**
```go
type Track struct {
    Name    string
    Artist  string
    Album   string
    URI     string
}

type Device struct {
    ID       string
    Name     string
    Type     string
    IsActive bool
    Volume   int
}

type PlaybackState struct {
    IsPlaying  bool
    Track      Track
    Device     Device
    ProgressMs int
    DurationMs int
    Volume     int
}

type Playlist struct {
    Name       string
    URI        string
    TrackCount int
}
```

**Error handling:** Map HTTP status codes to user-friendly messages (403 → Premium required, 404 → no active playback, 429 → rate limited).

**Tests:** Use `httptest.NewServer` to mock Spotify API responses for each method.

### Phase 4: Tool Implementation

**Files:**
- Create `tools/spotify/spotify.go`
- Create `tools/spotify/spotify_test.go`

**Implementation:**
- `SpotifyTool` struct with `db *SpotifyDB`, `oauth *SpotifyOAuth`
- Implements `tools.Tool` interface: `Name()` → `"spotify"`, `Description()`, `Schema()`, `ParseArgs()`, `Execute()`, `AdminOnly()` → `false`
- `Schema()` returns command enum + per-command parameters (following Calendar pattern)
- `Execute()` resolves `topicID` → `userID` via link table, gets token, creates authenticated client, dispatches to command handler
- Each command handler: validates params, calls client method, formats result string for LLM

**Token resolution flow in Execute:**
1. Get `topicID` from `auth.ChatDataFromContext(ctx)`
2. Look up `topic_links` → `userID`
3. If no link: return "Spotify is not connected to this topic. Connect it from topic settings."
4. Get token for `userID`
5. If no token: clean up orphaned link, return reconnect message
6. Create `PersistingTokenSource` → `oauth2.NewClient` → `NewClient`
7. Dispatch to command handler

**Expose accessors:** `DB() *SpotifyDB` and `OAuth() *SpotifyOAuth` for server handlers.

### Phase 5: Config + Wiring

**Files:**
- Modify `config/config.go` — add `SpotifyClientID`, `SpotifyClientSecret` fields
- Modify `main.go` — conditional registration + pass to server

**Config:**
```go
SpotifyClientID     string `env:"BOBOT_SPOTIFY_CLIENT_ID"`
SpotifyClientSecret string `env:"BOBOT_SPOTIFY_CLIENT_SECRET"`
```

**main.go wiring (follows Calendar pattern):**
```go
var spotifyTool *spotify.SpotifyTool
if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
    spotifyDB, err := spotify.NewSpotifyDB(filepath.Join(cfg.DataDir, "tool_spotify.db"))
    // handle err
    spotifyTool = spotify.NewSpotifyTool(spotifyDB, cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.BaseURL)
    registry.Register(spotifyTool)
}
```

Pass `spotifyTool` to `server.NewWithAssistant()` (add parameter, like `calendarTool`).

### Phase 6: Server Routes

**Files:**
- Create `server/spotify.go`
- Modify `server/server.go` — register Spotify routes

**Route handlers:**

`handleSpotifyAuth(w, r)`:
1. Validate session user + `topic_id` query param
2. Generate state + PKCE verifier
3. Save to `oauth_states` table
4. Redirect to Spotify auth URL

`handleSpotifyCallback(w, r)`:
1. Validate `state` param exists in DB, check `error` query param for user denial
2. Verify session user matches `oauth_states.user_id`
3. Exchange code for token with PKCE verifier
4. Call `/v1/me` to check Premium
5. If not Premium: redirect to `/settings?topic_id=X&error=spotify_premium_required`
6. Save token keyed by `user_id`, create topic link
7. Delete oauth state
8. Redirect to `/settings?topic_id=X`

`handleSpotifyLink(w, r)`:
1. Validate session user has a token (`HasToken`)
2. Validate user is a member of the target topic
3. Create topic link record
4. Return 204

`handleSpotifyUnlink(w, r)`:
1. Validate session user owns the link (or is topic owner/admin)
2. Delete topic link record
3. Return 204

`handleSpotifyDisconnect(w, r)`:
1. Validate session user is the token owner
2. Revoke token at Spotify (best-effort)
3. Call `Disconnect(userID)` — deletes token + all links
4. Return 204

### Phase 7: Settings UI

**Files:**
- Modify `server/pages.go` — add Spotify fields to `PageData`
- Modify `web/templates/settings.html` — add Spotify row in Topic Tools section

**PageData additions:**
```go
SpotifyEnabled   bool   // true when env vars configured
SpotifyConnected bool   // true when current user has a token
SpotifyLinked    bool   // true when current topic has a link
SpotifyError     string // from ?error= query param
```

**Settings template:**
Add a Spotify row in the Topic Tools section (after Google Calendar), following the same pattern:
- Show error banner when `SpotifyError` is set
- "Connect Spotify" button when `!SpotifyConnected && !SpotifyLinked`
- "Link Spotify" button when `SpotifyConnected && !SpotifyLinked`
- "Unlink" button when `SpotifyLinked && SpotifyConnected` (user owns the link)
- "Disconnect" button alongside "Unlink" for the token owner

**JavaScript:** Add fetch handlers for link/unlink/disconnect buttons in `settings.js`, following the Calendar connect/disconnect pattern.

### Phase 8: Skill File (Optional)

**Files:**
- Create `skills/spotify.md`

**Content:** Teach the LLM effective Spotify tool usage:
- How to chain `search` → `play` (use URI from search results)
- How to handle "play X" requests (search first, then play top result)
- When to check `devices` (if play fails with no active device)
- Natural language mappings ("turn it up" → volume + 10, "skip" → next)

## Dependencies & Risks

- **Spotify Developer App:** Requires creating an app at developer.spotify.com with the correct redirect URI
- **Premium requirement:** Limits user base to Premium subscribers only
- **Device availability:** Most playback commands fail without an active Spotify device — this will be the most common error
- **Rate limiting:** Spotify rate limits are per-app, not per-user. Heavy usage from one user could affect all users
- **Token refresh timing:** Access tokens expire after 1 hour. The `PersistingTokenSource` handles this transparently

## References & Research

### Internal References

- Calendar tool (pattern reference): `tools/calendar/`
- ThinQ tool (simpler pattern): `tools/thinq/`
- Tool registry: `tools/registry.go`
- Auth context: `auth/context.go`
- Server routes: `server/server.go`
- Settings template: `web/templates/settings.html`
- Config: `config/config.go`
- SQLite safety learnings: `docs/solutions/database-issues/sqlite-scheduler-safety-improvements.md`
- Shared logic patterns: `docs/solutions/architecture-patterns/extract-shared-auto-read-marking-logic.md`

### External References

- Spotify Web API: https://developer.spotify.com/documentation/web-api
- Spotify OAuth: https://developer.spotify.com/documentation/web-api/tutorials/code-pkce-flow
- Spotify Scopes: https://developer.spotify.com/documentation/web-api/concepts/scopes

### Related Issues

- Issue: [#44](https://github.com/esnunes/bobot/issues/44)
- Brainstorm: `docs/brainstorms/2026-02-27-spotify-control-brainstorm.md`
