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
	"github.com/esnunes/bobot/push"
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
	userData := auth.UserDataFromContext(r.Context())

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Register connection for multi-device support
	s.connections.Add(userData.UserID, conn)
	defer s.connections.Remove(userData.UserID, conn)

	ctx := r.Context()

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
			s.handleTopicChatMessage(ctx, userData.UserID, *msg.TopicID, msg.Content)
		} else {
			s.handlePrivateChatMessage(ctx, userData.UserID, msg.Content)
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

		s.markChatReadImplicit(userID, db.PrivateChatTopicID)
		return
	}

	// Use pipeline for the full message flow
	s.pipeline.SendPrivateMessage(ctx, userID, content)
	s.markChatReadImplicit(userID, db.PrivateChatTopicID)
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

		s.markChatReadImplicit(userID, topicID)
		return
	}

	// Save user message (raw_content includes display name for LLM context)
	rawContent := fmt.Sprintf("[%s]: %s", user.DisplayName, content)
	s.db.CreateTopicMessageWithContext(
		topicID, userID, "user", content, rawContent,
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

	// Send push to offline topic members (exclude sender)
	s.pushToTopicMembers(topicID, userID, user.DisplayName, content)

	// Check if assistant should respond
	if shouldTriggerAssistant(content) {
		s.handleTopicAssistantResponse(ctx, userID, topicID, content, user.DisplayName)
	}

	s.markChatReadImplicit(userID, topicID)
}

func shouldTriggerAssistant(content string) bool {
	return strings.Contains(strings.ToLower(content), "@bobot")
}

func (s *Server) handleTopicAssistantResponse(ctx context.Context, userID, topicID int64, content, displayName string) {
	response, err := s.engine.Chat(ctx, assistant.ChatOptions{
		Message:     content,
		TopicID:     topicID,
		DisplayName: displayName,
	})
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Broadcast to topic
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "assistant",
		"content":      response,
		"display_name": "bobot",
	})
	s.broadcastToTopic(topicID, assistantMsgJSON)

	// Send push to offline topic members (exclude nobody — all members get notified for bot responses)
	s.pushToTopicMembers(topicID, db.BobotUserID, "Bobot", response)
}

// pushToTopicMembers sends push notifications to all offline members of a topic, excluding the sender.
func (s *Server) pushToTopicMembers(topicID, senderID int64, senderName, content string) {
	if s.pushSender == nil {
		return
	}

	members, err := s.db.GetTopicMembers(topicID)
	if err != nil {
		return
	}

	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return
	}

	for _, member := range members {
		if member.UserID == senderID || member.UserID == db.BobotUserID {
			continue
		}
		if member.Muted {
			continue
		}
		if s.connections.Count(member.UserID) == 0 {
			// Check if user is blocked
			user, err := s.db.GetUserByID(member.UserID)
			if err != nil || user.Blocked {
				continue
			}
			title := fmt.Sprintf("%s in #%s", senderName, topic.Name)
			payload := push.BuildPayload(title, push.TruncateMessage(content, 200), fmt.Sprintf("/chats/%d", topicID), fmt.Sprintf("msg-topic-%d", topicID))
			go s.pushSender.NotifyUser(member.UserID, payload)
		}
	}
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

	// Auto-mark as read for members with auto-read enabled
	var latestID int64
	for _, member := range members {
		if !member.AutoRead {
			continue
		}
		if latestID == 0 {
			latestID, err = s.db.GetLatestTopicMessageID(topicID)
			if err != nil || latestID == 0 {
				break
			}
		}
		s.db.MarkChatRead(member.UserID, topicID, latestID)
		s.broadcastReadEvent(member.UserID, topicID)
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
