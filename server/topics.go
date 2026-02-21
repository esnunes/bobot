// server/topics.go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
)

func (s *Server) handleCreateTopic(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || len(name) > 100 {
		http.Error(w, "name required (max 100 chars)", http.StatusBadRequest)
		return
	}

	topic, err := s.db.CreateTopic(name, userData.UserID)
	if err != nil {
		http.Error(w, "failed to create topic", http.StatusInternalServerError)
		return
	}

	// Add creator as first member
	if err := s.db.AddTopicMember(topic.ID, userData.UserID); err != nil {
		http.Error(w, "failed to add owner to topic", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "/chats/`+strconv.FormatInt(topic.ID, 10)+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTopics(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topics, err := s.db.GetUserTopics(userData.UserID)
	if err != nil {
		http.Error(w, "failed to list topics", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(topics))
	for _, t := range topics {
		members, _ := s.db.GetTopicMembers(t.ID)
		result = append(result, map[string]interface{}{
			"id":           t.ID,
			"name":         t.Name,
			"owner_id":     t.OwnerID,
			"member_count": len(members),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGetTopic(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	// Check membership
	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	members, _ := s.db.GetTopicMembers(topicID)

	memberList := make([]map[string]interface{}, 0, len(members))
	for _, m := range members {
		memberList = append(memberList, map[string]interface{}{
			"user_id":      m.UserID,
			"username":     m.Username,
			"display_name": m.DisplayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":              topic.ID,
		"name":            topic.Name,
		"owner_id":        topic.OwnerID,
		"current_user_id": userData.UserID,
		"members":         memberList,
	})
}

func (s *Server) handleDeleteTopic(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	// Only owner can delete
	if topic.OwnerID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.SoftDeleteTopic(topicID); err != nil {
		http.Error(w, "failed to delete topic", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type addMemberRequest struct {
	Username string `json:"username"`
}

func (s *Server) handleAddTopicMember(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	// Only owner can add members
	if topic.OwnerID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByUsername(req.Username)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if err := s.db.AddTopicMember(topicID, user.ID); err != nil {
		// Could be duplicate - treat as success (idempotent)
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "failed to add member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleRemoveTopicMember(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	targetUserID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	// Owner cannot leave (must delete topic)
	if targetUserID == topic.OwnerID {
		http.Error(w, "owner cannot leave topic", http.StatusForbidden)
		return
	}

	// Only owner or self can remove
	if topic.OwnerID != userData.UserID && targetUserID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.RemoveTopicMember(topicID, targetUserID); err != nil {
		http.Error(w, "failed to remove member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMuteTopic(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	if _, err := s.db.GetTopicByID(topicID); err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.SetTopicMemberMuted(topicID, userData.UserID, true); err != nil {
		http.Error(w, "failed to mute topic", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnmuteTopic(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	if _, err := s.db.GetTopicByID(topicID); err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.SetTopicMemberMuted(topicID, userData.UserID, false); err != nil {
		http.Error(w, "failed to unmute topic", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTopicMessageHistory(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	messages, err := s.db.GetTopicMessagesBefore(topicID, beforeID, limit)
	if err != nil {
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		item := map[string]interface{}{
			"ID":        m.ID,
			"Role":      m.Role,
			"Content":   m.Content,
			"CreatedAt": m.CreatedAt,
		}
		switch m.Role {
		case "user", "command":
			if user, err := s.db.GetUserByID(m.SenderID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		case "assistant", "system":
			item["DisplayName"] = "bobot"
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleTopicMessageSync(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		http.Error(w, "invalid since parameter", http.StatusBadRequest)
		return
	}

	messages, err := s.db.GetTopicMessagesSince(topicID, since)
	if err != nil {
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		item := map[string]interface{}{
			"ID":        m.ID,
			"Role":      m.Role,
			"Content":   m.Content,
			"CreatedAt": m.CreatedAt,
		}
		switch m.Role {
		case "user", "command":
			if user, err := s.db.GetUserByID(m.SenderID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		case "assistant", "system":
			item["DisplayName"] = "bobot"
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
