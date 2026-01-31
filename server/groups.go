// server/groups.go
package server

import (
	"encoding/json"
	"net/http"
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
