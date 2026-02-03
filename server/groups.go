// server/groups.go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
)

type createGroupRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		http.Error(w, "name required (max 100 chars)", http.StatusBadRequest)
		return
	}

	group, err := s.db.CreateGroup(req.Name, userData.UserID)
	if err != nil {
		http.Error(w, "failed to create group", http.StatusInternalServerError)
		return
	}

	// Add creator as first member
	if err := s.db.AddGroupMember(group.ID, userData.UserID); err != nil {
		http.Error(w, "failed to add owner to group", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/groups/"+strconv.FormatInt(group.ID, 10))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       group.ID,
		"name":     group.Name,
		"owner_id": group.OwnerID,
	})
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groups, err := s.db.GetUserGroups(userData.UserID)
	if err != nil {
		http.Error(w, "failed to list groups", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		members, _ := s.db.GetGroupMembers(g.ID)
		result = append(result, map[string]interface{}{
			"id":           g.ID,
			"name":         g.Name,
			"owner_id":     g.OwnerID,
			"member_count": len(members),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	// Check membership
	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	members, _ := s.db.GetGroupMembers(groupID)

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
		"id":              group.ID,
		"name":            group.Name,
		"owner_id":        group.OwnerID,
		"current_user_id": userData.UserID,
		"members":         memberList,
	})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	// Only owner can delete
	if group.OwnerID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.SoftDeleteGroup(groupID); err != nil {
		http.Error(w, "failed to delete group", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type addMemberRequest struct {
	Username string `json:"username"`
}

func (s *Server) handleAddGroupMember(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	// Only owner can add members
	if group.OwnerID != userData.UserID {
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

	if err := s.db.AddGroupMember(groupID, user.ID); err != nil {
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

func (s *Server) handleRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	targetUserID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	group, err := s.db.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	// Owner cannot leave (must delete group)
	if targetUserID == group.OwnerID {
		http.Error(w, "owner cannot leave group", http.StatusForbidden)
		return
	}

	// Only owner or self can remove
	if group.OwnerID != userData.UserID && targetUserID != userData.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.db.RemoveGroupMember(groupID, targetUserID); err != nil {
		http.Error(w, "failed to remove member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGroupRecentMessages(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	// Check membership
	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
	if err != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	messages, err := s.db.GetGroupRecentMessages(groupID, limit)
	if err != nil {
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}

	// Enrich with user display names
	result := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		item := map[string]interface{}{
			"ID":        m.ID,
			"Role":      m.Role,
			"Content":   m.Content,
			"CreatedAt": m.CreatedAt,
		}
		if m.Role == "user" {
			if user, err := s.db.GetUserByID(m.UserID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGroupMessageHistory(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
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

	messages, err := s.db.GetGroupMessagesBefore(groupID, beforeID, limit)
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
		if m.Role == "user" {
			if user, err := s.db.GetUserByID(m.UserID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGroupMessageSync(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid group id", http.StatusBadRequest)
		return
	}

	isMember, err := s.db.IsGroupMember(groupID, userData.UserID)
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

	messages, err := s.db.GetGroupMessagesSince(groupID, since)
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
		if m.Role == "user" {
			if user, err := s.db.GetUserByID(m.UserID); err == nil {
				item["UserID"] = user.ID
				item["DisplayName"] = user.DisplayName
			}
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
