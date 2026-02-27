// tools/spotify/spotify.go
package spotify

import (
	"context"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
	"golang.org/x/oauth2"
)

// SpotifyTool implements the tools.Tool interface for Spotify playback control.
type SpotifyTool struct {
	db    *SpotifyDB
	oauth *SpotifyOAuth
}

// NewSpotifyTool creates a new SpotifyTool.
func NewSpotifyTool(db *SpotifyDB, clientID, clientSecret, baseURL string) *SpotifyTool {
	return &SpotifyTool{
		db:    db,
		oauth: NewOAuth(clientID, clientSecret, baseURL, db),
	}
}

func (t *SpotifyTool) Name() string    { return "spotify" }
func (t *SpotifyTool) AdminOnly() bool { return false }

func (t *SpotifyTool) Description() string {
	return "Control Spotify music playback. Search for songs, play/pause/skip tracks, control volume, list devices, browse playlists, and check what's currently playing. Spotify must be connected by a user in topic settings first."
}

func (t *SpotifyTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"search", "play", "pause", "resume", "next", "previous", "volume", "devices", "transfer", "status", "playlists", "play_playlist"},
				"description": "The Spotify operation to perform",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (for search command)",
			},
			"uri": map[string]any{
				"type":        "string",
				"description": "Spotify URI to play (for play and play_playlist commands)",
			},
			"device_id": map[string]any{
				"type":        "string",
				"description": "Target device ID (for play, play_playlist, and transfer commands)",
			},
			"level": map[string]any{
				"type":        "integer",
				"description": "Volume level 0-100 (for volume command)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *SpotifyTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing arguments. Usage: /spotify <command> [options]")
	}

	command := parts[0]
	result := map[string]any{"command": command}

	switch command {
	case "search":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /spotify search <query>")
		}
		result["query"] = strings.Join(parts[1:], " ")
	case "play":
		if len(parts) >= 2 {
			result["uri"] = parts[1]
		}
	case "play_playlist":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /spotify play_playlist <uri>")
		}
		result["uri"] = parts[1]
	case "volume":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /spotify volume <0-100>")
		}
		result["level"] = parts[1]
	case "transfer":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /spotify transfer <device_id>")
		}
		result["device_id"] = parts[1]
	case "pause", "resume", "next", "previous", "devices", "status", "playlists":
		// No extra args needed
	default:
		return nil, fmt.Errorf("unknown command: %s. Available: search, play, pause, resume, next, previous, volume, devices, transfer, status, playlists, play_playlist", command)
	}

	return result, nil
}

