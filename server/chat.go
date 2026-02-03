// server/chat.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type chatMessage struct {
	Content string `json:"content"`
	GroupID *int64 `json:"group_id"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get session from cookie
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	token, err := s.session.DecryptToken(cookie.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	// Check if past absolute deadline
	if s.session.IsPastDeadline(token) {
		http.Error(w, "session expired", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Register connection for multi-device support
	s.connections.Add(token.UserID, conn)
	defer s.connections.Remove(token.UserID, conn)

	// Create context with user data
	ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
		UserID: token.UserID,
		Role:   token.Role,
	})

	// Handle messages
	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		if msg.GroupID != nil {
			s.handleGroupChatMessage(ctx, token.UserID, *msg.GroupID, msg.Content)
		} else {
			s.handlePrivateChatMessage(ctx, token.UserID, msg.Content)
		}
	}
}

func (s *Server) handlePrivateChatMessage(ctx context.Context, userID int64, content string) {
	// Check for slash commands
	if response, handled := s.handleSlashCommand(ctx, content); handled {
		// User sends command: sender=user, receiver=bobot
		s.db.CreatePrivateMessageWithContextThreshold(
			userID, db.BobotUserID, "command", content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		userMsgJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "command",
			"content": content,
		})
		s.connections.Broadcast(userID, userMsgJSON)

		// System response: sender=bobot, receiver=user
		s.db.CreatePrivateMessageWithContextThreshold(
			db.BobotUserID, userID, "system", response,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		respJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "system",
			"content": response,
		})
		s.connections.Broadcast(userID, respJSON)
		return
	}

	// Save user message: sender=user, receiver=bobot
	s.db.CreatePrivateMessageWithContextThreshold(
		userID, db.BobotUserID, "user", content,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast user message
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "user",
		"content": content,
	})
	s.connections.Broadcast(userID, userMsgJSON)

	// Get assistant response
	response, err := s.engine.Chat(ctx, content)
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Save assistant message: sender=bobot, receiver=user
	s.db.CreatePrivateMessageWithContextThreshold(
		db.BobotUserID, userID, "assistant", response,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast assistant response
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "assistant",
		"content": response,
	})
	s.connections.Broadcast(userID, assistantMsgJSON)
}

func (s *Server) handleGroupChatMessage(ctx context.Context, userID, groupID int64, content string) {
	// Verify membership
	isMember, err := s.db.IsGroupMember(groupID, userID)
	if err != nil || !isMember {
		log.Printf("user %d not member of group %d", userID, groupID)
		return
	}

	// Get user info for display
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		log.Printf("failed to get user: %v", err)
		return
	}

	// Check for slash commands
	if response, handled := s.handleSlashCommand(ctx, content); handled {
		// Save command message
		s.db.CreateGroupMessageWithContext(
			groupID, userID, "command", content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Broadcast command to all group members
		cmdMsgJSON, _ := json.Marshal(map[string]interface{}{
			"group_id":     groupID,
			"role":         "command",
			"content":      content,
			"user_id":      userID,
			"display_name": user.DisplayName,
		})
		s.broadcastToGroup(groupID, cmdMsgJSON)

		// Save system response
		s.db.CreateGroupMessageWithContext(
			groupID, userID, "system", response,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Broadcast system response to all group members
		respJSON, _ := json.Marshal(map[string]interface{}{
			"group_id": groupID,
			"role":     "system",
			"content":  response,
		})
		s.broadcastToGroup(groupID, respJSON)
		return
	}

	// Save user message
	s.db.CreateGroupMessageWithContext(
		groupID, userID, "user", content,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast to all group members
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"group_id":     groupID,
		"role":         "user",
		"content":      content,
		"user_id":      userID,
		"display_name": user.DisplayName,
	})
	s.broadcastToGroup(groupID, userMsgJSON)

	// Check if assistant should respond
	if shouldTriggerAssistant(content) {
		s.handleGroupAssistantResponse(ctx, groupID)
	}
}

func shouldTriggerAssistant(content string) bool {
	return strings.Contains(strings.ToLower(content), "@assistant")
}

func (s *Server) handleGroupAssistantResponse(ctx context.Context, groupID int64) {
	// Get context messages
	messages, err := s.db.GetGroupContextMessages(groupID)
	if err != nil {
		log.Printf("failed to get group context: %v", err)
		return
	}

	// Build conversation for LLM with user attribution
	var conversation []string
	for _, m := range messages {
		if m.Role == "user" {
			user, _ := s.db.GetUserByID(m.SenderID)
			name := "User"
			if user != nil && user.DisplayName != "" {
				name = user.DisplayName
			}
			conversation = append(conversation, fmt.Sprintf("[%s]: %s", name, m.Content))
		} else {
			conversation = append(conversation, fmt.Sprintf("[assistant]: %s", m.Content))
		}
	}

	// Get response from engine
	response, err := s.engine.ChatWithContext(ctx, conversation)
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Save assistant message using bobot user ID
	_, err = s.db.CreateGroupMessageWithContext(
		groupID, db.BobotUserID, "assistant", response,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)
	if err != nil {
		log.Printf("failed to save assistant message: %v", err)
	}

	// Broadcast to group
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"group_id": groupID,
		"role":     "assistant",
		"content":  response,
	})
	s.broadcastToGroup(groupID, assistantMsgJSON)
}

func (s *Server) broadcastToGroup(groupID int64, data []byte) {
	members, err := s.db.GetGroupMembers(groupID)
	if err != nil {
		log.Printf("failed to get group members: %v", err)
		return
	}

	for _, member := range members {
		s.connections.Broadcast(member.UserID, data)
	}
}

// handleSlashCommand processes slash commands and returns the response.
// Returns (response, true) if the message was a slash command, ("", false) otherwise.
func (s *Server) handleSlashCommand(ctx context.Context, content string) (string, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) < 1 {
		return "", false
	}

	// Extract tool name (without leading /)
	toolName := parts[0][1:]

	// Check if tool exists
	tool, ok := s.registry.Get(toolName)
	if !ok {
		return "Error: unknown command /" + toolName, true
	}

	// Check if command is provided
	if len(parts) < 2 {
		return "Error: missing command. Usage: /" + toolName + " <command>", true
	}

	command := parts[1]

	// Build input for the tool
	input := map[string]interface{}{
		"command": command,
	}

	// Add additional arguments based on command
	if len(parts) > 2 {
		switch command {
		case "block", "unblock":
			input["username"] = parts[2]
		case "revoke":
			input["code"] = parts[2]
		}
	}

	// Execute the tool
	result, err := tool.Execute(ctx, input)
	if err != nil {
		return "Error: " + err.Error(), true
	}

	return result, true
}
