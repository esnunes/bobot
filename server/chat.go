// server/chat.go
package server

import (
	"encoding/json"
	"log"
	"net/http"

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

	// Create context with user ID and role
	ctx := auth.ContextWithUserID(r.Context(), claims.UserID)
	ctx = auth.ContextWithRole(ctx, claims.Role)

	// Handle messages
	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
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
