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

	s.render(w, r, "quick_actions", PageData{
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
		if err != nil {
			http.Error(w, "quick action not found", http.StatusNotFound)
			return
		}

		if err := s.canViewQuickAction(userData, qa.TopicID); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		s.render(w, r, "quick_action_form", PageData{
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

	// New quick action form — require management permission
	if topicID != 0 {
		if err := s.canManageTopicQuickActions(userData, topicID); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	s.render(w, r, "quick_action_form", PageData{
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

	label, message, mode, err := validateQuickActionFields(
		r.FormValue("label"), r.FormValue("message"), r.FormValue("mode"),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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

	if err := s.canManageQuickAction(userData, qa.TopicID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	label, message, mode, valErr := validateQuickActionFields(
		r.FormValue("label"), r.FormValue("message"), r.FormValue("mode"),
	)
	if valErr != nil {
		http.Error(w, valErr.Error(), http.StatusBadRequest)
		return
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

	if err := s.canManageQuickAction(userData, qa.TopicID); err != nil {
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

func validateQuickActionFields(label, message, mode string) (string, string, string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", "", "", fmt.Errorf("label required")
	}
	if len(label) > 100 {
		return "", "", "", fmt.Errorf("label must be 100 characters or less")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "", "", "", fmt.Errorf("message required")
	}
	if len(message) > 2000 {
		return "", "", "", fmt.Errorf("message must be 2000 characters or less")
	}
	if mode != "send" && mode != "fill" {
		mode = "send"
	}
	return label, message, mode, nil
}

func (s *Server) canManageTopicQuickActions(userData auth.UserData, topicID int64) error {
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	return auth.CanManageTopicResource(userData.Role, userData.UserID, topic.OwnerID)
}

func (s *Server) canManageQuickAction(userData auth.UserData, topicID int64) error {
	return s.canManageTopicQuickActions(userData, topicID)
}

func (s *Server) canViewQuickAction(userData auth.UserData, topicID int64) error {
	isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
	if err != nil || !isMember {
		return fmt.Errorf("forbidden")
	}
	return nil
}
