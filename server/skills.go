package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleSkillsPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicID, err := strconv.ParseInt(r.URL.Query().Get("topic_id"), 10, 64)
	if err != nil {
		http.Error(w, "topic_id required", http.StatusBadRequest)
		return
	}
	isMember, memberErr := s.db.IsTopicMember(topicID, userData.UserID)
	if memberErr != nil || !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	topic, topicErr := s.db.GetTopicByID(topicID)
	if topicErr != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	skills, skillErr := s.db.GetTopicSkills(topicID)
	if skillErr != nil {
		http.Error(w, "failed to load skills", http.StatusInternalServerError)
		return
	}

	skillViews := make([]SkillView, 0, len(skills))
	for _, sk := range skills {
		skillViews = append(skillViews, SkillView{
			ID:          sk.ID,
			Name:        sk.Name,
			Description: sk.Description,
		})
	}

	s.render(w, "skills", PageData{
		Title:     "Skills",
		TopicID:   topicID,
		TopicName: topic.Name,
		Skills:    skillViews,
	})
}

func (s *Server) handleSkillFormPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")
	var topicID int64
	if topicIDStr != "" {
		var err error
		topicID, err = strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
	}

	// Check if editing an existing skill
	idStr := r.PathValue("id")
	if idStr != "" {
		skillID, err := strconv.ParseInt(idStr, 10, 64)
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
			http.Error(w, "failed to load skill", http.StatusInternalServerError)
			return
		}

		// Verify ownership
		if err := s.canViewSkill(userData, skill); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if skill.TopicID != nil {
			topicID = *skill.TopicID
		}

		s.render(w, "skill_form", PageData{
			Title:   "Edit Skill",
			TopicID: topicID,
			Skill: &SkillView{
				ID:          skill.ID,
				Name:        skill.Name,
				Description: skill.Description,
				Content:     skill.Content,
			},
		})
		return
	}

	// New skill form
	s.render(w, "skill_form", PageData{
		Title:   "New Skill",
		TopicID: topicID,
	})
}

func (s *Server) handleCreateSkillForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))
	content := r.FormValue("content")
	topicIDStr := r.FormValue("topic_id")

	var topicID *int64
	redirectPath := "/skills"

	if topicIDStr != "" {
		tid, err := strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		if err := s.canManageTopicSkills(userData, tid); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		topicID = &tid
		redirectPath = fmt.Sprintf("/skills?topic_id=%d", tid)
	}

	_, err := s.db.CreateSkill(userData.UserID, topicID, name, description, content)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "a skill with that name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to create skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUpdateSkillForm(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "failed to load skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))
	content := r.FormValue("content")

	if err := s.db.UpdateSkill(skillID, description, content); err != nil {
		http.Error(w, "failed to update skill", http.StatusInternalServerError)
		return
	}

	redirectPath := "/skills"
	if skill.TopicID != nil {
		redirectPath = fmt.Sprintf("/skills?topic_id=%d", *skill.TopicID)
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSkillForm(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "failed to load skill", http.StatusInternalServerError)
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

	redirectPath := "/skills"
	if skill.TopicID != nil {
		redirectPath = fmt.Sprintf("/skills?topic_id=%d", *skill.TopicID)
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
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
