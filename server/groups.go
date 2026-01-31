// server/groups.go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
		"id":       group.ID,
		"name":     group.Name,
		"owner_id": group.OwnerID,
		"members":  memberList,
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
