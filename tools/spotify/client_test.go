package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(handler http.Handler) *Client {
	server := httptest.NewServer(handler)
	return &Client{http: server.Client(), baseURL: server.URL}
}

func TestSearch(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "bohemian rhapsody" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"tracks": map[string]any{
				"items": []map[string]any{
					{
						"name": "Bohemian Rhapsody",
						"uri":  "spotify:track:abc123",
						"album": map[string]any{"name": "A Night at the Opera"},
						"artists": []map[string]any{{"name": "Queen"}},
					},
				},
			},
		})
	}))

	tracks, err := client.Search(context.Background(), "bohemian rhapsody")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Name != "Bohemian Rhapsody" || tracks[0].Artist != "Queen" || tracks[0].URI != "spotify:track:abc123" {
		t.Errorf("unexpected track: %+v", tracks[0])
	}
}

func TestGetDevices(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"devices": []map[string]any{
				{"id": "dev1", "name": "Living Room Speaker", "type": "Speaker", "is_active": true, "volume_percent": 50},
				{"id": "dev2", "name": "Phone", "type": "Smartphone", "is_active": false, "volume_percent": 75},
			},
		})
	}))

	devices, err := client.GetDevices(context.Background())
	if err != nil {
		t.Fatalf("GetDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].Name != "Living Room Speaker" || !devices[0].IsActive {
		t.Errorf("unexpected device: %+v", devices[0])
	}
}

func TestGetPlaybackState(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"is_playing":  true,
			"progress_ms": 30000,
			"device": map[string]any{
				"id": "dev1", "name": "Speaker", "type": "Speaker", "is_active": true, "volume_percent": 60,
			},
			"item": map[string]any{
				"name":        "Test Song",
				"uri":         "spotify:track:xyz",
				"duration_ms": 180000,
				"album":       map[string]any{"name": "Test Album"},
				"artists":     []map[string]any{{"name": "Test Artist"}},
			},
		})
	}))

	state, err := client.GetPlaybackState(context.Background())
	if err != nil {
		t.Fatalf("GetPlaybackState: %v", err)
	}
	if state == nil {
		t.Fatal("expected playback state, got nil")
	}
	if !state.IsPlaying {
		t.Error("expected IsPlaying to be true")
	}
	if state.Track.Name != "Test Song" || state.Track.Artist != "Test Artist" {
		t.Errorf("unexpected track: %+v", state.Track)
	}
	if state.Device.Name != "Speaker" || state.Volume != 60 {
		t.Errorf("unexpected device: %+v, volume: %d", state.Device, state.Volume)
	}
}

func TestGetPlaybackStateNoContent(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	state, err := client.GetPlaybackState(context.Background())
	if err != nil {
		t.Fatalf("GetPlaybackState: %v", err)
	}
	if state != nil {
		t.Errorf("expected nil for no active playback, got %+v", state)
	}
}

func TestGetPlaylists(t *testing.T) {
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"name": "Workout Mix", "uri": "spotify:playlist:abc", "tracks": map[string]any{"total": 25}},
				{"name": "Chill Vibes", "uri": "spotify:playlist:def", "tracks": map[string]any{"total": 42}},
			},
		})
	}))

	playlists, err := client.GetPlaylists(context.Background())
	if err != nil {
		t.Fatalf("GetPlaylists: %v", err)
	}
	if len(playlists) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(playlists))
	}
	if playlists[0].Name != "Workout Mix" || playlists[0].TrackCount != 25 {
		t.Errorf("unexpected playlist: %+v", playlists[0])
	}
}

func TestPauseResume(t *testing.T) {
	var lastMethod, lastPath string
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		lastPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if lastMethod != "PUT" || lastPath != "/me/player/pause" {
		t.Errorf("Pause: expected PUT /me/player/pause, got %s %s", lastMethod, lastPath)
	}

	if err := client.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if lastMethod != "PUT" || lastPath != "/me/player/play" {
		t.Errorf("Resume: expected PUT /me/player/play, got %s %s", lastMethod, lastPath)
	}
}

func TestNextPrevious(t *testing.T) {
	var lastPath string
	client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := client.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if lastPath != "/me/player/next" {
		t.Errorf("Next: expected /me/player/next, got %s", lastPath)
	}

	if err := client.Previous(context.Background()); err != nil {
		t.Fatalf("Previous: %v", err)
	}
	if lastPath != "/me/player/previous" {
		t.Errorf("Previous: expected /me/player/previous, got %s", lastPath)
	}
}

func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		contains string
	}{
		{"unauthorized", 401, "expired or was revoked"},
		{"forbidden", 403, "Premium is required"},
		{"not found", 404, "No active playback"},
		{"rate limited", 429, "rate-limited"},
		{"server error", 500, "temporarily unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte("error"))
			}))

			err := client.Pause(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tt.contains) {
				t.Errorf("expected error containing %q, got %q", tt.contains, got)
			}
		})
	}
}
