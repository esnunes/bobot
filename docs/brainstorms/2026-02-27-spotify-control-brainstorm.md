# Brainstorm: Add Spotify Control

**Date:** 2026-02-27
**Issue:** [#44](https://github.com/esnunes/bobot/issues/44)
**Status:** Draft

## What We're Building

A Spotify integration tool that lets users control music playback through the chatbot's natural language interface. Users connect their Spotify account via OAuth2, and the LLM invokes tool commands for search, playback control, device selection, and status queries.

### Use Cases (from issue)

- Search for a song
- Select which device the music should be played on
- Play/pause/resume/next/previous
- Control volume of the device playing the song
- View currently playing track info (added during brainstorm)

## Why This Approach

### Data Model: User-keyed tokens + topic link table

Spotify tokens are stored per `user_id` (not per topic like Calendar). A separate `topic_links` table maps `topic_id -> user_id`, indicating which user's Spotify account is active in that topic.

**Rationale:** A user has one Spotify account. Storing the token by user avoids duplication and makes disconnect/reconnect clean. The link table lets multiple topics share the same connection without token ownership confusion.

### Connection Scope: Global per-user, linkable to topics

- A user connects their Spotify account once (from any topic's settings).
- That connection is linked to the topic where they connected.
- From other topics' settings, they can "link" their existing Spotify connection.
- Other users in a linked topic get full control of the connected Spotify account.

### Settings UI: Topic settings (following Calendar pattern)

- Connect/disconnect lives in topic-level settings, like Calendar.
- If the user already has a Spotify connection, topic settings show a "Link" option instead of "Connect".
- Disconnect from a topic just removes the link. Full disconnect (revoke token) only when disconnecting from the last linked topic, or via an explicit "Revoke" action.

## Key Decisions

1. **Token storage:** Per `user_id`, not per `topic_id`. Departs from Calendar pattern but better models "one user, one Spotify account."
2. **Topic linking:** Separate `topic_links` table. First connect creates token + link. Additional topics just add links.
3. **Permissions:** Full control for all users in a linked topic (no granular permissions for MVP).
4. **Status command:** Include a `status`/`now_playing` command that returns current track, artist, device, and playback state.
5. **Settings UI:** Topic settings only, following Calendar pattern. Show "Link existing" when user already has a connection.
6. **Queue management:** Deferred. Not in MVP.
7. **Playlist support:** Included in MVP. List playlists and play a playlist.
8. **Free-tier handling:** Block connection at OAuth callback with a clear Premium-required message.

## Tool Commands (MVP)

| Command | Description |
|---------|-------------|
| `search` | Search for tracks by query string |
| `play` | Play a track (by URI or from search results) |
| `pause` | Pause playback |
| `resume` | Resume playback |
| `next` | Skip to next track |
| `previous` | Go to previous track |
| `volume` | Set volume level (0-100) |
| `devices` | List available playback devices |
| `transfer` | Transfer playback to a specific device |
| `status` | Get currently playing track info |
| `playlists` | List user's playlists |
| `play_playlist` | Play a specific playlist |

## Spotify API Scopes Needed

- `user-read-playback-state` - Read playback state/devices
- `user-modify-playback-state` - Play, pause, skip, volume, transfer
- `user-read-currently-playing` - Currently playing track
- `playlist-read-private` - Read user's private playlists
- `playlist-read-collaborative` - Read collaborative playlists

## Implementation Layers (following repo patterns)

1. **Config:** `BOBOT_SPOTIFY_CLIENT_ID`, `BOBOT_SPOTIFY_CLIENT_SECRET` env vars
2. **Package:** `tools/spotify/` with `db.go`, `oauth.go`, `client.go`, `spotify.go`
3. **DB:** `tool_spotify.db` with tables for `oauth_states`, `tokens` (keyed by user_id), `topic_links`
4. **Server routes:** `/api/spotify/auth`, `/api/spotify/callback`, `DELETE /api/spotify`
5. **Settings UI:** Spotify row in topic tools section
6. **Skill file (optional):** `skills/spotify.md` for LLM guidance

## Resolved Questions

1. **Queue management:** Deferred to a future iteration. MVP focuses on direct playback control.
2. **Playlist browsing:** Included in MVP. Users can list their playlists and play a specific playlist.
3. **Spotify Premium requirement:** Check account type during OAuth callback. If the user is on the free tier, show a clear error message that Premium is required and don't complete the connection.
