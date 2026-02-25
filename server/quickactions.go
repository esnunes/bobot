package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleQuickActionsPage(w http.ResponseWriter, r *http.Request) {
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

	actions, qaErr := s.db.GetTopicQuickActions(topicID)
	if qaErr != nil {
		http.Error(w, "failed to load quick actions", http.StatusInternalServerError)
		return
	}

	qaViews := make([]QuickActionView, 0, len(actions))
	for _, qa := range actions {
		qaViews = append(qaViews, QuickActionView{
			ID:      qa.ID,
			Label:   qa.Label,
			Message: qa.Message,
			Mode:    qa.Mode,
		})
	}

	canManage := userData.Role == "admin" || topic.OwnerID == userData.UserID

	s.render(w, "quick_actions", PageData{
		Title:        "Quick Actions",
		TopicID:      topicID,
		TopicName:    topic.Name,
		OwnerID:      topic.OwnerID,
		QuickActions: qaViews,
		IsAdmin:      canManage,
	})
}

func (s *Server) handleQuickActionFormPage(w http.ResponseWriter, r *http.Request) {
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

	// Check if editing an existing quick action
	idStr := r.PathValue("id")
	if idStr != "" {
		qaID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid quick action id", http.StatusBadRequest)
			return
		}

		qa, err := s.db.GetQuickActionByID(qaID)
		if err == db.ErrNotFound {
			http.Error(w, "quick action not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load quick action", http.StatusInternalServerError)
			return
		}

		if err := s.canViewQuickAction(userData, qa); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		s.render(w, "quick_action_form", PageData{
			Title:   "Edit Quick Action",
			TopicID: qa.TopicID,
			QuickAction: &QuickActionView{
				ID:      qa.ID,
				Label:   qa.Label,
				Message: qa.Message,
				Mode:    qa.Mode,
			},
		})
		return
	}

	// New quick action form
	s.render(w, "quick_action_form", PageData{
		Title:   "New Quick Action",
		TopicID: topicID,
	})
}

func (s *Server) handleCreateQuickActionForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		http.Error(w, "label required", http.StatusBadRequest)
		return
	}
	if len(label) > 100 {
		http.Error(w, "label must be 100 characters or less", http.StatusBadRequest)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}
	if len(message) > 2000 {
		http.Error(w, "message must be 2000 characters or less", http.StatusBadRequest)
		return
	}

	mode := r.FormValue("mode")
	if mode != "send" && mode != "fill" {
		mode = "send"
	}

	topicIDStr := r.FormValue("topic_id")
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic_id", http.StatusBadRequest)
		return
	}

	if err := s.canManageTopicQuickActions(userData, topicID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	_, err = s.db.CreateQuickAction(userData.UserID, topicID, label, message, mode)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "a quick action with that label already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to create quick action", http.StatusInternalServerError)
		return
	}

	redirectPath := fmt.Sprintf("/quickactions?topic_id=%d", topicID)
	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUpdateQuickActionForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	qaID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid quick action id", http.StatusBadRequest)
		return
	}

	qa, err := s.db.GetQuickActionByID(qaID)
	if err == db.ErrNotFound {
		http.Error(w, "quick action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load quick action", http.StatusInternalServerError)
		return
	}

	if err := s.canManageQuickAction(userData, qa); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		http.Error(w, "label required", http.StatusBadRequest)
		return
	}
	if len(label) > 100 {
		http.Error(w, "label must be 100 characters or less", http.StatusBadRequest)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}
	if len(message) > 2000 {
		http.Error(w, "message must be 2000 characters or less", http.StatusBadRequest)
		return
	}

	mode := r.FormValue("mode")
	if mode != "send" && mode != "fill" {
		mode = "send"
	}

	if err := s.db.UpdateQuickAction(qaID, label, message, mode); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "a quick action with that label already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to update quick action", http.StatusInternalServerError)
		return
	}

	redirectPath := fmt.Sprintf("/quickactions?topic_id=%d", qa.TopicID)
	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteQuickActionForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	qaID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid quick action id", http.StatusBadRequest)
		return
	}

	qa, err := s.db.GetQuickActionByID(qaID)
	if err == db.ErrNotFound {
		http.Error(w, "quick action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load quick action", http.StatusInternalServerError)
		return
	}

	if err := s.canManageQuickAction(userData, qa); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := s.db.DeleteQuickAction(qaID); err != nil {
		http.Error(w, "failed to delete quick action", http.StatusInternalServerError)
		return
	}

	redirectPath := fmt.Sprintf("/quickactions?topic_id=%d", qa.TopicID)
	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) canManageTopicQuickActions(userData auth.UserData, topicID int64) error {
	if userData.Role == "admin" {
		return nil
	}
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	if topic.OwnerID != userData.UserID {
		return fmt.Errorf("only the topic owner or admins can manage quick actions")
	}
	return nil
}

func (s *Server) canManageQuickAction(userData auth.UserData, qa *db.QuickActionRow) error {
	return s.canManageTopicQuickActions(userData, qa.TopicID)
}

func (s *Server) canViewQuickAction(userData auth.UserData, qa *db.QuickActionRow) error {
	isMember, err := s.db.IsTopicMember(qa.TopicID, userData.UserID)
	if err != nil || !isMember {
		return fmt.Errorf("forbidden")
	}
	return nil
}
