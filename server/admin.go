// server/admin.go
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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

	allStats, _ := s.db.GetAllUserMessageStats()

	adminUsers := make([]AdminUserView, 0, len(users))
	for _, u := range users {
		if u.ID == db.BobotUserID {
			continue
		}
		lastMessageAt := "Never"
		if st, ok := allStats[u.ID]; ok && st.LastSentAt != nil {
			lastMessageAt = st.LastSentAt.Format("2006-01-02")
		}
		adminUsers = append(adminUsers, AdminUserView{
			ID:            u.ID,
			DisplayName:   u.DisplayName,
			Username:      u.Username,
			Role:          u.Role,
			Blocked:       u.Blocked,
			CreatedAt:     u.CreatedAt.Format("2006-01-02"),
			LastMessageAt: lastMessageAt,
		})
	}

	s.render(w, "admin", PageData{
		Title:      "Admin",
		IsAdmin:    true,
		AdminUsers: adminUsers,
	})
}

func (s *Server) handleAdminUserPage(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.PathValue("id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	if userID == db.BobotUserID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	user, err := s.db.GetUserByID(userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// User info
	lastMessageAt := "Never"
	var messageCount int
	if lastSent, count, err := s.db.GetUserMessageStats(userID); err == nil {
		messageCount = count
		if lastSent != nil {
			lastMessageAt = lastSent.Format("2006-01-02 15:04")
		}
	}

	// Topics
	topics, err := s.db.GetUserTopics(userID)
	if err != nil {
		slog.Warn("admin: failed to load user topics", "userID", userID, "error", err)
	}
	topicViews := make([]AdminUserTopicView, 0, len(topics))
	for _, t := range topics {
		members, err := s.db.GetTopicMembers(t.ID)
		if err != nil {
			slog.Warn("admin: failed to load topic members", "topicID", t.ID, "error", err)
		}
		topicViews = append(topicViews, AdminUserTopicView{
			ID:          t.ID,
			Name:        t.Name,
			IsOwner:     t.OwnerID == userID,
			MemberCount: len(members),
			AutoRespond: t.AutoRespond,
			CreatedAt:   t.CreatedAt.Format("2006-01-02"),
		})
	}

	// Profile
	profile, _, err := s.db.GetUserProfile(userID)
	if err != nil {
		slog.Warn("admin: failed to load user profile", "userID", userID, "error", err)
	}

	// Skills (from user's topics, created by this user)
	var skillViews []AdminUserSkillView
	for _, t := range topics {
		topicSkills, err := s.db.GetTopicSkills(t.ID)
		if err != nil {
			slog.Warn("admin: failed to load topic skills", "topicID", t.ID, "error", err)
		}
		for _, sk := range topicSkills {
			if sk.UserID == userID {
				skillViews = append(skillViews, AdminUserSkillView{
					Name:        sk.Name,
					Description: sk.Description,
					TopicName:   t.Name,
				})
			}
		}
	}

	// Push subscriptions
	pushSubs, err := s.db.GetPushSubscriptions(userID)
	if err != nil {
		slog.Warn("admin: failed to load push subscriptions", "userID", userID, "error", err)
	}
	pushSubViews := make([]AdminUserPushSubView, 0, len(pushSubs))
	for _, ps := range pushSubs {
		endpoint := ps.Endpoint
		if u, err := url.Parse(endpoint); err == nil {
			endpoint = u.Host + u.Path
			if len(endpoint) > 60 {
				endpoint = endpoint[:60] + "..."
			}
		}
		pushSubViews = append(pushSubViews, AdminUserPushSubView{
			Endpoint:  endpoint,
			CreatedAt: ps.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	// Read status
	readPositions, err := s.db.GetUserReadPositions(userID)
	if err != nil {
		slog.Warn("admin: failed to load user read positions", "userID", userID, "error", err)
	}
	// Build a set of topics with read data
	readTopicIDs := make(map[int64]bool)
	var readStatusViews []AdminUserReadStatusView
	for _, rp := range readPositions {
		readTopicIDs[rp.TopicID] = true
		readAt := "Unknown"
		if rp.ReadAt != nil {
			readAt = rp.ReadAt.Format("2006-01-02 15:04")
		}
		readStatusViews = append(readStatusViews, AdminUserReadStatusView{
			TopicName:         rp.TopicName,
			LastReadMessageID: rp.LastReadMessageID,
			ReadAt:            readAt,
		})
	}
	// Add topics without read data
	for _, t := range topics {
		if !readTopicIDs[t.ID] {
			readStatusViews = append(readStatusViews, AdminUserReadStatusView{
				TopicName: t.Name,
				ReadAt:    "Never opened",
			})
		}
	}

	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}

	s.render(w, "admin_user", PageData{
		Title:   "User - " + displayName,
		IsAdmin: true,
		AdminUserDetail: &AdminUserDetailView{
			ID:            user.ID,
			DisplayName:   user.DisplayName,
			Username:      user.Username,
			Role:          user.Role,
			Blocked:       user.Blocked,
			CreatedAt:     user.CreatedAt.Format("2006-01-02"),
			LastMessageAt: lastMessageAt,
			MessageCount:  messageCount,
			Topics:        topicViews,
			Profile:       profile,
			Skills:        skillViews,
			PushSubs:      pushSubViews,
			ReadStatus:    readStatusViews,
		},
	})
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

	// Determine back URL from "from" query param
	backURL := "/admin"
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if fromID, err := strconv.ParseInt(fromStr, 10, 64); err == nil && fromID > 0 {
			backURL = fmt.Sprintf("/admin/users/%d", fromID)
		}
	}

	// Build sender name map and members list from topic members
	dbMembers, err := s.db.GetTopicMembers(topicID)
	if err != nil {
		slog.Warn("admin: failed to load topic members", "topicID", topicID, "error", err)
	}
	senderNames := make(map[int64]string)
	var memberViews []MemberView
	for _, m := range dbMembers {
		name := m.DisplayName
		if name == "" {
			name = m.Username
		}
		senderNames[m.UserID] = name
		memberViews = append(memberViews, MemberView{
			UserID:      m.UserID,
			Username:    m.Username,
			DisplayName: m.DisplayName,
		})
	}
	senderNames[db.BobotUserID] = "bobot"

	pageData := buildContextPageData(topic.Name, inspection, s.cfg.LLM.Model, readPositions, senderNames, memberViews)
	pageData.Context.BackURL = backURL
	s.render(w, "admin_context", pageData)
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

func buildContextPageData(label string, inspection *assistant.ContextInspection, model string, readPositions map[int64][]string, senderNames map[int64]string, members []MemberView) PageData {
	msgs := make([]ContextMessageView, 0, len(inspection.Messages))
	for _, m := range inspection.Messages {
		content := m.Content
		rawContent := m.RawContent
		if rawContent == "" {
			rawContent = content
		}
		tokens := len(rawContent) / 4

		senderName := ""
		if m.Role == "user" {
			if name, ok := senderNames[m.SenderID]; ok {
				senderName = name
			}
		} else if m.Role == "assistant" {
			senderName = "bobot"
		}

		mv := ContextMessageView{
			Role:       m.Role,
			SenderName: senderName,
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
			Members:      members,
			TotalTokens:  inspection.TotalTokens,
			MaxTokens:    inspection.MaxTokens,
			RawJSON:      rawJSON,
		},
	}
}
