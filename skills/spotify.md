---
name: spotify
description: Guide for controlling Spotify music playback
---
## Using the Spotify tool

The `spotify` tool lets you control music playback for a connected Spotify account. Spotify must be connected by a user from the topic settings page before it can be used.

### Available commands

- `search` - Search for tracks by name, artist, or album. Always search before playing if the user asks for a specific song.
- `play` - Play a track by Spotify URI. Use a URI from search results.
- `pause` - Pause the current playback.
- `resume` - Resume paused playback. Also used by `play` with no URI.
- `next` - Skip to the next track.
- `previous` - Go back to the previous track.
- `volume` - Set volume (0-100).
- `devices` - List available playback devices. Use before `transfer` so the user can pick.
- `transfer` - Move playback to a different device by device ID.
- `status` - Show the currently playing track, artist, device, and progress.
- `playlists` - List the user's Spotify playlists.
- `play_playlist` - Play a playlist by URI.

### Tips

- When a user says "play X", search first, then play the top result.
- If playback fails with "no active device", suggest the user opens Spotify on a device and try `devices` to list them.
- Use `status` to check what's currently playing before making changes.
- For volume, convert natural language ("turn it up", "half volume") to a 0-100 number.