func (t *SpotifyTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command")
	}

	chatData := auth.ChatDataFromContext(ctx)
	topicID := chatData.TopicID
	if topicID == 0 {
		return "", fmt.Errorf("spotify tool requires a topic context")
	}

	// Resolve topic -> user via link table
	link, err := t.db.GetTopicLink(topicID)
	if err != nil {
		return "", fmt.Errorf("checking spotify link: %w", err)
	}
	if link == nil {
		return "Spotify is not connected to this topic. A user can connect it from the topic settings page.", nil
	}

	// Get token source for the linked user
	ts, err := t.oauth.GetTokenSource(link.UserID)
	if err != nil {
		return "", fmt.Errorf("getting spotify access: %w", err)
	}
	if ts == nil {
		// Orphaned link - clean up
		t.db.UnlinkTopic(topicID)
		return "Spotify connection was lost. Please reconnect from topic settings.", nil
	}

	// Create authenticated client
	client := NewClient(oauth2.NewClient(ctx, ts))

	switch command {
	case "search":
		return t.execSearch(ctx, client, input)
	case "play":
		return t.execPlay(ctx, client, input)
	case "pause":
		return t.execPause(ctx, client)
	case "resume":
		return t.execResume(ctx, client)
	case "next":
		return t.execNext(ctx, client)
	case "previous":
		return t.execPrevious(ctx, client)
	case "volume":
		return t.execVolume(ctx, client, input)
	case "devices":
		return t.execDevices(ctx, client)
	case "transfer":
		return t.execTransfer(ctx, client, input)
	case "status":
		return t.execStatus(ctx, client)
	case "playlists":
		return t.execPlaylists(ctx, client)
	case "play_playlist":
		return t.execPlayPlaylist(ctx, client, input)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *SpotifyTool) execSearch(ctx context.Context, client *Client, input map[string]any) (string, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required for search")
	}

	tracks, err := client.Search(ctx, query)
	if err != nil {
		return "", err
	}

	if len(tracks) == 0 {
		return fmt.Sprintf("No tracks found for '%s'.", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for '%s':\n\n", query))
	for i, track := range tracks {
		sb.WriteString(fmt.Sprintf("%d. **%s** by %s (Album: %s) [URI: %s]\n", i+1, track.Name, track.Artist, track.Album, track.URI))
	}

	return sb.String(), nil
}

func (t *SpotifyTool) execPlay(ctx context.Context, client *Client, input map[string]any) (string, error) {
	uri, _ := input["uri"].(string)
	if uri == "" {
		// No URI means resume
		if err := client.Resume(ctx); err != nil {
			return "", err
		}
		return "Playback resumed.", nil
	}

	deviceID, _ := input["device_id"].(string)
	if err := client.Play(ctx, uri, deviceID); err != nil {
		return "", err
	}

	return "Now playing.", nil
}

func (t *SpotifyTool) execPause(ctx context.Context, client *Client) (string, error) {
	if err := client.Pause(ctx); err != nil {
		return "", err
	}
	return "Playback paused.", nil
}

func (t *SpotifyTool) execResume(ctx context.Context, client *Client) (string, error) {
	if err := client.Resume(ctx); err != nil {
		return "", err
	}
	return "Playback resumed.", nil
}

func (t *SpotifyTool) execNext(ctx context.Context, client *Client) (string, error) {
	if err := client.Next(ctx); err != nil {
		return "", err
	}
	return "Skipped to next track.", nil
}

func (t *SpotifyTool) execPrevious(ctx context.Context, client *Client) (string, error) {
	if err := client.Previous(ctx); err != nil {
		return "", err
	}
	return "Went to previous track.", nil
}

func (t *SpotifyTool) execVolume(ctx context.Context, client *Client, input map[string]any) (string, error) {
	var level int
	switch v := input["level"].(type) {
	case float64:
		level = int(v)
	case int:
		level = v
	case string:
		// Try to parse
		fmt.Sscanf(v, "%d", &level)
	default:
		return "", fmt.Errorf("level is required for volume (0-100)")
	}

	if level < 0 || level > 100 {
		return "", fmt.Errorf("volume level must be between 0 and 100")
	}

	if err := client.SetVolume(ctx, level); err != nil {
		return "", err
	}
	return fmt.Sprintf("Volume set to %d%%.", level), nil
}

func (t *SpotifyTool) execDevices(ctx context.Context, client *Client) (string, error) {
	devices, err := client.GetDevices(ctx)
	if err != nil {
		return "", err
	}

	if len(devices) == 0 {
		return "No active Spotify devices found. Please open Spotify on your phone, computer, or speaker, then try again.", nil
	}

	var sb strings.Builder
	sb.WriteString("Available Spotify devices:\n\n")
	for _, d := range devices {
		active := ""
		if d.IsActive {
			active = " (active)"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s)%s Volume: %d%% [ID: %s]\n", d.Name, d.Type, active, d.Volume, d.ID))
	}

	return sb.String(), nil
}

func (t *SpotifyTool) execTransfer(ctx context.Context, client *Client, input map[string]any) (string, error) {
	deviceID, _ := input["device_id"].(string)
	if deviceID == "" {
		return "", fmt.Errorf("device_id is required for transfer")
	}

	if err := client.TransferPlayback(ctx, deviceID); err != nil {
		return "", err
	}
	return "Playback transferred.", nil
}

func (t *SpotifyTool) execStatus(ctx context.Context, client *Client) (string, error) {
	state, err := client.GetPlaybackState(ctx)
	if err != nil {
		return "", err
	}

	if state == nil {
		return "No active playback session.", nil
	}

	status := "Paused"
	if state.IsPlaying {
		status = "Playing"
	}

	progressSec := state.ProgressMs / 1000
	durationSec := state.DurationMs / 1000

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s**: %s\n", status, state.Track.Name))
	sb.WriteString(fmt.Sprintf("Artist: %s\n", state.Track.Artist))
	sb.WriteString(fmt.Sprintf("Album: %s\n", state.Track.Album))
	sb.WriteString(fmt.Sprintf("Progress: %d:%02d / %d:%02d\n", progressSec/60, progressSec%60, durationSec/60, durationSec%60))
	sb.WriteString(fmt.Sprintf("Device: %s (%s)\n", state.Device.Name, state.Device.Type))
	sb.WriteString(fmt.Sprintf("Volume: %d%%", state.Volume))

	return sb.String(), nil
}

func (t *SpotifyTool) execPlaylists(ctx context.Context, client *Client) (string, error) {
	playlists, err := client.GetPlaylists(ctx)
	if err != nil {
		return "", err
	}

	if len(playlists) == 0 {
		return "No playlists found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Your playlists:\n\n")
	for _, p := range playlists {
		sb.WriteString(fmt.Sprintf("- **%s** (%d tracks) [URI: %s]\n", p.Name, p.TrackCount, p.URI))
	}

	return sb.String(), nil
}

func (t *SpotifyTool) execPlayPlaylist(ctx context.Context, client *Client, input map[string]any) (string, error) {
	uri, _ := input["uri"].(string)
	if uri == "" {
		return "", fmt.Errorf("uri is required for play_playlist")
	}

	deviceID, _ := input["device_id"].(string)
	if err := client.PlayContext(ctx, uri, deviceID); err != nil {
		return "", err
	}

	return "Now playing playlist.", nil
}

// DB returns the spotify database for use by server handlers.
func (t *SpotifyTool) DB() *SpotifyDB {
	return t.db
}

// OAuth returns the OAuth config for use by server handlers.
func (t *SpotifyTool) OAuth() *SpotifyOAuth {
	return t.oauth
}
