package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")

	var skills []db.SkillRow
	var err error

	if topicIDStr != "" {
		topicID, parseErr := strconv.ParseInt(topicIDStr, 10, 64)
		if parseErr != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		// Verify membership
		isMember, memberErr := s.db.IsTopicMember(topicID, userData.UserID)
		if memberErr != nil || !isMember {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		skills, err = s.db.GetTopicSkills(topicID)
	} else {
		skills, err = s.db.GetPrivateChatSkills(userData.UserID)
	}

	if err != nil {
		http.Error(w, "failed to list skills", http.StatusInternalServerError)
		return
	}

	result := make([]map[string]any, 0, len(skills))
	for _, sk := range skills {
		item := map[string]any{
			"id":          sk.ID,
			"name":        sk.Name,
			"description": sk.Description,
			"content":     sk.Content,
			"user_id":     sk.UserID,
			"created_at":  sk.CreatedAt,
			"updated_at":  sk.UpdatedAt,
		}
		if sk.TopicID != nil {
			item["topic_id"] = *sk.TopicID
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type createSkillRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	TopicID     *int64 `json:"topic_id"`
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	var req createSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	// Permission check for topic skills
	if req.TopicID != nil {
		if err := s.canManageTopicSkills(userData, *req.TopicID); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	skill, err := s.db.CreateSkill(userData.UserID, req.TopicID, req.Name, req.Description, req.Content)
	if err != nil {
		http.Error(w, "failed to create skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": skill.ID, "name": skill.Name})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to get skill", http.StatusInternalServerError)
		return
	}

	// Verify ownership/membership
	if err := s.canViewSkill(userData, skill); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	result := map[string]any{
		"id":          skill.ID,
		"name":        skill.Name,
		"description": skill.Description,
		"content":     skill.Content,
		"user_id":     skill.UserID,
		"created_at":  skill.CreatedAt,
		"updated_at":  skill.UpdatedAt,
	}
	if skill.TopicID != nil {
		result["topic_id"] = *skill.TopicID
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type updateSkillRequest struct {
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to get skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var req updateSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateSkill(skillID, req.Description, req.Content); err != nil {
		http.Error(w, "failed to update skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": skillID, "status": "updated"})
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to get skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := s.db.DeleteSkill(skillID); err != nil {
		http.Error(w, "failed to delete skill", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// canManageTopicSkills checks if a user can create/modify topic skills.
func (s *Server) canManageTopicSkills(userData auth.UserData, topicID int64) error {
	if userData.Role == "admin" {
		return nil
	}
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	if topic.OwnerID != userData.UserID {
		return fmt.Errorf("only the topic owner or admins can manage topic skills")
	}
	return nil
}

// canManageSkill checks if a user can update/delete a specific skill.
func (s *Server) canManageSkill(userData auth.UserData, skill *db.SkillRow) error {
	if skill.TopicID != nil {
		return s.canManageTopicSkills(userData, *skill.TopicID)
	}
	// Private skill — must be the owner
	if skill.UserID != userData.UserID {
		return fmt.Errorf("forbidden")
	}
	return nil
}

// canViewSkill checks if a user can view a specific skill.
func (s *Server) canViewSkill(userData auth.UserData, skill *db.SkillRow) error {
	if skill.TopicID != nil {
		isMember, err := s.db.IsTopicMember(*skill.TopicID, userData.UserID)
		if err != nil || !isMember {
			return fmt.Errorf("forbidden")
		}
		return nil
	}
	if skill.UserID != userData.UserID {
		return fmt.Errorf("forbidden")
	}
	return nil
}
