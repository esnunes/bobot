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

	"github.com/esnunes/bobot/assistant"
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
	TopicID *int64 `json:"topic_id"`
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

		if msg.TopicID != nil {
			s.handleTopicChatMessage(ctx, token.UserID, *msg.TopicID, msg.Content)
		} else {
			s.handlePrivateChatMessage(ctx, token.UserID, msg.Content)
		}
	}
}

func (s *Server) handlePrivateChatMessage(ctx context.Context, userID int64, content string) {
	// Check for slash commands
	receiverID := db.BobotUserID
	if response, handled := s.handleSlashCommand(ctx, content, &receiverID, nil); handled {
		// User sends command: sender=user, receiver=bobot
		s.db.CreatePrivateMessageWithContextThreshold(
			userID, db.BobotUserID, "command", content, content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		userMsgJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "command",
			"content": content,
		})
		s.connections.Broadcast(userID, userMsgJSON)

		// System response: sender=bobot, receiver=user
		s.db.CreatePrivateMessageWithContextThreshold(
			db.BobotUserID, userID, "system", response, response,
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
		userID, db.BobotUserID, "user", content, content,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast user message
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "user",
		"content": content,
	})
	s.connections.Broadcast(userID, userMsgJSON)

	// Get assistant response (engine persists assistant messages internally)
	response, err := s.engine.Chat(ctx, assistant.ChatOptions{Message: content})
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Broadcast assistant response
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "assistant",
		"content": response,
	})
	s.connections.Broadcast(userID, assistantMsgJSON)
}

func (s *Server) handleTopicChatMessage(ctx context.Context, userID, topicID int64, content string) {
	// Verify membership
	isMember, err := s.db.IsTopicMember(topicID, userID)
	if err != nil || !isMember {
		log.Printf("user %d not member of topic %d", userID, topicID)
		return
	}

	// Get user info for display
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		log.Printf("failed to get user: %v", err)
		return
	}

	// Check for slash commands
	if response, handled := s.handleSlashCommand(ctx, content, nil, &topicID); handled {
		// Save command message
		s.db.CreateTopicMessageWithContext(
			topicID, userID, "command", content, content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Broadcast command to all topic members
		cmdMsgJSON, _ := json.Marshal(map[string]interface{}{
			"topic_id":     topicID,
			"role":         "command",
			"content":      content,
			"user_id":      userID,
			"display_name": user.DisplayName,
		})
		s.broadcastToTopic(topicID, cmdMsgJSON)

		// Save system response
		s.db.CreateTopicMessageWithContext(
			topicID, userID, "system", response, response,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Broadcast system response to all topic members
		respJSON, _ := json.Marshal(map[string]interface{}{
			"topic_id":     topicID,
			"role":         "system",
			"content":      response,
			"display_name": "bobot",
		})
		s.broadcastToTopic(topicID, respJSON)
		return
	}

	// Save user message
	s.db.CreateTopicMessageWithContext(
		topicID, userID, "user", content, content,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)

	// Broadcast to all topic members
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "user",
		"content":      content,
		"user_id":      userID,
		"display_name": user.DisplayName,
	})
	s.broadcastToTopic(topicID, userMsgJSON)

	// Check if assistant should respond
	if shouldTriggerAssistant(content) {
		s.handleTopicAssistantResponse(ctx, topicID)
	}
}

func shouldTriggerAssistant(content string) bool {
	return strings.Contains(strings.ToLower(content), "@bobot")
}

func (s *Server) handleTopicAssistantResponse(ctx context.Context, topicID int64) {
	// Get context messages
	messages, err := s.db.GetTopicContextMessages(topicID)
	if err != nil {
		log.Printf("failed to get topic context: %v", err)
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
	_, err = s.db.CreateTopicMessageWithContext(
		topicID, db.BobotUserID, "assistant", response, response,
		s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
	)
	if err != nil {
		log.Printf("failed to save assistant message: %v", err)
	}

	// Broadcast to topic
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "assistant",
		"content":      response,
		"display_name": "bobot",
	})
	s.broadcastToTopic(topicID, assistantMsgJSON)
}

func (s *Server) broadcastToTopic(topicID int64, data []byte) {
	members, err := s.db.GetTopicMembers(topicID)
	if err != nil {
		log.Printf("failed to get topic members: %v", err)
		return
	}

	for _, member := range members {
		s.connections.Broadcast(member.UserID, data)
	}
}

// handleSlashCommand processes slash commands and returns the response.
// Returns (response, true) if the message was a slash command, ("", false) otherwise.
func (s *Server) handleSlashCommand(ctx context.Context, content string, receiverID *int64, topicID *int64) (string, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	// Split on first space: tool name vs rest
	toolName := content[1:] // strip leading /
	var args string
	if idx := strings.IndexByte(toolName, ' '); idx != -1 {
		args = toolName[idx+1:]
		toolName = toolName[:idx]
	}

	if toolName == "" {
		return "", false
	}

	// Check if tool exists
	tool, ok := s.registry.Get(toolName)
	if !ok {
		return "Error: unknown command /" + toolName, true
	}

	// Parse raw args into map
	input, err := tool.ParseArgs(args)
	if err != nil {
		return "Error: " + err.Error(), true
	}

	// Inject chat context (TopicID / ReceiverID) into Go context
	ctx = auth.ContextWithChatData(ctx, auth.ChatData{
		ReceiverID: receiverID,
		TopicID:    topicID,
	})

	// Execute the tool
	result, err := tool.Execute(ctx, input)
	if err != nil {
		return "Error: " + err.Error(), true
	}

	return result, true
}
