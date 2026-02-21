// server/admin.go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

	// Build read positions for this user's private chat
	readPositions := make(map[int64][]string)
	lastReadID, _ := s.db.GetPrivateChatReadPosition(userID)
	if lastReadID > 0 {
		resolvedID := resolveReadPosition(lastReadID, inspection.Messages)
		if resolvedID > 0 {
			readPositions[resolvedID] = []string{"Read"}
		}
	}

	s.render(w, "admin_context", buildContextPageData(label, inspection, s.cfg.LLM.Model, readPositions))
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

	// Build read positions for all topic members
	readPositions := make(map[int64][]string)
	positions, _ := s.db.GetTopicReadPositions(topicID)
	for _, p := range positions {
		if p.LastReadID <= 0 {
			continue
		}
		resolvedID := resolveReadPosition(p.LastReadID, inspection.Messages)
		if resolvedID > 0 {
			readPositions[resolvedID] = append(readPositions[resolvedID], p.DisplayName)
		}
	}

	s.render(w, "admin_context", buildContextPageData(topic.Name, inspection, s.cfg.LLM.Model, readPositions))
}

// resolveReadPosition finds the highest context message ID where msg.ID <= lastReadID.
// Returns 0 if no message qualifies (read position is before the context window).
func resolveReadPosition(lastReadID int64, messages []assistant.ContextMessage) int64 {
	var resolved int64
	for _, m := range messages {
		if m.ID > 0 && m.ID <= lastReadID {
			resolved = m.ID
		}
	}
	return resolved
}

// formatReadBadge formats read-by users with truncation for >3 names.
func formatReadBadge(users []string) string {
	if len(users) <= 3 {
		return strings.Join(users, ", ")
	}
	return strings.Join(users[:3], ", ") + fmt.Sprintf(" +%d more", len(users)-3)
}

func buildContextPageData(label string, inspection *assistant.ContextInspection, model string, readPositions map[int64][]string) PageData {
	msgs := make([]ContextMessageView, 0, len(inspection.Messages))
	for _, m := range inspection.Messages {
		content := m.Content
		rawContent := m.RawContent
		if rawContent == "" {
			rawContent = content
		}
		tokens := len(rawContent) / 4
		mv := ContextMessageView{
			Role:       m.Role,
			Content:    content,
			RawContent: rawContent,
			Tokens:     tokens,
			Timestamp:  m.CreatedAt.UTC().Format("2006-01-02 15:04"),
		}
		if users, ok := readPositions[m.ID]; ok {
			mv.ReadBadge = formatReadBadge(users)
		}

		// Parse tool blocks from raw_content when it's a JSON array
		if len(rawContent) > 0 && rawContent[0] == '[' {
			var blocks []map[string]any
			if err := json.Unmarshal([]byte(rawContent), &blocks); err == nil {
				for _, b := range blocks {
					tb := ToolBlockView{}
					if t, ok := b["type"].(string); ok {
						tb.Type = t
					}
					switch tb.Type {
					case "tool_use":
						if name, ok := b["name"].(string); ok {
							tb.ToolName = name
						}
						if id, ok := b["id"].(string); ok {
							tb.ToolID = id
						}
						if input, ok := b["input"]; ok {
							inputJSON, _ := json.MarshalIndent(input, "", "  ")
							tb.Input = string(inputJSON)
						}
					case "tool_result":
						if id, ok := b["tool_use_id"].(string); ok {
							tb.ToolID = id
						}
						if c, ok := b["content"].(string); ok {
							tb.ResultStr = c
						}
					}
					mv.ToolBlocks = append(mv.ToolBlocks, tb)
				}
			}
		}

		msgs = append(msgs, mv)
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
