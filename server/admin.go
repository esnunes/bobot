// server/admin.go
package server

import (
	"net/http"
	"strconv"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers()
	if err != nil {
		http.Error(w, "failed to load users", http.StatusInternalServerError)
		return
	}

	adminUsers := make([]AdminUserView, 0, len(users))
	for _, u := range users {
		if u.ID == db.BobotUserID {
			continue
		}
		adminUsers = append(adminUsers, AdminUserView{
			ID:          u.ID,
			DisplayName: u.DisplayName,
			Username:    u.Username,
			Role:        u.Role,
			Blocked:     u.Blocked,
			CreatedAt:   u.CreatedAt.Format("2006-01-02"),
		})
	}

	topics, err := s.db.ListAllTopics()
	if err != nil {
		http.Error(w, "failed to load topics", http.StatusInternalServerError)
		return
	}

	adminTopics := make([]AdminTopicView, 0, len(topics))
	for _, t := range topics {
		members, _ := s.db.GetTopicMembers(t.ID)
		ownerName := ""
		if owner, err := s.db.GetUserByID(t.OwnerID); err == nil {
			ownerName = owner.DisplayName
			if ownerName == "" {
				ownerName = owner.Username
			}
		}
		adminTopics = append(adminTopics, AdminTopicView{
			ID:          t.ID,
			Name:        t.Name,
			OwnerName:   ownerName,
			MemberCount: len(members),
			CreatedAt:   t.CreatedAt.Format("2006-01-02"),
		})
	}

	s.render(w, "admin", PageData{
		Title:       "Admin",
		IsAdmin:     true,
		AdminUsers:  adminUsers,
		AdminTopics: adminTopics,
	})
}

func (s *Server) handleAdminUserContextPage(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.PathValue("id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByID(userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	inspection, err := s.engine.InspectPrivateContext(userID, user.Role)
	if err != nil {
		http.Error(w, "failed to inspect context", http.StatusInternalServerError)
		return
	}
	inspection.MaxTokens = s.cfg.Context.TokensMax

	label := user.DisplayName
	if label == "" {
		label = user.Username
	}

	s.render(w, "admin_context", buildContextPageData(label, inspection, s.cfg.LLM.Model))
}

func (s *Server) handleAdminTopicContextPage(w http.ResponseWriter, r *http.Request) {
	topicIDStr := r.PathValue("id")
	topicID, err := strconv.ParseInt(topicIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		http.Error(w, "topic not found", http.StatusNotFound)
		return
	}

	inspection, err := s.engine.InspectTopicContext(topicID)
	if err != nil {
		http.Error(w, "failed to inspect context", http.StatusInternalServerError)
		return
	}
	inspection.MaxTokens = s.cfg.Context.TokensMax

	s.render(w, "admin_context", buildContextPageData(topic.Name, inspection, s.cfg.LLM.Model))
}

func buildContextPageData(label string, inspection *assistant.ContextInspection, model string) PageData {
	msgs := make([]ContextMessageView, 0, len(inspection.Messages))
	for _, m := range inspection.Messages {
		content := m.Content
		rawContent := m.RawContent
		if rawContent == "" {
			rawContent = content
		}
		tokens := len(rawContent) / 4
		msgs = append(msgs, ContextMessageView{
			Role:       m.Role,
			Content:    content,
			RawContent: rawContent,
			Tokens:     tokens,
		})
	}

	tools := make([]ToolView, 0, len(inspection.Tools))
	for _, t := range inspection.Tools {
		tools = append(tools, ToolView{
			Name:        t.Name,
			Description: t.Description,
		})
	}

	rawJSON, _ := inspection.BuildRawJSON(model, 4096)

	return PageData{
		Title:   "Context - " + label,
		IsAdmin: true,
		Context: &ContextInspectionView{
			Label:        label,
			SystemPrompt: inspection.SystemPrompt,
			Messages:     msgs,
			Tools:        tools,
			TotalTokens:  inspection.TotalTokens,
			MaxTokens:    inspection.MaxTokens,
			RawJSON:      rawJSON,
		},
	}
}
