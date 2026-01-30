// server/chat.go
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/esnunes/bobot/auth"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type chatMessage struct {
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get token from query param
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := s.jwt.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
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
	s.connections.Add(claims.UserID, conn)
	defer s.connections.Remove(claims.UserID, conn)

	// Create context with user data
	ctx := auth.ContextWithUserData(r.Context(), auth.UserData{
		UserID: claims.UserID,
		Role:   claims.Role,
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

		// Check for slash commands
		if response, handled := s.handleSlashCommand(ctx, msg.Content); handled {
			// Broadcast user message first
			userMsgJSON, _ := json.Marshal(map[string]interface{}{
				"role":    "user",
				"content": msg.Content,
			})
			s.connections.Broadcast(claims.UserID, userMsgJSON)

			// Broadcast command response
			respJSON, _ := json.Marshal(map[string]interface{}{
				"role":    "system",
				"content": response,
			})
			s.connections.Broadcast(claims.UserID, respJSON)
			continue
		}

		// Save user message with context tracking
		s.db.CreateMessageWithContextThreshold(
			claims.UserID, "user", msg.Content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Broadcast user message to all connections
		userMsgJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "user",
			"content": msg.Content,
		})
		s.connections.Broadcast(claims.UserID, userMsgJSON)

		// Get assistant response
		response, err := s.engine.Chat(ctx, msg.Content)
		if err != nil {
			log.Printf("assistant error: %v", err)
			response = "Sorry, I encountered an error. Please try again."
		}

		// Save assistant message with context tracking
		s.db.CreateMessageWithContextThreshold(
			claims.UserID, "assistant", response,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Broadcast assistant response to all connections
		assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
			"role":    "assistant",
			"content": response,
		})
		s.connections.Broadcast(claims.UserID, assistantMsgJSON)
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
	if len(parts) < 2 {
		return "", false
	}

	// Extract tool name (without leading /)
	toolName := parts[0][1:]
	command := parts[1]

	// Get the tool from registry
	tool, ok := s.registry.Get(toolName)
	if !ok {
		return "", false
	}

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
