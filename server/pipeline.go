package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/push"
)

// ChatPipeline encapsulates the message flow: save → broadcast → engine.Chat → broadcast response → push.
// It is used by both WebSocket handlers and the scheduler.
type ChatPipeline struct {
	db          *db.CoreDB
	engine      *assistant.Engine
	connections *ConnectionRegistry
	pushSender  *push.PushSender
	cfg         *config.Config
}

// NewChatPipeline creates a ChatPipeline with all required dependencies.
func NewChatPipeline(coreDB *db.CoreDB, engine *assistant.Engine, connections *ConnectionRegistry, pushSender *push.PushSender, cfg *config.Config) *ChatPipeline {
	return &ChatPipeline{
		db:          coreDB,
		engine:      engine,
		connections: connections,
		pushSender:  pushSender,
		cfg:         cfg,
	}
}

// SendPrivateMessage saves the user message to DB, broadcasts it, calls engine.Chat(),
// broadcasts the assistant response, and sends push if offline. Returns the assistant response.
func (p *ChatPipeline) SendPrivateMessage(ctx context.Context, userID int64, content string) (string, error) {
	// Save user message: sender=user, receiver=bobot
	p.db.CreatePrivateMessageWithContextThreshold(
		userID, db.BobotUserID, "user", content, content,
		p.cfg.Context.TokensStart, p.cfg.Context.TokensMax,
	)

	// Broadcast user message
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "user",
		"content": content,
	})
	p.connections.Broadcast(userID, userMsgJSON)

	// Get assistant response (engine persists assistant messages internally)
	response, err := p.engine.Chat(ctx, assistant.ChatOptions{Message: content})
	if err != nil {
		slog.Error("pipeline: assistant error", "user_id", userID, "error", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Broadcast assistant response
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"role":    "assistant",
		"content": response,
	})
	p.connections.Broadcast(userID, assistantMsgJSON)

	// Send push notification if user has no active connections
	if p.pushSender != nil && p.connections.Count(userID) == 0 {
		payload := push.BuildPayload("Bobot", push.TruncateMessage(response, 200), "/chat", fmt.Sprintf("msg-private-%d", userID))
		go p.pushSender.NotifyUser(userID, payload)
	}

	return response, nil
}

// SendTopicMessage saves the user message to the topic, broadcasts to all members,
// calls engine.Chat(), broadcasts the response, and sends push to offline members.
// Returns the assistant response.
func (p *ChatPipeline) SendTopicMessage(ctx context.Context, userID int64, topicID int64, content string, displayName string) (string, error) {
	// Save user message (raw_content includes display name for LLM context)
	rawContent := fmt.Sprintf("[%s]: %s", displayName, content)
	p.db.CreateTopicMessageWithContext(
		topicID, userID, "user", content, rawContent,
		p.cfg.Context.TokensStart, p.cfg.Context.TokensMax,
	)

	// Broadcast to all topic members
	userMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "user",
		"content":      content,
		"user_id":      userID,
		"display_name": displayName,
	})
	members := p.broadcastToTopic(topicID, userMsgJSON)
	autoMarkReadForTopic(p.db, p.connections, topicID, members)

	// Get assistant response
	response, err := p.engine.Chat(ctx, assistant.ChatOptions{
		Message:     content,
		TopicID:     topicID,
		DisplayName: displayName,
	})
	if err != nil {
		slog.Error("pipeline: assistant error", "user_id", userID, "topic_id", topicID, "error", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Broadcast assistant response to topic
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "assistant",
		"content":      response,
		"display_name": "bobot",
	})
	members = p.broadcastToTopic(topicID, assistantMsgJSON)
	autoMarkReadForTopic(p.db, p.connections, topicID, members)

	// Send push to offline topic members
	p.pushToTopicMembers(topicID, db.BobotUserID, "Bobot", response)

	return response, nil
}

func (p *ChatPipeline) broadcastToTopic(topicID int64, data []byte) []db.TopicMember {
	members, err := p.db.GetTopicMembers(topicID)
	if err != nil {
		slog.Error("pipeline: failed to get topic members", "topic_id", topicID, "error", err)
		return nil
	}

	for _, member := range members {
		p.connections.Broadcast(member.UserID, data)
	}
	return members
}

func (p *ChatPipeline) pushToTopicMembers(topicID, senderID int64, senderName, content string) {
	if p.pushSender == nil {
		return
	}

	members, err := p.db.GetTopicMembers(topicID)
	if err != nil {
		return
	}

	topic, err := p.db.GetTopicByID(topicID)
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
		if p.connections.Count(member.UserID) == 0 {
			user, err := p.db.GetUserByID(member.UserID)
			if err != nil || user.Blocked {
				continue
			}
			title := fmt.Sprintf("%s in #%s", senderName, topic.Name)
			payload := push.BuildPayload(title, push.TruncateMessage(content, 200), fmt.Sprintf("/chats/%d", topicID), fmt.Sprintf("msg-topic-%d", topicID))
			go p.pushSender.NotifyUser(member.UserID, payload)
		}
	}
}
