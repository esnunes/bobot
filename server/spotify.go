// server/spotify.go
package server

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/tools/spotify"
)

func (s *Server) handleSpotifyAuth(w http.ResponseWriter, r *http.Request) {
	if s.spotifyTool == nil {
		http.Error(w, "Spotify not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr == "" {
		http.Error(w, "missing topic_id", http.StatusBadRequest)
		return
	}

	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	authURL, err := s.spotifyTool.OAuth().GenerateAuthURL(userData.UserID, topicID)
	if err != nil {
		slog.Error("spotify: failed to generate auth URL", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	if s.spotifyTool == nil {
		http.Error(w, "Spotify not configured", http.StatusNotFound)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	topicID, stateUserID, token, err := s.spotifyTool.OAuth().ExchangeCode(r.Context(), code, state)
	if err != nil {
		slog.Error("spotify: OAuth exchange failed", "error", err)
		http.Error(w, "OAuth authorization failed", http.StatusBadRequest)
		return
	}

	// Verify the session user matches the user who initiated the OAuth flow
	userData := auth.UserDataFromContext(r.Context())
	if userData.UserID != stateUserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Check if user has Spotify Premium
	isPremium, err := spotify.CheckPremium(r.Context(), token.AccessToken)
	if err != nil {
		slog.Error("spotify: failed to check premium status", "error", err)
		http.Error(w, "Failed to verify Spotify account", http.StatusInternalServerError)
		return
	}
	if !isPremium {
		http.Redirect(w, r, "/settings?topic_id="+strconv.FormatInt(topicID, 10)+"&spotify_error=premium_required", http.StatusFound)
		return
	}

	// Save token and link topic
	if err := s.spotifyTool.OAuth().SaveTokenAndLink(userData.UserID, topicID, token); err != nil {
		slog.Error("spotify: failed to save token and link", "error", err)
		http.Error(w, "Failed to complete Spotify connection", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings?topic_id="+strconv.FormatInt(topicID, 10), http.StatusFound)
}

func (s *Server) handleSpotifyLink(w http.ResponseWriter, r *http.Request) {
	if s.spotifyTool == nil {
		http.Error(w, "Spotify not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr == "" {
		http.Error(w, "missing topic_id", http.StatusBadRequest)
		return
	}

	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Verify user actually has a Spotify token
	hasToken, err := s.spotifyTool.DB().HasToken(userData.UserID)
	if err != nil {
		slog.Error("spotify: failed to check token", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !hasToken {
		http.Error(w, "no Spotify connection found", http.StatusBadRequest)
		return
	}

	if err := s.spotifyTool.DB().LinkTopic(topicID, userData.UserID); err != nil {
		slog.Error("spotify: failed to link topic", "error", err)
		http.Error(w, "failed to link Spotify", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpotifyUnlink(w http.ResponseWriter, r *http.Request) {
	if s.spotifyTool == nil {
		http.Error(w, "Spotify not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())
	topicIDStr := r.URL.Query().Get("topic_id")
	if topicIDStr == "" {
		http.Error(w, "missing topic_id", http.StatusBadRequest)
		return
	}

	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	if err := auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.spotifyTool.DB().UnlinkTopic(topicID); err != nil {
		slog.Error("spotify: failed to unlink topic", "error", err)
		http.Error(w, "failed to unlink Spotify", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpotifyDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.spotifyTool == nil {
		http.Error(w, "Spotify not configured", http.StatusNotFound)
		return
	}

	userData := auth.UserDataFromContext(r.Context())

	// Remove local token and all topic links
	// (Spotify has no token revocation endpoint; tokens expire naturally)
	if err := s.spotifyTool.DB().Disconnect(userData.UserID); err != nil {
		slog.Error("spotify: disconnect failed", "error", err)
		http.Error(w, "failed to disconnect", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
