// server/messages.go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/esnunes/bobot/auth"
)

func (s *Server) handleRecentMessages(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())
	if userData.UserID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > s.cfg.History.MaxLimit {
		limit = s.cfg.History.DefaultLimit
	}

	messages, err := s.db.GetPrivateChatRecentMessages(userData.UserID, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleMessageHistory(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())
	if userData.UserID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if beforeID <= 0 {
		http.Error(w, "invalid before parameter", http.StatusBadRequest)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > s.cfg.History.MaxLimit {
		limit = s.cfg.History.DefaultLimit
	}

	messages, err := s.db.GetPrivateChatMessagesBefore(userData.UserID, beforeID, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleMessageSync(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())
	if userData.UserID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		http.Error(w, "missing since parameter", http.StatusBadRequest)
		return
	}

	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		http.Error(w, "invalid since parameter", http.StatusBadRequest)
		return
	}

	// Apply max lookback
	minTime := time.Now().Add(-s.cfg.Sync.MaxLookback)
	if since.Before(minTime) {
		since = minTime
	}

	messages, err := s.db.GetPrivateChatMessagesSince(userData.UserID, since)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}
