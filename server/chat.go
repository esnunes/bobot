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

		// Resolve null topic_id to user's bobot topic
		topicID := msg.TopicID
		if topicID == nil {
			bobotTopic, err := s.db.GetUserBobotTopic(userData.UserID)
			if err != nil || bobotTopic == nil {
				log.Printf("failed to resolve bobot topic for user %d: %v", userData.UserID, err)
				continue
			}
			topicID = &bobotTopic.ID
		}

		s.handleTopicChatMessage(ctx, userData.UserID, *topicID, msg.Content)
	}
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
	if response, handled := s.handleSlashCommand(ctx, content, topicID); handled {
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
		members := s.broadcastToTopic(topicID, cmdMsgJSON)

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

		autoMarkReadForTopic(s.db, s.connections, topicID, members)
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
	members := s.broadcastToTopic(topicID, userMsgJSON)
	autoMarkReadForTopic(s.db, s.connections, topicID, members)

	// Send push to offline topic members (exclude sender)
	s.pushToTopicMembers(topicID, userID, user.DisplayName, content)

	// Check if assistant should respond (auto_respond or @bobot mention)
	topic, _ := s.db.GetTopicByID(topicID)
	if (topic != nil && topic.AutoRespond) || shouldTriggerAssistant(content) {
		s.handleTopicAssistantResponse(ctx, topicID, content, user.DisplayName)
	}

	s.markChatReadImplicit(userID, topicID)
}

func shouldTriggerAssistant(content string) bool {
	return strings.Contains(strings.ToLower(content), "@bobot")
}

func (s *Server) handleTopicAssistantResponse(ctx context.Context, topicID int64, content, displayName string) {
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
	members := s.broadcastToTopic(topicID, assistantMsgJSON)
	autoMarkReadForTopic(s.db, s.connections, topicID, members)

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

func (s *Server) broadcastToTopic(topicID int64, data []byte) []db.TopicMember {
	members, err := s.db.GetTopicMembers(topicID)
	if err != nil {
		log.Printf("failed to get topic members: %v", err)
		return nil
	}

	for _, member := range members {
		s.connections.Broadcast(member.UserID, data)
	}
	return members
}

// handleSlashCommand processes slash commands and returns the response.
// Returns (response, true) if the message was a slash command, ("", false) otherwise.
func (s *Server) handleSlashCommand(ctx context.Context, content string, topicID int64) (string, bool) {
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

	// Inject chat context (TopicID) into Go context
	ctx = auth.ContextWithChatData(ctx, auth.ChatData{
		TopicID: &topicID,
	})

	// Execute the tool
	result, err := tool.Execute(ctx, input)
	if err != nil {
		return "Error: " + err.Error(), true
	}

	return result, true
}
