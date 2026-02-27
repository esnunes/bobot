// tools/spotify/client.go
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const spotifyAPIBase = "https://api.spotify.com/v1"

// Client wraps HTTP calls to the Spotify Web API.
type Client struct {
	http    *http.Client
	baseURL string // overridable for testing
}

// NewClient creates a Spotify API client. The httpClient should be an OAuth2-authenticated client.
func NewClient(httpClient *http.Client) *Client {
	return &Client{http: httpClient, baseURL: spotifyAPIBase}
}

// Track represents a Spotify track.
type Track struct {
	Name   string
	Artist string
	Album  string
	URI    string
}

// Device represents a Spotify playback device.
type Device struct {
	ID       string
	Name     string
	Type     string
	IsActive bool
	Volume   int
}

// PlaybackState represents the current playback state.
type PlaybackState struct {
	IsPlaying  bool
	Track      Track
	Device     Device
	ProgressMs int
	DurationMs int
	Volume     int
}

// Playlist represents a user's Spotify playlist.
type Playlist struct {
	Name       string
	URI        string
	TrackCount int
}

// User represents basic Spotify user info.
type User struct {
	Product string
}

// Search searches for tracks by query.
func (c *Client) Search(ctx context.Context, query string) ([]Track, error) {
	u := c.baseURL + "/search?type=track&limit=5&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var result struct {
		Tracks struct {
			Items []struct {
				Name    string `json:"name"`
				URI     string `json:"uri"`
				Album   struct {
					Name string `json:"name"`
				} `json:"album"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	tracks := make([]Track, 0, len(result.Tracks.Items))
	for _, item := range result.Tracks.Items {
		artist := ""
		if len(item.Artists) > 0 {
			artist = item.Artists[0].Name
		}
		tracks = append(tracks, Track{
			Name:   item.Name,
			Artist: artist,
			Album:  item.Album.Name,
			URI:    item.URI,
		})
	}
	return tracks, nil
}

// Play starts playback of a specific URI on an optional device.
func (c *Client) Play(ctx context.Context, uri string, deviceID string) error {
	u := c.baseURL + "/me/player/play"
	if deviceID != "" {
		u += "?device_id=" + url.QueryEscape(deviceID)
	}

	body := map[string]any{"uris": []string{uri}}
	return c.putJSON(ctx, u, body)
}

// PlayContext starts playback of a context URI (album, playlist, artist).
func (c *Client) PlayContext(ctx context.Context, contextURI string, deviceID string) error {
	u := c.baseURL + "/me/player/play"
	if deviceID != "" {
		u += "?device_id=" + url.QueryEscape(deviceID)
	}

	body := map[string]any{"context_uri": contextURI}
	return c.putJSON(ctx, u, body)
}

// Resume resumes current playback.
func (c *Client) Resume(ctx context.Context) error {
	u := c.baseURL + "/me/player/play"
	return c.putJSON(ctx, u, nil)
}

// Pause pauses current playback.
func (c *Client) Pause(ctx context.Context) error {
	u := c.baseURL + "/me/player/pause"
	return c.putJSON(ctx, u, nil)
}

// Next skips to the next track.
func (c *Client) Next(ctx context.Context) error {
	return c.post(ctx, c.baseURL+"/me/player/next")
}

// Previous goes to the previous track.
func (c *Client) Previous(ctx context.Context) error {
	return c.post(ctx, c.baseURL+"/me/player/previous")
}

// SetVolume sets the playback volume (0-100).
func (c *Client) SetVolume(ctx context.Context, level int) error {
	u := c.baseURL + "/me/player/volume?volume_percent=" + strconv.Itoa(level)
	return c.putJSON(ctx, u, nil)
}

// GetDevices returns available playback devices.
func (c *Client) GetDevices(ctx context.Context) ([]Device, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/me/player/devices", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("devices request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var result struct {
		Devices []struct {
			ID               string `json:"id"`
			Name             string `json:"name"`
			Type             string `json:"type"`
			IsActive         bool   `json:"is_active"`
			VolumePercent    int    `json:"volume_percent"`
		} `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding devices: %w", err)
	}

	devices := make([]Device, 0, len(result.Devices))
	for _, d := range result.Devices {
		devices = append(devices, Device{
			ID:       d.ID,
			Name:     d.Name,
			Type:     d.Type,
			IsActive: d.IsActive,
			Volume:   d.VolumePercent,
		})
	}
	return devices, nil
}

// TransferPlayback transfers playback to a specific device.
func (c *Client) TransferPlayback(ctx context.Context, deviceID string) error {
	body := map[string]any{"device_ids": []string{deviceID}}
	return c.putJSON(ctx, c.baseURL+"/me/player", body)
}

// GetPlaybackState returns the current playback state.
func (c *Client) GetPlaybackState(ctx context.Context) (*PlaybackState, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/me/player", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("playback state request: %w", err)
	}
	defer resp.Body.Close()

	// 204 means no active playback
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var result struct {
		IsPlaying  bool `json:"is_playing"`
		ProgressMs int  `json:"progress_ms"`
		Device     struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Type          string `json:"type"`
			IsActive      bool   `json:"is_active"`
			VolumePercent int    `json:"volume_percent"`
		} `json:"device"`
		Item struct {
			Name       string `json:"name"`
			URI        string `json:"uri"`
			DurationMs int    `json:"duration_ms"`
			Album      struct {
				Name string `json:"name"`
			} `json:"album"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding playback state: %w", err)
	}

	artist := ""
	if len(result.Item.Artists) > 0 {
		artist = result.Item.Artists[0].Name
	}

	return &PlaybackState{
		IsPlaying:  result.IsPlaying,
		ProgressMs: result.ProgressMs,
		DurationMs: result.Item.DurationMs,
		Volume:     result.Device.VolumePercent,
		Track: Track{
			Name:   result.Item.Name,
			Artist: artist,
			Album:  result.Item.Album.Name,
			URI:    result.Item.URI,
		},
		Device: Device{
			ID:       result.Device.ID,
			Name:     result.Device.Name,
			Type:     result.Device.Type,
			IsActive: result.Device.IsActive,
			Volume:   result.Device.VolumePercent,
		},
	}, nil
}

// GetPlaylists returns the user's playlists.
func (c *Client) GetPlaylists(ctx context.Context) ([]Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/me/playlists?limit=20", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("playlists request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			Name   string `json:"name"`
			URI    string `json:"uri"`
			Tracks struct {
				Total int `json:"total"`
			} `json:"tracks"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding playlists: %w", err)
	}

	playlists := make([]Playlist, 0, len(result.Items))
	for _, item := range result.Items {
		playlists = append(playlists, Playlist{
			Name:       item.Name,
			URI:        item.URI,
			TrackCount: item.Tracks.Total,
		})
	}
	return playlists, nil
}

// helper methods

func (c *Client) putJSON(ctx context.Context, url string, body any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	return checkResponse(resp)
}

func (c *Client) post(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	return checkResponse(resp)
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case 401:
		return fmt.Errorf("Spotify access expired or was revoked. Reconnect from topic settings.")
	case 403:
		return fmt.Errorf("Spotify Premium is required for playback control. Your account may have been downgraded.")
	case 404:
		return fmt.Errorf("No active playback session. Try playing a song first.")
	case 429:
		return fmt.Errorf("Spotify is temporarily rate-limited. Please try again in a moment.")
	}

	if resp.StatusCode >= 500 {
		return fmt.Errorf("Spotify is temporarily unavailable.")
	}

	return fmt.Errorf("Spotify API error %d: %s", resp.StatusCode, body)
}
